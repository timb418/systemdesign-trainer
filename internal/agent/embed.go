package traineragent

import _ "embed"

//go:embed prompts/interviewer.md
var interviewerPrompt string

//go:embed prompts/evaluator.md
var evaluatorPrompt string

//go:embed prompts/compare.md
var comparePrompt string

//go:embed prompts/mentor.md
var mentorPrompt string
