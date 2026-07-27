package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/MISSmihu/MHcode/internal/tools"
)

var (
	ErrTaskIdleTimeout      = errors.New("agent task idle timeout")
	ErrToolExecutionTimeout = errors.New("tool execution timeout")
)

type taskWatchdogControl struct {
	stopOnce sync.Once
	stop     chan struct{}
	cancel   context.CancelCauseFunc
}

func (control *taskWatchdogControl) pause() {
	if control == nil {
		return
	}
	control.stopOnce.Do(func() { close(control.stop) })
}

func (control *taskWatchdogControl) close() {
	if control == nil {
		return
	}
	control.pause()
	control.cancel(context.Canceled)
}

func withTaskIdleWatchdog(
	parent context.Context,
	timeout time.Duration,
	sink ChatEventSink,
) (context.Context, ChatEventSink, *taskWatchdogControl) {
	if parent == nil {
		parent = context.Background()
	}
	if timeout <= 0 {
		return parent, sink, nil
	}

	ctx, cancel := context.WithCancelCause(parent)
	activity := make(chan struct{}, 1)
	control := &taskWatchdogControl{stop: make(chan struct{}), cancel: cancel}
	serializedSink := serializedChatEventSink(sink)
	touch := func() {
		select {
		case activity <- struct{}{}:
		default:
		}
	}
	wrappedSink := func(event ChatStreamEvent) {
		touch()
		emitChatEvent(serializedSink, event)
	}

	go func() {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-control.stop:
				return
			case <-activity:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(timeout)
			case <-timer.C:
				cause := fmt.Errorf("%w after %s without model or tool activity", ErrTaskIdleTimeout, timeout)
				emitChatEvent(serializedSink, ChatStreamEvent{
					Type:    "status",
					Status:  "failed",
					Message: fmt.Sprintf("任务已连续 %s 没有模型或工具活动，MHcode 已结束本轮任务。", timeout),
				})
				cancel(cause)
				return
			}
		}
	}()
	touch()
	return ctx, wrappedSink, control
}

func resolvedTaskContextError(ctx context.Context, err error) error {
	if ctx == nil {
		return err
	}
	cause := context.Cause(ctx)
	if !errors.Is(cause, ErrTaskIdleTimeout) {
		return err
	}
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return cause
	}
	return err
}

func chatTurnWasCancelled(ctx context.Context, err error) bool {
	if ctx != nil && errors.Is(context.Cause(ctx), ErrTaskIdleTimeout) {
		return false
	}
	return errors.Is(err, context.Canceled) || (ctx != nil && errors.Is(ctx.Err(), context.Canceled))
}

type toolExecutionOutcome struct {
	result tools.Result
	err    error
}

func (s *Service) runToolWithWatchdog(
	ctx context.Context,
	tool tools.Tool,
	name string,
	rawArgs json.RawMessage,
	timeout time.Duration,
) (tools.Result, error) {
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	retainsTaskContext := false
	if backgroundTool, ok := tool.(interface{ RetainsTaskContext() bool }); ok {
		retainsTaskContext = backgroundTool.RetainsTaskContext()
	}
	if retainsTaskContext && ctx.Err() != nil {
		return s.runToolWithApproval(ctx, tool, name, rawArgs)
	}
	toolCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	executionCtx := toolCtx
	if retainsTaskContext {
		executionCtx = ctx
	}

	done := make(chan toolExecutionOutcome, 1)
	go func() {
		if toolNeedsExclusiveWorkspaceAccess(name, tool) {
			s.toolMutationMu.Lock()
			defer s.toolMutationMu.Unlock()
		}
		result, err := s.runToolWithApproval(executionCtx, tool, name, rawArgs)
		select {
		case done <- toolExecutionOutcome{result: result, err: err}:
		case <-toolCtx.Done():
		}
	}()

	select {
	case outcome := <-done:
		return outcome.result, outcome.err
	case <-toolCtx.Done():
		if cause := context.Cause(ctx); cause != nil {
			return tools.Result{}, cause
		}
		if !errors.Is(toolCtx.Err(), context.DeadlineExceeded) {
			return tools.Result{}, toolCtx.Err()
		}
		summary := fmt.Sprintf("工具 %s 执行超过 %s，MHcode 已取消当前工具并忽略迟到结果。", name, timeout)
		return tools.Result{
			Summary: summary,
			IsError: true,
			Parts: []tools.ResultPart{{
				Kind:   tools.PartToolCall,
				Name:   name,
				Status: "error",
				Output: summary,
			}},
		}, nil
	}
}
