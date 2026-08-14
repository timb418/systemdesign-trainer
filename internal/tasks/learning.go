package tasks

import (
	"fmt"
	"io/fs"
	"strings"

	"gopkg.in/yaml.v3"
)

const LearningPhaseCount = 6

type Concept struct {
	ID      string `yaml:"id"`
	Title   string `yaml:"title"`
	Summary string `yaml:"summary"`
}

type LearningTrack struct {
	ID            string   `yaml:"id"`
	Title         string   `yaml:"title"`
	Description   string   `yaml:"description"`
	Prerequisites []string `yaml:"prerequisites"`
	Tasks         []string `yaml:"tasks"`
}

type LearningOverride struct {
	TaskID         string   `yaml:"task_id"`
	Concepts       []string `yaml:"concepts"`
	Prerequisites  []string `yaml:"prerequisites"`
	Objectives     []string `yaml:"objectives"`
	CommonMistakes []string `yaml:"common_mistakes"`
}

type LearningCatalog struct {
	Concepts []Concept          `yaml:"concepts"`
	Tracks   []LearningTrack    `yaml:"tracks"`
	Tasks    []LearningOverride `yaml:"tasks"`
}

type LearningPhase struct {
	ID    string
	Title string
	Goal  string
	Hints []string
}

type LearningBlueprint struct {
	TaskID         string
	Concepts       []Concept
	Prerequisites  []string
	Objectives     []string
	CommonMistakes []string
	Phases         []LearningPhase
}

func defaultLearningPhases() []LearningPhase {
	return []LearningPhase{
		{
			ID: "orientation", Title: "Ориентация",
			Goal: "Перескажите задачу своими словами и назовите, что нужно уточнить до проектирования.",
			Hints: []string{
				"Кто будет пользоваться системой и какое главное действие совершает?",
				"Отделите цель продукта от деталей реализации.",
				"Назовите знакомые архитектурные понятия и одно понятие, которое стоит пояснить.",
			},
		},
		{
			ID: "requirements", Title: "Требования и scope",
			Goal: "Сформулируйте функциональный scope v1 и важные нефункциональные ограничения.",
			Hints: []string{
				"Начните с ролей пользователя и двух главных сценариев.",
				"Спросите отдельно про функциональность, задержку, доступность и согласованность.",
				"Зафиксируйте, что сознательно не входит в первую версию.",
				"Если не знаете вопроса, пройдите оси: клиенты, география, данные, SLA, безопасность.",
			},
		},
		{
			ID: "scale", Title: "Оценка нагрузки",
			Goal: "Зафиксируйте исходные числа и оцените порядок QPS, объёма данных и трафика.",
			Hints: []string{
				"Сначала выпишите входные величины, не переходя к арифметике.",
				"Разделите среднюю и пиковую нагрузку, чтение и запись.",
				"Покажите формулу и порядок величины; точность до единицы не нужна.",
				"Свяжите получившиеся числа с будущим выбором хранения и масштабирования.",
			},
		},
		{
			ID: "hld", Title: "High-level design",
			Goal: "Предложите минимальную сквозную схему и объясните путь главного запроса.",
			Hints: []string{
				"Начните с клиента, входной точки, одного сервиса и основного хранилища.",
				"Проведите один запрос от входа до ответа, подписывая данные на стрелках.",
				"Добавляйте кэш или очередь только после того, как назвали решаемую ими проблему.",
				"Проверьте, где хранится источник истины и что происходит при повторном запросе.",
			},
		},
		{
			ID: "deep_dive", Title: "Deep dive и trade-offs",
			Goal: "Найдите узкое место, разберите отказ и сравните минимум две альтернативы.",
			Hints: []string{
				"Какой компонент первым упрётся в оценённую вами нагрузку?",
				"Что увидит пользователь, если этот компонент недоступен?",
				"Сравните две альтернативы по latency, consistency, стоимости и сложности эксплуатации.",
				"Выберите вариант под требования этой задачи и явно назовите цену выбора.",
			},
		},
		{
			ID: "reflection", Title: "Рефлексия",
			Goal: "Кратко пересоберите решение: что изменили бы и какой компромисс считаете главным.",
			Hints: []string{
				"Назовите одно сильное место и один риск своего решения.",
				"Какое новое требование заставило бы вас изменить архитектуру?",
				"Сформулируйте главный trade-off одним предложением.",
			},
		},
	}
}

func (b *Bank) loadLearning(fsys fs.FS) error {
	raw, err := fs.ReadFile(fsys, "learning.yaml")
	if err != nil {
		return fmt.Errorf("learning.yaml: %w", err)
	}
	if err := yaml.Unmarshal(raw, &b.learning); err != nil {
		return fmt.Errorf("learning.yaml parse: %w", err)
	}
	return b.validateLearning()
}

