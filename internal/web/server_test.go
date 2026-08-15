package web_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	traineragent "github.com/timb418/systemdesign-trainer/internal/agent"
	"github.com/timb418/systemdesign-trainer/internal/settings"
	"github.com/timb418/systemdesign-trainer/internal/store"
	"github.com/timb418/systemdesign-trainer/internal/tasks"
	"github.com/timb418/systemdesign-trainer/internal/web"
)

func testServer(t *testing.T) http.Handler {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	fsys, err := tasks.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	bank, err := tasks.Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	set, err := settings.Open()
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	srv, err := web.New(bank, st, set, traineragent.New(bank, set))
	if err != nil {
		t.Fatal(err)
	}
	return srv.Handler()
}

func TestCatalogAndSessionStart(t *testing.T) {
	h := testServer(t)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	client := ts.Client()

	res, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), "Сокращатель ссылок") {
		t.Fatalf("catalog %d %s", res.StatusCode, body)
	}

	res, err = client.Get(ts.URL + "/tasks/url-shortener-v1")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(b), "Полный mock") {
		t.Fatalf("task %d %s", res.StatusCode, b)
	}

	form := url.Values{"task_id": {"url-shortener-v1"}, "mode": {"full_mock"}}
	res, err = client.PostForm(ts.URL+"/sessions", form)
	if err != nil {
		t.Fatal(err)
	}
	loc := res.Request.URL.Path
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(loc, "/sessions/") {
		t.Fatalf("start %d loc=%s", res.StatusCode, loc)
	}
	html, _ := io.ReadAll(mustGet(t, client, ts.URL+loc))
	page := string(html)
	if !strings.Contains(page, "коротких ссылок") {
		t.Fatalf("session missing brief: %s", html)
	}
	if !strings.Contains(page, `data-wait-title="Оцениваем интервью"`) {
		t.Fatalf("complete form missing wait title: %s", html)
	}
}

func mustGet(t *testing.T, c *http.Client, u string) io.ReadCloser {
	t.Helper()
	res, err := c.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("GET %s -> %d", u, res.StatusCode)
	}
	return res.Body
}

func TestSettingsDefaultModels(t *testing.T) {
	h := testServer(t)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	body, _ := io.ReadAll(mustGet(t, ts.Client(), ts.URL+"/settings"))
	html := string(body)
	if !strings.Contains(html, settings.DefaultInterviewerModel) {
		t.Fatalf("settings page missing default model %q: %s", settings.DefaultInterviewerModel, html)
	}
	if !strings.Contains(html, "CoreWeave") || !strings.Contains(html, "DeepInfra") {
		t.Fatalf("settings page missing provider hint: %s", html)
	}
	if !strings.Contains(html, `name="reasoning_effort"`) {
		t.Fatalf("settings page missing reasoning_effort select: %s", html)
	}
	if !strings.Contains(html, `value="high" selected`) && !strings.Contains(html, `value="high"  selected`) {
		t.Fatalf("settings page missing default high effort: %s", html)
	}
	for _, effort := range settings.ReasoningEfforts {
		if !strings.Contains(html, `value="`+effort+`"`) {
			t.Fatalf("settings page missing effort %q: %s", effort, html)
		}
	}
	if strings.Contains(html, `value="none"`) {
		t.Fatal("settings page must not offer none (thinking is mandatory)")
	}
}

func TestSettingsMasksKey(t *testing.T) {
	h := testServer(t)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	form := url.Values{
		"api_key":           {"sk-or-test-secret-key"},
		"interviewer_model": {"openai/gpt-4o-mini"},
		"evaluator_model":   {"openai/gpt-4o"},
		"reasoning_effort":  {"xhigh"},
		"timer_enabled":     {"1"},
		"timer_minutes":     {"45"},
	}
	res, err := ts.Client().PostForm(ts.URL+"/settings", form)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if strings.Contains(string(body), "sk-or-test-secret-key") {
		t.Fatal("raw key leaked into HTML")
	}
	if !strings.Contains(string(body), "••••") {
		t.Fatalf("expected mask: %s", body)
	}
	if !strings.Contains(string(body), `value="xhigh" selected`) && !strings.Contains(string(body), `value="xhigh"  selected`) {
		t.Fatalf("expected saved xhigh effort: %s", body)
	}
}

func TestSettingsRejectsNoneEffort(t *testing.T) {
	h := testServer(t)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	form := url.Values{
		"interviewer_model": {settings.DefaultInterviewerModel},
		"evaluator_model":   {settings.DefaultEvaluatorModel},
		"reasoning_effort":  {"none"},
		"timer_enabled":     {"1"},
		"timer_minutes":     {"45"},
	}
	res, err := ts.Client().PostForm(ts.URL+"/settings", form)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	html := string(body)
	if strings.Contains(html, `value="none" selected`) {
		t.Fatal("none effort must not persist")
	}
	if !strings.Contains(html, `value="high" selected`) && !strings.Contains(html, `value="high"  selected`) {
		t.Fatalf("none should fall back to high: %s", html)
	}
}

