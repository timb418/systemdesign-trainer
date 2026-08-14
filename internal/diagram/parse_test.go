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
	if !strings.Contains(topo.Human(), "Узлы:") || !strings.Contains(topo.Human(), "Связи:") {
		t.Fatalf("human: %s", topo.Human())
	}
}

func TestParseEmpty(t *testing.T) {
	topo := diagram.Parse(diagram.EmptyXML)
	if len(topo.Nodes) != 0 {
		t.Fatalf("empty should have no nodes: %+v", topo.Nodes)
	}
	if topo.Human() != "(пустая доска)" {
		t.Fatalf("human empty: %q", topo.Human())
	}
}

func TestParseMultilineLabels(t *testing.T) {
	xml := `<mxGraphModel><root>
		<mxCell id="0"/>
		<mxCell id="1" parent="0"/>
		<mxCell id="a" value="API&lt;div&gt;k8s pool&lt;/div&gt;" vertex="1" parent="1"/>
		<mxCell id="b" value="Redis" vertex="1" parent="1"/>
		<mxCell id="e" value="GET" edge="1" parent="1" source="a" target="b"/>
	</root></mxGraphModel>`
	topo := diagram.Parse(xml)
	if len(topo.Nodes) != 2 || len(topo.Edges) != 1 {
		t.Fatalf("topo: %+v", topo)
	}
	var api diagram.Node
	for _, n := range topo.Nodes {
		if n.Title == "API" {
			api = n
		}
	}
	if api.Title != "API" || !strings.Contains(api.Label, "k8s pool") {
		t.Fatalf("api node: %+v", topo.Nodes)
	}
	if strings.Contains(topo.Dump, "k8s pool") {
		t.Fatalf("dump should use short titles: %s", topo.Dump)
	}
	if !strings.Contains(topo.Dump, "API --GET--> Redis") {
		t.Fatalf("dump: %s", topo.Dump)
	}
	h := topo.Human()
	if !strings.Contains(h, "2 узла, 1 связь") || !strings.Contains(h, "Узлы:") || !strings.Contains(h, "Связи:") || !strings.Contains(h, "k8s pool") {
		t.Fatalf("human: %s", h)
	}
}
