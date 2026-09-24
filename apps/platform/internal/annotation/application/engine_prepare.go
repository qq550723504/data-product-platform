package application

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	annotationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
)

func (s *EngineService) PrepareCampaign(
	ctx context.Context,
	cmd PrepareEngineCampaignCommand,
) (annotationdomain.EngineOperation, error) {
	if err := s.configured(); err != nil {
		return annotationdomain.EngineOperation{}, err
	}
	cmd.RequestID = strings.TrimSpace(cmd.RequestID)
	cmd.Title = strings.TrimSpace(cmd.Title)
	if cmd.WorkspaceID == uuid.Nil || cmd.CampaignID == uuid.Nil || cmd.RequestID == "" || cmd.Title == "" {
		return annotationdomain.EngineOperation{}, annotationdomain.ErrInvalidEngineOperation
	}

	campaign, err := s.repo.GetCampaign(ctx, cmd.CampaignID)
	if err != nil {
		return annotationdomain.EngineOperation{}, err
	}
	if campaign.WorkspaceID != cmd.WorkspaceID || campaign.Status != annotationdomain.CampaignActive {
		return annotationdomain.EngineOperation{}, annotationdomain.ErrInvalidCampaign
	}

	manifest := engineCampaignManifest{
		Kind:          annotationdomain.EngineOperationEnsureCampaign,
		WorkspaceID:   cmd.WorkspaceID.String(),
		CampaignID:    cmd.CampaignID.String(),
		Title:         cmd.Title,
		SchemaContent: campaign.Schema.ContentSnapshot,
		SchemaSHA256:  campaign.Schema.ContentSHA256,
	}
	return s.prepareOperation(ctx, campaign, cmd.RequestID, manifest, cmd.ActorID, cmd.TraceID)
}

func (s *EngineService) PrepareTasks(
	ctx context.Context,
	cmd PrepareEngineTasksCommand,
) (annotationdomain.EngineOperation, error) {
	if err := s.configured(); err != nil {
		return annotationdomain.EngineOperation{}, err
	}
	cmd.RequestID = strings.TrimSpace(cmd.RequestID)
	if cmd.WorkspaceID == uuid.Nil || cmd.CampaignID == uuid.Nil || cmd.RequestID == "" || len(cmd.Tasks) == 0 {
		return annotationdomain.EngineOperation{}, annotationdomain.ErrInvalidEngineOperation
	}

	campaign, err := s.repo.GetCampaign(ctx, cmd.CampaignID)
	if err != nil {
		return annotationdomain.EngineOperation{}, err
	}
	if campaign.WorkspaceID != cmd.WorkspaceID || campaign.Status != annotationdomain.CampaignActive {
		return annotationdomain.EngineOperation{}, annotationdomain.ErrInvalidCampaign
	}

	binding, err := s.repo.GetEngineCampaignBinding(ctx, cmd.CampaignID)
	if err != nil {
		return annotationdomain.EngineOperation{}, fmt.Errorf("annotation engine campaign is not bound: %w", err)
	}
	if binding.Provider != s.engine.Provider() || binding.ProviderInstance != s.engine.InstanceRef() {
		return annotationdomain.EngineOperation{}, annotationinfra.ErrEngineBindingConflict
	}

	coreTasks, err := s.repo.ListTasks(ctx, cmd.CampaignID)
	if err != nil {
		return annotationdomain.EngineOperation{}, err
	}
	coreByID := make(map[uuid.UUID]annotationdomain.Task, len(coreTasks))
	for _, task := range coreTasks {
		coreByID[task.ID] = task
	}

	tasks := append([]EngineTask(nil), cmd.Tasks...)
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].TaskID.String() < tasks[j].TaskID.String()
	})
	seen := make(map[uuid.UUID]struct{}, len(tasks))
	for i := range tasks {
		task := &tasks[i]
		coreTask, ok := coreByID[task.TaskID]
		if !ok || coreTask.WorkspaceID != cmd.WorkspaceID || coreTask.CampaignID != cmd.CampaignID {
			return annotationdomain.EngineOperation{}, annotationdomain.ErrInvalidTask
		}
		if _, exists := seen[task.TaskID]; exists {
			return annotationdomain.EngineOperation{}, annotationdomain.ErrInvalidTask
		}
		seen[task.TaskID] = struct{}{}

		task.SourceItemRef = strings.TrimSpace(task.SourceItemRef)
		task.SourceSHA256 = strings.TrimSpace(task.SourceSHA256)
		task.TaskTextSHA256 = strings.TrimSpace(task.TaskTextSHA256)
		task.CorrelationKey = strings.TrimSpace(task.CorrelationKey)

		if task.SourceItemRef != coreTask.SourceItemRef ||
			task.SourceSHA256 != coreTask.SourceContentSHA256 ||
			task.TaskTextSHA256 != coreTask.TaskTextSHA256 ||
			hashBytes([]byte(task.TaskText)) != coreTask.TaskTextSHA256 ||
			strings.TrimSpace(task.TaskText) == "" ||
			task.CorrelationKey == "" {
			return annotationdomain.EngineOperation{}, annotationdomain.ErrInvalidTask
		}
	}
	if len(tasks) != len(coreTasks) {
		return annotationdomain.EngineOperation{}, fmt.Errorf("annotation engine submission must cover the full campaign task set")
	}

	manifest := engineTasksManifest{
		Kind:        annotationdomain.EngineOperationSubmitTasks,
		WorkspaceID: cmd.WorkspaceID.String(),
		CampaignID:  cmd.CampaignID.String(),
		Binding: engineBinding{
			Provider:          binding.Provider,
			ProviderInstance:  binding.ProviderInstance,
			ExternalProjectID: binding.ExternalProjectID,
			RequestID:         binding.RequestID,
			ConfigSHA256:      binding.ConfigSHA256,
		},
		Tasks: tasks,
	}
	return s.prepareOperation(ctx, campaign, cmd.RequestID, manifest, cmd.ActorID, cmd.TraceID)
}

