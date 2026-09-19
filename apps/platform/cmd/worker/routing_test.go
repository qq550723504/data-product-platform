package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
)

type stubProjector struct {
	err   error
	event outbox.PublishedEvent
	calls int
}

type stubEnqueuer struct{}

func (stubEnqueuer) EnqueueExecution(context.Context, uuid.UUID) error { return nil }

type failingEnqueuer struct{ err error }

func (e failingEnqueuer) EnqueueExecution(context.Context, uuid.UUID) error { return e.err }

func (s *stubProjector) HandleOutboxEvent(_ context.Context, event outbox.PublishedEvent) error {
	s.calls++
	s.event = event
	return s.err
}

// Without a governance provider the dispatcher must still build, and the
// governance route must be a declared retention-only obligation.
func TestNewOutboxDispatcherWithoutGovernanceProvider(t *testing.T) {
	dispatcher, err := newOutboxDispatcher(nil, slog.New(slog.DiscardHandler), nil, stubEnqueuer{})
	if err != nil {
		t.Fatalf("newOutboxDispatcher: %v", err)
	}
	if dispatcher.RouterVersion() != routing.Version {
		t.Fatalf("router version = %q, want %q", dispatcher.RouterVersion(), routing.Version)
	}
	if names := dispatcher.HandlerNames(); len(names) != 1 || names[0] != routing.HandlerExecutionQueue {
		t.Fatalf("handler names = %v, want [%s]", names, routing.HandlerExecutionQueue)
	}
}

func TestNewOutboxDispatcherWithGovernanceProvider(t *testing.T) {
	dispatcher, err := newOutboxDispatcher(nil, slog.New(slog.DiscardHandler), &stubProjector{}, stubEnqueuer{})
	if err != nil {
		t.Fatalf("newOutboxDispatcher: %v", err)
	}
	if dispatcher.RouterVersion() != routing.VersionFor(true) {
		t.Fatalf("router version = %q, want %q", dispatcher.RouterVersion(), routing.VersionFor(true))
	}
	if names := dispatcher.HandlerNames(); len(names) != 2 || names[0] != routing.HandlerExecutionQueue || names[1] != routing.HandlerMetadataProjection {
		t.Fatalf("handler names = %v, want [%s %s]", names, routing.HandlerExecutionQueue, routing.HandlerMetadataProjection)
	}
}

// The adapter must surface the projection error so the dispatcher treats the
// handler as unconfirmed instead of recording a false success.
func TestMetadataProjectionHandlerPropagatesError(t *testing.T) {
	sentinel := errors.New("openmetadata is unavailable")
	projector := &stubProjector{err: sentinel}
	handle := metadataProjectionHandler(projector, slog.New(slog.DiscardHandler))

	event := outbox.PublishedEvent{ID: uuid.New(), EventType: "ProductReleased"}
	if err := handle(context.Background(), event); !errors.Is(err, sentinel) {
		t.Fatalf("handler error = %v, want %v", err, sentinel)
	}
	if projector.calls != 1 || projector.event.ID != event.ID {
		t.Fatalf("projector calls = %d event = %v, want one call for %s", projector.calls, projector.event.ID, event.ID)
	}
}

func TestExecutionQueueHandlerUsesAggregateIDAndPropagatesFailure(t *testing.T) {
	called := uuid.New()
	var got uuid.UUID
	handle := executionQueueHandler(enqueuerFunc(func(_ context.Context, id uuid.UUID) error {
		got = id
		return nil
	}))
	if err := handle(context.Background(), outbox.PublishedEvent{AggregateType: "EXECUTION", AggregateID: called}); err != nil {
		t.Fatalf("execution queue handler: %v", err)
	}
	if got != called {
		t.Fatalf("enqueued execution = %s, want %s", got, called)
	}

	sentinel := errors.New("redis unavailable")
	if err := executionQueueHandler(failingEnqueuer{err: sentinel})(context.Background(), outbox.PublishedEvent{AggregateType: "EXECUTION", AggregateID: called}); !errors.Is(err, sentinel) {
		t.Fatalf("queue failure = %v, want %v", err, sentinel)
	}
}

type enqueuerFunc func(context.Context, uuid.UUID) error

func (f enqueuerFunc) EnqueueExecution(ctx context.Context, id uuid.UUID) error { return f(ctx, id) }

// The registered handler names must be exactly the ones the routing table can
// require, otherwise startup validation is vacuous.
func TestWorkerHandlerRegistrationsMatchRoutingNames(t *testing.T) {
	registrations := workerHandlerRegistrations(&stubProjector{}, stubEnqueuer{}, slog.New(slog.DiscardHandler))
	if len(registrations) != 2 {
		t.Fatalf("registrations = %d, want 1", len(registrations))
	}
	if registrations[0].Name != routing.HandlerExecutionQueue || registrations[1].Name != routing.HandlerMetadataProjection {
		t.Fatalf("registration names = %q/%q, want %q/%q", registrations[0].Name, registrations[1].Name, routing.HandlerExecutionQueue, routing.HandlerMetadataProjection)
	}
	if registrations[0].Handle == nil {
		t.Fatal("registration must carry a handler implementation")
	}
	if names := workerHandlerRegistrations(nil, stubEnqueuer{}, slog.New(slog.DiscardHandler)); len(names) != 1 || names[0].Name != routing.HandlerExecutionQueue {
		t.Fatalf("registrations without a provider = %v, want execution queue only", names)
	}
}
