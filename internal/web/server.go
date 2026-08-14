package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	traineragent "github.com/timb418/systemdesign-trainer/internal/agent"
	"github.com/timb418/systemdesign-trainer/internal/diagram"
	"github.com/timb418/systemdesign-trainer/internal/settings"
	"github.com/timb418/systemdesign-trainer/internal/store"
	"github.com/timb418/systemdesign-trainer/internal/tasks"
	webassets "github.com/timb418/systemdesign-trainer/web"
)

type Server struct {
	bank   *tasks.Bank
	store  *store.Store
	set    *settings.Store
	agents *traineragent.Agents
	pages  map[string]*template.Template
	drawio string
}

func New(bank *tasks.Bank, st *store.Store, set *settings.Store, agents *traineragent.Agents) (*Server, error) {
	s := &Server{bank: bank, store: st, set: set, agents: agents, pages: map[string]*template.Template{}, drawio: drawioSrc()}
	pages := []string{"catalog.html", "task.html", "session.html", "learning_session.html", "learn.html", "settings.html", "history.html", "rubric.html", "compare.html", "error.html"}
	for _, p := range pages {
		t, err := template.New("").Funcs(template.FuncMap{
			"join":           strings.Join,
			"isBoardShare":   isBoardShare,
			"boardShareDump": boardShareDump,
			"boardShareMeta": boardShareMeta,
		}).ParseFS(webassets.FS, "templates/base.html", "templates/"+p)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		s.pages[p] = t
	}
	return s, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(webassets.FS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	if dir := localDrawioDir(); dir != "" {
		mux.Handle("GET /drawio/", http.StripPrefix("/drawio/", http.FileServer(http.Dir(dir))))
	}
	mux.HandleFunc("GET /{$}", s.catalog)
	mux.HandleFunc("GET /learn", s.learningHub)
	mux.HandleFunc("GET /learn/{track}", s.learningHub)
	mux.HandleFunc("GET /tasks/{id}", s.task)
	mux.HandleFunc("POST /tasks/{id}/solved", s.toggleSolved)
	mux.HandleFunc("POST /sessions", s.startSession)
	mux.HandleFunc("GET /sessions/{id}", s.showSession)
	mux.HandleFunc("GET /sessions/{id}/board.xml", s.boardXML)
	mux.HandleFunc("GET /sessions/{id}/gold.xml", s.goldXML)
	mux.HandleFunc("POST /sessions/{id}/messages", s.postMessage)
	mux.HandleFunc("POST /sessions/{id}/learning/hint", s.learningHint)
	mux.HandleFunc("POST /sessions/{id}/learning/advance", s.learningAdvance)
	mux.HandleFunc("POST /sessions/{id}/board", s.postBoard)
	mux.HandleFunc("POST /sessions/{id}/board/upload", s.uploadBoard)
	mux.HandleFunc("POST /sessions/{id}/complete", s.complete)
	mux.HandleFunc("GET /sessions/{id}/rubric", s.rubric)
	mux.HandleFunc("GET /sessions/{id}/compare", s.compare)
	mux.HandleFunc("POST /sessions/{id}/compare", s.compareAnalyze)
	mux.HandleFunc("GET /history", s.history)
	mux.HandleFunc("GET /settings", s.settingsGet)
	mux.HandleFunc("POST /settings", s.settingsPost)
	return withSecurity(mux)
}

func withSecurity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data: blob:; frame-src 'self' https://embed.diagrams.net; connect-src 'self' https://embed.diagrams.net; form-action 'self'; base-uri 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	t := s.pages[name]
	if t == nil {
		http.Error(w, "template", 500)
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "base.html", data); err != nil {
		log.Printf("template %s: %v", name, err)
		http.Error(w, "template error", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) fail(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	s.render(w, "error.html", map[string]any{"Title": "Ошибка", "Err": msg})
}

type page struct {
	Title string
}

type catalogItem struct {
	tasks.PublicTask
	Solved bool
}

func (s *Server) catalog(w http.ResponseWriter, r *http.Request) {
	typeID := r.URL.Query().Get("type")
	diff := 0
	if v := r.URL.Query().Get("difficulty"); v != "" {
		diff, _ = strconv.Atoi(v)
	}
	status := r.URL.Query().Get("status")
	if status != "open" && status != "solved" {
		status = ""
	}
	solved, err := s.store.SolvedSet(r.Context())
	if err != nil {
		s.fail(w, 500, err.Error())
		return
	}
	public := s.bank.PublicList(typeID, diff)
	items := make([]catalogItem, 0, len(public))
	solvedCount := 0
	for _, p := range public {
		_, ok := solved[p.ID]
		if ok {
			solvedCount++
		}
		items = append(items, catalogItem{PublicTask: p, Solved: ok})
	}
	taskCount := len(items)
	if status == "open" || status == "solved" {
		filtered := make([]catalogItem, 0, len(items))
		want := status == "solved"
		for _, it := range items {
			if it.Solved == want {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Solved != items[j].Solved {
			return !items[i].Solved
		}
		return false
	})
	s.render(w, "catalog.html", map[string]any{
		"Title":        "Каталог",
		"Types":        s.bank.Types(),
		"Tasks":        items,
		"TypeFilter":   typeID,
		"DiffFilter":   diff,
		"StatusFilter": status,
		"Difficulties": []int{1, 2, 3, 4, 5},
		"SolvedCount":  solvedCount,
		"TaskCount":    taskCount,
		"CatalogNext":  catalogNext(typeID, diff, status),
	})
}

func catalogNext(typeID string, diff int, status string) string {
	q := url.Values{}
	if typeID != "" {
		q.Set("type", typeID)
	}
	if diff != 0 {
		q.Set("difficulty", strconv.Itoa(diff))
	}
	if status != "" {
		q.Set("status", status)
	}
	if enc := q.Encode(); enc != "" {
		return "/?" + enc
	}
	return "/"
}

func safeNext(next string) string {
	if next == "" || strings.Contains(next, "://") || strings.HasPrefix(next, "//") || strings.Contains(next, "\\") {
		return "/"
	}
	if next == "/" || strings.HasPrefix(next, "/?") || strings.HasPrefix(next, "/tasks/") {
		return next
	}
	return "/"
}

func (s *Server) toggleSolved(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, 400, "некорректная форма")
		return
	}
	id := r.PathValue("id")
	if _, ok := s.bank.Get(id); !ok {
		s.fail(w, 404, "задача не найдена")
		return
	}
	solved := s.store.IsSolved(r.Context(), id)
	if err := s.store.SetSolved(r.Context(), id, !solved); err != nil {
		s.fail(w, 500, err.Error())
		return
	}
	http.Redirect(w, r, safeNext(r.FormValue("next")), http.StatusSeeOther)
}

func (s *Server) task(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, ok := s.bank.Get(id)
	if !ok {
		s.fail(w, 404, "задача не найдена")
		return
	}
	var names []string
	for _, a := range t.ArchitectureTypes {
		names = append(names, s.bank.TypeName(a))
	}
	s.render(w, "task.html", map[string]any{
		"Title":      t.Title,
		"Task":       t.Public(),
		"TypeNames":  names,
		"CanCompare": s.store.HasCompleted(r.Context(), t.ID),
		"Solved":     s.store.IsSolved(r.Context(), t.ID),
		"Err":        r.URL.Query().Get("err"),
	})
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, 400, "некорректная форма")
		return
	}
	taskID := r.FormValue("task_id")
	t, ok := s.bank.Get(taskID)
	if !ok {
		s.fail(w, 404, "задача не найдена")
		return
	}
	mode, err := store.ParseMode(r.FormValue("mode"))
	if err != nil {
		s.fail(w, 400, err.Error())
		return
	}
	if mode == store.ModeCompareGold && !s.store.HasCompleted(r.Context(), taskID) {
		http.Redirect(w, r, "/tasks/"+taskID+"?err="+urlQuery("сначала завершите mock или практику паттерна"), http.StatusSeeOther)
		return
	}
	cfg, _ := s.set.Load()
	if mode == store.ModeLearning {
		cfg.TimerEnabled = false
	}
	sess, err := s.store.CreateSession(r.Context(), taskID, mode, cfg.TimerEnabled, cfg.TimerMinutes)
	if err != nil {
		s.fail(w, 500, err.Error())
		return
	}
	if mode == store.ModeCompareGold {
		http.Redirect(w, r, "/sessions/"+sess.ID+"/compare", http.StatusSeeOther)
		return
	}
	content := strings.TrimSpace(t.PromptPublic)
	if mode == store.ModeLearning {
		blueprint, _ := s.bank.LearningBlueprint(t.ID)
		if err := s.store.CreateLearningState(r.Context(), sess.ID, blueprint.Phases[0].ID); err != nil {
			s.fail(w, 500, err.Error())
			return
		}
		content = learningIntro(t, blueprint)
	}
	_, _ = s.store.AddMessage(r.Context(), store.Message{
		SessionID: sess.ID,
		Role:      "assistant",
		Content:   content,
	})
	http.Redirect(w, r, "/sessions/"+sess.ID, http.StatusSeeOther)
}

