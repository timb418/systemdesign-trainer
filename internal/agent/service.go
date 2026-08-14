package traineragent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/openai/openai-go/v3/option"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
	openaimodel "google.golang.org/adk/v2/model/openaimodel"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"google.golang.org/genai"

	"github.com/timb418/systemdesign-trainer/internal/settings"
	"github.com/timb418/systemdesign-trainer/internal/store"
	"github.com/timb418/systemdesign-trainer/internal/tasks"
)

const (
	appInterview = "sdt-interview"
	appEval      = "sdt-eval"
	appCompare   = "sdt-compare"
	localUser    = "local"
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

	mu   sync.Mutex
	sess session.Service
}

func New(bank *tasks.Bank, set *settings.Store) *Agents {
	return &Agents{bank: bank, set: set, sess: session.InMemoryService()}
}

func (a *Agents) model(ctx context.Context, modelID, apiKey, effort string) (model.LLM, error) {
	return openaimodel.NewModel(ctx, modelID, &openaimodel.ClientConfig{
		APIKey:  apiKey,
		BaseURL: settings.OpenRouterBaseURL,
		Options: []option.RequestOption{
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
		},
	})
}

type revealIn struct {
	Topic string `json:"topic"`
}

type revealOut struct {
	Facts string `json:"facts"`
}

func (a *Agents) revealTool(t tasks.Task) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "reveal_facts",
		Description: "Верни скрытые факты карточки по теме: scale, functional, nonfunctional или id правила. Вызывай, когда кандидат спросил про нагрузку, фичи или NFR. В реплике кандидату используй только пункты, которые отвечают на его вопрос; не зачитывай весь список.",
	}, func(_ agent.Context, in revealIn) (revealOut, error) {
		log.Printf("reveal_facts topic=%q task=%s", in.Topic, t.ID)
		facts := t.Reveal(in.Topic)
		if strings.Contains(facts, "нет заранее заданных фактов") {
			log.Printf("reveal_facts: no facts for topic=%q task=%s", in.Topic, t.ID)
		}
		return revealOut{Facts: facts}, nil
	})
}

func (a *Agents) interviewAgent(ctx context.Context, llm model.LLM, t tasks.Task, mode store.Mode) (agent.Agent, error) {
	reveal, err := a.revealTool(t)
	if err != nil {
		return nil, err
	}
	temp := float32(0.7)
	instruction := interviewerInstruction(t, mode)
	return llmagent.New(llmagent.Config{
		Name:        "interviewer",
		Model:       llm,
		Description: "Интервьюер system design",
		Instruction: instruction,
		Tools:       []tool.Tool{reveal},
		GenerateContentConfig: &genai.GenerateContentConfig{
			Temperature: &temp,
		},
		DisallowTransferToParent: true,
		DisallowTransferToPeers:  true,
	})
}

func (a *Agents) evalAgent(ctx context.Context, llm model.LLM) (agent.Agent, error) {
	temp := float32(0.2)
	return llmagent.New(llmagent.Config{
		Name:         "evaluator",
		Model:        llm,
		Description:  "Оценщик рубрики",
		Instruction:  evaluatorPrompt,
		OutputSchema: rubricSchema(),
		GenerateContentConfig: &genai.GenerateContentConfig{
			Temperature: &temp,
		},
		DisallowTransferToParent: true,
		DisallowTransferToPeers:  true,
	})
}

func (a *Agents) compareAgent(ctx context.Context, llm model.LLM) (agent.Agent, error) {
	temp := float32(0.2)
	return llmagent.New(llmagent.Config{
		Name:         "compare",
		Model:        llm,
		Description:  "Разбор отличий от эталона",
		Instruction:  comparePrompt,
		OutputSchema: compareSchema(),
		GenerateContentConfig: &genai.GenerateContentConfig{
			Temperature: &temp,
		},
		DisallowTransferToParent: true,
		DisallowTransferToPeers:  true,
	})
}

