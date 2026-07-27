package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/MISSmihu/MHcode/internal/tools"
)

type contextIgnoringTool struct {
	name    string
	release <-chan struct{}
}

func (tool contextIgnoringTool) Name() string {
	if strings.TrimSpace(tool.name) != "" {
		return tool.name
	}
	return "context_ignoring"
}
func (contextIgnoringTool) Description() string { return "waits without honoring context" }
func (contextIgnoringTool) InputSchema() map[string]any {
	return map[string]any{"type": "object"}
}

func TestToolWatchdogBoundsUncooperativeMCPAndPluginTools(t *testing.T) {
	for _, name := range []string{"mcp__stalled__forever", "plugin__stalled__forever"} {
		t.Run(name, func(t *testing.T) {
			release := make(chan struct{})
			service := NewService(ServiceConfig{SkillsDir: t.TempDir()})
			service.runtimeSettings.ApprovalPolicy = "never"
			startedAt := time.Now()
			result, err := service.runToolWithWatchdog(
				context.Background(),
				contextIgnoringTool{name: name, release: release},
				name,
				json.RawMessage(`{}`),
				35*time.Millisecond,
			)
			close(release)
			if err != nil {
				t.Fatal(err)
			}
			if !result.IsError || !strings.Contains(result.Summary, "忽略迟到结果") {
				t.Fatalf("timeout result = %#v", result)
			}
			if elapsed := time.Since(startedAt); elapsed > 250*time.Millisecond {
				t.Fatalf("watchdog returned too slowly: %s", elapsed)
			}
		})
	}
}
func (tool contextIgnoringTool) Execute(context.Context, json.RawMessage) (tools.Result, error) {
	<-tool.release
	return tools.Result{Summary: "late result"}, nil
}

func TestToolWatchdogReturnsWhenToolIgnoresContext(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	service := NewService(ServiceConfig{SkillsDir: t.TempDir()})
	startedAt := time.Now()
	result, err := service.runToolWithWatchdog(
		context.Background(),
		contextIgnoringTool{release: release},
		"context_ignoring",
		json.RawMessage(`{}`),
		35*time.Millisecond,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Summary, "忽略迟到结果") {
		t.Fatalf("timeout result = %#v", result)
	}
	if elapsed := time.Since(startedAt); elapsed > 250*time.Millisecond {
		t.Fatalf("watchdog returned too slowly: %s", elapsed)
	}
}

func TestToolWatchdogReturnsParentCancellation(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	service := NewService(ServiceConfig{SkillsDir: t.TempDir()})
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(25*time.Millisecond, cancel)
	startedAt := time.Now()
	_, err := service.runToolWithWatchdog(
		ctx,
		contextIgnoringTool{release: release},
		"context_ignoring",
		json.RawMessage(`{}`),
		time.Second,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
	if elapsed := time.Since(startedAt); elapsed > 250*time.Millisecond {
		t.Fatalf("cancellation returned too slowly: %s", elapsed)
	}
}

func TestTaskIdleWatchdogEmitsFailedTerminalStatus(t *testing.T) {
	var events []ChatStreamEvent
	ctx, _, control := withTaskIdleWatchdog(context.Background(), 35*time.Millisecond, func(event ChatStreamEvent) {
		events = append(events, event)
	})
	defer control.close()
	select {
	case <-ctx.Done():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("idle watchdog did not cancel the task")
	}
	if !errors.Is(context.Cause(ctx), ErrTaskIdleTimeout) {
		t.Fatalf("cause = %v", context.Cause(ctx))
	}
	if len(events) != 1 || events[0].Status != "failed" {
		t.Fatalf("events = %#v", events)
	}
}

func TestTaskIdleWatchdogResetsOnActivity(t *testing.T) {
	ctx, sink, control := withTaskIdleWatchdog(context.Background(), 80*time.Millisecond, nil)
	defer control.close()
	for index := 0; index < 4; index++ {
		time.Sleep(30 * time.Millisecond)
		sink(ChatStreamEvent{Type: "status", Status: "running"})
	}
	select {
	case <-ctx.Done():
		t.Fatalf("watchdog expired despite activity: %v", context.Cause(ctx))
	default:
	}
	control.pause()
}

func TestResolvedTaskContextErrorKeepsIdleTimeoutAsFailure(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(fmt.Errorf("%w: test", ErrTaskIdleTimeout))
	err := resolvedTaskContextError(ctx, context.Canceled)
	if !errors.Is(err, ErrTaskIdleTimeout) || chatTurnWasCancelled(ctx, err) {
		t.Fatalf("resolved error = %v, cancelled=%v", err, chatTurnWasCancelled(ctx, err))
	}
}
