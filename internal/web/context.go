package web

import (
	"context"
	"fmt"
	"log"
	"strings"

	traineragent "github.com/timb418/systemdesign-trainer/internal/agent"
	"github.com/timb418/systemdesign-trainer/internal/store"
)

const (
	contextRawTail  = 10
	contextMinBatch = 6
)

func (s *Server) compactConversationHistory(ctx context.Context, sessionID string, history []store.Message) ([]store.Message, traineragent.Usage) {
	if len(history) == 0 {
		return nil, traineragent.Usage{}
	}

	summary, err := s.store.GetContextSummary(ctx, sessionID)
	if err != nil {
		log.Printf("load context summary session=%s: %v", sessionID, err)
		return boundedHistory(history, store.ContextSummary{}), traineragent.Usage{}
	}

	cutoff := len(history) - contextRawTail
	if cutoff > 1 {
		var pending []store.Message
		for _, m := range history[1:cutoff] {
			if m.ID > summary.ThroughMessageID && !isBoardShare(m.Content) {
				pending = append(pending, m)
			}
		}
		if len(pending) >= contextMinBatch {
			payload := summaryPayload(summary.Content, pending)
			content, usage, summarizeErr := s.agents.Summarize(ctx, payload)
			if summarizeErr != nil {
				log.Printf("compact context session=%s: %v", sessionID, summarizeErr)
				return boundedHistory(history, summary), traineragent.Usage{}
			}
			if strings.TrimSpace(content) != "" {
				summary = store.ContextSummary{
					SessionID:        sessionID,
					Content:          strings.TrimSpace(content),
					ThroughMessageID: history[cutoff-1].ID,
				}
				if saveErr := s.store.SaveContextSummary(ctx, summary); saveErr != nil {
					log.Printf("save context summary session=%s: %v", sessionID, saveErr)
				}
			}
			return boundedHistory(history, summary), usage
		}
	}

	return boundedHistory(history, summary), traineragent.Usage{}
}

func summaryPayload(previous string, messages []store.Message) string {
	var b strings.Builder
	if strings.TrimSpace(previous) != "" {
		b.WriteString("Предыдущее summary:\n")
		b.WriteString(strings.TrimSpace(previous))
		b.WriteString("\n\n")
	}
	b.WriteString("Новая часть диалога для сжатия:\n")
	for _, m := range messages {
		fmt.Fprintf(&b, "[%s]\n%s\n\n", m.Role, strings.TrimSpace(m.Content))
	}
	return b.String()
}

func boundedHistory(history []store.Message, summary store.ContextSummary) []store.Message {
	if len(history) == 0 {
		return nil
	}

	out := []store.Message{history[0]}
	if strings.TrimSpace(summary.Content) != "" {
		out = append(out, store.Message{
			Role:    "system",
			Content: "Рабочая память старой части диалога:\n" + strings.TrimSpace(summary.Content),
		})
	}

	var latestBoard *store.Message
	for i := range history {
		if isBoardShare(history[i].Content) {
			copy := history[i]
			latestBoard = &copy
		}
	}
	if latestBoard != nil && latestBoard.ID <= summary.ThroughMessageID {
		out = append(out, *latestBoard)
	}

	for i := range history {
		m := history[i]
		if isBoardShare(m.Content) && (latestBoard == nil || m.ID != latestBoard.ID) {
			continue
		}
		if i == 0 || m.ID <= summary.ThroughMessageID {
			continue
		}
		out = append(out, m)
	}

	if summary.Content == "" && len(out) > contextRawTail+1 {
		tail := append([]store.Message(nil), out[len(out)-contextRawTail:]...)
		out = append([]store.Message{history[0]}, tail...)
	}
	return out
}
