package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MISSmihu/MHcode/internal/protocol"
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
