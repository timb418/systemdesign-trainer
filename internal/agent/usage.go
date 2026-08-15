package traineragent

import (
	"strconv"

	"github.com/openai/openai-go/v3"
)

// usageFrom reads standard token counts plus OpenRouter's "cost" extension,
// which rides along as an undeclared field on the usage object (present only
// when the request set usage.include=true, see newClient).
func usageFrom(u openai.CompletionUsage) Usage {
	usage := Usage{
		PromptTokens:     int(u.PromptTokens),
		CompletionTokens: int(u.CompletionTokens),
	}
	if f, ok := u.JSON.ExtraFields["cost"]; ok && f.Valid() {
		if cost, err := strconv.ParseFloat(f.Raw(), 64); err == nil {
			usage.Cost = cost
		}
	}
	return usage
}

func mergeUsage(current, next Usage) Usage {
	if next.PromptTokens+next.CompletionTokens > 0 || next.Cost > 0 {
		return next
	}
	return current
}
