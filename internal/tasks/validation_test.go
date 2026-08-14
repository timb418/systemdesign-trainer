package tasks

import (
	"testing"
	"testing/fstest"
)

func TestValidateTaskContentContract(t *testing.T) {
	files := fstest.MapFS{
		"diagrams/gold.drawio":    {Data: []byte("<mxfile/>")},
		"diagrams/starter.drawio": {Data: []byte("<mxfile/>")},
	}
	types := map[string]struct{}{"test": {}}

	valid := Task{
		ID:                "test-task",
		Title:             "Тест",
		Difficulty:        3,
		ArchitectureTypes: []string{"test"},
		DurationMin:       45,
		Canvas:            "blank",
		PromptPublic:      "Спроектируйте тестовую систему.",
		Hidden: Hidden{
			Functional:    []string{"f1", "f2", "f3", "f4"},
			Nonfunctional: []string{"n1", "n2", "n3", "n4"},
			Scale:         []string{"s1", "s2", "s3", "s4"},
		},
		RevealOnQuestion: []RevealRule{
			{ID: "functional", Keywords: []string{"функции"}, Reveal: []string{"hidden.functional"}},
			{ID: "nonfunctional", Keywords: []string{"nfr"}, Reveal: []string{"hidden.nonfunctional"}},
			{ID: "scale", Keywords: []string{"qps"}, Reveal: []string{"hidden.scale"}},
		},
		PreferredSolution: PreferredSolution{
			Narrative: "Эталонное решение",
			Tradeoffs: []string{"t1", "t2", "t3"},
			Diagram:   "diagrams/gold.drawio",
		},
		RubricOverrides: []string{"r1", "r2"},
	}

	if err := validateTask(valid, types, files); err != nil {
		t.Fatalf("valid task rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Task)
	}{
		{"missing hidden facts", func(task *Task) { task.Hidden.Scale = nil }},
		{"incomplete reveal coverage", func(task *Task) { task.RevealOnQuestion = task.RevealOnQuestion[:2] }},
		{"missing gold diagram", func(task *Task) { task.PreferredSolution.Diagram = "" }},
		{"missing rubric focus", func(task *Task) { task.RubricOverrides = nil }},
		{"starter on blank canvas", func(task *Task) { task.StarterDiagram = "diagrams/starter.drawio" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			task := valid
			tc.mutate(&task)
			if err := validateTask(task, types, files); err == nil {
				t.Fatal("invalid task accepted")
			}
		})
	}

	sketch := valid
	sketch.Canvas = "sketch"
	sketch.StarterDiagram = "diagrams/starter.drawio"
	if err := validateTask(sketch, types, files); err != nil {
		t.Fatalf("valid sketch rejected: %v", err)
	}
}
