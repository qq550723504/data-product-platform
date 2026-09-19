package main

import "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"

// Handler names are the per-handler confirmation identities recorded in
// outbox_event_consumption. A handler name must stay stable across releases:
// changing it makes every historical confirmation look unconfirmed and replays
// the handler, so rename only with a deliberate migration.
const (
	handlerExecutionQueue     = "execution-queue"
	handlerMetadataProjection = "metadata-projection"
)

// outboxRoutingVersion identifies the base layout of the routing table below.
// Bump it whenever an event type's required-handler set changes. Dispatch logs
// and dead-letter diagnostics reference it, so the obligation that was in force
// when an event was handled stays auditable.
const outboxRoutingVersion = "c1-v1"

// effectiveRoutingVersion folds the deployment profile into the routing version
// so the same version string always describes the same required-handler set.
// Enabling or disabling governance projection is a new version, never a silent
// mutation of an existing one.
func effectiveRoutingVersion(governanceProjection bool) string {
	if governanceProjection {
		return outboxRoutingVersion + "+governance"
	}
	return outboxRoutingVersion
}

// workerRoutes declares, for every event type the platform can emit, which
// handler obligations must be confirmed before the event may be marked
// PUBLISHED. The declaration is deterministic and versioned: an event type
// absent from the table is an error, never an implicit success, and events with
// no external side effect are declared retention-only (empty RequiredHandlers)
// rather than relying on a default no-op.
//
// governanceProjection selects the deployment profile. It is fixed for the
// lifetime of the worker process, so a restart with a different profile starts a
// different routing version instead of silently erasing obligations that were
// already being handled.
func workerRoutes(governanceProjection bool) []outbox.Route {
	productReleased := []string(nil)
	if governanceProjection {
		productReleased = []string{handlerMetadataProjection}
	}

	return []outbox.Route{
		// Governance projection is only owed when the deployment has a
		// governance provider configured. Without one the release event is
		// explicitly retention-only, not an implicit no-op.
		{EventType: "ProductReleased", RequiredHandlers: productReleased},

		// Execution dispatch still goes through the direct queue call in this
		// round. T2 moves both Create and Retry onto handlerExecutionQueue and
		// bumps the routing version; until then the outbox event itself only
		// needs to be retained, because the direct call still delivers it.
		{EventType: "ExecutionQueued"},
		{EventType: "ExecutionRetried"},

		// Retention-only: recorded for audit and traceability, with no
		// external side-effect obligation.
		{EventType: "ExecutionStarted"},
		{EventType: "ExecutionSucceeded"},
		{EventType: "ExecutionFailed"},
		{EventType: "ExecutionEngineSelected"},
		{EventType: "ExecutionSubmitting"},
		{EventType: "DataResourceCreated"},
		{EventType: "DatasetCreated"},
		{EventType: "DatasetVersionCreated"},
		{EventType: "DatasetVersionInvalidated"},
		{EventType: "EntityMappingDecisionRecorded"},
		{EventType: "EntityMatchCompleted"},
		{EventType: "WorkflowVersionCreated"},
		{EventType: "DataProductCreated"},
		{EventType: "ProductVersionCreated"},
		{EventType: "ProductReleaseDraftCreated"},
		{EventType: "ProductReleaseValidationStarted"},
		{EventType: "ProductReleaseReady"},
		{EventType: "ContractVersionCreated"},
		{EventType: "ContractPublished"},
		{EventType: "AuthorizationCreated"},
		{EventType: "AuthorizationSubmitted"},
		{EventType: "AuthorizationApproved"},
		{EventType: "AuthorizationActivated"},
		{EventType: "AuthorizationSuspended"},
		{EventType: "AuthorizationRevoked"},
		{EventType: "AuthorizationExpired"},
		{EventType: "RightsSnapshotCreated"},
		{EventType: "QualityPassed"},
		{EventType: "QualityFailed"},
		{EventType: "QualityReviewRequired"},
		{EventType: "CompliancePassed"},
		{EventType: "ComplianceFailed"},
		{EventType: "ComplianceReviewRequired"},
	}
}
