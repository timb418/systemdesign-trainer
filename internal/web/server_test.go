package web_test

import (
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
