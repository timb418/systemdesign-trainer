package web

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/timb418/systemdesign-trainer/internal/diagram"
	"github.com/timb418/systemdesign-trainer/internal/store"
	"github.com/timb418/systemdesign-trainer/internal/tasks"
)

type criterionView struct {
	ID      string
	Title   string
	Level   string
	Comment string
}

var criterionTitles = map[string]string{
	"requirements":  "Уточнение требований и scope",
	"scale":         "Оценка нагрузки",
	"hld":           "High-level design и схема",
	"bottlenecks":   "Узкие места и масштабирование",
	"reliability":   "Отказоустойчивость и consistency",
	"tradeoffs":     "Trade-offs",
	"communication": "Коммуникация",
}

func parseCriteria(raw string) []criterionView {
	var parsed struct {
		Criteria []struct {
			ID      string `json:"id"`
			Level   string `json:"level"`
			Comment string `json:"comment"`
		} `json:"criteria"`
	}
	if err := json.Unmarshal([]byte(extractJSON(raw)), &parsed); err != nil {
		return []criterionView{{Title: "Ответ оценщика", Level: "ok", Comment: raw}}
	}
	var out []criterionView
	for _, c := range parsed.Criteria {
		title := criterionTitles[c.ID]
		if title == "" {
			title = c.ID
		}
		out = append(out, criterionView{ID: c.ID, Title: title, Level: c.Level, Comment: c.Comment})
	}
	return out
}

func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			return s[i : j+1]
		}
	}
	return s
}

func formatCompare(raw string) string {
	var parsed struct {
		Narrative string   `json:"narrative"`
		Points    []string `json:"points"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return raw
	}
	var b strings.Builder
	b.WriteString(strings.TrimSpace(parsed.Narrative))
	if len(parsed.Points) > 0 {
		b.WriteString("\n\n")
		for _, p := range parsed.Points {
			b.WriteString("• ")
			b.WriteString(p)
			b.WriteByte('\n')
		}
	}
	return strings.TrimSpace(b.String())
}

func buildEvalPayload(sess store.Session, t tasks.Task, msgs []store.Message, topo diagram.Topology) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Режим: %s\nЗадача: %s\nСложность: %d\n\n", sess.Mode.Label(), t.Title, t.Difficulty)
	b.WriteString("Скрытые требования карточки:\n")
	for _, l := range t.Hidden.Functional {
		b.WriteString("- ")
		b.WriteString(l)
		b.WriteByte('\n')
	}
	b.WriteString("NFR:\n")
	for _, l := range t.Hidden.Nonfunctional {
		b.WriteString("- ")
		b.WriteString(l)
		b.WriteByte('\n')
	}
	b.WriteString("Нагрузка:\n")
	for _, l := range t.Hidden.Scale {
		b.WriteString("- ")
		b.WriteString(l)
		b.WriteByte('\n')
	}
	b.WriteString("\nЭталон (один рабочий подход):\n")
	b.WriteString(t.PreferredSolution.Narrative)
	b.WriteString("\nTrade-offs эталона:\n")
	for _, l := range t.PreferredSolution.Tradeoffs {
		b.WriteString("- ")
		b.WriteString(l)
		b.WriteByte('\n')
	}
	b.WriteString("\nОсобые акценты оценки для этой задачи:\n")
	for _, l := range t.RubricOverrides {
		b.WriteString("- ")
		b.WriteString(l)
		b.WriteByte('\n')
	}
	b.WriteString("\nТранскрипт:\n")
	for _, m := range msgs {
		fmt.Fprintf(&b, "[%s]\n%s\n\n", m.Role, m.Content)
	}
	b.WriteString("Схема (каноническая проекция, не XML):\n")
	b.WriteString(topo.Human())
	b.WriteByte('\n')
	return b.String()
}

func buildComparePayload(t tasks.Task, msgs []store.Message, topo diagram.Topology) string {
	var b strings.Builder
	b.WriteString("Эталонный нарратив:\n")
	b.WriteString(t.PreferredSolution.Narrative)
	b.WriteString("\n\nЭталонные trade-offs:\n")
	for _, l := range t.PreferredSolution.Tradeoffs {
		b.WriteString("- ")
		b.WriteString(l)
		b.WriteByte('\n')
	}
	if t.PreferredSolution.Diagram != "" {
		// gold topology is added by caller dump in narrative; include candidate
	}
	b.WriteString("\nСхема кандидата:\n")
	b.WriteString(topo.Human())
	b.WriteString("\n\nТранскрипт:\n")
	for _, m := range msgs {
		fmt.Fprintf(&b, "[%s]\n%s\n\n", m.Role, m.Content)
	}
	return b.String()
}
