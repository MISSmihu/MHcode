package agent

import "fmt"

type ReasoningLevel string

const (
	ReasoningLow    ReasoningLevel = "low"
	ReasoningMedium ReasoningLevel = "medium"
	ReasoningHigh   ReasoningLevel = "high"
	ReasoningUltra  ReasoningLevel = "ultra"
)

const DefaultReasoningLevel = ReasoningUltra

type ReasoningBudget struct {
	MaxToolCalls  int    `json:"maxToolCalls"`
	ContextPolicy string `json:"contextPolicy"`
	CachePolicy   string `json:"cachePolicy"`
	Planner       bool   `json:"planner"`
}

type ReasoningProfile struct {
	ID          ReasoningLevel  `json:"id"`
	Label       string          `json:"label"`
	Description string          `json:"description"`
	Budget      ReasoningBudget `json:"budget"`
}

var reasoningProfiles = []ReasoningProfile{
	{
		ID:          ReasoningLow,
		Label:       "低",
		Description: "简单问答、轻量编辑、低成本优先",
		Budget: ReasoningBudget{
			MaxToolCalls:  3,
			ContextPolicy: "minimal",
			CachePolicy:   "reuse-prefix",
			Planner:       false,
		},
	},
	{
		ID:          ReasoningMedium,
		Label:       "中",
		Description: "普通代码修改、单文件任务",
		Budget: ReasoningBudget{
			MaxToolCalls:  8,
			ContextPolicy: "task-summary",
			CachePolicy:   "reuse-prefix",
			Planner:       false,
		},
	},
	{
		ID:          ReasoningHigh,
		Label:       "高",
		Description: "跨文件修改、复杂 bug、测试修复",
		Budget: ReasoningBudget{
			MaxToolCalls:  16,
			ContextPolicy: "expanded",
			CachePolicy:   "stable-prefix",
			Planner:       true,
		},
	},
	{
		ID:          ReasoningUltra,
		Label:       "超高",
		Description: "协议设计、Agent 架构、发布级检查",
		Budget: ReasoningBudget{
			MaxToolCalls:  32,
			ContextPolicy: "full-relevant",
			CachePolicy:   "strict-stable-prefix",
			Planner:       true,
		},
	},
}

func ReasoningProfiles() []ReasoningProfile {
	profiles := make([]ReasoningProfile, len(reasoningProfiles))
	copy(profiles, reasoningProfiles)
	return profiles
}

func ParseReasoningLevel(value string) (ReasoningLevel, error) {
	level := ReasoningLevel(value)
	if _, ok := ReasoningProfileFor(level); !ok {
		return "", fmt.Errorf("unknown reasoning level %q", value)
	}
	return level, nil
}

func ReasoningProfileFor(level ReasoningLevel) (ReasoningProfile, bool) {
	for _, profile := range reasoningProfiles {
		if profile.ID == level {
			return profile, true
		}
	}
	return ReasoningProfile{}, false
}
