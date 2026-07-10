package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/MISSmihu/MHcode/internal/agent"
)

type App struct {
	ctx     context.Context
	service *agent.Service
}

func NewApp() *App {
	return &App{
		service: agent.NewService(agent.ServiceConfig{
			SkillsDir:    "skills",
			SettingsPath: runtimeSettingsPath(),
		}),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) GetWorkbenchState() agent.WorkbenchState {
	return a.service.WorkbenchState()
}

func (a *App) SetReasoningLevel(level string) (agent.WorkbenchState, error) {
	return a.service.SetReasoningLevel(agent.ReasoningLevel(level))
}

func (a *App) SaveDeepSeekAPIKey(apiKey string) (agent.WorkbenchState, error) {
	return a.service.SaveDeepSeekAPIKey(apiKey)
}

func (a *App) ClearDeepSeekAPIKey() (agent.WorkbenchState, error) {
	return a.service.ClearDeepSeekAPIKey()
}

func (a *App) TestDeepSeekConnection() (agent.WorkbenchState, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.service.TestDeepSeekConnection(ctx)
}

func (a *App) SendDeepSeekMessage(prompt string) (agent.ChatResult, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.service.SendDeepSeekMessage(ctx, prompt)
}

func (a *App) ResetDeepSeekSession() (agent.WorkbenchState, error) {
	return a.service.ResetDeepSeekSession()
}

func (a *App) SaveRuntimeSettings(settings agent.RuntimeSettings) (agent.WorkbenchState, error) {
	return a.service.SaveRuntimeSettings(settings)
}

func (a *App) SaveModelProviderAPIKey(providerID string, apiKey string) (agent.WorkbenchState, error) {
	return a.service.SaveModelProviderAPIKey(providerID, apiKey)
}

func (a *App) ClearModelProviderAPIKey(providerID string) (agent.WorkbenchState, error) {
	return a.service.ClearModelProviderAPIKey(providerID)
}

func (a *App) RefreshModelProviderModels(providerID string) (agent.WorkbenchState, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.service.RefreshModelProviderModels(ctx, providerID)
}

func runtimeSettingsPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil || configDir == "" {
		return "mhcode.runtime.json"
	}
	return filepath.Join(configDir, "MHcode", "runtime-settings.json")
}
