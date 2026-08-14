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
	if len(bank.Types()) != 14 {
		t.Fatalf("types: got %d want 14", len(bank.Types()))
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
