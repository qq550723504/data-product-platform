// Package routing declares the platform-wide outbox routing table.
//
// It is the single source of truth for which handlers must confirm each event
// type before the event may be marked PUBLISHED. Both composition roots use it:
// every composition root freezes the obligation on events as they are recorded;
// the dispatcher only consumes the obligation persisted on each event.
//
// The table is versioned and deterministic. An event type absent from the table
// is an error, never an implicit success; events with no external side effect
// are declared retention-only (empty RequiredHandlers) rather than relying on a
// default no-op.
package routing

import (
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
)

// Handler names are the per-handler confirmation identities recorded in
// outbox_event_consumption. A handler name must stay stable across releases:
// changing it makes every historical confirmation look unconfirmed and replays
// the handler, so rename only with a deliberate migration.
const (
	HandlerExecutionQueue           = "execution-queue"
	HandlerMetadataProjection       = "metadata-projection"
	HandlerSourceSchemaObservation  = "source-schema-observation"
	HandlerExternalPublicationQueue = "external-publication-queue"
	HandlerLifecycleRecoveryQueue   = "lifecycle-recovery-queue"
)

// Version identifies the base layout of the routing table below. Bump it
// whenever an event type's required-handler set changes. Dispatch logs and
// dead-letter diagnostics reference it, so the obligation that was in force when
// an event was handled stays auditable.
const Version = "c1-v28+ext1+exec1+usage1+rem1+life3+schema1+obj1+s1"

// VersionFor folds the deployment profile into the routing version so the same
// version string always describes the same required-handler set. Enabling or
// disabling governance projection is a new version, never a silent mutation of
// an existing one.
func VersionFor(governanceProjection bool) string {
	if governanceProjection {
		return Version + "+governance"
	}
	return Version
}