func (a *Agents) Interview(ctx context.Context, sess store.Session, t tasks.Task, history []store.Message, userText string, onToken TokenFn) (string, Usage, error) {
	key, err := a.set.APIKey()
	if err != nil {
		return "", Usage{}, err
	}
	if key == "" {
		return "", Usage{}, fmt.Errorf("нет ключа OpenRouter — укажите его в настройках")
	}
	cfg, err := a.set.Load()
	if err != nil {
		return "", Usage{}, err
	}
	llm, err := a.model(ctx, cfg.InterviewerModel, key, cfg.ReasoningEffort)
	if err != nil {
		return "", Usage{}, err
	}
	ag, err := a.interviewAgent(ctx, llm, t, sess.Mode)
	if err != nil {
		return "", Usage{}, err
	}
	r, err := runner.New(runner.Config{
		AppName:           appInterview,
		Agent:             ag,
		SessionService:    a.sess,
		AutoCreateSession: false,
	})
	if err != nil {
		return "", Usage{}, err
	}
	if err := a.ensureInterviewSession(ctx, sess, t, history); err != nil {
		return "", Usage{}, err
	}
	msg := genai.NewContentFromText(userText, genai.RoleUser)
	var full strings.Builder
	var usage Usage
	for event, err := range r.Run(ctx, localUser, sess.ID, msg, agent.RunConfig{StreamingMode: agent.StreamingModeSSE}) {
		if err != nil {
			return full.String(), usage, err
		}
		if event == nil {
			continue
		}
		appendInterviewText(&full, event, onToken)
		if u := usageFrom(event); u.PromptTokens+u.CompletionTokens > 0 || u.Cost > 0 {
			usage = u
		}
	}
	return strings.TrimSpace(full.String()), usage, nil
}

func (a *Agents) Evaluate(ctx context.Context, payload string) (string, Usage, error) {
	return a.oneShot(ctx, appEval, func(ctx context.Context, llm model.LLM) (agent.Agent, error) {
		return a.evalAgent(ctx, llm)
	}, true, payload)
}

func (a *Agents) Compare(ctx context.Context, payload string) (string, Usage, error) {
	return a.oneShot(ctx, appCompare, func(ctx context.Context, llm model.LLM) (agent.Agent, error) {
		return a.compareAgent(ctx, llm)
	}, true, payload)
}

func (a *Agents) oneShot(ctx context.Context, app string, makeAgent func(context.Context, model.LLM) (agent.Agent, error), evaluator bool, payload string) (string, Usage, error) {
	key, err := a.set.APIKey()
	if err != nil {
		return "", Usage{}, err
	}
	if key == "" {
		return "", Usage{}, fmt.Errorf("нет ключа OpenRouter — укажите его в настройках")
	}
	cfg, err := a.set.Load()
	if err != nil {
		return "", Usage{}, err
	}
	modelID := cfg.InterviewerModel
	if evaluator {
		modelID = cfg.EvaluatorModel
	}
	llm, err := a.model(ctx, modelID, key, cfg.ReasoningEffort)
	if err != nil {
		return "", Usage{}, err
	}
	ag, err := makeAgent(ctx, llm)
	if err != nil {
		return "", Usage{}, err
	}
	sessSvc := session.InMemoryService()
	created, err := sessSvc.Create(ctx, &session.CreateRequest{AppName: app, UserID: localUser})
	if err != nil {
		return "", Usage{}, err
	}
	r, err := runner.New(runner.Config{AppName: app, Agent: ag, SessionService: sessSvc})
	if err != nil {
		return "", Usage{}, err
	}
	msg := genai.NewContentFromText(payload, genai.RoleUser)
	var full strings.Builder
	var usage Usage
	for event, err := range r.Run(ctx, localUser, created.Session.ID(), msg, agent.RunConfig{StreamingMode: agent.StreamingModeNone}) {
		if err != nil {
			return full.String(), usage, err
		}
		if event == nil {
			continue
		}
		if event.IsFinalResponse() {
			full.WriteString(eventText(event))
		}
		if u := usageFrom(event); u.PromptTokens+u.CompletionTokens > 0 || u.Cost > 0 {
			usage = u
		}
	}
	return strings.TrimSpace(full.String()), usage, nil
}

