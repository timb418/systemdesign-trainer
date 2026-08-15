package traineragent

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/timb418/systemdesign-trainer/internal/settings"
)

func newClient(apiKey, effort string) openai.Client {
	return openai.NewClient(
		option.WithBaseURL(settings.OpenRouterBaseURL),
		option.WithAPIKey(apiKey),
		option.WithHeader("HTTP-Referer", "http://127.0.0.1:8080"),
		option.WithHeader("X-Title", "System Design Trainer"),
		option.WithJSONSet("provider", map[string]any{
			"order":           settings.DefaultProviderOrder,
			"allow_fallbacks": true,
		}),
		option.WithJSONSet("reasoning", map[string]any{
			"effort":  settings.NormalizeReasoningEffort(effort),
			"enabled": true,
		}),
		option.WithJSONSet("usage", map[string]any{
			"include": true,
		}),
	)
}
