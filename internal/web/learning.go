package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/timb418/systemdesign-trainer/internal/store"
	"github.com/timb418/systemdesign-trainer/internal/tasks"
)

type learningTaskView struct {
	tasks.PublicTask
	SessionID       string
	CurrentPhase    string
	CompletedPhases int
	Done            bool
	Recommended     bool
}

type learningTrackView struct {
	tasks.LearningTrack
	Tasks []learningTaskView
}

type learningPhaseView struct {
	tasks.LearningPhase
	Current    bool
	Completed  bool
	Assistance string
	Forced     bool
	Notes      string
}

type forcedPhaseView struct {
	Title string
	Notes string
}

type conceptMasteryView struct {
	Title       string
	Practiced   int
	Independent int
}

func (s *Server) learningHub(w http.ResponseWriter, r *http.Request) {
	catalog := s.bank.LearningCatalog()
	selectedID := r.PathValue("track")
	if selectedID == "" && len(catalog.Tracks) > 0 {
		selectedID = catalog.Tracks[0].ID
	}
	selected, ok := s.bank.LearningTrack(selectedID)
	if !ok {
		s.fail(w, 404, "учебный трек не найден")
		return
	}
	progress, err := s.store.LearningProgress(r.Context())
	if err != nil {
		s.fail(w, 500, err.Error())
		return
	}
	assistanceByTask := map[string]map[string]string{}
	weakByTask := map[string]int{}
	for taskID, item := range progress {
		assistance, _ := s.store.LearningPhaseAssistance(r.Context(), item.SessionID)
		assistanceByTask[taskID] = assistance
		for _, level := range assistance {
			if level == "explained" {
				weakByTask[taskID]++
			}
		}
	}
	recommended := recommendTask(s.bank, selected, progress, weakByTask)
	var items []learningTaskView
	inTrack := map[string]struct{}{}
	mastery := map[string]*conceptMasteryView{}
	for _, taskID := range selected.Tasks {
		inTrack[taskID] = struct{}{}
		task, exists := s.bank.Get(taskID)
		if !exists {
			continue
		}
		public := task.Public()
		for _, typeID := range task.ArchitectureTypes {
			public.TypeNames = append(public.TypeNames, s.bank.TypeName(typeID))
		}
		p := progress[taskID]
		items = append(items, learningTaskView{
			PublicTask: public, SessionID: p.SessionID, CurrentPhase: p.CurrentPhase,
			CompletedPhases: p.CompletedPhases, Done: p.CompletedPhases >= tasks.LearningPhaseCount,
			Recommended: taskID == recommended,
		})
		blueprint, _ := s.bank.LearningBlueprint(taskID)
		for _, concept := range blueprint.Concepts {
			item := mastery[concept.ID]
			if item == nil {
				item = &conceptMasteryView{Title: concept.Title}
				mastery[concept.ID] = item
			}
			for _, level := range assistanceByTask[taskID] {
				item.Practiced++
				if level == "independent" {
					item.Independent++
				}
			}
		}
	}
	var prerequisite *learningTaskView
	if recommended != "" {
		if _, exists := inTrack[recommended]; !exists {
			if task, ok := s.bank.Get(recommended); ok {
				public := task.Public()
				for _, typeID := range task.ArchitectureTypes {
					public.TypeNames = append(public.TypeNames, s.bank.TypeName(typeID))
				}
				p := progress[recommended]
				prerequisite = &learningTaskView{
					PublicTask: public, SessionID: p.SessionID, CurrentPhase: p.CurrentPhase,
					CompletedPhases: p.CompletedPhases, Done: p.CompletedPhases >= tasks.LearningPhaseCount,
					Recommended: true,
				}
			}
		}
	}
	var masteryList []conceptMasteryView
	for _, item := range mastery {
		if item.Practiced > 0 {
			masteryList = append(masteryList, *item)
		}
	}
	sort.Slice(masteryList, func(i, j int) bool { return masteryList[i].Title < masteryList[j].Title })
	s.render(w, "learn.html", map[string]any{
		"Title": "Обучение", "Tracks": catalog.Tracks, "Track": learningTrackView{LearningTrack: selected, Tasks: items},
		"Recommended": recommended, "Prerequisite": prerequisite, "Mastery": masteryList,
	})
}

func recommendTask(bank *tasks.Bank, track tasks.LearningTrack, progress map[string]store.LearningProgress, weak map[string]int) string {
	for _, prerequisiteTrack := range track.Prerequisites {
		if required, ok := bank.LearningTrack(prerequisiteTrack); ok {
			for _, taskID := range required.Tasks {
				if progress[taskID].CompletedPhases < tasks.LearningPhaseCount {
					return recommendTask(bank, required, progress, weak)
				}
			}
		}
	}
	for _, taskID := range track.Tasks {
		blueprint, ok := bank.LearningBlueprint(taskID)
		if !ok {
			continue
		}
		for _, prerequisite := range blueprint.Prerequisites {
			if progress[prerequisite].CompletedPhases < tasks.LearningPhaseCount {
				return prerequisite
			}
		}
		if p := progress[taskID]; p.SessionID != "" && p.CompletedPhases < tasks.LearningPhaseCount {
			return taskID
		}
	}
	for _, taskID := range track.Tasks {
		if progress[taskID].CompletedPhases >= tasks.LearningPhaseCount && weak[taskID] > 0 {
			return taskID
		}
	}
	for _, taskID := range track.Tasks {
		if progress[taskID].CompletedPhases < tasks.LearningPhaseCount {
			return taskID
		}
	}
	return ""
}