// Routes declares, for every event type the platform can emit, which handler
// obligations must be confirmed before the event may be marked PUBLISHED.
//
// governanceProjection selects the deployment profile. It is fixed for the
// lifetime of a process, so a restart with a different profile produces a
// different routing version. Events that already carry a frozen obligation keep
// it regardless of the profile a later process runs.
func Routes(governanceProjection bool) []outbox.Route {
	productReleased := []string{}
	if governanceProjection {
		productReleased = []string{HandlerMetadataProjection}
	}

	return []outbox.Route{
		{EventType: "SourceAdmissionPrepared"},
		{EventType: "SourceAdmissionAccepted"},
		{EventType: "SourceAdmissionRecorded"},
		{EventType: "SourceAdmissionRejected"},
		// Governance projection is only owed when the deployment has a
		// governance provider configured. Without one the release event is
		// explicitly retention-only, not an implicit no-op.
		{EventType: "ProductReleased", RequiredHandlers: productReleased},
		{EventType: "SourceSchemaObservationRequested", RequiredHandlers: sourceSchemaHandlers(governanceProjection)},
		{EventType: "SourceSchemaScanStarted"},
		{EventType: "SourceSchemaObserved"},
		{EventType: "SourceSchemaHandlingRecorded"},

		// New execution acceptance is delivered only through this outbox
		// obligation.
		{EventType: "ExecutionQueued", RequiredHandlers: []string{HandlerExecutionQueue}},
		{EventType: "ExecutionRetried", RequiredHandlers: []string{HandlerExecutionQueue}},

		// Retention-only: recorded for audit and traceability, with no
		// external side-effect obligation.
		{EventType: "ExecutionStarted"},
		{EventType: "ExecutionRecoveryStarted"},
		{EventType: "ExecutionDependenciesPrepared"},
		{EventType: "ExecutionSucceeded"},
		{EventType: "ExecutionFailed"},
		{EventType: "ExecutionEngineSelected"},
		{EventType: "ExecutionSubmitting"},
		{EventType: "DataResourceCreated"},
		{EventType: "ExternalIdentityBound"},
		{EventType: "ExternalIdentityInvalidated"},
		{EventType: "ExternalIdentifierRecorded"},
		{EventType: "ExternalIdentifierInvalidated"},
		{EventType: "ExternalPublicationCreated"},
		// #84 records intent only; #86 owns future dispatch obligations.
		{EventType: "ExternalPublicationRequested"},
		{EventType: "ExternalPublicationExecutionRequested", RequiredHandlers: []string{HandlerExternalPublicationQueue}},
		{EventType: "ExternalProviderCallStarted"},
		{EventType: "ExternalProviderCallDispatched"},
		{EventType: "ExternalProviderObservationRecorded"},
		{EventType: "ExternalPublicationAttemptResolved"},
		{EventType: "ExternalPublicationRegistered"},
		{EventType: "ExternalPublicationPublished"},
		// Usage pulls are durable PG work, consumed by the existing worker scan.
		{EventType: "ExternalUsageSourceCreated"},
		{EventType: "ExternalUsagePullRequested"},
		{EventType: "ExternalUsageCallStarted"},
		{EventType: "ExternalUsageCallDispatched"},
		{EventType: "ExternalUsageOutcomeRecorded"},
		{EventType: "ExternalUsageRecorded"},
		{EventType: "ExternalUsageQuarantined"},
		{EventType: "ExternalUsageAccepted"},
		{EventType: "ExternalUsageConflictQuarantined"},
		{EventType: "ExternalUsageCheckpointCommitted"},
		{EventType: "ExternalUsagePullResolved"},
		{EventType: "IngestionPrepared"},
		{EventType: "IngestionInvocationStarted"},
		{EventType: "IngestionInvocationDispatched"},
		{EventType: "IngestionInvocationOutcomeRecorded"},
		{EventType: "IngestionObserved"},
		{EventType: "IngestionAccepted"},
		{EventType: "LifecycleDispositionPlanPrepared"},
		{EventType: "LifecycleContentUseRestricted"},
		{EventType: "LifecycleContentHoldPlaced"},
		{EventType: "LifecycleContentHoldReleased"},
		{EventType: "LifecycleReaderAdmitted"},
		{EventType: "LifecycleReaderTerminated"},
		{EventType: "LifecycleProcessingOwnerStopped"},
		{EventType: "LifecycleHoldDispatchConflict"},
		{EventType: "LifecycleDispositionApproved"},
		{EventType: "LifecycleApprovalRevoked"},
		{EventType: "LifecycleDispositionDispatched"},
		{EventType: "LifecycleDispositionRecoveryRequested", RequiredHandlers: []string{HandlerLifecycleRecoveryQueue}},
		{EventType: "LifecycleDispositionRecoveryStarted"},
		{EventType: "LifecycleDispositionRecoveryObserved"},
		{EventType: "LifecycleDispositionBlocked"},
		{EventType: "LifecycleProviderAttemptObserved"},
		{EventType: "LifecycleDispositionReservationReleased"},
		{EventType: "LifecycleStorageCallStarted"},
		{EventType: "LifecycleStorageCallObserved"},
		{EventType: "ProtectedObjectIntentCreated"},
		{EventType: "ProtectedObjectVerified"},
		{EventType: "OperatorCustodyPrepared"},
		{EventType: "OperatorCustodyCallStarted"},
		{EventType: "OperatorCustodyCallObserved"},
		{EventType: "OperatorCustodyTerminated"},
		{EventType: "LifecycleFixtureDisposalBasisRegistered"},
		{EventType: "LifecycleFixtureReferenceRequired"},
		{EventType: "LifecycleFixtureReferenceReleased"},
		{EventType: "LifecycleProviderSendClaimed"},
		{EventType: "LifecycleExactCopyAbsenceVerified"},
		{EventType: "LifecycleContentDispositionFinalized"},
		{EventType: "LifecycleCopyPolicyBound"},
		{EventType: "LifecycleCopyPolicyRenewed"},
		{EventType: "LifecycleCopyPrepared"},
		{EventType: "LifecycleCopyRecoveryRequested", RequiredHandlers: []string{HandlerLifecycleRecoveryQueue}},
		{EventType: "LifecycleCopyRecoveryStarted"},
		{EventType: "LifecycleCopyRecoveryObserved"},
		{EventType: "LifecycleCopyTargetRecovered"},
		{EventType: "LifecycleCopyMaterialized"},
		{EventType: "LifecycleCopyVerified"},
		{EventType: "LifecycleRestoreCheckpointExported"},
		{EventType: "LifecycleCopyAdmissionEvaluated"},
		{EventType: "DatasetCreated"},
		{EventType: "DatasetVersionCreated"},
		{EventType: "DatasetVersionSuperseded"},
		{EventType: "DatasetVersionFailed"},
		{EventType: "DatasetVersionInvalidated"},
		{EventType: "AnnotationCampaignCreated"},
		{EventType: "AnnotationTasksCreated"},
		{EventType: "AnnotationCampaignActivated"},
		{EventType: "AnnotationResultRecorded"},
		{EventType: "AnnotationSourceObserved"},
		{EventType: "AnnotationSubmissionBatchPrepared"},
		{EventType: "AnnotationSubmissionBatchObserved"},
		{EventType: "AnnotationReviewAttemptStarted"},
		{EventType: "AnnotationReviewAttemptFailed"},
		{EventType: "AnnotationReviewed"},
		{EventType: "AnnotationSnapshotFinalized"},
		{EventType: "GoldProductionBindingFinalized"},
		{EventType: "AnnotationEngineOperationPrepared"},
		{EventType: "AnnotationEngineAttemptStarted"},
		{EventType: "AnnotationEngineOperationResolved"},
		{EventType: "AnnotationEngineManualResolutionRequired"},
		{EventType: "AnnotationEngineManualResolutionApplied"},
		{EventType: "AnnotationEngineActorBound"},
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
		{EventType: "RightsDeclarationCreated"},
		{EventType: "RightsDeclarationVerified"},
		{EventType: "RightsDeclarationRejected"},
		{EventType: "RightsDeclarationInvalidated"},
		{EventType: "RightsDeclarationSuperseded"},
		{EventType: "AuthorizationProvenanceBound"},
		{EventType: "AuthorizationProvenanceBindingInvalidated"},
		{EventType: "AuthorizationProvenanceBindingSuperseded"},
		{EventType: "GrantorAuthorityDelegationChainCreated"},
		{EventType: "GrantorAuthorityDelegationChainFinalized"},
		{EventType: "GrantorAuthorityDelegationInvalidated"},
		{EventType: "GrantorAuthorityDelegationRevoked"},
		{EventType: "GrantorAuthorityDelegationSuperseded"},
		{EventType: "EffectiveRightsCalculated"},
		{EventType: "EffectiveRightsFinalized"},
		{EventType: "DatasetCertified"},
		{EventType: "DatasetCertificationInputsPrepared"},
		{EventType: "DatasetCertificationRejected"},
		{EventType: "DatasetCertificationRevoked"},
		{EventType: "DatasetCertificationSuperseded"},
		{EventType: "CertificationProfileCreated"},
		{EventType: "QualityAssessmentAttemptFailed"},
		{EventType: "QualityRemediationDetected"},
		{EventType: "QualityRemediationAssigned"},
		{EventType: "QualityRemediationExecutionAccepted"},
		{EventType: "QualityRemediationRecheckStarted"},
		{EventType: "QualityRemediationRechecked"},
		{EventType: "QualityRemediationManuallyClosed"},
		{EventType: "QualityPassed"},
		{EventType: "QualityFailed"},
		{EventType: "QualityReviewRequired"},
		{EventType: "CompliancePassed"},
		{EventType: "ComplianceFailed"},
		{EventType: "ComplianceReviewRequired"},
		{EventType: "DatasetDeliveryGateEvaluated"},
		{EventType: "DatasetDeliveryIssuancePending"},
		{EventType: "DatasetDeliveryProviderAttemptStarted"},
		{EventType: "DatasetDeliveryProviderObservationRecorded"},
		{EventType: "DatasetCredentialReplayDecision"},
		{EventType: "DatasetCredentialReplayContainmentPending"},
		{EventType: "DatasetDeliveryContainmentPending"},
		{EventType: "DatasetDeliveryContainmentResolved"},
		{EventType: "DatasetDeliveryIssued"},
		{EventType: "DatasetDeliveryBlocked"},
		{EventType: "DatasetDeliveryFailed"},
	}
}

// NewRouter builds the validated routing table for a deployment profile.
func NewRouter(governanceProjection bool) (*outbox.Router, error) {
	return outbox.NewRouter(VersionFor(governanceProjection), Routes(governanceProjection))
}

func sourceSchemaHandlers(enabled bool) []string {
	if enabled {
		return []string{HandlerSourceSchemaObservation}
	}
	return []string{}
}