func TestPostBoardShowStreamsShownEvent(t *testing.T) {
	h := testServer(t)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	client := ts.Client()

	form := url.Values{"task_id": {"url-shortener-v1"}, "mode": {"full_mock"}}
	res, err := client.PostForm(ts.URL+"/sessions", form)
	if err != nil {
		t.Fatal(err)
	}
	loc := res.Request.URL.Path
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(loc, "/sessions/") {
		t.Fatalf("start %d loc=%s", res.StatusCode, loc)
	}

	xml := `<mxfile><diagram><mxGraphModel><root><mxCell id="0"/><mxCell id="1" parent="0"/><mxCell id="2" value="API" vertex="1" parent="1"/><mxCell id="3" value="DB" vertex="1" parent="1"/><mxCell id="4" edge="1" source="2" target="3" parent="1"/></root></mxGraphModel></diagram></mxfile>`
	payload, err := json.Marshal(map[string]any{"xml": xml, "show": true})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+loc+"/board", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("board show %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content-type %s", ct)
	}
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, `"type":"shown"`) {
		t.Fatalf("missing shown event: %s", text)
	}
	if !strings.Contains(text, `"dump"`) {
		t.Fatalf("missing dump: %s", text)
	}
	if !strings.Contains(text, "2 узла") || !strings.Contains(text, "1 связь") {
		t.Fatalf("dump missing node/edge counts: %s", text)
	}
}

func TestGoldDiagramOnComparePage(t *testing.T) {
	h := testServer(t)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	client := ts.Client()

	form := url.Values{"task_id": {"url-shortener-v1"}, "mode": {"full_mock"}}
	res, err := client.PostForm(ts.URL+"/sessions", form)
	if err != nil {
		t.Fatal(err)
	}
	loc := res.Request.URL.Path
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(loc, "/sessions/") {
		t.Fatalf("start %d loc=%s", res.StatusCode, loc)
	}

	gold, _ := io.ReadAll(mustGet(t, client, ts.URL+loc+"/gold.xml"))
	xml := string(gold)
	if !strings.Contains(xml, "<mxfile") {
		t.Fatalf("gold.xml missing mxfile: %s", xml)
	}
	if !strings.Contains(xml, "Redirect/API Service") {
		t.Fatalf("gold.xml missing gold node: %s", xml)
	}

	html, _ := io.ReadAll(mustGet(t, client, ts.URL+loc+"/compare"))
	page := string(html)
	if !strings.Contains(page, `class="diagram-frame"`) {
		t.Fatalf("compare page missing diagram iframe: %s", page)
	}
	if !strings.Contains(page, `data-xml-url="`+loc+`/gold.xml"`) {
		t.Fatalf("compare page missing gold iframe: %s", page)
	}
	if !strings.Contains(page, `data-xml-url="`+loc+`/board.xml"`) {
		t.Fatalf("compare page missing candidate iframe: %s", page)
	}
}

func TestMarkTaskSolved(t *testing.T) {
	h := testServer(t)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	client := ts.Client()

	catalog, _ := io.ReadAll(mustGet(t, client, ts.URL+"/"))
	if strings.Contains(string(catalog), `class="card is-solved"`) {
		t.Fatal("catalog should start without solved cards")
	}
	if !strings.Contains(string(catalog), "Пометить как решённая") {
		t.Fatalf("catalog missing mark button: %s", catalog)
	}

	form := url.Values{"next": {"/"}}
	res, err := client.PostForm(ts.URL+"/tasks/url-shortener-v1/solved", form)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	html := string(body)
	if res.StatusCode != 200 {
		t.Fatalf("mark %d %s", res.StatusCode, html)
	}
	if !strings.Contains(html, `class="card is-solved"`) {
		t.Fatalf("catalog missing solved class: %s", html)
	}
	if !strings.Contains(html, "Снять отметку") || !strings.Contains(html, `<span class="badge">решена</span>`) {
		t.Fatalf("catalog missing solved UI: %s", html)
	}

	taskPage, _ := io.ReadAll(mustGet(t, client, ts.URL+"/tasks/url-shortener-v1"))
	if !strings.Contains(string(taskPage), "Снять отметку") {
		t.Fatalf("task page missing unmark: %s", taskPage)
	}

	res, err = client.PostForm(ts.URL+"/tasks/url-shortener-v1/solved", url.Values{"next": {"/"}})
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(res.Body)
	res.Body.Close()
	html = string(body)
	if strings.Contains(html, `class="card is-solved"`) {
		t.Fatalf("catalog still marked after unmark: %s", html)
	}
}

