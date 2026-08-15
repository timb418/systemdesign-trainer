package traineragent

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"

	"github.com/timb418/systemdesign-trainer/internal/tasks"
)

const revealFactsToolName = "reveal_facts"

type revealIn struct {
	Topic string `json:"topic"`
}

type revealOut struct {
	Facts string `json:"facts"`
}

func revealFactsTool() openai.ChatCompletionToolUnionParam {
	return openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
		Name:        revealFactsToolName,
		Description: openai.String("Верни скрытые факты карточки по теме: scale, functional, nonfunctional или id правила. Вызывай, когда кандидат спросил про нагрузку, фичи или NFR. В реплике кандидату используй только пункты, которые отвечают на его вопрос; не зачитывай весь список."),
		Parameters: shared.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"topic": map[string]any{"type": "string"},
			},
			"required": []string{"topic"},
		},
	})
}

func runRevealFacts(t tasks.Task, argsJSON string) (string, error) {
	var in revealIn
	if err := json.Unmarshal([]byte(argsJSON), &in); err != nil {
		return "", err
	}
	log.Printf("reveal_facts topic=%q task=%s", in.Topic, t.ID)
	facts := t.Reveal(in.Topic)
	if strings.Contains(facts, "нет заранее заданных фактов") {
		log.Printf("reveal_facts: no facts for topic=%q task=%s", in.Topic, t.ID)
	}
	out, err := json.Marshal(revealOut{Facts: facts})
	if err != nil {
		return "", err
	}
	return string(out), nil
}
