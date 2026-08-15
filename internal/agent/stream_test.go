package traineragent

import (
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
)

func textChunk(text string) openai.ChatCompletionChunk {
	return openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{Delta: openai.ChatCompletionChunkChoiceDelta{Content: text}},
		},
	}
}

func toolCallChunk() openai.ChatCompletionChunk {
	return openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{Delta: openai.ChatCompletionChunkChoiceDelta{
				ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{
					{Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{Name: "reveal_facts"}},
				},
			}},
		},
	}
}

func TestChunkTextSkipsWhenToolCallPresent(t *testing.T) {
	if got := chunkText(textChunk("visible")); got != "visible" {
		t.Fatalf("chunkText = %q, want %q", got, "visible")
	}
	if got := chunkText(openai.ChatCompletionChunk{}); got != "" {
		t.Fatalf("chunkText on empty chunk = %q, want empty", got)
	}
}

func TestHasToolCallDelta(t *testing.T) {
	if hasToolCallDelta(openai.ChatCompletionChunk{}) {
		t.Fatal("empty chunk should not have a tool call delta")
	}
	if hasToolCallDelta(textChunk("hi")) {
		t.Fatal("plain text chunk should not have a tool call delta")
	}
	if !hasToolCallDelta(toolCallChunk()) {
		t.Fatal("tool call chunk should have a tool call delta")
	}
}

func TestAppendStreamTextDropsPreToolDraft(t *testing.T) {
	var draft strings.Builder
	appendStreamText(&draft, textChunk("Вызываю reveal_facts"), nil)
	appendStreamText(&draft, toolCallChunk(), nil)
	appendStreamText(&draft, textChunk("10 млн созданий"), nil)

	if got := draft.String(); got != "10 млн созданий" {
		t.Fatalf("draft = %q, want candidate-facing reply only", got)
	}
}
