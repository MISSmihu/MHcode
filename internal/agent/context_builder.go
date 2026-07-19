package agent

import (
	"github.com/MISSmihu/MHcode/internal/cache"
	"github.com/MISSmihu/MHcode/internal/mcp"
	"github.com/MISSmihu/MHcode/internal/skills"
)

type ContextSection struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type StableContext struct {
	ProductIdentity string               `json:"productIdentity"`
	SystemRules     []string             `json:"systemRules"`
	Reasoning       ReasoningProfile     `json:"reasoning"`
	SkillsIndex     []skills.IndexEntry  `json:"skillsIndex"`
	MCPSnapshots    []mcp.ServerSnapshot `json:"mcpSnapshots"`
	ProjectSummary  string               `json:"projectSummary"`
	RoutingPolicy   string               `json:"routingPolicy"`
}

type VolatileContext struct {
	UserInput          string                  `json:"userInput"`
	TriggeredSkills    []string                `json:"triggeredSkills,omitempty"`
	ProjectContext     string                  `json:"projectContext,omitempty"`
	RecentDiffSummary  string                  `json:"recentDiffSummary,omitempty"`
	ToolCallSummaries  []mcp.ToolResultSummary `json:"toolCallSummaries,omitempty"`
	OutputRequirements []string                `json:"outputRequirements"`
}

type RequestContext struct {
	StablePrefix []ContextSection `json:"stablePrefix"`
	VolatileTail []ContextSection `json:"volatileTail"`
	PrefixHash   string           `json:"prefixHash"`
}

type ContextBuilder struct{}

func NewContextBuilder() ContextBuilder {
	return ContextBuilder{}
}

func (ContextBuilder) Build(stable StableContext, volatile VolatileContext) RequestContext {
	prefix := []ContextSection{
		{Name: "product_identity", Content: stable.ProductIdentity},
		{Name: "system_rules", Content: joinLines(stable.SystemRules)},
		{Name: "reasoning", Content: string(stable.Reasoning.ID) + ":" + stable.Reasoning.Budget.CachePolicy},
		{Name: "skills_index", Content: skills.FormatStableIndex(stable.SkillsIndex)},
		{Name: "mcp_schema_snapshot", Content: mcp.FormatSnapshots(stable.MCPSnapshots)},
		{Name: "project_summary", Content: stable.ProjectSummary},
		{Name: "routing_policy", Content: stable.RoutingPolicy},
	}
	tail := []ContextSection{
		{Name: "user_input", Content: volatile.UserInput},
		{Name: "triggered_skills", Content: joinLines(volatile.TriggeredSkills)},
		{Name: "project_context", Content: volatile.ProjectContext},
		{Name: "recent_diff", Content: volatile.RecentDiffSummary},
		{Name: "tool_results", Content: mcp.FormatToolResults(volatile.ToolCallSummaries)},
		{Name: "output_requirements", Content: joinLines(volatile.OutputRequirements)},
	}

	return RequestContext{
		StablePrefix: prefix,
		VolatileTail: tail,
		PrefixHash:   cache.HashStablePrefix(prefix),
	}
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	out := lines[0]
	for _, line := range lines[1:] {
		out += "\n" + line
	}
	return out
}
