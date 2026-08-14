package settings

import (
	"os"
	"reflect"
	"testing"
)

func TestDefaultModels(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	want := "deepseek/deepseek-v4-flash-0731"
	if cfg.InterviewerModel != want {
		t.Errorf("InterviewerModel = %q, want %q", cfg.InterviewerModel, want)
	}
	if cfg.EvaluatorModel != want {
		t.Errorf("EvaluatorModel = %q, want %q", cfg.EvaluatorModel, want)
	}
	if cfg.ReasoningEffort != DefaultReasoningEffort {
		t.Errorf("ReasoningEffort = %q, want %q", cfg.ReasoningEffort, DefaultReasoningEffort)
	}
}

func TestNormalizeReasoningEffort(t *testing.T) {
	cases := map[string]string{
		"":        DefaultReasoningEffort,
		"  ":      DefaultReasoningEffort,
		"none":    DefaultReasoningEffort,
		"bogus":   DefaultReasoningEffort,
		"HIGH":    "high",
		"xhigh":   "xhigh",
		"max":     "max",
		" medium": "medium",
		"low":     "low",
		"minimal": "minimal",
	}
	for in, want := range cases {
		if got := NormalizeReasoningEffort(in); got != want {
			t.Errorf("NormalizeReasoningEffort(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadNormalizesReasoningEffort(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.settingsPath(), []byte(`{"reasoning_effort":"none"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ReasoningEffort != DefaultReasoningEffort {
		t.Fatalf("ReasoningEffort = %q, want %q", cfg.ReasoningEffort, DefaultReasoningEffort)
	}
}

func TestSaveNormalizesReasoningEffort(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save(Settings{ReasoningEffort: "XHIGH"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ReasoningEffort != "xhigh" {
		t.Fatalf("ReasoningEffort = %q, want xhigh", cfg.ReasoningEffort)
	}
}

func TestDefaultProviderOrder(t *testing.T) {
	want := []string{"coreweave", "streamlake", "decart", "deepinfra"}
	if !reflect.DeepEqual(DefaultProviderOrder, want) {
		t.Errorf("DefaultProviderOrder = %v, want %v", DefaultProviderOrder, want)
	}
}
