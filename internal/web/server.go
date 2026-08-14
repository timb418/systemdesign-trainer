package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
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
	pages := []string{"catalog.html", "task.html", "session.html", "settings.html", "history.html", "rubric.html", "compare.html", "error.html"}
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
	mux.HandleFunc("GET /tasks/{id}", s.task)
	mux.HandleFunc("POST /sessions", s.startSession)
	mux.HandleFunc("GET /sessions/{id}", s.showSession)
	mux.HandleFunc("GET /sessions/{id}/board.xml", s.boardXML)
	mux.HandleFunc("POST /sessions/{id}/messages", s.postMessage)
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

func (s *Server) catalog(w http.ResponseWriter, r *http.Request) {
	typeID := r.URL.Query().Get("type")
	diff := 0
	if v := r.URL.Query().Get("difficulty"); v != "" {
		diff, _ = strconv.Atoi(v)
	}
	s.render(w, "catalog.html", map[string]any{
		"Title":        "Каталог",
		"Types":        s.bank.Types(),
		"Tasks":        s.bank.PublicList(typeID, diff),
		"TypeFilter":   typeID,
		"DiffFilter":   diff,
		"Difficulties": []int{1, 2, 3, 4, 5},
	})
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
		http.Redirect(w, r, "/tasks/"+taskID+"?err="+urlQuery("сначала завершите mock или дрилл"), http.StatusSeeOther)
		return
	}
	cfg, _ := s.set.Load()
	sess, err := s.store.CreateSession(r.Context(), taskID, mode, cfg.TimerEnabled, cfg.TimerMinutes)
	if err != nil {
		s.fail(w, 500, err.Error())
		return
	}
	if mode == store.ModeCompareGold {
		http.Redirect(w, r, "/sessions/"+sess.ID+"/compare", http.StatusSeeOther)
		return
	}
	_, _ = s.store.AddMessage(r.Context(), store.Message{
		SessionID: sess.ID,
		Role:      "assistant",
		Content:   strings.TrimSpace(t.PromptPublic),
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
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)
	writeEvt := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if flusher != nil {
			flusher.Flush()
		}
	}
	text, usage, err := s.agents.Interview(r.Context(), sess, t, history[:len(history)-1], body.Content, func(tok string) {
		writeEvt(map[string]any{"type": "token", "text": tok})
	})
	if err != nil {
		writeEvt(map[string]any{"type": "error", "message": err.Error()})
		return
	}
	if text == "" {
		text = "…"
		writeEvt(map[string]any{"type": "token", "text": text})
	}
	_, _ = s.store.AddMessage(r.Context(), store.Message{
		SessionID:        sess.ID,
		Role:             "assistant",
		Content:          text,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		Cost:             usage.Cost,
	})
	updated, _ := s.store.GetSession(r.Context(), sess.ID)
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
		reply := ""
		t, ok := s.bank.Get(sess.TaskID)
		if ok {
			history, _ := s.store.ListMessages(r.Context(), sess.ID)
			text, usage, err := s.agents.Interview(r.Context(), sess, t, history[:len(history)-1], msg, nil)
			if err == nil && text != "" {
				reply = text
				_, _ = s.store.AddMessage(r.Context(), store.Message{
					SessionID: sess.ID, Role: "assistant", Content: reply,
					PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens, Cost: usage.Cost,
				})
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"shown": true,
			"dump":  dump,
			"nodes": len(topo.Nodes),
			"edges": len(topo.Edges),
			"reply": reply,
		})
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
		"CanCompare": sess.Mode == store.ModeFullMock || sess.Mode == store.ModeDrill,
		"Usage":      usageLabel(sess),
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
	s.render(w, "compare.html", map[string]any{
		"Title":         "Эталон",
		"Session":       orig,
		"CandidateDump": candDump,
		"GoldDump":      goldDump,
		"GoldNarrative": t.PreferredSolution.Narrative,
		"GoldTradeoffs": t.PreferredSolution.Tradeoffs,
		"Notes":         orig.CompareNotes,
		"CanAnalyze":    s.set.HasAPIKey(),
	})
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