func (a *Agents) ensureInterviewSession(ctx context.Context, sess store.Session, t tasks.Task, history []store.Message) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, err := a.sess.Get(ctx, &session.GetRequest{AppName: appInterview, UserID: localUser, SessionID: sess.ID})
	if err == nil {
		return nil
	}
	created, err := a.sess.Create(ctx, &session.CreateRequest{
		AppName:   appInterview,
		UserID:    localUser,
		SessionID: sess.ID,
		State:     map[string]any{"task_id": t.ID},
	})
	if err != nil {
		return err
	}
	for _, m := range history {
		ev := session.NewEvent(ctx, "hydrate")
		var role genai.Role = genai.RoleUser
		ev.Author = "user"
		if m.Role == "assistant" {
			role = genai.RoleModel
			ev.Author = "interviewer"
		}
		ev.LLMResponse.Content = genai.NewContentFromText(m.Content, role)
		if err := a.sess.AppendEvent(ctx, created.Session, ev); err != nil {
			return err
		}
	}
	return nil
}

func appendInterviewText(full *strings.Builder, event *session.Event, onToken TokenFn) {
	if event == nil {
		return
	}
	if hasToolParts(event) {
		full.Reset()
		return
	}
	text := eventText(event)
	if text == "" {
		return
	}
	if event.Partial {
		full.WriteString(text)
		if onToken != nil {
			onToken(text)
		}
		return
	}
	if event.IsFinalResponse() && full.Len() == 0 {
		full.WriteString(text)
		if onToken != nil {
			onToken(text)
		}
	}
}

func hasToolParts(e *session.Event) bool {
	if e == nil || e.Content == nil {
		return false
	}
	for _, p := range e.Content.Parts {
		if p == nil {
			continue
		}
		if p.FunctionCall != nil || p.FunctionResponse != nil {
			return true
		}
	}
	return false
}

func eventText(e *session.Event) string {
	if e == nil || e.Content == nil {
		return ""
	}
	var b strings.Builder
	for _, p := range e.Content.Parts {
		if p == nil || p.Thought || p.FunctionCall != nil || p.FunctionResponse != nil {
			continue
		}
		b.WriteString(p.Text)
	}
	return b.String()
}

func usageFrom(e *session.Event) Usage {
	var u Usage
	if e == nil || e.UsageMetadata == nil {
		return u
	}
	m := e.UsageMetadata
	u.PromptTokens = int(m.PromptTokenCount)
	u.CompletionTokens = int(m.CandidatesTokenCount)
	if u.CompletionTokens == 0 && m.TotalTokenCount > 0 {
		u.CompletionTokens = int(m.TotalTokenCount) - u.PromptTokens
	}
	if e.CustomMetadata != nil {
		if c, ok := e.CustomMetadata["cost"]; ok {
			switch v := c.(type) {
			case float64:
				u.Cost = v
			case json.Number:
				u.Cost, _ = v.Float64()
			}
		}
	}
	return u
}

func interviewerInstruction(t tasks.Task, mode store.Mode) string {
	s := interviewerPrompt
	s = strings.ReplaceAll(s, "{{mode}}", mode.Label())
	s = strings.ReplaceAll(s, "{{difficulty}}", fmt.Sprintf("%d", t.Difficulty))
	s = strings.ReplaceAll(s, "{{arch}}", strings.Join(t.ArchitectureTypes, ", "))
	var rules strings.Builder
	rules.WriteString("\n\nКогда вызывать reveal_facts:\n")
	for _, r := range t.RevealOnQuestion {
		rules.WriteString("- topic=")
		rules.WriteString(r.ID)
		rules.WriteString(" если речь про: ")
		rules.WriteString(strings.Join(r.Keywords, ", "))
		rules.WriteByte('\n')
	}
	switch mode {
	case store.ModeRequirements:
		s += "\nСейчас режим только уточнения требований. Не требуй схему. Когда кандидат зафиксировал scope — коротко подведи и остановись."
	case store.ModeDrill:
		s += "\nРежим дрилла: уже фокус на паттерне этого типа. Deep dive уже и короче, в «фирменное» узкое место."
	}
	return s + rules.String()
}

func rubricSchema() *genai.Schema {
	crit := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"id":      {Type: genai.TypeString, Description: "requirements|scale|hld|bottlenecks|reliability|tradeoffs|communication"},
			"level":   {Type: genai.TypeString, Description: "weak|ok|strong|n_a"},
			"comment": {Type: genai.TypeString},
		},
		Required: []string{"id", "level", "comment"},
	}
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"criteria": {Type: genai.TypeArray, Items: crit},
		},
		Required: []string{"criteria"},
	}
}

func compareSchema() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"narrative": {Type: genai.TypeString},
			"points":    {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
		},
		Required: []string{"narrative", "points"},
	}
}