func (b *Bank) validateLearning() error {
	taskIDs := map[string]struct{}{}
	for _, task := range b.tasks {
		taskIDs[task.ID] = struct{}{}
	}
	conceptIDs := map[string]struct{}{}
	for _, concept := range b.learning.Concepts {
		if strings.TrimSpace(concept.ID) == "" || strings.TrimSpace(concept.Title) == "" || strings.TrimSpace(concept.Summary) == "" {
			return fmt.Errorf("learning.yaml: каждой концепции нужны id, title и summary")
		}
		if _, exists := conceptIDs[concept.ID]; exists {
			return fmt.Errorf("learning.yaml: дубль concept id %s", concept.ID)
		}
		conceptIDs[concept.ID] = struct{}{}
	}
	trackIDs := map[string]struct{}{}
	for _, track := range b.learning.Tracks {
		if track.ID == "" || track.Title == "" || len(track.Tasks) == 0 {
			return fmt.Errorf("learning.yaml: треку нужны id, title и tasks")
		}
		if _, exists := trackIDs[track.ID]; exists {
			return fmt.Errorf("learning.yaml: дубль track id %s", track.ID)
		}
		trackIDs[track.ID] = struct{}{}
		for _, taskID := range track.Tasks {
			if _, exists := taskIDs[taskID]; !exists {
				return fmt.Errorf("learning.yaml: трек %s ссылается на неизвестную задачу %s", track.ID, taskID)
			}
		}
	}
	for _, track := range b.learning.Tracks {
		for _, prerequisite := range track.Prerequisites {
			if _, exists := trackIDs[prerequisite]; !exists {
				return fmt.Errorf("learning.yaml: трек %s ссылается на неизвестный prerequisite %s", track.ID, prerequisite)
			}
			if prerequisite == track.ID {
				return fmt.Errorf("learning.yaml: трек %s ссылается сам на себя", track.ID)
			}
		}
	}
	overrides := map[string]struct{}{}
	for _, override := range b.learning.Tasks {
		if _, exists := taskIDs[override.TaskID]; !exists {
			return fmt.Errorf("learning.yaml: override неизвестной задачи %s", override.TaskID)
		}
		if _, exists := overrides[override.TaskID]; exists {
			return fmt.Errorf("learning.yaml: дубль override задачи %s", override.TaskID)
		}
		overrides[override.TaskID] = struct{}{}
		for _, conceptID := range override.Concepts {
			if _, exists := conceptIDs[conceptID]; !exists {
				return fmt.Errorf("learning.yaml: неизвестная концепция %s", conceptID)
			}
		}
		for _, prerequisite := range override.Prerequisites {
			if _, exists := taskIDs[prerequisite]; !exists {
				return fmt.Errorf("learning.yaml: неизвестный prerequisite %s", prerequisite)
			}
		}
	}
	return nil
}

func (b *Bank) LearningCatalog() LearningCatalog { return b.learning }

func (b *Bank) LearningBlueprint(taskID string) (LearningBlueprint, bool) {
	task, ok := b.Get(taskID)
	if !ok {
		return LearningBlueprint{}, false
	}
	override := LearningOverride{}
	for _, candidate := range b.learning.Tasks {
		if candidate.TaskID == taskID {
			override = candidate
			break
		}
	}
	conceptIDs := override.Concepts
	if len(conceptIDs) == 0 {
		conceptIDs = task.ArchitectureTypes
	}
	var concepts []Concept
	for _, id := range conceptIDs {
		found := false
		for _, concept := range b.learning.Concepts {
			if concept.ID == id {
				concepts = append(concepts, concept)
				found = true
				break
			}
		}
		if !found {
			concepts = append(concepts, Concept{ID: id, Title: b.TypeName(id), Summary: "Архитектурный паттерн этой задачи; его детали будут разобраны по ходу решения."})
		}
	}
	objectives := override.Objectives
	if len(objectives) == 0 {
		objectives = []string{
			"Выделить требования и оценить порядок нагрузки.",
			"Построить сквозную high-level схему.",
			"Обосновать ключевые trade-offs и поведение при отказах.",
		}
	}
	return LearningBlueprint{
		TaskID: taskID, Concepts: concepts, Prerequisites: override.Prerequisites,
		Objectives: objectives, CommonMistakes: override.CommonMistakes, Phases: defaultLearningPhases(),
	}, true
}

func (b *Bank) LearningTrack(id string) (LearningTrack, bool) {
	for _, track := range b.learning.Tracks {
		if track.ID == id {
			return track, true
		}
	}
	return LearningTrack{}, false
}
