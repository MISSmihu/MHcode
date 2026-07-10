package config

import "github.com/MISSmihu/MHcode/internal/agent"

type AppConfig struct {
	DefaultReasoning agent.ReasoningLevel `json:"defaultReasoning"`
	SkillsDir        string               `json:"skillsDir"`
	DatabasePath     string               `json:"databasePath"`
}

func Default() AppConfig {
	return AppConfig{
		DefaultReasoning: agent.DefaultReasoningLevel,
		SkillsDir:        "skills",
		DatabasePath:     "mhcode.db",
	}
}
