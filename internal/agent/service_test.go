package traineragent

import (
	"strings"
	"testing"

	"github.com/timb418/systemdesign-trainer/internal/tasks"
)

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
	instruction := mentorInstruction(task, blueprint, phase, false)
	if strings.Contains(instruction, task.PreferredSolution.Narrative) ||
		strings.Contains(instruction, task.PreferredSolution.Tradeoffs[0]) {
		t.Fatal("mentor instruction leaked preferred solution")
	}
	if !strings.Contains(instruction, phase.Goal) {
		t.Fatalf("mentor instruction missing phase-scoped context: %s", instruction)
	}
	if strings.Contains(instruction, task.PromptPublic) {
		t.Fatal("mentor instruction duplicates the public brief already pinned in chat history")
	}
}

func TestMentorInstructionHintMode(t *testing.T) {
	task := tasks.Task{Title: "Тестовая задача", Difficulty: 2}
	blueprint := tasks.LearningBlueprint{}
	phase := tasks.LearningPhase{ID: "hld", Title: "HLD", Goal: "Нарисовать сквозной путь"}
	without := mentorInstruction(task, blueprint, phase, false)
	if strings.Contains(without, "Подсказка по разговору") {
		t.Fatalf("hint note leaked without hint mode: %s", without)
	}
	with := mentorInstruction(task, blueprint, phase, true)
	if !strings.Contains(with, "Подсказка по разговору") {
		t.Fatalf("hint mode missing its instruction: %s", with)
	}
}
