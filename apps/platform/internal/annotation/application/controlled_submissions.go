package application

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"sort"
	"time"
)

type PrepareSubmissionBatchCommand struct {
	WorkspaceID uuid.UUID
	CampaignID  uuid.UUID
	Expected    []annotationdomain.SubmissionExpectation
	PageBudget  int
	// Trusted operator/orchestration attestation, never accepted from a public
	// request field. This protocol only enumerates a stopped finite batch.
	Quiescent bool
	ActorID   *uuid.UUID
}

func (s *Service) ValidateSourceCollectionTx(ctx context.Context, tx pgx.Tx, campaignID uuid.UUID) error {
	campaign, err := s.repo.GetCampaignTx(ctx, tx, campaignID)
	if err != nil {
		return err
	}
	if _, err := deliveryfence.Lock(ctx, tx, campaign.WorkspaceID); err != nil {
		return err
	}
	campaign, err = s.repo.LockCampaignTx(ctx, tx, campaignID)
	if err != nil {
		return err
	}
	guard, ok := s.activationGuard.(EngineSendAuthorizationGuard)
	if !ok || guard == nil {
		return ErrActivationGuardRequired
	}
	return guard.ValidateEngineSendTx(ctx, tx, campaign)
}
func (s *Service) PrepareControlledSubmissionBatch(ctx context.Context, cmd PrepareSubmissionBatchCommand) error {
	if !cmd.Quiescent || len(cmd.Expected) == 0 || cmd.PageBudget < 1 || cmd.PageBudget > 1000 {
		return annotationdomain.ErrSourceIntegrity
	}
	return s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := deliveryfence.Lock(ctx, tx, cmd.WorkspaceID); err != nil {
			return err
		}
		campaign, err := s.repo.LockCampaignTx(ctx, tx, cmd.CampaignID)
		if err != nil {
			return err
		}
		if campaign.WorkspaceID != cmd.WorkspaceID || campaign.Status != annotationdomain.CampaignActive {
			return annotationdomain.ErrSourceIntegrity
		}
		if err := s.ValidateSourceCollectionTx(ctx, tx, cmd.CampaignID); err != nil {
			return err
		}
		created, err := s.repo.PrepareSubmissionBatchTx(ctx, tx, cmd.CampaignID, cmd.WorkspaceID, cmd.Expected, cmd.PageBudget)
		if err != nil {
			return err
		}
		if !created {
			return nil
		}
		metadata := map[string]any{"campaignId": cmd.CampaignID, "expectedCount": len(cmd.Expected), "pageBudget": cmd.PageBudget}
		if err := appendEvent(ctx, tx, "ANNOTATION_CAMPAIGN", cmd.CampaignID, "AnnotationSubmissionBatchPrepared", metadata); err != nil {
			return err
		}
		if _, err := evidence.Append(ctx, tx, evidence.Record{WorkspaceID: cmd.WorkspaceID, EvidenceType: "ANNOTATION_SUBMISSION_BATCH_PREPARED",
			Title: "Finite quiescent Submission batch prepared", SourceType: "CORE", Metadata: metadata, CreatedBy: cmd.ActorID},
			evidence.Relation{ObjectType: "ANNOTATION_CAMPAIGN", ObjectID: cmd.CampaignID, RelationType: "SUBMISSION_BATCH"}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{WorkspaceID: &cmd.WorkspaceID, ActorType: actorType(cmd.ActorID), ActorID: cmd.ActorID,
			Action: "ANNOTATION_SUBMISSION_BATCH_PREPARED", ObjectType: "ANNOTATION_CAMPAIGN", ObjectID: cmd.CampaignID, AfterState: metadata})
	})
}
func (r *EngineResultReconciler) reconcileControlled(ctx context.Context, campaign annotationdomain.Campaign, binding annotationdomain.EngineCampaignBinding) (retErr error) {
	if campaign.Status != annotationdomain.CampaignActive && campaign.Status != annotationdomain.CampaignSealed {
		return annotationdomain.ErrInvalidCampaign
	}
	if binding.Provider != r.engine.Provider() || binding.ProviderInstance != r.engine.InstanceRef() {
		return annotationdomain.ErrSourceIntegrity
	}
	expected, budget, err := r.repo.SubmissionBatch(ctx, campaign.ID)
	if err != nil {
		return err
	}
	operation, err := r.repo.GetMatchedTaskSubmissionOperation(ctx, campaign.ID, binding.Provider, binding.ProviderInstance)
	if err != nil {
		return err
	}
	manifest, err := decodeTasksManifest(operation)
	if err != nil {
		return err
	}
	request := engineLookupRequest(engineSubmitRequest(operation, manifest))
	request.Binding.SourceContract = binding.SourceContract
	scanID, err := r.batchReceipt(ctx, campaign, false, uuid.Nil, nil)
	if err != nil {
		return err
	}
	defer func() {
		if retErr != nil {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_, receiptErr := r.batchReceipt(cleanupCtx, campaign, false, uuid.Nil, nil)
			retErr = errors.Join(retErr, receiptErr)
		}
	}()
	taskBindings, err := r.repo.ListEngineTaskBindings(ctx, campaign.ID)
	if err != nil {
		return err
	}
	tasks, err := r.repo.ListTasks(ctx, campaign.ID)
	if err != nil {
		return err
	}
	byTask := map[uuid.UUID]annotationdomain.Task{}
	byBinding := map[uuid.UUID]annotationdomain.EngineTaskBinding{}
	for _, t := range tasks {
		byTask[t.ID] = t
	}
	for _, b := range taskBindings {
		byBinding[b.TaskID] = b
	}
	collect := func() (map[uuid.UUID]annotationdomain.SourceObservation, error) {
		found := map[uuid.UUID]annotationdomain.SourceObservation{}
		cursor := EngineResultCursor{}
		for pageNo := 0; pageNo < budget; pageNo++ {
			observer := &EngineInvocationObserver{
				Start: func(ctx context.Context) (annotationdomain.EngineAttempt, error) {
					return r.startFetchAttempt(ctx, operation)
				},
				Finish: func(ctx context.Context, attempt annotationdomain.EngineAttempt, remoteErr error) error {
					observationCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
					defer cancel()
					return r.finishFetchAttempt(observationCtx, operation, attempt, remoteErr)
				},
			}
			page, fetchErr := r.engine.FetchResults(WithEngineInvocationObserver(ctx, observer), request, cursor)
			if fetchErr != nil {
				return nil, fetchErr
			}
			for _, o := range page.Results {
				task, ok := byTask[o.TaskID]
				taskBinding, bound := byBinding[o.TaskID]
				if !ok || !bound || o.Source == nil || !o.ProviderSubmitted || o.ProviderCancelled {
					return nil, annotationdomain.ErrSourceIntegrity
				}
				actor, err := r.repo.ResolveEngineActorBinding(ctx, campaign.WorkspaceID, binding.Provider, binding.ProviderInstance, o.ExternalAuthorRef)
				if err != nil {
					return nil, err
				}
				f := annotationdomain.SourceFingerprint{WorkspaceID: campaign.WorkspaceID, ConnectionID: binding.ConnectionID,
					ProviderInstance: binding.ProviderInstance, ProviderIncarnation: binding.ProviderIncarnation,
					SourceKind: annotationdomain.ImmutableSubmissionSource, ExternalID: o.Source.ExternalID, AssignmentID: o.Source.AssignmentID,
					SubmissionRevision: o.Source.Revision, ExternalProjectID: o.Source.ExternalProjectID, ExternalTaskID: o.ExternalTaskID,
					ExternalAnnotationID: o.ExternalAnnotationID, ExternalAuthorRef: o.ExternalAuthorRef, CoreAuthorRef: actor.CoreActorRef,
					CampaignID: campaign.ID, TaskID: task.ID, InputVersionID: campaign.InputDatasetVersionID, InputSHA256: campaign.InputChecksumSHA256,
					SourceItemRef: task.SourceItemRef, SourceSHA256: task.SourceContentSHA256, TaskTextSHA256: task.TaskTextSHA256,
					CampaignBindingID: binding.ID, TaskBindingID: taskBinding.ID, ActorBindingID: actor.ID,
					ConfigSHA256: binding.ConfigSHA256, SchemaSHA256: campaign.Schema.ContentSHA256, TaxonomySHA256: campaign.Taxonomy.ContentSHA256,
					RubricSHA256: campaign.Rubric.ContentSHA256, RendererSHA256: campaign.Renderer.ContentSHA256, ReviewPolicySHA256: campaign.ReviewPolicy.ContentSHA256,
					NormalizerVersion: o.NormalizerVersion, SnapshotSHA256: o.Source.SnapshotSHA256, CanonicalPayloadSHA256: o.CanonicalPayloadSHA256}
				f.MappingSHA256 = f.MappingHash()
				source := annotationdomain.SourceObservation{ID: f.Identity(), SourceFingerprint: f, Snapshot: o.Source.Snapshot, CanonicalPayload: o.CanonicalPayload}
				if err := source.Validate(); err != nil {
					return nil, err
				}
				if taskBinding.ExternalTaskID != o.ExternalTaskID || actor.CoreActorRef != task.PrimaryAnnotatorRef ||
					taskBinding.CampaignBindingID != binding.ID {
					return nil, annotationdomain.ErrSourceIntegrity
				}
				disposition := "OBSERVED"
				if task.Status == annotationdomain.TaskReviewed || campaign.Status == annotationdomain.CampaignSealed {
					disposition = "LATE"
				}
				if o.Quarantined {
					disposition = "QUARANTINED"
				}
				if err := r.sourceReceipt(ctx, source, o.Source.PhysicalAttemptID, disposition); err != nil {
					return nil, err
				}
				if o.Quarantined {
					return nil, fmt.Errorf("fork human review mixed into Core-only-review workflow")
				}
				if _, duplicate := found[source.ID]; duplicate {
					return nil, annotationdomain.ErrSourceConflict
				}
				found[source.ID] = source
			}
			if page.NextCursor == nil {
				return found, nil
			}
			if page.NextCursor.Offset <= cursor.Offset {
				return nil, annotationdomain.ErrSourceIntegrity
			}
			cursor = *page.NextCursor
		}
		return nil, fmt.Errorf("controlled Submission page budget exhausted")
	}
	first, err := collect()
	if err != nil {
		return err
	}
	if err := verifyExpectedSources(expected, first); err != nil {
		return err
	}
	second, err := collect()
	if err != nil {
		return err
	}
	if len(first) != len(second) {
		return annotationdomain.ErrSourceConflict
	}
	for id, source := range first {
		other, ok := second[id]
		if !ok || other.Hash() != source.Hash() {
			return annotationdomain.ErrSourceConflict
		}
	}
	sources := make([]annotationdomain.SourceObservation, 0, len(second))
	for _, source := range second {
		sources = append(sources, source)
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].ID.String() < sources[j].ID.String() })
	for _, source := range sources {
		task, err := r.repo.GetTask(ctx, source.TaskID)
		if err != nil {
			return err
		}
		if task.Status == annotationdomain.TaskReviewed || campaign.Status == annotationdomain.CampaignSealed {
			continue
		}
		if _, err := r.recorder.RecordAnnotationResult(ctx, RecordResultCommand{WorkspaceID: source.WorkspaceID, CampaignID: source.CampaignID,
			TaskID: source.TaskID, ExpectedTaskRevision: task.Revision, AuthorRef: source.CoreAuthorRef, ProviderBindingRef: source.CampaignBindingID.String(),
			ExternalTaskID: source.ExternalTaskID, ExternalAnnotationID: source.ExternalAnnotationID, ExternalRevision: source.ExternalRevision(),
			ObservationKey: "immutable-source:" + source.ID.String() + ":" + source.Hash(), CanonicalPayload: source.CanonicalPayload,
			CanonicalPayloadSHA256: source.CanonicalPayloadSHA256, NormalizerVersion: source.NormalizerVersion, Source: &source}); err != nil {
			return err
		}
	}
	_, err = r.batchReceipt(ctx, campaign, true, scanID, sources)
	return err
}
func verifyExpectedSources(expected []annotationdomain.SubmissionExpectation, found map[uuid.UUID]annotationdomain.SourceObservation) error {
	if len(expected) != len(found) {
		return annotationdomain.ErrSourceIntegrity
	}
	keys := map[string]bool{}
	for _, e := range expected {
		key := fmt.Sprintf("%s/%s/%d", e.TaskID, e.AssignmentID, e.Revision)
		if keys[key] {
			return annotationdomain.ErrSourceConflict
		}
		keys[key] = true
	}
	for _, s := range found {
		key := fmt.Sprintf("%s/%s/%d", s.TaskID, s.AssignmentID, s.SubmissionRevision)
		if !keys[key] {
			return annotationdomain.ErrSourceConflict
		}
		delete(keys, key)
	}
	if len(keys) != 0 {
		return annotationdomain.ErrSourceIntegrity
	}
	return nil
}
func (r *EngineResultReconciler) sourceReceipt(ctx context.Context, source annotationdomain.SourceObservation, attemptID uuid.UUID, disposition string) error {
	conflict := false
	err := r.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		receipt, err := r.repo.AppendSourceReceiptTx(ctx, tx, source, attemptID, disposition)
		if err != nil {
			return err
		}
		conflict = receipt.Disposition == "CONFLICT"
		if !receipt.Created {
			return nil
		}
		metadata := map[string]any{"incomingSourceId": source.ID, "fingerprint": source.Hash(), "attemptId": attemptID, "disposition": receipt.Disposition}
		if err := appendEvent(ctx, tx, "ANNOTATION_TASK", source.TaskID, "AnnotationSourceObserved", metadata); err != nil {
			return err
		}
		if _, err := evidence.Append(ctx, tx, evidence.Record{WorkspaceID: source.WorkspaceID, EvidenceType: "ANNOTATION_SOURCE_OBSERVED", Title: "Immutable annotation source observed", SourceType: "ENGINE_ADAPTER", Metadata: metadata},
			evidence.Relation{ObjectType: "ANNOTATION_SOURCE_RECEIPT", ObjectID: receipt.ID, RelationType: "SOURCE_OBSERVATION"}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{WorkspaceID: &source.WorkspaceID, ActorType: "SYSTEM", Action: "ANNOTATION_SOURCE_OBSERVED", ObjectType: "ANNOTATION_SOURCE_RECEIPT", ObjectID: receipt.ID, AfterState: metadata})
	})
	if err != nil {
		return err
	}
	if conflict {
		return annotationdomain.ErrSourceConflict
	}
	return nil
}
func (r *EngineResultReconciler) batchReceipt(ctx context.Context, campaign annotationdomain.Campaign, complete bool, scanID uuid.UUID, sources []annotationdomain.SourceObservation) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		id, err = r.repo.AppendBatchReceiptTx(ctx, tx, campaign.ID, complete, scanID, sources)
		if err != nil {
			return err
		}
		metadata := map[string]any{"campaignId": campaign.ID, "receiptId": id, "complete": complete, "sourceCount": len(sources), "observedAt": time.Now().UTC()}
		if err := appendEvent(ctx, tx, "ANNOTATION_CAMPAIGN", campaign.ID, "AnnotationSubmissionBatchObserved", metadata); err != nil {
			return err
		}
		if _, err := evidence.Append(ctx, tx, evidence.Record{WorkspaceID: campaign.WorkspaceID, EvidenceType: "ANNOTATION_SUBMISSION_BATCH_OBSERVED", Title: "Finite Submission batch observed", SourceType: "ENGINE_ADAPTER", Metadata: metadata},
			evidence.Relation{ObjectType: "ANNOTATION_SUBMISSION_BATCH_RECEIPT", ObjectID: id, RelationType: "SUBMISSION_BATCH"}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{WorkspaceID: &campaign.WorkspaceID, ActorType: "SYSTEM", Action: "ANNOTATION_SUBMISSION_BATCH_OBSERVED", ObjectType: "ANNOTATION_CAMPAIGN", ObjectID: campaign.ID, AfterState: metadata})
	})
	return id, err
}
