package agent

import (
	"path/filepath"
	"testing"

	"github.com/MISSmihu/MHcode/internal/protocol"
	"github.com/MISSmihu/MHcode/internal/storage"
)

func TestUsageLedgerRecordsCostAndRestoresCurrentSession(t *testing.T) {
	root := t.TempDir()
	usagePath := filepath.Join(root, "usage.db")
	sessions := filepath.Join(root, "sessions")
	projects := filepath.Join(root, "projects.json")

	store, err := storage.Open(usagePath)
	if err != nil {
		t.Fatal(err)
	}
	config := ServiceConfig{
		SkillsDir:    t.TempDir(),
		SessionsDir:  sessions,
		ProjectsPath: projects,
		UsageStore:   store,
	}
	service := NewService(config)
	provider := ModelProviderSetting{
		ID:                    "custom",
		Name:                  "Custom",
		InputPricePerMillion:  1,
		OutputPricePerMillion: 2,
	}
	route := chatRoute{Provider: provider, ModelID: "model-a"}
	metrics := usageMetricsFor(provider, &protocol.TokenUsage{
		PromptTokens:          100,
		CompletionTokens:      10,
		PromptCacheHitTokens:  80,
		PromptCacheMissTokens: 20,
	})
	if metrics.EffectiveCost < 0.000119 || metrics.EffectiveCost > 0.000121 {
		t.Fatalf("cost = %f, want 0.00012", metrics.EffectiveCost)
	}
	service.metrics = metrics
	service.recordUsageMetrics(metrics, route)
	state := service.WorkbenchState()
	if state.UsageLedger.TotalSamples != 1 || state.UsageLedger.TotalInputTokens != 100 {
		t.Fatalf("ledger state = %#v", state.UsageLedger)
	}
	service.Close()

	store, err = storage.Open(usagePath)
	if err != nil {
		t.Fatal(err)
	}
	reloaded := NewService(ServiceConfig{
		SkillsDir:    t.TempDir(),
		SessionsDir:  sessions,
		ProjectsPath: projects,
		UsageStore:   store,
	})
	defer reloaded.Close()
	reloadedState := reloaded.WorkbenchState()
	if reloadedState.UsageMetrics.InputTokens != 100 || reloadedState.UsageMetrics.OutputTokens != 10 {
		t.Fatalf("restored usage = %#v", reloadedState.UsageMetrics)
	}
	if reloadedState.DeepSeekSession.SessionCacheHitTokens != 80 || reloadedState.DeepSeekSession.SessionCacheMissTokens != 20 {
		t.Fatalf("restored session cache = %#v", reloadedState.DeepSeekSession)
	}
}