func (s *Server) showLearningSession(w http.ResponseWriter, r *http.Request, sess store.Session, task tasks.Task, messages []store.Message) {
	blueprint, ok := s.bank.LearningBlueprint(task.ID)
	if !ok {
		s.fail(w, 500, "нет учебного blueprint")
		return
	}
	state, err := s.store.GetLearningState(r.Context(), sess.ID)
	if err != nil {
		s.fail(w, 500, err.Error())
		return
	}
	results, _ := s.store.LearningPhaseResults(r.Context(), sess.ID)
	currentIndex := len(blueprint.Phases)
	var current tasks.LearningPhase
	var phases []learningPhaseView
	for i, phase := range blueprint.Phases {
		if phase.ID == state.Phase {
			currentIndex = i
			current = phase
		}
		res := results[phase.ID]
		phases = append(phases, learningPhaseView{
			LearningPhase: phase, Current: phase.ID == state.Phase,
			Completed: res.Assistance != "", Assistance: res.Assistance,
			Forced: res.Forced, Notes: res.Notes,
		})
	}
	hint := ""
	if currentIndex < len(blueprint.Phases) && state.HintLevel > 0 {
		index := state.HintLevel - 1
		if index >= len(current.Hints) {
			index = len(current.Hints) - 1
		}
		if index >= 0 {
			hint = current.Hints[index]
		}
	}
	s.render(w, "learning_session.html", map[string]any{
		"Title": task.Title, "Session": sess, "Task": task.Public(), "Messages": messages,
		"DrawioURL": s.drawio, "HasKey": s.set.HasAPIKey(), "Blueprint": blueprint,
		"State": state, "CurrentPhase": current, "Phases": phases, "Hint": hint,
		"CanHint":      currentIndex < len(blueprint.Phases),
		"GoldUnlocked": state.Phase == "complete",
	})
}

func (s *Server) learningHint(w http.ResponseWriter, r *http.Request) {
	sess, _, state, phase, ok := s.learningRequest(w, r)
	if !ok {
		return
	}
	if state.Phase == "complete" {
		s.fail(w, 400, "обучение уже завершено")
		return
	}
	next, err := s.store.IncreaseLearningHint(r.Context(), sess.ID)
	if err != nil {
		s.fail(w, 500, err.Error())
		return
	}
	content := "Заготовленные подсказки закончились — сформулируйте конкретный вопрос наставнику в чате, и он подскажет дальше."
	if next.HintLevel <= len(phase.Hints) {
		content = fmt.Sprintf("Подсказка %d/%d: %s", next.HintLevel, len(phase.Hints), phase.Hints[next.HintLevel-1])
	}
	_, _ = s.store.AddMessage(r.Context(), store.Message{SessionID: sess.ID, Role: "system", Content: content})
	http.Redirect(w, r, "/sessions/"+sess.ID, http.StatusSeeOther)
}

func (s *Server) learningAdvance(w http.ResponseWriter, r *http.Request) {
	sess, blueprint, state, phase, ok := s.learningRequest(w, r)
	if !ok {
		return
	}
	if state.Phase == "complete" {
		http.Redirect(w, r, "/sessions/"+sess.ID, http.StatusSeeOther)
		return
	}
	messages, _ := s.store.ListMessages(r.Context(), sess.ID)
	if !hasLearningAttempt(messages) {
		s.fail(w, 400, "сначала сделайте попытку в чате или покажите схему")
		return
	}
	t, ok := s.bank.Get(sess.TaskID)
	if !ok {
		s.fail(w, 404, "нет задачи")
		return
	}
	forced, notes := s.checkPhaseCompletion(r.Context(), t, blueprint, phase, state, messages)
	if err := s.advancePhase(r.Context(), sess, blueprint, state.Phase, forced, notes); err != nil {
		s.fail(w, 400, err.Error())
		return
	}
	http.Redirect(w, r, "/sessions/"+sess.ID, http.StatusSeeOther)
}