func urlQuery(s string) string {
	return strings.ReplaceAll(s, " ", "+")
}

func (s *Server) showSession(w http.ResponseWriter, r *http.Request) {
	sess, err := s.store.GetSession(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, 404, err.Error())
		return
	}
	t, ok := s.bank.Get(sess.TaskID)
	if !ok {
		s.fail(w, 404, "задача сессии не найдена")
		return
	}
	msgs, _ := s.store.ListMessages(r.Context(), sess.ID)
	if sess.Mode == store.ModeLearning {
		s.showLearningSession(w, r, sess, t, msgs)
		return
	}
	s.render(w, "session.html", map[string]any{
		"Title":     t.Title,
		"Session":   sess,
		"Task":      t.Public(),
		"Messages":  msgs,
		"DrawioURL": s.drawio,
		"HasKey":    s.set.HasAPIKey(),
	})
}

func (s *Server) boardXML(w http.ResponseWriter, r *http.Request) {
	sess, err := s.store.GetSession(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	xmlText := diagram.EmptyXML
	if rev, err := s.store.LatestDiagram(r.Context(), sess.ID); err == nil && rev.XML != "" {
		xmlText = rev.XML
	} else {
		t, ok := s.bank.Get(sess.TaskID)
		if ok && t.Canvas == "sketch" && t.StarterDiagram != "" {
			if raw, err := s.bank.ReadDiagram(t.StarterDiagram); err == nil {
				xmlText = raw
			}
		}
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = io.WriteString(w, xmlText)
}

func (s *Server) goldXML(w http.ResponseWriter, r *http.Request) {
	sess, err := s.store.GetSession(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	t, ok := s.bank.Get(sess.TaskID)
	if !ok || t.PreferredSolution.Diagram == "" {
		http.Error(w, "нет эталонной схемы", 404)
		return
	}
	if sess.Mode == store.ModeLearning && !s.learningGoldUnlocked(r.Context(), sess) {
		http.Error(w, "эталон откроется после рефлексии", http.StatusForbidden)
		return
	}
	raw, err := s.bank.ReadDiagram(t.PreferredSolution.Diagram)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = io.WriteString(w, raw)
}

func (s *Server) postMessage(w http.ResponseWriter, r *http.Request) {
	sess, err := s.store.GetSession(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	if sess.Status != store.StatusInProgress {
		http.Error(w, "сессия завершена", 400)
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Content) == "" {
		http.Error(w, "пустое сообщение", 400)
		return
	}
	t, ok := s.bank.Get(sess.TaskID)
	if !ok {
		http.Error(w, "нет задачи", 404)
		return
	}
	_, _ = s.store.AddMessage(r.Context(), store.Message{SessionID: sess.ID, Role: "user", Content: body.Content})
	history, _ := s.store.ListMessages(r.Context(), sess.ID)
	s.streamConversation(r.Context(), startSSE(w), sess, t, history[:len(history)-1], body.Content)
}

func startSSE(w http.ResponseWriter) func(any) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)
	return func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if flusher != nil {
			flusher.Flush()
		}
	}
}

func (s *Server) streamConversation(ctx context.Context, writeEvt func(any), sess store.Session, t tasks.Task, history []store.Message, userText string) {
	var text string
	var usage traineragent.Usage
	var err error
	history, compactionUsage := s.compactConversationHistory(ctx, sess.ID, history)
	onToken := func(tok string) {
		writeEvt(map[string]any{"type": "token", "text": tok})
	}
	if sess.Mode == store.ModeLearning {
		blueprint, _ := s.bank.LearningBlueprint(t.ID)
		state, stateErr := s.store.GetLearningState(ctx, sess.ID)
		if stateErr != nil {
			writeEvt(map[string]any{"type": "error", "message": stateErr.Error()})
			return
		}
		phase, ok := learningPhase(blueprint, state.Phase)
		if !ok {
			writeEvt(map[string]any{"type": "error", "message": "обучение завершено"})
			return
		}
		text, usage, err = s.agents.Mentor(ctx, sess, t, blueprint, phase, history, userText, onToken)
	} else {
		text, usage, err = s.agents.Interview(ctx, sess, t, history, userText, onToken)
	}
	if err != nil {
		writeEvt(map[string]any{"type": "error", "message": err.Error()})
		return
	}
	usage.PromptTokens += compactionUsage.PromptTokens
	usage.CompletionTokens += compactionUsage.CompletionTokens
	usage.Cost += compactionUsage.Cost
	if text == "" {
		text = "…"
		writeEvt(map[string]any{"type": "token", "text": text})
	}
	_, _ = s.store.AddMessage(ctx, store.Message{
		SessionID:        sess.ID,
		Role:             "assistant",
		Content:          text,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		Cost:             usage.Cost,
	})
	updated, _ := s.store.GetSession(ctx, sess.ID)
	writeEvt(map[string]any{"type": "usage", "label": usageLabel(updated)})
}

func usageLabel(sess store.Session) string {
	if sess.PromptTokens+sess.CompletionTokens == 0 {
		return ""
	}
	s := fmt.Sprintf("%d+%d ток.", sess.PromptTokens, sess.CompletionTokens)
	if sess.Cost > 0 {
		s += fmt.Sprintf(" · $%.4f", sess.Cost)
	}
	return s
}

func (s *Server) postBoard(w http.ResponseWriter, r *http.Request) {
	sess, err := s.store.GetSession(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	var body struct {
		XML  string `json:"xml"`
		Show bool   `json:"show"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.XML == "" {
		http.Error(w, "нет xml", 400)
		return
	}
	topo := diagram.Parse(body.XML)
	_, err = s.store.SaveDiagram(r.Context(), store.DiagramRevision{
		SessionID:          sess.ID,
		XML:                body.XML,
		CanonicalJSON:      store.MustJSON(topo),
		ShownToInterviewer: body.Show,
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if body.Show && sess.Status == store.StatusInProgress {
		dump := topo.Human()
		msg := "Кандидат показал доску. Каноническая проекция:\n" + dump
		_, _ = s.store.AddMessage(r.Context(), store.Message{SessionID: sess.ID, Role: "user", Content: msg})
		writeEvt := startSSE(w)
		writeEvt(map[string]any{
			"type":  "shown",
			"dump":  dump,
			"nodes": len(topo.Nodes),
			"edges": len(topo.Edges),
		})
		t, ok := s.bank.Get(sess.TaskID)
		if !ok {
			writeEvt(map[string]any{"type": "error", "message": "нет задачи"})
			return
		}
		history, _ := s.store.ListMessages(r.Context(), sess.ID)
		s.streamConversation(r.Context(), writeEvt, sess, t, history[:len(history)-1], msg)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) uploadBoard(w http.ResponseWriter, r *http.Request) {
	sess, err := s.store.GetSession(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, 404, err.Error())
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		s.fail(w, 400, "выберите файл")
		return
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 2<<20))
	if err != nil {
		s.fail(w, 400, err.Error())
		return
	}
	xmlText := string(raw)
	topo := diagram.Parse(xmlText)
	_, _ = s.store.SaveDiagram(r.Context(), store.DiagramRevision{
		SessionID: sess.ID, XML: xmlText, CanonicalJSON: store.MustJSON(topo),
	})
	http.Redirect(w, r, "/sessions/"+sess.ID, http.StatusSeeOther)
}

func (s *Server) complete(w http.ResponseWriter, r *http.Request) {
	sess, err := s.store.GetSession(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, 404, err.Error())
		return
	}
	t, ok := s.bank.Get(sess.TaskID)
	if !ok {
		s.fail(w, 404, "нет задачи")
		return
	}
	if sess.Mode == store.ModeLearning && !s.learningGoldUnlocked(r.Context(), sess) {
		s.fail(w, 400, "сначала завершите рефлексию в обучающем режиме")
		return
	}
	msgs, _ := s.store.ListMessages(r.Context(), sess.ID)
	var topo diagram.Topology
	xmlText := ""
	if rev, err := s.store.LatestDiagram(r.Context(), sess.ID); err == nil {
		xmlText = rev.XML
		topo = diagram.Parse(rev.XML)
	}
	if xmlText != "" && sess.Status == store.StatusInProgress {
		_, _ = s.store.SaveDiagram(r.Context(), store.DiagramRevision{
			SessionID: sess.ID, XML: xmlText, CanonicalJSON: store.MustJSON(topo), ShownToInterviewer: true,
		})
	}
	payload := buildEvalPayload(sess, t, msgs, topo)
	if sess.Mode == store.ModeLearning {
		if assistance, err := s.store.LearningPhaseAssistance(r.Context(), sess.ID); err == nil {
			payload += "\nПомощь по учебным этапам (не штрафовать за сам факт помощи):\n"
			for _, phaseID := range []string{"orientation", "requirements", "scale", "hld", "deep_dive", "reflection"} {
				if level := assistance[phaseID]; level != "" {
					payload += fmt.Sprintf("- %s: %s\n", phaseID, level)
				}
			}
		}
	}
	raw, usage, err := s.agents.Evaluate(r.Context(), payload)
	if err != nil {
		s.fail(w, 502, "оценщик: "+err.Error())
		return
	}
	raw = extractJSON(raw)
	if err := s.store.SaveRubric(r.Context(), sess.ID, raw); err != nil {
		s.fail(w, 500, err.Error())
		return
	}
	if usage.PromptTokens > 0 {
		_, _ = s.store.AddMessage(r.Context(), store.Message{
			SessionID: sess.ID, Role: "system", Content: "Сессия завершена, рубрика заполнена.",
			PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens, Cost: usage.Cost,
		})
	}
	http.Redirect(w, r, "/sessions/"+sess.ID+"/rubric", http.StatusSeeOther)
}

func (s *Server) rubric(w http.ResponseWriter, r *http.Request) {
	sess, err := s.store.GetSession(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, 404, err.Error())
		return
	}
	t, _ := s.bank.Get(sess.TaskID)
	rub, err := s.store.GetRubric(r.Context(), sess.ID)
	if err != nil {
		s.fail(w, 404, err.Error())
		return
	}
	s.render(w, "rubric.html", map[string]any{
		"Title":      "Рубрика",
		"Session":    sess,
		"Task":       t.Public(),
		"Criteria":   parseCriteria(rub.JSON),
		"CanCompare": sess.Mode == store.ModeFullMock || sess.Mode == store.ModeDrill || sess.Mode == store.ModeLearning,
		"Usage":      usageLabel(sess),
		"Learning":   sess.Mode == store.ModeLearning,
	})
}

func (s *Server) compare(w http.ResponseWriter, r *http.Request) {
	orig, err := s.store.GetSession(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, 404, err.Error())
		return
	}
	t, ok := s.bank.Get(orig.TaskID)
	if !ok {
		s.fail(w, 404, "нет задачи")
		return
	}
	if orig.Mode == store.ModeLearning && !s.learningGoldUnlocked(r.Context(), orig) {
		s.fail(w, 403, "эталон откроется после рефлексии")
		return
	}
	attempt := orig
	if orig.Mode == store.ModeCompareGold {
		prev, err := s.store.LatestCompleted(r.Context(), orig.TaskID, orig.ID)
		if err != nil {
			s.fail(w, 400, err.Error())
			return
		}
		attempt = prev
	}
	candDump := "(нет схемы)"
	if rev, err := s.store.LatestDiagram(r.Context(), attempt.ID); err == nil {
		candDump = diagram.Parse(rev.XML).Human()
	}
	goldDump := "(нет эталонной схемы)"
	if t.PreferredSolution.Diagram != "" {
		if raw, err := s.bank.ReadDiagram(t.PreferredSolution.Diagram); err == nil {
			goldDump = diagram.Parse(raw).Human()
		}
	}
	view := map[string]any{
		"Title":         "Эталон",
		"Session":       orig,
		"AttemptID":     attempt.ID,
		"DrawioURL":     s.drawio,
		"CandidateDump": candDump,
		"GoldDump":      goldDump,
		"GoldNarrative": t.PreferredSolution.Narrative,
		"GoldTradeoffs": t.PreferredSolution.Tradeoffs,
		"Notes":         orig.CompareNotes,
		"CanAnalyze":    s.set.HasAPIKey(),
		"Learning":      orig.Mode == store.ModeLearning,
	}
	if orig.Mode == store.ModeLearning {
		blueprint, _ := s.bank.LearningBlueprint(orig.TaskID)
		view["Concepts"] = blueprint.Concepts
		view["CommonMistakes"] = blueprint.CommonMistakes
		view["Assistance"], _ = s.store.LearningPhaseAssistance(r.Context(), orig.ID)
	}
	s.render(w, "compare.html", view)
}

func (s *Server) compareAnalyze(w http.ResponseWriter, r *http.Request) {
	orig, err := s.store.GetSession(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, 404, err.Error())
		return
	}
	t, ok := s.bank.Get(orig.TaskID)
	if !ok {
		s.fail(w, 404, "нет задачи")
		return
	}
	attempt := orig
	if orig.Mode == store.ModeCompareGold {
		prev, err := s.store.LatestCompleted(r.Context(), orig.TaskID, orig.ID)
		if err != nil {
			s.fail(w, 400, err.Error())
			return
		}
		attempt = prev
	}
	msgs, _ := s.store.ListMessages(r.Context(), attempt.ID)
	var topo diagram.Topology
	if rev, err := s.store.LatestDiagram(r.Context(), attempt.ID); err == nil {
		topo = diagram.Parse(rev.XML)
	}
	payload := buildComparePayload(t, msgs, topo)
	raw, _, err := s.agents.Compare(r.Context(), payload)
	if err != nil {
		s.fail(w, 502, err.Error())
		return
	}
	notes := formatCompare(extractJSON(raw))
	_ = s.store.SaveCompareNotes(r.Context(), orig.ID, notes)
	http.Redirect(w, r, "/sessions/"+orig.ID+"/compare", http.StatusSeeOther)
}

func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListSessions(r.Context())
	if err != nil {
		s.fail(w, 500, err.Error())
		return
	}
	titles := map[string]string{}
	for _, sess := range list {
		if t, ok := s.bank.Get(sess.TaskID); ok {
			titles[sess.TaskID] = t.Title
		} else {
			titles[sess.TaskID] = sess.TaskID
		}
	}
	s.render(w, "history.html", map[string]any{"Title": "История", "Sessions": list, "Titles": titles})
}

func (s *Server) settingsGet(w http.ResponseWriter, r *http.Request) {
	cfg, _ := s.set.Load()
	s.render(w, "settings.html", s.settingsView(cfg, r.URL.Query().Get("saved") == "1", ""))
}

func (s *Server) settingsPost(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	cfg, _ := s.set.Load()
	cfg.InterviewerModel = strings.TrimSpace(r.FormValue("interviewer_model"))
	cfg.EvaluatorModel = strings.TrimSpace(r.FormValue("evaluator_model"))
	cfg.ReasoningEffort = settings.NormalizeReasoningEffort(r.FormValue("reasoning_effort"))
	cfg.TimerEnabled = r.FormValue("timer_enabled") == "1"
	if n, err := strconv.Atoi(r.FormValue("timer_minutes")); err == nil {
		cfg.TimerMinutes = n
	}
	if err := s.set.Save(cfg); err != nil {
		s.render(w, "settings.html", s.settingsView(cfg, false, err.Error()))
		return
	}
	if k := r.FormValue("api_key"); strings.TrimSpace(k) != "" && !strings.Contains(k, "••••") {
		if err := s.set.SaveAPIKey(k); err != nil {
			s.render(w, "settings.html", s.settingsView(cfg, false, err.Error()))
			return
		}
	}
	http.Redirect(w, r, "/settings?saved=1", http.StatusSeeOther)
}

func (s *Server) settingsView(cfg settings.Settings, saved bool, errMsg string) map[string]any {
	return map[string]any{
		"Title":            "Настройки",
		"Settings":         cfg,
		"Masked":           s.set.MaskedKey(),
		"Saved":            saved,
		"Err":              errMsg,
		"ReasoningEfforts": settings.ReasoningEfforts,
	}
}

func localDrawioDir() string {
	cands := []string{
		filepath.Join("third_party", "drawio", "src", "main", "webapp"),
		filepath.Join("third_party", "drawio"),
	}
	if exe, err := os.Executable(); err == nil {
		cands = append(cands, filepath.Join(filepath.Dir(exe), "third_party", "drawio", "src", "main", "webapp"))
	}
	for _, d := range cands {
		if _, err := os.Stat(filepath.Join(d, "index.html")); err == nil {
			abs, _ := filepath.Abs(d)
			return abs
		}
	}
	return ""
}

func drawioSrc() string {
	if localDrawioDir() != "" {
		return "/drawio/index.html"
	}
	return "https://embed.diagrams.net/"
}

func ListenAndServe(addr string, h http.Handler) error {
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host != "127.0.0.1" && host != "localhost" {
		return fmt.Errorf("слушаем только 127.0.0.1, получено %s", addr)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	log.Printf("System Design Trainer: http://%s", addr)
	return http.Serve(ln, h)
}
