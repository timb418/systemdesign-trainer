package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/timb418/systemdesign-trainer/internal/store"
)

func TestSessionRoundtrip(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	sess, err := st.CreateSession(ctx, "url-shortener-v1", store.ModeFullMock, true, 45)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.AddMessage(ctx, store.Message{SessionID: sess.ID, Role: "assistant", Content: "бриф"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TaskID != "url-shortener-v1" || got.Mode != store.ModeFullMock {
		t.Fatalf("%+v", got)
	}
	msgs, err := st.ListMessages(ctx, sess.ID)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("msgs %v %v", msgs, err)
	}
	if err := st.SaveRubric(ctx, sess.ID, `{"criteria":[]}`); err != nil {
		t.Fatal(err)
	}
	got, err = st.GetSession(ctx, sess.ID)
	if err != nil || got.Status != store.StatusCompleted {
		t.Fatalf("completed: %+v %v", got, err)
	}
}

func TestSolvedToggle(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()

	if st.IsSolved(ctx, "url-shortener-v1") {
		t.Fatal("expected unsolved")
	}
	if err := st.SetSolved(ctx, "url-shortener-v1", true); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSolved(ctx, "url-shortener-v1", true); err != nil {
		t.Fatal(err)
	}
	if !st.IsSolved(ctx, "url-shortener-v1") {
		t.Fatal("expected solved")
	}
	set, err := st.SolvedSet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := set["url-shortener-v1"]; !ok || len(set) != 1 {
		t.Fatalf("set %+v", set)
	}
	if err := st.SetSolved(ctx, "url-shortener-v1", false); err != nil {
		t.Fatal(err)
	}
	if st.IsSolved(ctx, "url-shortener-v1") {
		t.Fatal("expected unsolved after clear")
	}
	set, err = st.SolvedSet(ctx)
	if err != nil || len(set) != 0 {
		t.Fatalf("empty set %+v %v", set, err)
	}
}

func TestLearningProgressRoundtrip(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "learning.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	sess, err := st.CreateSession(ctx, "url-shortener-v1", store.ModeLearning, false, 45)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateLearningState(ctx, sess.ID, "orientation"); err != nil {
		t.Fatal(err)
	}
	state, err := st.IncreaseLearningHint(ctx, sess.ID, 4)
	if err != nil || state.HintLevel != 1 {
		t.Fatalf("hint state %+v err=%v", state, err)
	}
	if err := st.AdvanceLearningPhase(ctx, sess.ID, "orientation", "requirements"); err != nil {
		t.Fatal(err)
	}
	state, err = st.GetLearningState(ctx, sess.ID)
	if err != nil || state.Phase != "requirements" || state.HintLevel != 0 {
		t.Fatalf("advanced state %+v err=%v", state, err)
	}
	assistance, err := st.LearningPhaseAssistance(ctx, sess.ID)
	if err != nil || assistance["orientation"] != "hinted" {
		t.Fatalf("assistance %+v err=%v", assistance, err)
	}
	progress, err := st.LearningProgress(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := progress["url-shortener-v1"]
	if got.SessionID != sess.ID || got.CurrentPhase != "requirements" || got.CompletedPhases != 1 {
		t.Fatalf("progress %+v", got)
	}
}
