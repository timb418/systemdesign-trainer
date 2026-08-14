package traineragent

import (
	"strings"
	"testing"

	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"

	"github.com/timb418/systemdesign-trainer/internal/tasks"
)

func textEvent(partial bool, parts ...*genai.Part) *session.Event {
	return &session.Event{
		LLMResponse: model.LLMResponse{
			Partial: partial,
			Content: &genai.Content{Parts: parts},
		},
	}
}

func TestMentorInstructionDoesNotContainGold(t *testing.T) {
	task := tasks.Task{
		Title: "Тестовая задача", Difficulty: 2, PromptPublic: "Спроектируйте сервис.",
		PreferredSolution: tasks.PreferredSolution{
			Narrative: "СЕКРЕТНЫЙ ЭТАЛОН С КОНКРЕТНОЙ АРХИТЕКТУРОЙ",
			Tradeoffs: []string{"секретный компромисс"},
		},
		RevealOnQuestion: []tasks.RevealRule{{ID: "scale", Keywords: []string{"qps"}}},
	}
	blueprint := tasks.LearningBlueprint{
		Objectives: []string{"Сделать самостоятельную попытку"},
		Concepts:   []tasks.Concept{{Title: "Кэш", Summary: "Ускоряет чтение."}},
	}
	phase := tasks.LearningPhase{ID: "hld", Title: "HLD", Goal: "Нарисовать сквозной путь"}
	instruction := mentorInstruction(task, blueprint, phase)
	if strings.Contains(instruction, task.PreferredSolution.Narrative) ||
		strings.Contains(instruction, task.PreferredSolution.Tradeoffs[0]) {
		t.Fatal("mentor instruction leaked preferred solution")
	}
	if !strings.Contains(instruction, phase.Goal) || !strings.Contains(instruction, task.PromptPublic) {
		t.Fatalf("mentor instruction missing phase-scoped context: %s", instruction)
	}
}

func TestEventTextSkipsThoughtAndToolParts(t *testing.T) {
	e := textEvent(false,
		&genai.Part{Text: "visible"},
		&genai.Part{Text: "thinking", Thought: true},
		&genai.Part{Text: "call", FunctionCall: &genai.FunctionCall{Name: "reveal_facts"}},
		&genai.Part{Text: "resp", FunctionResponse: &genai.FunctionResponse{Name: "reveal_facts"}},
	)
	if got := eventText(e); got != "visible" {
		t.Fatalf("eventText = %q, want %q", got, "visible")
	}
}

func TestHasToolParts(t *testing.T) {
	if hasToolParts(nil) {
		t.Fatal("nil event")
	}
	if hasToolParts(textEvent(true, &genai.Part{Text: "hi"})) {
		t.Fatal("plain text should not have tool parts")
	}
	if !hasToolParts(textEvent(false, &genai.Part{FunctionCall: &genai.FunctionCall{Name: "reveal_facts"}})) {
		t.Fatal("function call should have tool parts")
	}
	if !hasToolParts(textEvent(false, &genai.Part{FunctionResponse: &genai.FunctionResponse{Name: "reveal_facts"}})) {
		t.Fatal("function response should have tool parts")
	}
}

func TestAppendInterviewTextDropsPreToolDraft(t *testing.T) {
	var full strings.Builder
	appendInterviewText(&full, textEvent(true, &genai.Part{Text: "Вызываю reveal_facts"}), nil)
	appendInterviewText(&full, textEvent(false, &genai.Part{
		FunctionCall: &genai.FunctionCall{Name: "reveal_facts"},
	}), nil)
	appendInterviewText(&full, textEvent(true, &genai.Part{Text: "10 млн созданий"}), nil)

	if got := full.String(); got != "10 млн созданий" {
		t.Fatalf("full = %q, want candidate-facing reply only", got)
	}
}
