package traineragent

import (
	"context"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"

	"github.com/timb418/systemdesign-trainer/internal/tasks"
)

// maxToolTurns guards against a model looping on tool calls forever.
const maxToolTurns = 4

func hasToolCallDelta(chunk openai.ChatCompletionChunk) bool {
	return len(chunk.Choices) > 0 && len(chunk.Choices[0].Delta.ToolCalls) > 0
}

func chunkText(chunk openai.ChatCompletionChunk) string {
	if len(chunk.Choices) == 0 {
		return ""
	}
	return chunk.Choices[0].Delta.Content
}

// appendStreamText forwards streamed text to onToken and accumulates it in
// draft, but discards any draft text once the model starts emitting a tool
// call — only text produced after the tool round-trip reaches the candidate.
func appendStreamText(draft *strings.Builder, chunk openai.ChatCompletionChunk, onToken TokenFn) {
	if hasToolCallDelta(chunk) {
		draft.Reset()
		return
	}
	text := chunkText(chunk)
	if text == "" {
		return
	}
	draft.WriteString(text)
	if onToken != nil {
		onToken(text)
	}
}

// runToolLoop streams a chat completion, executing reveal_facts round-trips
// as the model requests them, until it produces a final text response.
func runToolLoop(ctx context.Context, client openai.Client, params openai.ChatCompletionNewParams, t tasks.Task, onToken TokenFn) (string, Usage, error) {
	var usage Usage
	for turn := 0; turn < maxToolTurns; turn++ {
		stream := client.Chat.Completions.NewStreaming(ctx, params)
		acc := openai.ChatCompletionAccumulator{}
		var draft strings.Builder
		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)
			appendStreamText(&draft, chunk, onToken)
		}
		if err := stream.Err(); err != nil {
			return strings.TrimSpace(draft.String()), usage, err
		}
		usage = mergeUsage(usage, usageFrom(acc.Usage))

		if len(acc.Choices) == 0 {
			return strings.TrimSpace(draft.String()), usage, nil
		}
		choice := acc.Choices[0]
		if choice.FinishReason != "tool_calls" || len(choice.Message.ToolCalls) == 0 {
			return strings.TrimSpace(draft.String()), usage, nil
		}

		params.Messages = append(params.Messages, choice.Message.ToParam())
		for _, call := range choice.Message.ToolCalls {
			result, err := runRevealFacts(t, call.Function.Arguments)
			if err != nil {
				result = fmt.Sprintf(`{"facts":"ошибка инструмента: %s"}`, err)
			}
			params.Messages = append(params.Messages, openai.ToolMessage(result, call.ID))
		}
	}
	return "", usage, fmt.Errorf("модель слишком долго вызывает инструменты")
}
