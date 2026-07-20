package agent

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/MISSmihu/MHcode/internal/protocol"
)

type failoverProviderStub struct {
	name     string
	complete func(protocol.ChatRequest) (protocol.CompletionResult, error)
	stream   func(protocol.ChatRequest) (<-chan protocol.StreamEvent, error)
}

func (p failoverProviderStub) Name() string                                         { return p.name }
func (p failoverProviderStub) ListModels(context.Context) ([]protocol.Model, error) { return nil, nil }
func (p failoverProviderStub) Complete(_ context.Context, request protocol.ChatRequest) (protocol.CompletionResult, error) {
	return p.complete(request)
}
func (p failoverProviderStub) Stream(_ context.Context, request protocol.ChatRequest) (<-chan protocol.StreamEvent, error) {
	return p.stream(request)
}

func TestFailoverProviderRetriesCompletionOnAlternateRoute(t *testing.T) {
	provider := &failoverProvider{candidates: []routedProvider{
		{route: chatRoute{Provider: ModelProviderSetting{ID: "one", Name: "one"}, ModelID: "model-one"}, provider: failoverProviderStub{
			name: "one", complete: func(protocol.ChatRequest) (protocol.CompletionResult, error) {
				return protocol.CompletionResult{}, io.EOF
			},
		}},
		{route: chatRoute{Provider: ModelProviderSetting{ID: "two", Name: "two"}, ModelID: "model-two"}, provider: failoverProviderStub{
			name: "two", complete: func(request protocol.ChatRequest) (protocol.CompletionResult, error) {
				if request.Model != "model-two" {
					t.Fatalf("fallback model = %q", request.Model)
				}
				return protocol.CompletionResult{Content: "ok"}, nil
			},
		}},
	}}
	result, err := provider.Complete(context.Background(), protocol.ChatRequest{Model: "ignored"})
	if err != nil || result.Content != "ok" || provider.ActiveRoute().Provider.ID != "two" {
		t.Fatalf("result=%#v route=%#v err=%v", result, provider.ActiveRoute(), err)
	}
}

func TestFailoverProviderRetriesStreamBeforeVisibleOutput(t *testing.T) {
	errorStream := make(chan protocol.StreamEvent, 1)
	errorStream <- protocol.StreamEvent{Type: "error", Error: "unexpected EOF"}
	close(errorStream)
	okStream := make(chan protocol.StreamEvent, 2)
	okStream <- protocol.StreamEvent{Type: "delta", Delta: "ok"}
	okStream <- protocol.StreamEvent{Type: "done"}
	close(okStream)
	provider := &failoverProvider{candidates: []routedProvider{
		{route: chatRoute{Provider: ModelProviderSetting{ID: "one", Name: "one"}, ModelID: "one"}, provider: failoverProviderStub{name: "one", stream: func(protocol.ChatRequest) (<-chan protocol.StreamEvent, error) { return errorStream, nil }}},
		{route: chatRoute{Provider: ModelProviderSetting{ID: "two", Name: "two"}, ModelID: "two"}, provider: failoverProviderStub{name: "two", stream: func(protocol.ChatRequest) (<-chan protocol.StreamEvent, error) { return okStream, nil }}},
	}}
	events, err := provider.Stream(context.Background(), protocol.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var delta string
	for event := range events {
		if event.Type == "error" {
			t.Fatal(errors.New(event.Error))
		}
		delta += event.Delta
	}
	if delta != "ok" || provider.ActiveRoute().Provider.ID != "two" {
		t.Fatalf("delta=%q route=%#v", delta, provider.ActiveRoute())
	}
}

func TestFailoverProviderStopsWhileCandidateStreamIsOpen(t *testing.T) {
	stalled := make(chan protocol.StreamEvent)
	provider := &failoverProvider{candidates: []routedProvider{{
		route: chatRoute{Provider: ModelProviderSetting{ID: "one", Name: "one"}, ModelID: "one"},
		provider: failoverProviderStub{name: "one", stream: func(protocol.ChatRequest) (<-chan protocol.StreamEvent, error) {
			return stalled, nil
		}},
	}}}
	ctx, cancel := context.WithCancel(context.Background())
	events, err := provider.Stream(ctx, protocol.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("cancelled failover stream emitted an unexpected event")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("failover stream did not close promptly after cancellation")
	}
	close(stalled)
}
