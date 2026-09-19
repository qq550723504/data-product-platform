package main

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
)

// eventVocabulary is the second witness to the routing table: every event type
// the platform can emit must appear exactly once. Adding a domain event without
// declaring its routing obligation fails this test, and an undeclared event
// fails dispatch at runtime with a diagnosable error instead of being completed
// implicitly.
var eventVocabulary = []string{
	"AuthorizationActivated",
	"AuthorizationApproved",
	"AuthorizationCreated",
	"AuthorizationExpired",
	"AuthorizationRevoked",
	"AuthorizationSubmitted",
	"AuthorizationSuspended",
	"ComplianceFailed",
	"CompliancePassed",
	"ComplianceReviewRequired",
	"ContractPublished",
	"ContractVersionCreated",
	"DataProductCreated",
	"DataResourceCreated",
	"DatasetCreated",
	"DatasetVersionCreated",
	"DatasetVersionInvalidated",
	"EntityMappingDecisionRecorded",
	"EntityMatchCompleted",
	"ExecutionEngineSelected",
	"ExecutionFailed",
	"ExecutionQueued",
	"ExecutionRetried",
	"ExecutionStarted",
	"ExecutionSubmitting",
	"ExecutionSucceeded",
	"ProductReleaseDraftCreated",
	"ProductReleased",
	"ProductReleaseReady",
	"ProductReleaseValidationStarted",
	"ProductVersionCreated",
	"QualityFailed",
	"QualityPassed",
	"QualityReviewRequired",
	"RightsSnapshotCreated",
	"WorkflowVersionCreated",
}

func TestWorkerRoutesDeclareEveryEventTypeExactlyOnce(t *testing.T) {
	if outboxRoutingVersion == "" {
		t.Fatal("outbox routing version must be set")
	}
	router, err := outbox.NewRouter(effectiveRoutingVersion(true), workerRoutes(true))
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	declared := router.EventTypes()
	sort.Strings(declared)
	expected := append([]string(nil), eventVocabulary...)
	sort.Strings(expected)
	if len(declared) != len(expected) {
		t.Fatalf("declared %d event types, want %d\ndeclared: %v\nwant:     %v", len(declared), len(expected), declared, expected)
	}
	for i := range expected {
		if declared[i] != expected[i] {
			t.Fatalf("event vocabulary mismatch at %d: declared %q, want %q", i, declared[i], expected[i])
		}
	}
}

func TestWorkerRoutesResolveGovernanceProjectionProfile(t *testing.T) {
	if effectiveRoutingVersion(true) == effectiveRoutingVersion(false) {
		t.Fatal("the governance profile must be encoded in the routing version")
	}
	withProjection, err := outbox.NewRouter(effectiveRoutingVersion(true), workerRoutes(true))
	if err != nil {
		t.Fatalf("NewRouter(projection on): %v", err)
	}
	required, ok := withProjection.RequiredHandlers("ProductReleased")
	if !ok {
		t.Fatal("ProductReleased must be declared")
	}
	if len(required) != 1 || required[0] != handlerMetadataProjection {
		t.Fatalf("ProductReleased required handlers = %v, want [%s]", required, handlerMetadataProjection)
	}

	withoutProjection, err := outbox.NewRouter(effectiveRoutingVersion(false), workerRoutes(false))
	if err != nil {
		t.Fatalf("NewRouter(projection off): %v", err)
	}
	required, ok = withoutProjection.RequiredHandlers("ProductReleased")
	if !ok {
		t.Fatal("ProductReleased must stay declared without a governance provider")
	}
	if len(required) != 0 {
		t.Fatalf("ProductReleased required handlers = %v, want an explicit retention-only declaration", required)
	}
}

func TestWorkerRoutesKeepExecutionDispatchExplicitForT2(t *testing.T) {
	router, err := outbox.NewRouter(effectiveRoutingVersion(true), workerRoutes(true))
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	for _, eventType := range []string{"ExecutionQueued", "ExecutionRetried"} {
		required, ok := router.RequiredHandlers(eventType)
		if !ok {
			t.Fatalf("%s must be declared even while it is retention-only", eventType)
		}
		if len(required) != 0 {
			t.Fatalf("%s required handlers = %v; T2 moves enqueue onto %s and bumps the routing version", eventType, required, handlerExecutionQueue)
		}
	}
}

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
	if dispatcher.RouterVersion() != outboxRoutingVersion {
		t.Fatalf("router version = %q, want %q", dispatcher.RouterVersion(), outboxRoutingVersion)
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
	if dispatcher.RouterVersion() != effectiveRoutingVersion(true) {
		t.Fatalf("router version = %q, want %q", dispatcher.RouterVersion(), effectiveRoutingVersion(true))
	}
	if names := dispatcher.HandlerNames(); len(names) != 1 || names[0] != handlerMetadataProjection {
		t.Fatalf("handler names = %v, want [%s]", names, handlerMetadataProjection)
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
	if registrations[0].Name != handlerMetadataProjection {
		t.Fatalf("registration name = %q, want %q", registrations[0].Name, handlerMetadataProjection)
	}
	if registrations[0].Handle == nil {
		t.Fatal("registration must carry a handler implementation")
	}
	if names := workerHandlerRegistrations(nil, slog.New(slog.DiscardHandler)); names != nil {
		t.Fatalf("registrations without a provider = %v, want nil", names)
	}
}
