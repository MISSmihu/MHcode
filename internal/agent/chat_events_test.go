package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MISSmihu/MHcode/internal/protocol"
	"github.com/MISSmihu/MHcode/internal/tools"
)

type stalledStreamProvider struct {
	started chan struct{}
	events  chan protocol.StreamEvent
}

type stalledOpenProvider struct {
	started chan struct{}
	release chan struct{}
}

func (p *stalledOpenProvider) Name() string { return "stalled-open" }

func (p *stalledOpenProvider) ListModels(context.Context) ([]protocol.Model, error) {
	return nil, nil
}

func (p *stalledOpenProvider) Stream(context.Context, protocol.ChatRequest) (<-chan protocol.StreamEvent, error) {
	close(p.started)
	<-p.release
	events := make(chan protocol.StreamEvent)
	close(events)
	return events, nil
}

func (p *stalledStreamProvider) Name() string { return "stalled" }

func (p *stalledStreamProvider) ListModels(context.Context) ([]protocol.Model, error) {
	return nil, nil
}

func (p *stalledStreamProvider) Stream(context.Context, protocol.ChatRequest) (<-chan protocol.StreamEvent, error) {
	close(p.started)
	return p.events, nil
}

func TestCollectProviderStreamStopsWhileEventStreamIsStillOpen(t *testing.T) {
	provider := &stalledStreamProvider{
		started: make(chan struct{}),
		events:  make(chan protocol.StreamEvent),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := collectProviderStream(ctx, provider, protocol.ChatRequest{}, nil)
		done <- err
	}()

	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("provider stream did not start")
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("collect error = %v, want context cancellation", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("collectProviderStream did not stop promptly")
	}
	close(provider.events)
}

func TestCollectProviderStreamReturnsTextReceivedBeforeCancellation(t *testing.T) {
	provider := &stalledStreamProvider{
		started: make(chan struct{}),
		events:  make(chan protocol.StreamEvent),
	}
	ctx, cancel := context.WithCancel(context.Background())
	type streamResult struct {
		completion protocol.CompletionResult
		err        error
	}
	done := make(chan streamResult, 1)
	go func() {
		completion, err := collectProviderStream(ctx, provider, protocol.ChatRequest{}, nil)
		done <- streamResult{completion: completion, err: err}
	}()

	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("provider stream did not start")
	}
	provider.events <- protocol.StreamEvent{Type: "delta", Delta: "partial answer"}
	cancel()

	select {
	case result := <-done:
		if !errors.Is(result.err, context.Canceled) {
			t.Fatalf("collect error = %v, want context cancellation", result.err)
		}
		if result.completion.Content != "partial answer" {
			t.Fatalf("partial content = %q", result.completion.Content)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("collectProviderStream did not return partial content promptly")
	}
	close(provider.events)
}

func TestCollectProviderStreamStopsWhileProviderOpenIsBlocked(t *testing.T) {
	provider := &stalledOpenProvider{started: make(chan struct{}), release: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := collectProviderStream(ctx, provider, protocol.ChatRequest{}, nil)
		done <- err
	}()

	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("provider open did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("collect error = %v, want context cancellation", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("blocked provider open prevented prompt cancellation")
	}
	close(provider.release)
}

func TestCollectProviderStreamEmitsAndCollectsProviderNotice(t *testing.T) {
	notice := protocol.ProviderNotice{
		Kind: protocol.ProviderNoticeModelReroute, Severity: "warning",
		RequestedModel: "gpt-requested", EffectiveModel: "gpt-restricted", RequestID: "req-1",
	}
	events := make(chan protocol.StreamEvent, 4)
	events <- protocol.StreamEvent{Type: "server_model", Delta: "gpt-restricted"}
	events <- protocol.StreamEvent{Type: "provider_notice", Notice: &notice}
	events <- protocol.StreamEvent{Type: "delta", Delta: "done"}
	events <- protocol.StreamEvent{Type: "done"}
	close(events)
	provider := &stalledStreamProvider{started: make(chan struct{}), events: events}
	var emitted []ChatStreamEvent
	result, err := collectProviderStream(context.Background(), provider, protocol.ChatRequest{}, func(event ChatStreamEvent) {
		emitted = append(emitted, event)
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "done" || result.EffectiveModel != "gpt-restricted" || len(result.Notices) != 1 {
		t.Fatalf("result = %#v", result)
	}
	found := false
	for _, event := range emitted {
		if event.Type == "provider_notice" && len(event.Parts) == 1 && event.Parts[0].Kind == tools.PartProviderNotice {
			found = true
		}
	}
	if !found {
		t.Fatalf("provider notice was not emitted: %#v", emitted)
	}
}

func TestCollectProviderStreamPreservesTypedProviderError(t *testing.T) {
	events := make(chan protocol.StreamEvent, 1)
	events <- protocol.StreamEvent{
		Type: "error", Error: "request blocked",
		ProviderError: &protocol.ProviderErrorInfo{Code: "cyber_policy", Message: "request blocked", Retryable: false},
	}
	close(events)
	provider := &stalledStreamProvider{started: make(chan struct{}), events: events}
	_, err := collectProviderStream(context.Background(), provider, protocol.ChatRequest{}, nil)
	info, ok := protocol.ProviderErrorDetails(err)
	if !ok || info.Code != "cyber_policy" || info.Retryable {
		t.Fatalf("error = %v, info = %#v, ok = %v", err, info, ok)
	}
}

func TestProviderErrorNoticeKindDistinguishesPolicyAndTransportFailures(t *testing.T) {
	policy := providerErrorNoticePart(protocol.ProviderErrorInfo{Code: "cyber_policy", Message: "blocked"})
	if policy.NoticeKind != protocol.ProviderNoticePolicyError {
		t.Fatalf("policy notice kind = %q", policy.NoticeKind)
	}

	ordinary := providerErrorNoticePart(protocol.ProviderErrorInfo{
		Code: "upstream_timeout", Message: "gateway timeout", HTTPStatus: 504, Retryable: true,
	})
	if ordinary.NoticeKind != protocol.ProviderNoticeProviderError {
		t.Fatalf("ordinary provider notice kind = %q", ordinary.NoticeKind)
	}
}
