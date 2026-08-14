package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultInterviewerModel = "deepseek/deepseek-v4-flash-0731"
	DefaultEvaluatorModel   = "deepseek/deepseek-v4-flash-0731"
	DefaultTimerMinutes     = 45
	OpenRouterBaseURL       = "https://openrouter.ai/api/v1"
)

// DefaultProviderOrder is OpenRouter provider.order: try these slugs first, then other hosts of the same model.
var DefaultProviderOrder = []string{"coreweave", "streamlake", "decart", "deepinfra"}

type Settings struct {
	InterviewerModel string `json:"interviewer_model"`
	EvaluatorModel   string `json:"evaluator_model"`
	TimerEnabled     bool   `json:"timer_enabled"`
	TimerMinutes     int    `json:"timer_minutes"`
}

type Store struct {
	dir string
}

func Open() (*Store, error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}
	return &Store{dir: dir}, nil
}

func (s *Store) Dir() string { return s.dir }

func (s *Store) Load() (Settings, error) {
	cfg := Settings{
		InterviewerModel: DefaultInterviewerModel,
		EvaluatorModel:   DefaultEvaluatorModel,
		TimerEnabled:     true,
		TimerMinutes:     DefaultTimerMinutes,
	}
	b, err := os.ReadFile(s.settingsPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("parse settings: %w", err)
	}
	if cfg.InterviewerModel == "" {
		cfg.InterviewerModel = DefaultInterviewerModel
	}
	if cfg.EvaluatorModel == "" {
		cfg.EvaluatorModel = DefaultEvaluatorModel
	}
	if cfg.TimerMinutes <= 0 {
		cfg.TimerMinutes = DefaultTimerMinutes
	}
	return cfg, nil
}

func (s *Store) Save(cfg Settings) error {
	if cfg.InterviewerModel == "" {
		cfg.InterviewerModel = DefaultInterviewerModel
	}
	if cfg.EvaluatorModel == "" {
		cfg.EvaluatorModel = DefaultEvaluatorModel
	}
	if cfg.TimerMinutes <= 0 {
		cfg.TimerMinutes = DefaultTimerMinutes
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.settingsPath(), b, 0o600)
}

func (s *Store) APIKey() (string, error) {
	b, err := os.ReadFile(s.keyPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func (s *Store) HasAPIKey() bool {
	k, err := s.APIKey()
	return err == nil && k != ""
}

func (s *Store) SaveAPIKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	if err := os.WriteFile(s.keyPath(), []byte(key+"\n"), 0o600); err != nil {
		return err
	}
	return os.Chmod(s.keyPath(), 0o600)
}

func (s *Store) MaskedKey() string {
	k, err := s.APIKey()
	if err != nil || k == "" {
		return ""
	}
	if len(k) <= 4 {
		return "••••"
	}
	return "••••" + k[len(k)-4:]
}

func (s *Store) settingsPath() string { return filepath.Join(s.dir, "settings.json") }
func (s *Store) keyPath() string      { return filepath.Join(s.dir, "openrouter.key") }

func configDir() (string, error) {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "systemdesign-trainer"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemdesign-trainer"), nil
}

func DataDir() (string, error) {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "systemdesign-trainer"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "systemdesign-trainer"), nil
}
