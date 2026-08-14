package tasks

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Bank struct {
	types []ArchType
	tasks []Task
	files fs.FS
}

type typeFile struct {
	Types []ArchType `yaml:"types"`
}

func Load(fsys fs.FS) (*Bank, error) {
	b := &Bank{files: fsys}
	raw, err := fs.ReadFile(fsys, "types.yaml")
	if err != nil {
		return nil, fmt.Errorf("types.yaml: %w", err)
	}
	var tf typeFile
	if err := yaml.Unmarshal(raw, &tf); err != nil {
		return nil, fmt.Errorf("types.yaml parse: %w", err)
	}
	if len(tf.Types) == 0 {
		return nil, fmt.Errorf("types.yaml: пустой каталог типов")
	}
	seenType := map[string]struct{}{}
	for _, t := range tf.Types {
		if t.ID == "" || t.Name == "" {
			return nil, fmt.Errorf("types.yaml: нужен id и name")
		}
		if _, ok := seenType[t.ID]; ok {
			return nil, fmt.Errorf("types.yaml: дубль id %s", t.ID)
		}
		seenType[t.ID] = struct{}{}
	}
	b.types = tf.Types

	seenTask := map[string]struct{}{}
	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || p == "types.yaml" || !strings.HasSuffix(p, ".yaml") {
			return nil
		}
		raw, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		var t Task
		if err := yaml.Unmarshal(raw, &t); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		if err := validateTask(t, seenType, fsys); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		if _, ok := seenTask[t.ID]; ok {
			return fmt.Errorf("дубль task id %s", t.ID)
		}
		seenTask[t.ID] = struct{}{}
		b.tasks = append(b.tasks, t)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(b.tasks, func(i, j int) bool {
		if b.tasks[i].Difficulty != b.tasks[j].Difficulty {
			return b.tasks[i].Difficulty < b.tasks[j].Difficulty
		}
		return b.tasks[i].Title < b.tasks[j].Title
	})
	return b, nil
}

func validateTask(t Task, types map[string]struct{}, fsys fs.FS) error {
	if t.ID == "" || t.Title == "" {
		return fmt.Errorf("нужны id и title")
	}
	if t.Difficulty < 1 || t.Difficulty > 5 {
		return fmt.Errorf("difficulty должен быть 1–5")
	}
	if t.Canvas != "blank" && t.Canvas != "sketch" {
		return fmt.Errorf("canvas: blank или sketch")
	}
	if t.PromptPublic == "" {
		return fmt.Errorf("пустой prompt_public")
	}
	if len(t.ArchitectureTypes) == 0 {
		return fmt.Errorf("нужен хотя бы один architecture_types")
	}
	for _, id := range t.ArchitectureTypes {
		if _, ok := types[id]; !ok {
			return fmt.Errorf("неизвестный тип %s", id)
		}
	}
	if t.Canvas == "sketch" {
		if t.StarterDiagram == "" {
			return fmt.Errorf("sketch без starter_diagram")
		}
		if _, err := fs.ReadFile(fsys, t.StarterDiagram); err != nil {
			return fmt.Errorf("starter_diagram %s: %w", t.StarterDiagram, err)
		}
	}
	if t.PreferredSolution.Diagram != "" {
		if _, err := fs.ReadFile(fsys, t.PreferredSolution.Diagram); err != nil {
			return fmt.Errorf("preferred_solution.diagram %s: %w", t.PreferredSolution.Diagram, err)
		}
	}
	return nil
}

func (b *Bank) Types() []ArchType { return b.types }

func (b *Bank) TypeName(id string) string {
	for _, t := range b.types {
		if t.ID == id {
			return t.Name
		}
	}
	return id
}

func (b *Bank) All() []Task { return b.tasks }

func (b *Bank) Get(id string) (Task, bool) {
	for _, t := range b.tasks {
		if t.ID == id {
			return t, true
		}
	}
	return Task{}, false
}

func (b *Bank) Filter(typeID string, difficulty int) []Task {
	var out []Task
	for _, t := range b.tasks {
		if difficulty > 0 && t.Difficulty != difficulty {
			continue
		}
		if typeID != "" && !hasType(t, typeID) {
			continue
		}
		out = append(out, t)
	}
	return out
}

func hasType(t Task, id string) bool {
	for _, x := range t.ArchitectureTypes {
		if x == id {
			return true
		}
	}
	return false
}

func (b *Bank) PublicList(typeID string, difficulty int) []PublicTask {
	tasks := b.Filter(typeID, difficulty)
	out := make([]PublicTask, 0, len(tasks))
	for _, t := range tasks {
		p := t.Public()
		for _, id := range t.ArchitectureTypes {
			p.TypeNames = append(p.TypeNames, b.TypeName(id))
		}
		out = append(out, p)
	}
	return out
}

func (b *Bank) ReadDiagram(rel string) (string, error) {
	rel = path.Clean(rel)
	if rel == "." || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("некорректный путь схемы")
	}
	raw, err := fs.ReadFile(b.files, rel)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (t Task) Reveal(topic string) string {
	topic = strings.ToLower(strings.TrimSpace(topic))
	var fields []string
	for _, rule := range t.RevealOnQuestion {
		if strings.EqualFold(rule.ID, topic) {
			fields = append(fields, rule.Reveal...)
			continue
		}
		for _, kw := range rule.Keywords {
			if strings.Contains(topic, strings.ToLower(kw)) {
				fields = append(fields, rule.Reveal...)
				break
			}
		}
	}
	if len(fields) == 0 {
		switch topic {
		case "scale", "hidden.scale":
			fields = []string{"hidden.scale"}
		case "functional", "hidden.functional":
			fields = []string{"hidden.functional"}
		case "nonfunctional", "nfr", "hidden.nonfunctional":
			fields = []string{"hidden.nonfunctional"}
		}
	}
	if len(fields) == 0 {
		return "По этой теме в карточке нет заранее заданных фактов. Не выдумывай цифры."
	}
	var b strings.Builder
	seen := map[string]struct{}{}
	for _, f := range fields {
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		switch f {
		case "hidden.scale":
			b.WriteString("Нагрузка:\n")
			writeLines(&b, t.Hidden.Scale)
		case "hidden.functional":
			b.WriteString("Функциональные требования:\n")
			writeLines(&b, t.Hidden.Functional)
		case "hidden.nonfunctional":
			b.WriteString("Нефункциональные требования:\n")
			writeLines(&b, t.Hidden.Nonfunctional)
		}
	}
	return strings.TrimSpace(b.String())
}

func writeLines(b *strings.Builder, lines []string) {
	for _, l := range lines {
		b.WriteString("- ")
		b.WriteString(l)
		b.WriteByte('\n')
	}
}
