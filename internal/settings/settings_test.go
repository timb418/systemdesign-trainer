package settings

import (
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
}

func TestDefaultProviderOrder(t *testing.T) {
	want := []string{"coreweave", "streamlake", "decart", "deepinfra"}
	if !reflect.DeepEqual(DefaultProviderOrder, want) {
		t.Errorf("DefaultProviderOrder = %v, want %v", DefaultProviderOrder, want)
	}
}
