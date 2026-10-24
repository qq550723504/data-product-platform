package routing

import (
	"sort"
	"strings"
	"testing"
)

func TestVersionFitsPersistedOutboxContract(t *testing.T) {
	// outbox_event.required_handlers_version is varchar(64). Both profile
	// identities must fit before any event is recorded under that profile.
	for _, governance := range []bool{false, true} {
		if got := len(VersionFor(governance)); got == 0 || got > 64 {
			t.Fatalf("governance=%t routing version has %d bytes, want 1..64", governance, got)
		}
	}
	if VersionFor(false) == VersionFor(true) {
		t.Fatal("governance profiles must have distinct routing identities")
	}
}

// eventVocabulary is the second witness to the routing table: every event type
// the platform can emit must appear exactly once. Adding a domain event without
// declaring its routing obligation fails this test, and an undeclared event
// fails dispatch at runtime with a diagnosable error instead of being completed
// implicitly.
var eventVocabulary = []string{
	"AgentTaskCreated", "AgentPlanRevised", "AgentPlanApproved", "AgentTaskPaused", "AgentTaskResumed", "AgentTaskStopped", "AgentActionRequested",
	"SourceAdmissionPrepared",
	"SourceAdmissionAccepted",
	"SourceAdmissionRecorded",
	"SourceAdmissionRejected",
	"OperatorCustodyPrepared",
	"OperatorCustodyCallStarted",
	"OperatorCustodyCallObserved",
	"OperatorCustodyTerminated",
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
	"AnnotationEngineActorBound",
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
	"DatasetCertificationInputsPrepared",
	"DatasetCertificationRejected",
	"DatasetCertificationRevoked",
	"DatasetCertificationSuperseded",
	"DataResourceCreated",
	"ExternalIdentityBound",
	"ExternalIdentityInvalidated",
	"ExternalIdentifierRecorded",
	"ExternalIdentifierInvalidated",
	"ExternalPublicationCreated",
	"ExternalPublicationRequested",
	"ExternalPublicationExecutionRequested",
	"ExternalProviderCallStarted",
	"ExternalProviderCallDispatched",
	"ExternalProviderObservationRecorded",
	"ExternalPublicationAttemptResolved",
	"ExternalPublicationRegistered",
	"ExternalPublicationPublished",
	"ExternalUsageSourceCreated",
	"ExternalUsagePullRequested",
	"ExternalUsageCallStarted",
	"ExternalUsageCallDispatched",
	"ExternalUsageOutcomeRecorded",
	"ExternalUsageRecorded",
	"ExternalUsageQuarantined",
	"ExternalUsageAccepted",
	"ExternalUsageConflictQuarantined",
	"ExternalUsageCheckpointCommitted",
	"ExternalUsagePullResolved",
	"IngestionPrepared",
	"IngestionInvocationStarted",
	"IngestionInvocationDispatched",
	"IngestionInvocationOutcomeRecorded",
	"IngestionObserved",
	"IngestionAccepted",
	"LifecycleDispositionPlanPrepared",
	"LifecycleContentUseRestricted",
	"LifecycleContentHoldPlaced",
	"LifecycleContentHoldReleased",
	"LifecycleReaderAdmitted",
	"LifecycleReaderTerminated",
	"LifecycleProcessingOwnerStopped",
	"LifecycleHoldDispatchConflict",
	"LifecycleDispositionApproved",
	"LifecycleApprovalRevoked",
	"LifecycleDispositionDispatched",
	"LifecycleDispositionRecoveryRequested",
	"LifecycleDispositionRecoveryStarted",
	"LifecycleDispositionRecoveryObserved",
	"LifecycleDispositionBlocked",
	"LifecycleProviderAttemptObserved",
	"LifecycleStorageCallStarted",
	"LifecycleStorageCallObserved",
	"ProtectedObjectIntentCreated",
	"ProtectedObjectVerified",
	"LifecycleFixtureDisposalBasisRegistered",
	"LifecycleFixtureReferenceRequired",
	"LifecycleFixtureReferenceReleased",
	"LifecycleProviderSendClaimed",
	"LifecycleExactCopyAbsenceVerified",
	"LifecycleContentDispositionFinalized",
	"LifecycleCopyPolicyBound",
	"LifecycleCopyPolicyRenewed",
	"LifecycleCopyPrepared",
	"LifecycleCopyRecoveryRequested",
	"LifecycleCopyRecoveryStarted",
	"LifecycleCopyRecoveryObserved",
	"LifecycleCopyTargetRecovered",
	"LifecycleCopyMaterialized",
	"LifecycleCopyVerified",
	"LifecycleRestoreCheckpointExported",
	"LifecycleCopyAdmissionEvaluated",
	"LifecycleDispositionReservationReleased",
	"DatasetCreated",
	"DatasetVersionCreated",
	"DatasetVersionSuperseded",
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
	"GoldProductionBindingFinalized",
	"ProductReleaseDraftCreated",
	"ProductReleased",
	"ProductReleaseReady",
	"ProductReleaseValidationStarted",
	"ProductVersionCreated",
	"QualityFailed",
	"QualityAssessmentAttemptFailed",
	"QualityRemediationDetected",
	"QualityRemediationAssigned",
	"QualityRemediationExecutionAccepted",
	"QualityRemediationRecheckStarted",
	"QualityRemediationRechecked",
	"QualityRemediationManuallyClosed",
	"QualityPassed",
	"SourceSchemaObservationRequested",
	"SourceSchemaScanStarted",
	"SourceSchemaObserved",
	"SourceSchemaHandlingRecorded",
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

func TestWorkerProfileDoesNotAcceptOrEraseFutureAgentObligations(t *testing.T) {
	for _, governance := range []bool{false, true} {
		producer, err := NewRouter(governance)
		if err != nil {
			t.Fatal(err)
		}
		worker, err := NewWorkerRouter(governance)
		if err != nil {
			t.Fatal(err)
		}
		if worker.Version() == producer.Version() || len(worker.Version()) > 64 {
			t.Fatal("consumer profile must have a distinct bounded identity")
		}
		required, ok := producer.RequiredHandlers("AgentActionRequested")
		if !ok || len(required) != 1 || required[0] != "agent-action-dispatch" {
			t.Fatal("Agent producer obligation erased", required)
		}
		if _, ok = worker.RequiredHandlers("AgentActionRequested"); ok {
			t.Fatal("unimplemented Agent route must not become retention-only")
		}
		for _, event := range producer.EventTypes() {
			if event == "AgentActionRequested" {
				continue
			}
			a, _ := producer.RequiredHandlers(event)
			b, ok := worker.RequiredHandlers(event)
			if !ok || strings.Join(a, "|") != strings.Join(b, "|") {
				t.Fatal("unrelated worker route changed", event)
			}
		}
	}
}
