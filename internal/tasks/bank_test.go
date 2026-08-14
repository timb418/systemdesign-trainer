package tasks_test

import (
	"strings"
	"testing"

	"github.com/timb418/systemdesign-trainer/internal/tasks"
)

func TestLoadEmbeddedBank(t *testing.T) {
	fsys, err := tasks.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	bank, err := tasks.Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(bank.Types()) != 20 {
		t.Fatalf("types: got %d want 20", len(bank.Types()))
	}
	if len(bank.All()) != 75 {
		t.Fatalf("tasks: got %d want 75", len(bank.All()))
	}
	task, ok := bank.Get("url-shortener-v1")
	if !ok {
		t.Fatal("missing url-shortener-v1")
	}
	if task.Canvas != "blank" || task.Difficulty != 1 {
		t.Fatalf("unexpected task: %+v", task)
	}
	const noFacts = "По этой теме в карточке нет заранее заданных фактов. Не выдумывай цифры и требования. Если вопрос уместный — попроси кандидата сделать разумное предположение и дальше опираться на него; когда назовёт — оцени, реалистично ли оно или совсем мимо."
	for _, topic := range []string{"qps", "scale", "сколько пользователей"} {
		facts := task.Reveal(topic)
		if !strings.Contains(facts, "20k QPS") {
			t.Fatalf("reveal %q: %q", topic, facts)
		}
	}
	if got := task.Reveal("unknown-topic"); got != noFacts {
		t.Fatalf("reveal unknown: %q", got)
	}
	if _, err := bank.ReadDiagram(task.PreferredSolution.Diagram); err != nil {
		t.Fatal(err)
	}
}

func TestEmbeddedBankCoverage(t *testing.T) {
	fsys, err := tasks.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	bank, err := tasks.Load(fsys)
	if err != nil {
		t.Fatal(err)
	}

	wantDifficulty := map[int]int{1: 10, 2: 18, 3: 24, 4: 16, 5: 7}
	gotDifficulty := map[int]int{}
	usedTypes := map[string]int{}
	sketches := 0
	ids := map[string]struct{}{}
	for _, task := range bank.All() {
		if _, ok := ids[task.ID]; ok {
			t.Errorf("duplicate task id %q", task.ID)
		}
		ids[task.ID] = struct{}{}
		gotDifficulty[task.Difficulty]++
		for _, typeID := range task.ArchitectureTypes {
			usedTypes[typeID]++
		}
		if task.Canvas == "sketch" {
			sketches++
		}
		for _, section := range [][]string{
			task.Hidden.Functional,
			task.Hidden.Nonfunctional,
			task.Hidden.Scale,
			task.PreferredSolution.Tradeoffs,
			task.RubricOverrides,
		} {
			for _, item := range section {
				if strings.TrimSpace(item) == "" {
					t.Errorf("%s contains an empty content item", task.ID)
				}
			}
		}
	}
	for difficulty, want := range wantDifficulty {
		if got := gotDifficulty[difficulty]; got != want {
			t.Errorf("difficulty %d: got %d want %d", difficulty, got, want)
		}
	}
	if sketches != 10 {
		t.Errorf("sketch tasks: got %d want 10", sketches)
	}
	for _, archType := range bank.Types() {
		if usedTypes[archType.ID] == 0 {
			t.Errorf("architecture type %q has no tasks", archType.ID)
		}
	}
}