func (s *EngineService) prepareOperation(
	ctx context.Context,
	campaign annotationdomain.Campaign,
	requestID string,
	manifest any,
	actorID *uuid.UUID,
	traceID string,
) (annotationdomain.EngineOperation, error) {
	payload, err := json.Marshal(manifest)
	if err != nil {
		return annotationdomain.EngineOperation{}, err
	}
	kind := annotationdomain.EngineOperationEnsureCampaign
	if _, ok := manifest.(engineTasksManifest); ok {
		kind = annotationdomain.EngineOperationSubmitTasks
	}
	now := time.Now().UTC()
	operation := annotationdomain.EngineOperation{
		ID: uuid.NewSHA1(
			uuid.NameSpaceURL,
			[]byte("annotation-engine-operation:"+campaign.WorkspaceID.String()+":"+
				s.engine.Provider()+":"+s.engine.InstanceRef()+":"+requestID),
		),
		WorkspaceID:           campaign.WorkspaceID,
		CampaignID:            campaign.ID,
		Provider:              s.engine.Provider(),
		ProviderInstanceRef:   s.engine.InstanceRef(),
		OperationKind:         kind,
		RequestID:             requestID,
		RequestFingerprint:    hashBytes(payload),
		PayloadManifest:       payload,
		PayloadManifestHash:   payload,
		PayloadManifestSHA256: hashBytes(payload),
		Status:                annotationdomain.EngineOperationPending,
		Revision:              1,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := operation.Validate(); err != nil {
		return annotationdomain.EngineOperation{}, err
	}

	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		lockedCampaign, err := s.repo.LockCampaignTx(ctx, tx, operation.CampaignID)
		if err != nil {
			return err
		}
		if lockedCampaign.WorkspaceID != operation.WorkspaceID ||
			lockedCampaign.Status != annotationdomain.CampaignActive {
			return annotationdomain.ErrInvalidCampaign
		}
		created, err := s.repo.InsertEngineOperation(ctx, tx, operation)
		if err != nil {
			return err
		}
		if !created {
			return nil
		}
		if err := appendEvent(
			ctx,
			tx,
			"ANNOTATION_ENGINE_OPERATION",
			operation.ID,
			"AnnotationEngineOperationPrepared",
			map[string]any{
				"operationId":        operation.ID,
				"workspaceId":        operation.WorkspaceID,
				"campaignId":         operation.CampaignID,
				"operationKind":      operation.OperationKind,
				"provider":           operation.Provider,
				"providerInstance":   operation.ProviderInstanceRef,
				"requestId":          operation.RequestID,
				"requestFingerprint": operation.RequestFingerprint,
			},
		); err != nil {
			return err
		}
		if _, err := evidence.Append(
			ctx,
			tx,
			evidence.Record{
				WorkspaceID:  operation.WorkspaceID,
				EvidenceType: "ANNOTATION_ENGINE_OPERATION_PREPARED",
				Title:        "Annotation engine operation prepared",
				SourceType:   "CORE",
				Metadata: map[string]any{
					"campaignId":            operation.CampaignID,
					"operationKind":         operation.OperationKind,
					"provider":              operation.Provider,
					"providerInstance":      operation.ProviderInstanceRef,
					"requestId":             operation.RequestID,
					"requestFingerprint":    operation.RequestFingerprint,
					"payloadManifestSha256": operation.PayloadManifestSHA256,
				},
				CreatedBy: actorID,
			},
			evidence.Relation{
				ObjectType:   "ANNOTATION_ENGINE_OPERATION",
				ObjectID:     operation.ID,
				RelationType: "PREPARATION_EVIDENCE",
			},
		); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &operation.WorkspaceID,
			ActorType:   actorType(actorID),
			ActorID:     actorID,
			Action:      "ANNOTATION_ENGINE_OPERATION_PREPARED",
			ObjectType:  "ANNOTATION_ENGINE_OPERATION",
			ObjectID:    operation.ID,
			AfterState: map[string]any{
				"status":        operation.Status,
				"operationKind": operation.OperationKind,
				"provider":      operation.Provider,
				"requestId":     operation.RequestID,
			},
			TraceID: traceID,
		})
	})
	if err != nil {
		return annotationdomain.EngineOperation{}, err
	}
	return s.repo.GetEngineOperation(ctx, operation.ID)
}
