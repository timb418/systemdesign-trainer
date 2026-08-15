package traineragent

import (
	"context"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

func rubricSchema() map[string]any {
	criterion := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":      map[string]any{"type": "string", "description": "requirements|scale|hld|bottlenecks|reliability|tradeoffs|communication"},
			"level":   map[string]any{"type": "string", "description": "weak|ok|strong|n_a"},
			"comment": map[string]any{"type": "string"},
		},
		"required": []string{"id", "level", "comment"},
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"criteria": map[string]any{"type": "array", "items": criterion},
		},
		"required": []string{"criteria"},
	}
}

func phaseCheckSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"complete": map[string]any{"type": "boolean"},
			"missing":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"feedback": map[string]any{"type": "string"},
		},
		"required": []string{"complete", "missing", "feedback"},
	}
}

func compareSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"narrative": map[string]any{"type": "string"},
			"points":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required": []string{"narrative", "points"},
	}
}

func jsonSchemaResponseFormat(name string, schema map[string]any) openai.ChatCompletionNewParamsResponseFormatUnion {
	return openai.ChatCompletionNewParamsResponseFormatUnion{
		OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
			JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
				Name:   name,
				Schema: schema,
				Strict: openai.Bool(true),
			},
		},
	}
}

func runOneShot(ctx context.Context, client openai.Client, params openai.ChatCompletionNewParams) (string, Usage, error) {
	resp, err := client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", Usage{}, err
	}
	usage := usageFrom(resp.Usage)
	if len(resp.Choices) == 0 {
		return "", usage, nil
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content), usage, nil
}
