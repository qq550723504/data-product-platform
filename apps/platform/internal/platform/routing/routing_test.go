package routing

import (
	"sort"
	"testing"
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
	"AuthorizationProvenanceBindingInvalidated",
	"AuthorizationProvenanceBindingSuperseded",
	"AuthorizationProvenanceBound",
	"AuthorizationRevoked",
	"AuthorizationSubmitted",
	"AuthorizationSuspended",
	"AnnotationCampaignActivated",
	"AnnotationCampaignCreated",
	"AnnotationEngineAttemptStarted",
	"AnnotationEngineManualResolutionApplied",
	"AnnotationEngineManualResolutionRequired",
	"AnnotationEngineOperationPrepared",
	"AnnotationEngineOperationResolved",
	"AnnotationResultRecorded",
	"AnnotationReviewAttemptFailed",
	"AnnotationReviewAttemptStarted",
	"AnnotationReviewed",
	"AnnotationSnapshotFinalized",
	"AnnotationTasksCreated",
	"ComplianceFailed",
	"CompliancePassed",
	"ComplianceReviewRequired",
	"DatasetCredentialReplayContainmentPending",
	"DatasetCredentialReplayDecision",
	"DatasetDeliveryBlocked",
	"DatasetDeliveryContainmentPending",
	"DatasetDeliveryContainmentResolved",
	"DatasetDeliveryFailed",
	"DatasetDeliveryGateEvaluated",
	"DatasetDeliveryIssued",
	"DatasetDeliveryIssuancePending",
	"DatasetDeliveryProviderAttemptStarted",
	"DatasetDeliveryProviderObservationRecorded",
	"ContractPublished",
	"ContractVersionCreated",
	"CertificationProfileCreated",
	"DataProductCreated",
	"DatasetCertified",
	"DatasetCertificationRejected",
	"DatasetCertificationRevoked",
	"DatasetCertificationSuperseded",
	"DataResourceCreated",
	"DatasetCreated",
	"DatasetVersionCreated",
	"DatasetVersionFailed",
	"DatasetVersionInvalidated",
	"EntityMappingDecisionRecorded",
	"EntityMatchCompleted",
	"ExecutionEngineSelected",
	"ExecutionDependenciesPrepared",
	"ExecutionFailed",
	"ExecutionQueued",
	"ExecutionRecoveryStarted",
	"ExecutionRetried",
	"ExecutionStarted",
	"ExecutionSubmitting",
	"ExecutionSucceeded",
	"EffectiveRightsCalculated",
	"EffectiveRightsFinalized",
	"GrantorAuthorityDelegationChainCreated",
	"GrantorAuthorityDelegationChainFinalized",
	"GrantorAuthorityDelegationInvalidated",
	"GrantorAuthorityDelegationRevoked",
	"GrantorAuthorityDelegationSuperseded",
	"ProductReleaseDraftCreated",
	"ProductReleased",
	"ProductReleaseReady",
	"ProductReleaseValidationStarted",
	"ProductVersionCreated",
	"QualityFailed",
	"QualityAssessmentAttemptFailed",
	"QualityPassed",
	"QualityReviewRequired",
	"RightsSnapshotCreated",
	"RightsDeclarationCreated",
	"RightsDeclarationVerified",
	"RightsDeclarationRejected",
	"RightsDeclarationInvalidated",
	"RightsDeclarationSuperseded",
	"WorkflowVersionCreated",
}

func TestRoutesDeclareEveryEventTypeExactlyOnce(t *testing.T) {
	if Version == "" {
		t.Fatal("routing version must be set")
	}
	router, err := NewRouter(true)
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

func TestRoutesResolveGovernanceProjectionProfile(t *testing.T) {
	if VersionFor(true) == VersionFor(false) {
		t.Fatal("the governance profile must be encoded in the routing version")
	}
	withProjection, err := NewRouter(true)
	if err != nil {
		t.Fatalf("NewRouter(projection on): %v", err)
	}
	required, ok := withProjection.RequiredHandlers("ProductReleased")
	if !ok {
		t.Fatal("ProductReleased must be declared")
	}
	if len(required) != 1 || required[0] != HandlerMetadataProjection {
		t.Fatalf("ProductReleased required handlers = %v, want [%s]", required, HandlerMetadataProjection)
	}

	withoutProjection, err := NewRouter(false)
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

func TestRoutesRequireExecutionQueueForNewDispatchEvents(t *testing.T) {
	router, err := NewRouter(true)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	for _, eventType := range []string{"ExecutionQueued", "ExecutionRetried"} {
		required, ok := router.RequiredHandlers(eventType)
		if !ok {
			t.Fatalf("%s must be declared even while it is retention-only", eventType)
		}
		if len(required) != 1 || required[0] != HandlerExecutionQueue {
			t.Fatalf("%s required handlers = %v, want [%s]", eventType, required, HandlerExecutionQueue)
		}
	}
}

// NewRouter must be usable as an ObligationSource without a live database, so
// the API can freeze the obligation on events as it records them.
func TestRouterImplementsObligationSource(t *testing.T) {
	governance, err := NewRouter(true)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	version, handlers, ok := governance.Obligation("ProductReleased")
	if !ok {
		t.Fatal("ProductReleased must be declared")
	}
	if version != VersionFor(true) {
		t.Fatalf("obligation version = %q, want %q", version, VersionFor(true))
	}
	if len(handlers) != 1 || handlers[0] != HandlerMetadataProjection {
		t.Fatalf("obligation handlers = %v, want [%s]", handlers, HandlerMetadataProjection)
	}

	if _, _, ok := governance.Obligation("NotADeclaredEvent"); ok {
		t.Fatal("an undeclared event type must not produce an obligation")
	}

	retention, err := NewRouter(false)
	if err != nil {
		t.Fatalf("NewRouter(false): %v", err)
	}
	version, handlers, ok = retention.Obligation("ProductReleased")
	if !ok || version != VersionFor(false) || len(handlers) != 0 {
		t.Fatalf("retention obligation = (%q, %v, %v), want (%q, [], true)", version, handlers, ok, VersionFor(false))
	}
	if handlers == nil {
		t.Fatal("a retention-only obligation must be a non-nil empty handler set, not nil")
	}
}