// advancePhase moves the session to the next blueprint phase (or "complete" past the last
// one), recording whether the transition was forced (see checkPhaseCompletion).
func (s *Server) advancePhase(ctx context.Context, sess store.Session, blueprint tasks.LearningBlueprint, currentPhase string, forced bool, notes string) error {
	index := -1
	for i, phase := range blueprint.Phases {
		if phase.ID == currentPhase {
			index = i
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("неизвестный учебный этап")
	}
	next := "complete"
	nextMessage := "Рефлексия завершена. Теперь можно завершить сессию и открыть эталонный разбор."
	if index+1 < len(blueprint.Phases) {
		nextPhase := blueprint.Phases[index+1]
		next = nextPhase.ID
		nextMessage = fmt.Sprintf("Учебный этап: %s\nЦель: %s", nextPhase.Title, nextPhase.Goal)
	}
	if err := s.store.AdvanceLearningPhase(ctx, sess.ID, currentPhase, next, forced, notes); err != nil {
		return err
	}
	_, _ = s.store.AddMessage(ctx, store.Message{SessionID: sess.ID, Role: "system", Content: nextMessage})
	return nil
}

// checkPhaseCompletion asks the LLM whether the current phase's goal was actually met by
// the conversation since the phase started. It fails open: if the check itself errors (no
// API key, network issue, unparsable response), the phase is treated as forced/incomplete
// so a manual "Этап готов" click still gets the candidate unstuck, with the reason recorded.
func (s *Server) checkPhaseCompletion(ctx context.Context, t tasks.Task, blueprint tasks.LearningBlueprint, phase tasks.LearningPhase, state store.LearningState, messages []store.Message) (forced bool, notes string) {
	payload := buildPhaseCheckPayload(t, blueprint, phase, state.HintLevel, messagesSincePhaseStart(messages))
	raw, _, err := s.agents.CheckPhaseCompletion(ctx, payload)
	if err != nil {
		return true, "Автоматическая проверка недоступна: " + err.Error()
	}
	var parsed struct {
		Complete bool     `json:"complete"`
		Missing  []string `json:"missing"`
		Feedback string   `json:"feedback"`
	}
	if err := json.Unmarshal([]byte(extractJSON(raw)), &parsed); err != nil {
		return true, "Не удалось разобрать ответ проверки этапа."
	}
	if parsed.Complete {
		return false, ""
	}
	notes = strings.TrimSpace(parsed.Feedback)
	if len(parsed.Missing) > 0 {
		notes = strings.TrimSpace(notes + "\nНе хватает: " + strings.Join(parsed.Missing, "; "))
	}
	return true, notes
}

func (s *Server) learningRequest(w http.ResponseWriter, r *http.Request) (store.Session, tasks.LearningBlueprint, store.LearningState, tasks.LearningPhase, bool) {
	sess, err := s.store.GetSession(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, 404, err.Error())
		return store.Session{}, tasks.LearningBlueprint{}, store.LearningState{}, tasks.LearningPhase{}, false
	}
	if sess.Mode != store.ModeLearning || sess.Status != store.StatusInProgress {
		s.fail(w, 400, "это не активная учебная сессия")
		return store.Session{}, tasks.LearningBlueprint{}, store.LearningState{}, tasks.LearningPhase{}, false
	}
	blueprint, ok := s.bank.LearningBlueprint(sess.TaskID)
	if !ok {
		s.fail(w, 404, "нет учебного blueprint")
		return store.Session{}, tasks.LearningBlueprint{}, store.LearningState{}, tasks.LearningPhase{}, false
	}
	state, err := s.store.GetLearningState(r.Context(), sess.ID)
	if err != nil {
		s.fail(w, 500, err.Error())
		return store.Session{}, tasks.LearningBlueprint{}, store.LearningState{}, tasks.LearningPhase{}, false
	}
	phase, _ := learningPhase(blueprint, state.Phase)
	return sess, blueprint, state, phase, true
}

func learningPhase(blueprint tasks.LearningBlueprint, id string) (tasks.LearningPhase, bool) {
	for _, phase := range blueprint.Phases {
		if phase.ID == id {
			return phase, true
		}
	}
	return tasks.LearningPhase{}, false
}

func hasLearningAttempt(messages []store.Message) bool {
	for _, m := range messagesSincePhaseStart(messages) {
		if m.Role == "user" {
			return true
		}
	}
	return false
}

// messagesSincePhaseStart returns the tail of messages after the most recent
// "Учебный этап:" marker (written by advancePhase), or the whole slice if the
// session is still in its first phase.
func messagesSincePhaseStart(messages []store.Message) []store.Message {
	start := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "system" && strings.HasPrefix(messages[i].Content, "Учебный этап:") {
			start = i + 1
			break
		}
	}
	return messages[start:]
}

func learningIntro(task tasks.Task, blueprint tasks.LearningBlueprint) string {
	var concepts []string
	for _, concept := range blueprint.Concepts {
		concepts = append(concepts, concept.Title)
	}
	return fmt.Sprintf(
		"Будем решать задачу по этапам. Я не покажу готовую архитектуру до вашей рефлексии.\n\n%s\n\nПервый этап — %s.\nЦель: %s\nПонятия для практики: %s",
		strings.TrimSpace(task.PromptPublic), blueprint.Phases[0].Title, blueprint.Phases[0].Goal, strings.Join(concepts, ", "),
	)
}

func (s *Server) learningGoldUnlocked(ctx context.Context, sess store.Session) bool {
	if sess.Status == store.StatusCompleted {
		return true
	}
	state, err := s.store.GetLearningState(ctx, sess.ID)
	return err == nil && state.Phase == "complete"
}
