package diagram

import (
	"encoding/xml"
	"html"
	"regexp"
	"strings"
)

type Node struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
}

type Edge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label"`
}

type Topology struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
	Dump  string `json:"dump"`
}

type mxFile struct {
	Diagrams []mxDiagram `xml:"diagram"`
}

type mxDiagram struct {
	Model mxGraphModel `xml:"mxGraphModel"`
}

type mxGraphModel struct {
	Root mxRoot `xml:"root"`
}

type mxRoot struct {
	Cells []mxCell `xml:"mxCell"`
}

type mxCell struct {
	ID     string `xml:"id,attr"`
	Value  string `xml:"value,attr"`
	Style  string `xml:"style,attr"`
	Vertex string `xml:"vertex,attr"`
	Edge   string `xml:"edge,attr"`
	Source string `xml:"source,attr"`
	Target string `xml:"target,attr"`
}

var tagRe = regexp.MustCompile(`<[^>]+>`)

func Parse(xmlText string) Topology {
	var file mxFile
	if err := xml.Unmarshal([]byte(xmlText), &file); err != nil {
		return Topology{Dump: "(не удалось разобрать XML схемы)"}
	}
	var cells []mxCell
	if len(file.Diagrams) > 0 {
		cells = file.Diagrams[0].Model.Root.Cells
	}
	if len(cells) == 0 {
		// sometimes mxGraphModel is the root
		var model mxGraphModel
		if err := xml.Unmarshal([]byte(xmlText), &model); err == nil {
			cells = model.Root.Cells
		}
	}
	byID := map[string]Node{}
	var nodes []Node
	var edges []Edge
	for _, c := range cells {
		if c.ID == "" || c.ID == "0" || c.ID == "1" {
			continue
		}
		if c.Vertex == "1" {
			n := Node{
				ID:    c.ID,
				Label: cleanLabel(c.Value),
				Kind:  inferKind(c.Value, c.Style),
			}
			if n.Label == "" {
				n.Label = c.ID
			}
			nodes = append(nodes, n)
			byID[c.ID] = n
		}
	}
	for _, c := range cells {
		if c.Edge != "1" || c.Source == "" || c.Target == "" {
			continue
		}
		edges = append(edges, Edge{
			From:  c.Source,
			To:    c.Target,
			Label: cleanLabel(c.Value),
		})
	}
	var dump []string
	for _, e := range edges {
		from := e.From
		to := e.To
		if n, ok := byID[e.From]; ok {
			from = n.Label
		}
		if n, ok := byID[e.To]; ok {
			to = n.Label
		}
		arrow := from + " --> " + to
		if e.Label != "" {
			arrow = from + " --" + e.Label + "--> " + to
		}
		dump = append(dump, arrow)
	}
	if len(dump) == 0 {
		for _, n := range nodes {
			dump = append(dump, n.Kind+": "+n.Label)
		}
	}
	return Topology{Nodes: nodes, Edges: edges, Dump: strings.Join(dump, "\n")}
}

func (t Topology) Human() string {
	if t.Dump == "" {
		return "(пустая доска)"
	}
	return t.Dump
}

func cleanLabel(s string) string {
	s = html.UnescapeString(s)
	s = tagRe.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func inferKind(label, style string) string {
	l := strings.ToLower(label + " " + style)
	switch {
	case containsAny(l, "client", "клиент", "browser", "браузер", "mobile", "ios", "android"):
		return "клиент"
	case containsAny(l, "postgres", "mysql", "sql", "бд", "database", "cassandra", "mongo", "dynamo", "spanner"):
		return "БД"
	case containsAny(l, "redis", "memcache", "кэш", "cache", "cdn"):
		return "кэш"
	case containsAny(l, "kafka", "queue", "очеред", "sqs", "pubsub", "rabbit", "nats"):
		return "очередь"
	case containsAny(l, "s3", "gcs", "blob", "object storage", "minio"):
		return "хранилище"
	case containsAny(l, "external", "stripe", "oauth", "third", "внешн"):
		return "внешняя система"
	case strings.Contains(style, "cylinder"):
		return "БД"
	default:
		return "сервис"
	}
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

const EmptyXML = `<mxfile host="app.diagrams.net"><diagram id="page-1" name="Доска"><mxGraphModel><root><mxCell id="0"/><mxCell id="1" parent="0"/></root></mxGraphModel></diagram></mxfile>`