func TestLearningModeProgressiveDisclosure(t *testing.T) {
	h := testServer(t)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	client := ts.Client()

	hub, _ := io.ReadAll(mustGet(t, client, ts.URL+"/learn"))
	if !strings.Contains(string(hub), "Основы system design") || !strings.Contains(string(hub), "этапов 0/6") {
		t.Fatalf("learning hub missing track: %s", hub)
	}

	res, err := client.PostForm(ts.URL+"/sessions", url.Values{
		"task_id": {"url-shortener-v1"}, "mode": {"learning"},
	})
	if err != nil {
		t.Fatal(err)
	}
	sessionPath := res.Request.URL.Path
	page, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(sessionPath, "/sessions/") || !strings.Contains(string(page), "Ориентация") {
		t.Fatalf("learning start path=%s page=%s", sessionPath, page)
	}
	if !strings.Contains(string(page), `data-timer="0"`) || !strings.Contains(string(page), "Наставник") {
		t.Fatalf("learning session missing mode-specific UI: %s", page)
	}

	goldRes, err := client.Get(ts.URL + sessionPath + "/gold.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, goldRes.Body)
	goldRes.Body.Close()
	if goldRes.StatusCode != http.StatusForbidden {
		t.Fatalf("gold before reflection = %d, want 403", goldRes.StatusCode)
	}

	blocked, err := client.PostForm(ts.URL+sessionPath+"/learning/advance", url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, blocked.Body)
	blocked.Body.Close()
	if blocked.StatusCode != http.StatusBadRequest {
		t.Fatalf("advance without attempt = %d, want 400", blocked.StatusCode)
	}

	// All of the phase's hints are already rendered as spoilers on first load — no
	// button/click needed to reveal them.
	if !strings.Contains(string(page), "Подсказка 1") || !strings.Contains(string(page), "Подсказка 2") {
		t.Fatalf("static hint list not rendered upfront: %s", page)
	}

	hintRes, err := client.Post(ts.URL+sessionPath+"/learning/hint", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, hintRes.Body)
	hintRes.Body.Close()
	if hintRes.StatusCode != http.StatusNoContent {
		t.Fatalf("learning/hint ping status = %d, want 204", hintRes.StatusCode)
	}

	// The contextual (LLM) hint reuses the chat SSE stream and also counts toward
	// assistance scoring, just like the static hint ping above.
	ctxHintRes, err := client.Post(ts.URL+sessionPath+"/learning/context-hint", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, ctxHintRes.Body)
	ctxHintRes.Body.Close()
	if ctxHintRes.StatusCode != http.StatusOK {
		t.Fatalf("learning/context-hint status = %d, want 200", ctxHintRes.StatusCode)
	}

	for phase := 0; phase < tasks.LearningPhaseCount; phase++ {
		raw, _ := json.Marshal(map[string]string{"content": "Моя самостоятельная попытка для этапа"})
		req, err := http.NewRequest(http.MethodPost, ts.URL+sessionPath+"/messages", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		msgRes, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, msgRes.Body)
		msgRes.Body.Close()
		if msgRes.StatusCode != http.StatusOK {
			t.Fatalf("phase %d message status %d", phase, msgRes.StatusCode)
		}
		advance, err := client.PostForm(ts.URL+sessionPath+"/learning/advance", url.Values{})
		if err != nil {
			t.Fatal(err)
		}
		advancedPage, _ := io.ReadAll(advance.Body)
		advance.Body.Close()
		if advance.StatusCode != http.StatusOK {
			t.Fatalf("phase %d advance status %d", phase, advance.StatusCode)
		}
		// Without an API key the phase-completion check fails open (forced=true),
		// so the sidebar must mark the just-completed phase as forced.
		if !strings.Contains(string(advancedPage), "принудительно") {
			t.Fatalf("phase %d advance missing forced marker: %s", phase, advancedPage)
		}
		if phase == 0 && !strings.Contains(string(advancedPage), "с подсказкой") {
			t.Fatalf("phase 0 should be marked hinted after static+contextual hint use: %s", advancedPage)
		}
	}

	gold, _ := io.ReadAll(mustGet(t, client, ts.URL+sessionPath+"/gold.xml"))
	if !strings.Contains(string(gold), "<mxfile") {
		t.Fatalf("gold should unlock after reflection: %s", gold)
	}
	hub, _ = io.ReadAll(mustGet(t, client, ts.URL+"/learn"))
	if !strings.Contains(string(hub), "этапов 6/6") {
		t.Fatalf("learning completion missing from hub: %s", hub)
	}

	comparePage, _ := io.ReadAll(mustGet(t, client, ts.URL+sessionPath+"/compare"))
	if !strings.Contains(string(comparePage), "Пройдено принудительно") {
		t.Fatalf("compare page missing forced-phase summary: %s", comparePage)
	}
}
