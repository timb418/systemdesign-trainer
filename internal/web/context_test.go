package web

import (
	"strings"
	"testing"

	"github.com/timb418/systemdesign-trainer/internal/store"
)

func TestBoundedHistoryKeepsSummaryTailAndLatestBoard(t *testing.T) {
	history := []store.Message{{ID: 1, Role: "assistant", Content: "публичный бриф"}}
	for i := int64(2); i <= 18; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		history = append(history, store.Message{ID: i, Role: role, Content: "сообщение"})
	}
	history[5].Content = boardSharePrefix + "\nстарая схема"
	history[12].Content = boardSharePrefix + "\nпоследняя схема"

	got := boundedHistory(history, store.ContextSummary{
		Content:          "- раскрыто требование",
		ThroughMessageID: 8,
	})

	if got[0].Content != "публичный бриф" {
		t.Fatalf("first message = %q", got[0].Content)
	}
	if !strings.Contains(got[1].Content, "раскрыто требование") {
		t.Fatalf("missing summary: %+v", got)
	}
	boardCount := 0
	for _, m := range got {
		if isBoardShare(m.Content) {
			boardCount++
			if !strings.Contains(m.Content, "последняя схема") {
				t.Fatalf("kept stale board: %q", m.Content)
			}
		}
		if m.ID > 0 && m.ID <= 8 && m.ID != 1 {
			t.Fatalf("kept compacted message %d", m.ID)
		}
	}
	if boardCount != 1 {
		t.Fatalf("board count = %d", boardCount)
	}
}

func TestRelevantTranscriptDropsSyntheticMessages(t *testing.T) {
	msgs := []store.Message{
		{Role: "assistant", Content: "вопрос"},
		{Role: "user", Content: "ответ"},
		{Role: "system", Content: "служебное"},
		{Role: "user", Content: boardSharePrefix + "\nA -> B"},
	}
	var b strings.Builder
	writeRelevantTranscript(&b, msgs)
	got := b.String()
	if !strings.Contains(got, "[interviewer] вопрос") || !strings.Contains(got, "[candidate] ответ") {
		t.Fatalf("missing dialogue: %q", got)
	}
	if strings.Contains(got, "служебное") || strings.Contains(got, "A -> B") {
		t.Fatalf("synthetic content leaked: %q", got)
	}
}
