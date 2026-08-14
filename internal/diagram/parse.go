package diagram

import (
	"encoding/xml"
	"fmt"
	"html"
	"regexp"
	"strings"
)

type Node struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Title string `json:"title"`
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

var (
	tagRe   = regexp.MustCompile(`<[^>]+>`)
	breakRe = regexp.MustCompile(`(?i)<br\s*/?>|</?div[^>]*>|</?p[^>]*>|</?h[1-6][^>]*>`)
)

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
			title, full := labelParts(c.Value)
			if full == "" {
				full = c.ID
			}
			if title == "" {
				title = full
			}
			n := Node{
				ID:    c.ID,
				Label: full,
				Title: title,
				Kind:  inferKind(c.Value, c.Style),
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
			from = n.Name()
		}
		if n, ok := byID[e.To]; ok {
			to = n.Name()
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
	if len(t.Nodes) == 0 && len(t.Edges) == 0 {
		if t.Dump != "" {
			return t.Dump
		}
		return "(пустая доска)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s, %s\n", ruCount(len(t.Nodes), "узел", "узла", "узлов"), ruCount(len(t.Edges), "связь", "связи", "связей"))
	if len(t.Nodes) > 0 {
		b.WriteString("\nУзлы:\n")
		for _, n := range t.Nodes {
			b.WriteString("• ")
			b.WriteString(n.Kind)
			b.WriteString(" — ")
			b.WriteString(n.Name())
			if extra := n.Detail(); extra != "" {
				b.WriteString(": ")
				b.WriteString(extra)
			}
			b.WriteByte('\n')
		}
	}
	if len(t.Edges) > 0 {
		b.WriteString("\nСвязи:\n")
		b.WriteString(t.Dump)
	}
	return strings.TrimSpace(b.String())
}

func (n Node) Name() string {
	if n.Title != "" {
		return n.Title
	}
	return n.Label
}

func (n Node) Detail() string {
	if n.Title == "" || n.Label == n.Title {
		return ""
	}
	rest := strings.TrimPrefix(n.Label, n.Title)
	rest = strings.TrimSpace(rest)
	rest = strings.TrimPrefix(rest, "·")
	return strings.TrimSpace(rest)
}

func labelParts(s string) (title, full string) {
	s = html.UnescapeString(s)
	s = breakRe.ReplaceAllString(s, "\n")
	s = tagRe.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	var parts []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			parts = append(parts, line)
		}
	}
	if len(parts) == 0 {
		return "", ""
	}
	return parts[0], strings.Join(parts, " · ")
}

func cleanLabel(s string) string {
	_, full := labelParts(s)
	return full
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

func ruCount(n int, one, few, many string) string {
	n100 := n % 100
	word := many
	if n100 < 11 || n100 > 14 {
		switch n % 10 {
		case 1:
			word = one
		case 2, 3, 4:
			word = few
		}
	}
	return fmt.Sprintf("%d %s", n, word)
}

const EmptyXML = `<mxfile host="app.diagrams.net"><diagram id="page-1" name="Доска"><mxGraphModel><root><mxCell id="0"/><mxCell id="1" parent="0"/></root></mxGraphModel></diagram></mxfile>`
