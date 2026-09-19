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

func (s *stubProjector) HandleOutboxEvent(_ context.Context, event outbox.PublishedEvent) error {
	s.calls++
	s.event = event
	return s.err
}

// Without a governance provider the dispatcher must still build, and the
// governance route must be a declared retention-only obligation.
func TestNewOutboxDispatcherWithoutGovernanceProvider(t *testing.T) {
	dispatcher, err := newOutboxDispatcher(nil, slog.New(slog.DiscardHandler), nil)
	if err != nil {
		t.Fatalf("newOutboxDispatcher: %v", err)
	}
	if dispatcher.RouterVersion() != routing.Version {
		t.Fatalf("router version = %q, want %q", dispatcher.RouterVersion(), routing.Version)
	}
	if names := dispatcher.HandlerNames(); len(names) != 0 {
		t.Fatalf("handler names = %v, want none", names)
	}
}

func TestNewOutboxDispatcherWithGovernanceProvider(t *testing.T) {
	dispatcher, err := newOutboxDispatcher(nil, slog.New(slog.DiscardHandler), &stubProjector{})
	if err != nil {
		t.Fatalf("newOutboxDispatcher: %v", err)
	}
	if dispatcher.RouterVersion() != routing.VersionFor(true) {
		t.Fatalf("router version = %q, want %q", dispatcher.RouterVersion(), routing.VersionFor(true))
	}
	if names := dispatcher.HandlerNames(); len(names) != 1 || names[0] != routing.HandlerMetadataProjection {
		t.Fatalf("handler names = %v, want [%s]", names, routing.HandlerMetadataProjection)
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

// The registered handler names must be exactly the ones the routing table can
// require, otherwise startup validation is vacuous.
func TestWorkerHandlerRegistrationsMatchRoutingNames(t *testing.T) {
	registrations := workerHandlerRegistrations(&stubProjector{}, slog.New(slog.DiscardHandler))
	if len(registrations) != 1 {
		t.Fatalf("registrations = %d, want 1", len(registrations))
	}
	if registrations[0].Name != routing.HandlerMetadataProjection {
		t.Fatalf("registration name = %q, want %q", registrations[0].Name, routing.HandlerMetadataProjection)
	}
	if registrations[0].Handle == nil {
		t.Fatal("registration must carry a handler implementation")
	}
	if names := workerHandlerRegistrations(nil, slog.New(slog.DiscardHandler)); names != nil {
		t.Fatalf("registrations without a provider = %v, want nil", names)
	}
}
