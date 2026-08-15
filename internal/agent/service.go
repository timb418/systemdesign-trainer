package traineragent

import (
	"context"
	"fmt"

	"github.com/openai/openai-go/v3"

	"github.com/timb418/systemdesign-trainer/internal/settings"
	"github.com/timb418/systemdesign-trainer/internal/store"
	"github.com/timb418/systemdesign-trainer/internal/tasks"
)

type Usage struct {
	PromptTokens     int
	CompletionTokens int
	Cost             float64
}

type TokenFn func(text string)

type Agents struct {
	bank *tasks.Bank
	set  *settings.Store
}

func New(bank *tasks.Bank, set *settings.Store) *Agents {
	return &Agents{bank: bank, set: set}
}

func (a *Agents) Interview(ctx context.Context, sess store.Session, t tasks.Task, history []store.Message, userText string, onToken TokenFn) (string, Usage, error) {
	key, cfg, err := a.keyAndSettings()
	if err != nil {
		return "", Usage{}, err
	}
	client := newClient(key, cfg.ReasoningEffort)
	params := openai.ChatCompletionNewParams{
		Model:       cfg.InterviewerModel,
		Messages:    buildHistory(interviewerInstruction(t, sess.Mode), history, userText),
		Tools:       []openai.ChatCompletionToolUnionParam{revealFactsTool()},
		Temperature: openai.Float(0.7),
	}
	return runToolLoop(ctx, client, params, t, onToken)
}

func (a *Agents) Mentor(ctx context.Context, sess store.Session, t tasks.Task, blueprint tasks.LearningBlueprint, phase tasks.LearningPhase, history []store.Message, userText string, hintMode bool, onToken TokenFn) (string, Usage, error) {
	key, cfg, err := a.keyAndSettings()
	if err != nil {
		return "", Usage{}, err
	}
	client := newClient(key, cfg.ReasoningEffort)
	params := openai.ChatCompletionNewParams{
		Model:       cfg.InterviewerModel,
		Messages:    buildHistory(mentorInstruction(t, blueprint, phase, hintMode), history, userText),
		Tools:       []openai.ChatCompletionToolUnionParam{revealFactsTool()},
		Temperature: openai.Float(0.6),
	}
	return runToolLoop(ctx, client, params, t, onToken)
}

func (a *Agents) Evaluate(ctx context.Context, payload string) (string, Usage, error) {
	return a.oneShot(ctx, true, evaluatorPrompt, jsonSchemaResponseFormat("rubric", rubricSchema()), 0.2, payload)
}

func (a *Agents) Compare(ctx context.Context, payload string) (string, Usage, error) {
	return a.oneShot(ctx, true, comparePrompt, jsonSchemaResponseFormat("compare", compareSchema()), 0.2, payload)
}

func (a *Agents) CheckPhaseCompletion(ctx context.Context, payload string) (string, Usage, error) {
	return a.oneShot(ctx, true, phaseCheckPrompt, jsonSchemaResponseFormat("phase_check", phaseCheckSchema()), 0.2, payload)
}

func (a *Agents) Summarize(ctx context.Context, payload string) (string, Usage, error) {
	return a.oneShot(ctx, false, summarizerPrompt, openai.ChatCompletionNewParamsResponseFormatUnion{}, 0.1, payload)
}

func (a *Agents) oneShot(ctx context.Context, evaluator bool, instruction string, responseFormat openai.ChatCompletionNewParamsResponseFormatUnion, temp float64, payload string) (string, Usage, error) {
	key, cfg, err := a.keyAndSettings()
	if err != nil {
		return "", Usage{}, err
	}
	modelID := cfg.InterviewerModel
	if evaluator {
		modelID = cfg.EvaluatorModel
	}
	client := newClient(key, cfg.ReasoningEffort)
	params := openai.ChatCompletionNewParams{
		Model: modelID,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(instruction),
			openai.UserMessage(payload),
		},
		ResponseFormat: responseFormat,
		Temperature:    openai.Float(temp),
	}
	return runOneShot(ctx, client, params)
}

func (a *Agents) keyAndSettings() (string, settings.Settings, error) {
	key, err := a.set.APIKey()
	if err != nil {
		return "", settings.Settings{}, err
	}
	if key == "" {
		return "", settings.Settings{}, fmt.Errorf("нет ключа OpenRouter — укажите его в настройках")
	}
	cfg, err := a.set.Load()
	if err != nil {
		return "", settings.Settings{}, err
	}
	return key, cfg, nil
}

func buildHistory(instruction string, history []store.Message, userText string) []openai.ChatCompletionMessageParamUnion {
	msgs := []openai.ChatCompletionMessageParamUnion{openai.SystemMessage(instruction)}
	for _, m := range history {
		if m.Role == "assistant" {
			msgs = append(msgs, openai.AssistantMessage(m.Content))
		} else {
			msgs = append(msgs, openai.UserMessage(m.Content))
		}
	}
	return append(msgs, openai.UserMessage(userText))
}
