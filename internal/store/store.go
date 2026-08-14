package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type Mode string

const (
	ModeFullMock     Mode = "full_mock"
	ModeDrill        Mode = "drill"
	ModeRequirements Mode = "requirements_only"
	ModeCompareGold  Mode = "compare_gold"
)

func (m Mode) String() string { return string(m) }

func (m Mode) Label() string {
	switch m {
	case ModeFullMock:
		return "Полный mock"
	case ModeDrill:
		return "Практика паттерна"
	case ModeRequirements:
		return "Только требования"
	case ModeCompareGold:
		return "Сравнение с эталоном"
	default:
		return string(m)
	}
}

func ParseMode(s string) (Mode, error) {
	switch Mode(s) {
	case ModeFullMock, ModeDrill, ModeRequirements, ModeCompareGold:
		return Mode(s), nil
	default:
		return "", fmt.Errorf("неизвестный режим %q", s)
	}
}

type Status string

const (
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
)

func (s Status) Label() string {
	switch s {
	case StatusCompleted:
		return "завершена"
	default:
		return "в процессе"
	}
}

type Session struct {
	ID               string
	TaskID           string
	Mode             Mode
	Status           Status
	TimerEnabled     bool
	TimerMinutes     int
	StartedAt        time.Time
	EndedAt          *time.Time
	PromptTokens     int
	CompletionTokens int
	Cost             float64
	CompareNotes     string
}

type Message struct {
	ID               int64
	SessionID        string
	Role             string
	Content          string
	CreatedAt        time.Time
	PromptTokens     int
	CompletionTokens int
	Cost             float64
}

type DiagramRevision struct {
	ID                 int64
	SessionID          string
	XML                string
	CanonicalJSON      string
	ShownToInterviewer bool
	CreatedAt          time.Time
}

