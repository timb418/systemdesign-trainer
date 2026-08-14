package tasks

type ArchType struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}

type RevealRule struct {
	ID       string   `yaml:"id"`
	Keywords []string `yaml:"keywords"`
	Reveal   []string `yaml:"reveal"`
}

type Hidden struct {
	Functional    []string `yaml:"functional"`
	Nonfunctional []string `yaml:"nonfunctional"`
	Scale         []string `yaml:"scale"`
}

type PreferredSolution struct {
	Narrative string   `yaml:"narrative"`
	Tradeoffs []string `yaml:"tradeoffs"`
	Diagram   string   `yaml:"diagram"`
}

type Task struct {
	ID                string            `yaml:"id"`
	Title             string            `yaml:"title"`
	Difficulty        int               `yaml:"difficulty"`
	ArchitectureTypes []string          `yaml:"architecture_types"`
	DurationMin       int               `yaml:"duration_min"`
	Canvas            string            `yaml:"canvas"`
	StarterDiagram    string            `yaml:"starter_diagram"`
	PromptPublic      string            `yaml:"prompt_public"`
	Hidden            Hidden            `yaml:"hidden"`
	RevealOnQuestion  []RevealRule      `yaml:"reveal_on_question"`
	PreferredSolution PreferredSolution `yaml:"preferred_solution"`
	RubricOverrides   []string          `yaml:"rubric_overrides"`
}

type PublicTask struct {
	ID                string
	Title             string
	Difficulty        int
	ArchitectureTypes []string
	DurationMin       int
	Canvas            string
	TypeNames         []string
}

func (t Task) Public() PublicTask {
	return PublicTask{
		ID:                t.ID,
		Title:             t.Title,
		Difficulty:        t.Difficulty,
		ArchitectureTypes: t.ArchitectureTypes,
		DurationMin:       t.DurationMin,
		Canvas:            t.Canvas,
	}
}
