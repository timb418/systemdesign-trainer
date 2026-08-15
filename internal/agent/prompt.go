package traineragent

import (
	"fmt"
	"strings"

	"github.com/timb418/systemdesign-trainer/internal/store"
	"github.com/timb418/systemdesign-trainer/internal/tasks"
)

func interviewerInstruction(t tasks.Task, mode store.Mode) string {
	s := interviewerPrompt
	s = strings.ReplaceAll(s, "{{mode}}", mode.Label())
	s = strings.ReplaceAll(s, "{{difficulty}}", fmt.Sprintf("%d", t.Difficulty))
	s = strings.ReplaceAll(s, "{{arch}}", strings.Join(t.ArchitectureTypes, ", "))
	var rules strings.Builder
	rules.WriteString("\n\nКогда вызывать reveal_facts:\n")
	for _, r := range t.RevealOnQuestion {
		rules.WriteString("- topic=")
		rules.WriteString(r.ID)
		rules.WriteString(" если речь про: ")
		rules.WriteString(strings.Join(r.Keywords, ", "))
		rules.WriteByte('\n')
	}
	switch mode {
	case store.ModeRequirements:
		s += "\nСейчас режим только уточнения требований. Не требуй схему. Когда кандидат зафиксировал scope — коротко подведи и остановись."
	case store.ModeDrill:
		s += "\nРежим практики паттерна: уже фокус на паттерне этого типа. Deep dive уже и короче, в «фирменное» узкое место."
	}
	return s + rules.String()
}

func mentorInstruction(t tasks.Task, blueprint tasks.LearningBlueprint, phase tasks.LearningPhase, hintMode bool) string {
	s := mentorPrompt
	hintNote := ""
	if hintMode {
		hintNote = "Ученик прямо сейчас нажал кнопку «Подсказка по разговору» и просит осмысленную подсказку с учётом уже обсуждённого. Дай ровно одну конкретную подсказку про следующий шаг, без встречного вопроса и без готовой архитектуры."
	}
	replacements := map[string]string{
		"{{phase_title}}":       phase.Title,
		"{{phase_goal}}":        phase.Goal,
		"{{task_title}}":        t.Title,
		"{{difficulty}}":        fmt.Sprintf("%d", t.Difficulty),
		"{{objectives}}":        bulletList(blueprint.Objectives),
		"{{concepts}}":          conceptList(blueprint.Concepts),
		"{{hint_request_note}}": hintNote,
	}
	for old, value := range replacements {
		s = strings.ReplaceAll(s, old, value)
	}
	var rules strings.Builder
	rules.WriteString("\n\nДоступные темы reveal_facts:\n")
	for _, rule := range t.RevealOnQuestion {
		fmt.Fprintf(&rules, "- %s: %s\n", rule.ID, strings.Join(rule.Keywords, ", "))
	}
	return s + rules.String()
}

func bulletList(items []string) string {
	if len(items) == 0 {
		return "- Общая практика этапа."
	}
	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "- %s\n", item)
	}
	return strings.TrimSpace(b.String())
}

func conceptList(concepts []tasks.Concept) string {
	var lines []string
	for _, concept := range concepts {
		lines = append(lines, fmt.Sprintf("- %s: %s", concept.Title, concept.Summary))
	}
	return strings.Join(lines, "\n")
}