type Rubric struct {
	SessionID string
	JSON      string
	CreatedAt time.Time
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  mode TEXT NOT NULL,
  status TEXT NOT NULL,
  timer_enabled INTEGER NOT NULL,
  timer_minutes INTEGER NOT NULL,
  started_at TEXT NOT NULL,
  ended_at TEXT,
  prompt_tokens INTEGER NOT NULL DEFAULT 0,
  completion_tokens INTEGER NOT NULL DEFAULT 0,
  cost REAL NOT NULL DEFAULT 0,
  compare_notes TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  role TEXT NOT NULL,
  content TEXT NOT NULL,
  created_at TEXT NOT NULL,
  prompt_tokens INTEGER NOT NULL DEFAULT 0,
  completion_tokens INTEGER NOT NULL DEFAULT 0,
  cost REAL NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS diagram_revisions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  xml TEXT NOT NULL,
  canonical_json TEXT NOT NULL,
  shown_to_interviewer INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS rubrics (
  session_id TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
  json TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS solved_tasks (
  task_id TEXT PRIMARY KEY,
  solved_at TEXT NOT NULL
);
`)
	return err
}

func (s *Store) CreateSession(ctx context.Context, taskID string, mode Mode, timerEnabled bool, timerMinutes int) (Session, error) {
	sess := Session{
		ID:           uuid.NewString(),
		TaskID:       taskID,
		Mode:         mode,
		Status:       StatusInProgress,
		TimerEnabled: timerEnabled,
		TimerMinutes: timerMinutes,
		StartedAt:    time.Now().UTC(),
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO sessions (id, task_id, mode, status, timer_enabled, timer_minutes, started_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.TaskID, sess.Mode, sess.Status, boolInt(sess.TimerEnabled), sess.TimerMinutes, sess.StartedAt.Format(time.RFC3339Nano))
	return sess, err
}

func (s *Store) GetSession(ctx context.Context, id string) (Session, error) {
	var sess Session
	var started, ended sql.NullString
	var timer int
	err := s.db.QueryRowContext(ctx, `
SELECT id, task_id, mode, status, timer_enabled, timer_minutes, started_at, ended_at,
       prompt_tokens, completion_tokens, cost, compare_notes
FROM sessions WHERE id = ?`, id).Scan(
		&sess.ID, &sess.TaskID, &sess.Mode, &sess.Status, &timer, &sess.TimerMinutes,
		&started, &ended, &sess.PromptTokens, &sess.CompletionTokens, &sess.Cost, &sess.CompareNotes,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return sess, fmt.Errorf("сессия не найдена")
	}
	if err != nil {
		return sess, err
	}
	sess.TimerEnabled = timer != 0
	sess.StartedAt, _ = time.Parse(time.RFC3339Nano, started.String)
	if ended.Valid && ended.String != "" {
		t, _ := time.Parse(time.RFC3339Nano, ended.String)
		sess.EndedAt = &t
	}
	return sess, nil
}

func (s *Store) ListSessions(ctx context.Context) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, task_id, mode, status, timer_enabled, timer_minutes, started_at, ended_at,
       prompt_tokens, completion_tokens, cost, compare_notes
FROM sessions ORDER BY started_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var sess Session
		var started, ended sql.NullString
		var timer int
		if err := rows.Scan(&sess.ID, &sess.TaskID, &sess.Mode, &sess.Status, &timer, &sess.TimerMinutes,
			&started, &ended, &sess.PromptTokens, &sess.CompletionTokens, &sess.Cost, &sess.CompareNotes); err != nil {
			return nil, err
		}
		sess.TimerEnabled = timer != 0
		sess.StartedAt, _ = time.Parse(time.RFC3339Nano, started.String)
		if ended.Valid && ended.String != "" {
			t, _ := time.Parse(time.RFC3339Nano, ended.String)
			sess.EndedAt = &t
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

func (s *Store) LatestCompleted(ctx context.Context, taskID string, excludeID string) (Session, error) {
	var sess Session
	var started, ended sql.NullString
	var timer int
	err := s.db.QueryRowContext(ctx, `
SELECT id, task_id, mode, status, timer_enabled, timer_minutes, started_at, ended_at,
       prompt_tokens, completion_tokens, cost, compare_notes
FROM sessions
WHERE task_id = ? AND status = ? AND mode IN (?, ?) AND id != ?
ORDER BY started_at DESC LIMIT 1`,
		taskID, StatusCompleted, ModeFullMock, ModeDrill, excludeID,
	).Scan(&sess.ID, &sess.TaskID, &sess.Mode, &sess.Status, &timer, &sess.TimerMinutes,
		&started, &ended, &sess.PromptTokens, &sess.CompletionTokens, &sess.Cost, &sess.CompareNotes)
	if errors.Is(err, sql.ErrNoRows) {
		return sess, fmt.Errorf("нет завершённой попытки по этой задаче")
	}
	if err != nil {
		return sess, err
	}
	sess.TimerEnabled = timer != 0
	sess.StartedAt, _ = time.Parse(time.RFC3339Nano, started.String)
	if ended.Valid && ended.String != "" {
		t, _ := time.Parse(time.RFC3339Nano, ended.String)
		sess.EndedAt = &t
	}
	return sess, nil
}

func (s *Store) HasCompleted(ctx context.Context, taskID string) bool {
	var n int
	_ = s.db.QueryRowContext(ctx, `
SELECT COUNT(1) FROM sessions WHERE task_id = ? AND status = ? AND mode IN (?, ?)`,
		taskID, StatusCompleted, ModeFullMock, ModeDrill).Scan(&n)
	return n > 0
}

func (s *Store) SetSolved(ctx context.Context, taskID string, solved bool) error {
	if !solved {
		_, err := s.db.ExecContext(ctx, `DELETE FROM solved_tasks WHERE task_id = ?`, taskID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO solved_tasks (task_id, solved_at) VALUES (?, ?)
ON CONFLICT(task_id) DO NOTHING`,
		taskID, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) SolvedSet(ctx context.Context) (map[string]struct{}, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT task_id FROM solved_tasks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

func (s *Store) IsSolved(ctx context.Context, taskID string) bool {
	var n int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM solved_tasks WHERE task_id = ?`, taskID).Scan(&n)
	return n > 0
}

func (s *Store) AddMessage(ctx context.Context, m Message) (Message, error) {
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO messages (session_id, role, content, created_at, prompt_tokens, completion_tokens, cost)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		m.SessionID, m.Role, m.Content, m.CreatedAt.Format(time.RFC3339Nano),
		m.PromptTokens, m.CompletionTokens, m.Cost)
	if err != nil {
		return m, err
	}
	m.ID, _ = res.LastInsertId()
	_, _ = s.db.ExecContext(ctx, `
UPDATE sessions SET prompt_tokens = prompt_tokens + ?, completion_tokens = completion_tokens + ?, cost = cost + ?
WHERE id = ?`, m.PromptTokens, m.CompletionTokens, m.Cost, m.SessionID)
	return m, nil
}

func (s *Store) ListMessages(ctx context.Context, sessionID string) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, role, content, created_at, prompt_tokens, completion_tokens, cost
FROM messages WHERE session_id = ? ORDER BY id ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		var created string
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &created, &m.PromptTokens, &m.CompletionTokens, &m.Cost); err != nil {
			return nil, err
		}
		m.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) SaveDiagram(ctx context.Context, r DiagramRevision) (DiagramRevision, error) {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO diagram_revisions (session_id, xml, canonical_json, shown_to_interviewer, created_at)
VALUES (?, ?, ?, ?, ?)`,
		r.SessionID, r.XML, r.CanonicalJSON, boolInt(r.ShownToInterviewer), r.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return r, err
	}
	r.ID, _ = res.LastInsertId()
	return r, nil
}

func (s *Store) LatestDiagram(ctx context.Context, sessionID string) (DiagramRevision, error) {
	var r DiagramRevision
	var created string
	var shown int
	err := s.db.QueryRowContext(ctx, `
SELECT id, session_id, xml, canonical_json, shown_to_interviewer, created_at
FROM diagram_revisions WHERE session_id = ? ORDER BY id DESC LIMIT 1`, sessionID).Scan(
		&r.ID, &r.SessionID, &r.XML, &r.CanonicalJSON, &shown, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return r, fmt.Errorf("нет схемы")
	}
	if err != nil {
		return r, err
	}
	r.ShownToInterviewer = shown != 0
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return r, nil
}

func (s *Store) SaveRubric(ctx context.Context, sessionID, raw string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO rubrics (session_id, json, created_at) VALUES (?, ?, ?)
ON CONFLICT(session_id) DO UPDATE SET json = excluded.json, created_at = excluded.created_at`,
		sessionID, raw, now)
	if err != nil {
		return err
	}
	ended := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `UPDATE sessions SET status = ?, ended_at = ? WHERE id = ?`, StatusCompleted, ended, sessionID)
	return err
}

func (s *Store) GetRubric(ctx context.Context, sessionID string) (Rubric, error) {
	var r Rubric
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT session_id, json, created_at FROM rubrics WHERE session_id = ?`, sessionID).
		Scan(&r.SessionID, &r.JSON, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return r, fmt.Errorf("рубрика не найдена")
	}
	if err != nil {
		return r, err
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return r, nil
}

func (s *Store) SaveCompareNotes(ctx context.Context, sessionID, notes string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET compare_notes = ? WHERE id = ?`, notes, sessionID)
	return err
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func MustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
