package diagram_test

import (
	"strings"
	"testing"

	"github.com/timb418/systemdesign-trainer/internal/diagram"
	"github.com/timb418/systemdesign-trainer/internal/tasks"
)

func TestParseGoldDiagram(t *testing.T) {
	fsys, err := tasks.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	bank, err := tasks.Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	task, ok := bank.Get("url-shortener-v1")
	if !ok {
		t.Fatal("no task")
	}
	xml, err := bank.ReadDiagram(task.PreferredSolution.Diagram)
	if err != nil {
		t.Fatal(err)
	}
	topo := diagram.Parse(xml)
	if len(topo.Nodes) < 4 {
		t.Fatalf("nodes: %+v", topo.Nodes)
	}
	if len(topo.Edges) < 3 {
		t.Fatalf("edges: %+v", topo.Edges)
	}
	if !strings.Contains(topo.Dump, "-->") {
		t.Fatalf("dump: %s", topo.Dump)
	}
}

func TestParseEmpty(t *testing.T) {
	topo := diagram.Parse(diagram.EmptyXML)
	if len(topo.Nodes) != 0 {
		t.Fatalf("empty should have no nodes: %+v", topo.Nodes)
	}
}
