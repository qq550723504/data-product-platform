package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	annotationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	goldinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/gold/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/tabular"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
)

const (
	ProcessorGoldDatasetBuilderV1 = "GOLD_DATASET_BUILDER_V1"
	createBuildCommandType        = "GOLD.CREATE_BUILD"
)

var (
	ErrInvalidBuildRequest = errors.New("invalid Gold build request")
	ErrBuildConflict       = errors.New("Gold build idempotency conflict")
	ErrBuildBlocked        = errors.New("Gold build is blocked by frozen facts")
)

type ObjectStore interface {
	Get(ctx context.Context, storageURI string) (io.ReadCloser, error)
}

type CreateBuildCommand struct {
	WorkspaceID                      uuid.UUID
	WorkflowVersionID                uuid.UUID
	OutputDatasetID                  uuid.UUID
	InputDatasetVersionID            uuid.UUID
	InputCertificationID             uuid.UUID
	AnnotationCampaignID             uuid.UUID
	AnnotationSnapshotID             uuid.UUID
	AnnotationContributionResourceID uuid.UUID
	IdempotencyKey                   string
	ActorID                          *uuid.UUID
	TraceID                          string
}

type Service struct {
	tx             *transaction.Manager
	goldRepo       *goldinfra.PostgresRepository
	workflowRepo   *workflowinfra.PostgresRepository
	annotationRepo *annotationinfra.Repository
	datasetRepo    *datasetinfra.PostgresRepository
	datasetWriter  *datasetapp.UploadVersionService
	store          ObjectStore
}

func NewService(
	tx *transaction.Manager,
	goldRepo *goldinfra.PostgresRepository,
	workflowRepo *workflowinfra.PostgresRepository,
	annotationRepo *annotationinfra.Repository,
	datasetRepo *datasetinfra.PostgresRepository,
	datasetWriter *datasetapp.UploadVersionService,
	store ObjectStore,
) *Service {
	return &Service{
		tx: tx, goldRepo: goldRepo, workflowRepo: workflowRepo,
		annotationRepo: annotationRepo, datasetRepo: datasetRepo,
		datasetWriter: datasetWriter, store: store,
	}
}

func (s *Service) CreateBuild(ctx context.Context, cmd CreateBuildCommand) (workflowdomain.Execution, error) {
	if s == nil || s.tx == nil || s.goldRepo == nil || s.workflowRepo == nil || s.annotationRepo == nil {
		return workflowdomain.Execution{}, ErrInvalidBuildRequest
	}
	if cmd.WorkspaceID == uuid.Nil || cmd.WorkflowVersionID == uuid.Nil || cmd.OutputDatasetID == uuid.Nil ||
		cmd.InputDatasetVersionID == uuid.Nil || cmd.InputCertificationID == uuid.Nil ||
		cmd.AnnotationCampaignID == uuid.Nil || cmd.AnnotationSnapshotID == uuid.Nil ||
		cmd.AnnotationContributionResourceID == uuid.Nil {
		return workflowdomain.Execution{}, ErrInvalidBuildRequest
	}
	key, err := workflowdomain.NormalizeIdempotencyKey(cmd.IdempotencyKey)
	if err != nil {
		return workflowdomain.Execution{}, err
	}
	workflowVersion, err := s.workflowRepo.GetVersion(ctx, cmd.WorkflowVersionID)
	if err != nil {
		return workflowdomain.Execution{}, err
	}
	if goldProcessor(workflowVersion.Definition) != ProcessorGoldDatasetBuilderV1 ||
		workflowRequiresTargetPeriod(workflowVersion.Definition) {
		return workflowdomain.Execution{}, ErrInvalidBuildRequest
	}

	snapshot, err := s.annotationRepo.GetSnapshotByCampaign(ctx, cmd.WorkspaceID, cmd.AnnotationCampaignID)
	if err != nil {
		return workflowdomain.Execution{}, err
	}
	if snapshot.ID != cmd.AnnotationSnapshotID || snapshot.Status != annotationdomain.SnapshotFinalized ||
		snapshot.FinalizedAt == nil {
		return workflowdomain.Execution{}, ErrInvalidBuildRequest
	}
	integrityValid, err := s.annotationRepo.GetSnapshotIntegrity(ctx, snapshot.ID)
	if err != nil {
		return workflowdomain.Execution{}, err
	}
	if !integrityValid {
		return workflowdomain.Execution{}, ErrInvalidBuildRequest
	}
	campaign, err := s.annotationRepo.GetCampaign(ctx, cmd.AnnotationCampaignID)
	if err != nil {
		return workflowdomain.Execution{}, err
	}
	if campaign.WorkspaceID != cmd.WorkspaceID ||
		campaign.InputDatasetVersionID != cmd.InputDatasetVersionID ||
		campaign.InputCertificationID != cmd.InputCertificationID ||
		campaign.AnnotationContributionID != cmd.AnnotationContributionResourceID {
		return workflowdomain.Execution{}, ErrInvalidBuildRequest
	}

	inputs := []workflowdomain.InputBinding{{
		Name:             "gold_input",
		DatasetVersionID: cmd.InputDatasetVersionID,
	}}
	execution, err := workflowdomain.NewExecution(
		cmd.WorkspaceID, cmd.WorkflowVersionID, cmd.OutputDatasetID, "", inputs, cmd.ActorID,
	)
	if err != nil {
		return workflowdomain.Execution{}, err
	}
	fingerprint, err := buildFingerprint(cmd, snapshot.RootHash)
	if err != nil {
		return workflowdomain.Execution{}, err
	}

	var result workflowdomain.Execution
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		inserted, err := s.workflowRepo.TryInsertIdempotency(
			ctx, tx, cmd.WorkspaceID, createBuildCommandType, key, execution.ID, &execution.ID, fingerprint,
		)
		if err != nil {
			return err
		}
		if !inserted {
			record, found, err := s.workflowRepo.FindIdempotencyTx(ctx, tx, cmd.WorkspaceID, createBuildCommandType, key)
			if err != nil {
				return err
			}
			if !found || record.RequestFingerprint != fingerprint {
				return ErrBuildConflict
			}
			existing, err := s.workflowRepo.GetExecutionTx(ctx, tx, record.ObjectID, true)
			if err != nil {
				return err
			}
			stored, err := s.goldRepo.GetBuildRequestTx(ctx, tx, existing.ID)
			if err != nil {
				return err
			}
			if stored.RequestFingerprint != fingerprint {
				return ErrBuildConflict
			}
			result = existing
			return nil
		}

		if err := s.workflowRepo.ValidateExecutionReferences(
			ctx, tx, execution.WorkspaceID, execution.WorkflowVersionID, execution.OutputDatasetID, execution.Inputs,
		); err != nil {
			return err
		}
		if err := s.workflowRepo.InsertExecution(ctx, tx, execution); err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := s.goldRepo.InsertBuildRequest(ctx, tx, goldinfra.BuildRequest{
			ExecutionID:                      execution.ID,
			WorkspaceID:                      cmd.WorkspaceID,
			InputDatasetVersionID:            cmd.InputDatasetVersionID,
			InputCertificationID:             cmd.InputCertificationID,
			AnnotationCampaignID:             cmd.AnnotationCampaignID,
			AnnotationSnapshotID:             cmd.AnnotationSnapshotID,
			AnnotationContributionResourceID: cmd.AnnotationContributionResourceID,
			SnapshotRootHash:                 snapshot.RootHash,
			RequestFingerprint:               fingerprint,
			CreatedAt:                        now,
			CreatedBy:                        cmd.ActorID,
		}); err != nil {
			return err
		}
		if _, err := evidence.Append(ctx, tx, evidence.Record{
			WorkspaceID:  cmd.WorkspaceID,
			EvidenceType: "GOLD_BUILD_REQUEST_FROZEN",
			Title:        "Gold build request frozen",
			SourceType:   "EXECUTION",
			SourceID:     &execution.ID,
			Metadata: map[string]any{
				"inputDatasetVersionId": cmd.InputDatasetVersionID,
				"inputCertificationId":  cmd.InputCertificationID,
				"annotationCampaignId":  cmd.AnnotationCampaignID,
				"annotationSnapshotId":  cmd.AnnotationSnapshotID,
				"snapshotRootHash":      snapshot.RootHash,
				"requestFingerprint":    fingerprint,
			},
			CreatedBy: cmd.ActorID,
		}, evidence.Relation{ObjectType: "EXECUTION", ObjectID: execution.ID, RelationType: "GOLD_BUILD_REQUEST"}); err != nil {
			return err
		}
		event, err := outbox.NewEvent("EXECUTION", execution.ID, "ExecutionQueued", map[string]any{
			"executionId":            execution.ID,
			"workflowVersionId":      execution.WorkflowVersionID,
			"status":                 execution.Status,
			"attempt":                execution.Attempt,
			"outputDatasetVersionId": execution.OutputDatasetVersionID,
		})
		if err != nil {
			return err
		}
		if err := outbox.Append(ctx, tx, event); err != nil {
			return err
		}
		if err := audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &cmd.WorkspaceID,
			ActorType:   actorType(cmd.ActorID),
			ActorID:     cmd.ActorID,
			Action:      "GOLD_BUILD_QUEUED",
			ObjectType:  "EXECUTION",
			ObjectID:    execution.ID,
			AfterState: map[string]any{
				"status":                execution.Status,
				"inputDatasetVersionId": cmd.InputDatasetVersionID,
				"annotationSnapshotId":  cmd.AnnotationSnapshotID,
				"snapshotRootHash":      snapshot.RootHash,
			},
			TraceID: cmd.TraceID,
		}); err != nil {
			return err
		}
		result = execution
		return nil
	})
	if err != nil {
		return workflowdomain.Execution{}, err
	}
	return result, nil
}

func (s *Service) Execute(ctx context.Context, request workflowapp.ProcessingRequest) (workflowapp.ProcessingResult, error) {
	if s == nil || s.goldRepo == nil || s.annotationRepo == nil || s.datasetRepo == nil ||
		s.datasetWriter == nil || s.store == nil || s.tx == nil {
		return workflowapp.ProcessingResult{}, ErrInvalidBuildRequest
	}
	if goldProcessor(request.WorkflowVersion.Definition) != ProcessorGoldDatasetBuilderV1 {
		return workflowapp.ProcessingResult{}, ErrInvalidBuildRequest
	}

	if existing, err := s.goldRepo.GetBindingByExecution(ctx, request.ExecutionID); err == nil {
		if existing.Status != "FINALIZED" {
			return workflowapp.ProcessingResult{}, ErrInvalidBuildRequest
		}
		return workflowapp.ProcessingResult{
			OutputDatasetVersionID: existing.OutputDatasetVersionID,
			EngineExecutionID:      "gold:" + request.ExecutionID.String(),
			Metrics: map[string]any{
				"outputRows":      existing.OutputRowCount,
				"bindingId":       existing.ID,
				"bindingRootHash": existing.RootHash,
				"outputReused":    true,
			},
		}, nil
	} else if !errors.Is(err, goldinfra.ErrNotFound) {
		return workflowapp.ProcessingResult{}, err
	}

	buildRequest, err := s.goldRepo.GetBuildRequest(ctx, request.ExecutionID)
	if err != nil {
		return workflowapp.ProcessingResult{}, err
	}
	if buildRequest.WorkspaceID != request.WorkspaceID ||
		buildRequest.InputDatasetVersionID != requiredInput(request.Inputs, "gold_input") {
		return workflowapp.ProcessingResult{}, ErrInvalidBuildRequest
	}

	snapshot, err := s.annotationRepo.GetSnapshotByCampaign(
		ctx, buildRequest.WorkspaceID, buildRequest.AnnotationCampaignID,
	)
	if err != nil {
		return workflowapp.ProcessingResult{}, err
	}
	if snapshot.ID != buildRequest.AnnotationSnapshotID ||
		snapshot.Status != annotationdomain.SnapshotFinalized ||
		snapshot.RootHash != buildRequest.SnapshotRootHash {
		return workflowapp.ProcessingResult{}, ErrInvalidBuildRequest
	}
	valid, err := s.annotationRepo.GetSnapshotIntegrity(ctx, snapshot.ID)
	if err != nil {
		return workflowapp.ProcessingResult{}, err
	}
	if !valid {
		return workflowapp.ProcessingResult{}, ErrInvalidBuildRequest
	}
	campaign, err := s.annotationRepo.GetCampaign(ctx, buildRequest.AnnotationCampaignID)
	if err != nil {
		return workflowapp.ProcessingResult{}, err
	}

	inputVersion, err := s.datasetRepo.GetVersion(ctx, buildRequest.InputDatasetVersionID)
	if err != nil {
		return workflowapp.ProcessingResult{}, err
	}
	if inputVersion.Status != datasetdomain.VersionReady && inputVersion.Status != datasetdomain.VersionSuperseded {
		return workflowapp.ProcessingResult{}, ErrInvalidBuildRequest
	}
	inputBytes, err := s.readInput(ctx, inputVersion.StorageURI)
	if err != nil {
		return workflowapp.ProcessingResult{}, err
	}
	if hashBytes(inputBytes) != strings.ToLower(inputVersion.ChecksumValue) {
		return workflowapp.ProcessingResult{}, fmt.Errorf("%w: input checksum mismatch", ErrInvalidBuildRequest)
	}
	table, err := tabular.ReadCSV(bytes.NewReader(inputBytes))
	if err != nil {
		return workflowapp.ProcessingResult{}, fmt.Errorf("%w: Gold Pilot requires CSV input: %v", ErrInvalidBuildRequest, err)
	}
	members, err := s.goldRepo.ListSnapshotMembers(ctx, snapshot.ID)
	if err != nil {
		return workflowapp.ProcessingResult{}, err
	}
	if len(members) != len(table.Rows) || len(members) == 0 {
		return workflowapp.ProcessingResult{}, fmt.Errorf("%w: frozen member count does not match input rows", ErrInvalidBuildRequest)
	}

	content, productionMembers, outputRows, err := buildGoldCSV(table, members)
	if err != nil {
		return workflowapp.ProcessingResult{}, err
	}
	var binding goldinfra.ProductionBinding
	outputVersion, err := s.datasetWriter.HandleWithFinalizer(ctx, datasetapp.UploadVersionCommand{
		DatasetID:              request.OutputDatasetID,
		Filename:               "gold-" + snapshot.ID.String() + ".csv",
		ContentType:            "text/csv; charset=utf-8",
		Content:                content,
		TraceID:                request.ExecutionID.String(),
		GeneratedByExecutionID: &request.ExecutionID,
		Metadata: map[string]any{
			"goldCandidate":          true,
			"goldBuilder":            ProcessorGoldDatasetBuilderV1,
			"workflowVersionId":      request.WorkflowVersion.ID,
			"inputDatasetVersionId":  buildRequest.InputDatasetVersionID,
			"annotationSnapshotId":   snapshot.ID,
			"annotationSnapshotRoot": snapshot.RootHash,
			"gateStatus":             "PENDING_GOLD_QUALITY_AND_CERTIFICATION",
		},
	}, func(ctx context.Context, tx pgx.Tx, published datasetdomain.DatasetVersion) error {
		if published.RowCount == nil || *published.RowCount != int64(outputRows) {
			return fmt.Errorf("%w: output row count mismatch", ErrInvalidBuildRequest)
		}
		if existing, readErr := s.goldRepo.GetBindingByExecution(ctx, request.ExecutionID); readErr == nil {
			if existing.Status != "FINALIZED" || existing.OutputDatasetVersionID != published.ID {
				return ErrInvalidBuildRequest
			}
			binding = existing
			return nil
		} else if !errors.Is(readErr, goldinfra.ErrNotFound) {
			return readErr
		}

		built, buildErr := buildProductionBinding(
			request, buildRequest, campaign, inputVersion, published, productionMembers,
		)
		if buildErr != nil {
			return buildErr
		}
		built.CreatedBy = buildRequest.CreatedBy
		binding = built
		if err := s.datasetRepo.AddLineage(
			ctx, tx, published.ID, inputVersion.ID, "DERIVED_FROM", &request.ExecutionID,
		); err != nil {
			return err
		}
		finalizedAt := time.Now().UTC().Truncate(time.Microsecond)
		if err := s.goldRepo.InsertAndFinalizeBinding(ctx, tx, built, productionMembers, finalizedAt); err != nil {
			return err
		}
		if err := cost.Append(ctx, tx, cost.Event{
			WorkspaceID: request.WorkspaceID,
			ExecutionID: &request.ExecutionID,
			ActivityID:  request.ExecutionID,
			CostType:    "GOLD_BUILD_EXECUTION",
			Quantity:    1,
			Unit:        "build",
			PricingMode: "ACTUAL",
			Metadata: map[string]any{
				"outputDatasetVersionId":  published.ID,
				"goldProductionBindingId": built.ID,
				"annotationSnapshotId":    snapshot.ID,
			},
			OccurredAt: finalizedAt,
		}); err != nil {
			return err
		}
		bindingID := built.ID
		if _, err := evidence.Append(ctx, tx, evidence.Record{
			WorkspaceID:  request.WorkspaceID,
			EvidenceType: "GOLD_PRODUCTION_BINDING_FINALIZED",
			Title:        "Gold production binding finalized",
			SourceType:   "EXECUTION",
			SourceID:     &request.ExecutionID,
			Metadata: map[string]any{
				"bindingId":              bindingID,
				"inputDatasetVersionId":  inputVersion.ID,
				"outputDatasetVersionId": published.ID,
				"annotationSnapshotId":   snapshot.ID,
				"snapshotRootHash":       snapshot.RootHash,
				"bindingRootHash":        built.RootHash,
				"outputRowCount":         outputRows,
			},
			CreatedBy: buildRequest.CreatedBy,
		}, evidence.Relation{ObjectType: "DATASET_VERSION", ObjectID: published.ID, RelationType: "GOLD_PRODUCTION"}); err != nil {
			return err
		}
		event, err := outbox.NewEvent("GOLD_PRODUCTION_BINDING", built.ID, "GoldProductionBindingFinalized", map[string]any{
			"bindingId":              built.ID,
			"executionId":            request.ExecutionID,
			"inputDatasetVersionId":  inputVersion.ID,
			"outputDatasetVersionId": published.ID,
			"annotationSnapshotId":   snapshot.ID,
			"rootHash":               built.RootHash,
		})
		if err != nil {
			return err
		}
		if err := outbox.Append(ctx, tx, event); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &request.WorkspaceID,
			ActorType:   actorType(buildRequest.CreatedBy),
			ActorID:     buildRequest.CreatedBy,
			Action:      "GOLD_PRODUCTION_BINDING_FINALIZED",
			ObjectType:  "GOLD_PRODUCTION_BINDING",
			ObjectID:    built.ID,
			AfterState: map[string]any{
				"status":                 "FINALIZED",
				"outputDatasetVersionId": published.ID,
				"annotationSnapshotId":   snapshot.ID,
				"rootHash":               built.RootHash,
				"outputRowCount":         outputRows,
			},
			TraceID: request.ExecutionID.String(),
		})
	})
	if err != nil {
		return workflowapp.ProcessingResult{}, err
	}
	if binding.ID == uuid.Nil {
		existing, readErr := s.goldRepo.GetBindingByExecution(ctx, request.ExecutionID)
		if readErr != nil || existing.Status != "FINALIZED" || existing.OutputDatasetVersionID != outputVersion.ID {
			return workflowapp.ProcessingResult{}, fmt.Errorf("%w: finalized Gold binding was not resolved", ErrInvalidBuildRequest)
		}
		binding = existing
	}

	return workflowapp.ProcessingResult{
		OutputDatasetVersionID: outputVersion.ID,
		EngineExecutionID:      "gold:" + request.ExecutionID.String(),
		Metrics: map[string]any{
			"inputRows":        len(table.Rows),
			"outputRows":       outputRows,
			"rejectedRows":     len(table.Rows) - outputRows,
			"bindingId":        binding.ID,
			"bindingRootHash":  binding.RootHash,
			"snapshotId":       snapshot.ID,
			"snapshotRootHash": snapshot.RootHash,
		},
	}, nil
}

func (s *Service) readInput(ctx context.Context, storageURI string) ([]byte, error) {
	reader, err := s.store.Get(ctx, storageURI)
	if err != nil {
		return nil, fmt.Errorf("read Gold input object: %w", err)
	}
	defer reader.Close()
	payload, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read Gold input bytes: %w", err)
	}
	return payload, nil
}

func buildGoldCSV(table tabular.Table, frozen []goldinfra.FrozenMember) ([]byte, []goldinfra.ProductionMember, int, error) {
	byRef := make(map[string]goldinfra.FrozenMember, len(frozen))
	for _, member := range frozen {
		if _, exists := byRef[member.SourceItemRef]; exists {
			return nil, nil, 0, fmt.Errorf("%w: duplicate frozen source item %s", ErrInvalidBuildRequest, member.SourceItemRef)
		}
		byRef[member.SourceItemRef] = member
	}
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	headers := append([]string(nil), table.Headers...)
	headers = append(headers, "gold_label", "annotation_result_id", "review_decision_id")
	if err := writer.Write(headers); err != nil {
		return nil, nil, 0, err
	}

	productionMembers := make([]goldinfra.ProductionMember, 0, len(table.Rows))
	outputIndex := 0
	for i := range table.Rows {
		ref := fmt.Sprintf("row:%d", i+1)
		member, ok := byRef[ref]
		if !ok {
			return nil, nil, 0, fmt.Errorf("%w: frozen snapshot is missing %s", ErrInvalidBuildRequest, ref)
		}
		sourceHash, err := goldSourceRowHash(table.Rows[i])
		if err != nil {
			return nil, nil, 0, err
		}
		if sourceHash != strings.ToLower(member.SourceContentSHA256) {
			return nil, nil, 0, fmt.Errorf("%w: frozen source hash mismatch for %s", ErrInvalidBuildRequest, ref)
		}
		production := goldinfra.ProductionMember{
			TaskID:              member.TaskID,
			SourceItemRef:       member.SourceItemRef,
			SourceContentSHA256: member.SourceContentSHA256,
			DecisionID:          member.DecisionID,
			Outcome:             member.Outcome,
			ReviewedResultID:    member.ReviewedResultID,
			SelectedResultID:    member.SelectedResultID,
		}
		switch member.Outcome {
		case annotationdomain.ReviewReject:
			if member.SelectedResultID != nil {
				return nil, nil, 0, fmt.Errorf("%w: rejected task has selected result", ErrInvalidBuildRequest)
			}
		case annotationdomain.ReviewAccept, annotationdomain.ReviewCorrect:
			if member.SelectedResultID == nil || len(member.SelectedPayload) == 0 || member.SelectedResultSHA256 == "" {
				return nil, nil, 0, fmt.Errorf("%w: selected result provenance is incomplete", ErrInvalidBuildRequest)
			}
			if hashBytes(member.SelectedPayload) != strings.ToLower(member.SelectedResultSHA256) {
				return nil, nil, 0, fmt.Errorf("%w: selected result checksum mismatch", ErrInvalidBuildRequest)
			}
			var payload struct {
				Label string `json:"label"`
			}
			if err := json.Unmarshal(member.SelectedPayload, &payload); err != nil || strings.TrimSpace(payload.Label) == "" {
				return nil, nil, 0, fmt.Errorf("%w: selected result payload is invalid", ErrInvalidBuildRequest)
			}
			row := make([]string, 0, len(headers))
			for _, header := range table.Headers {
				row = append(row, table.RawValue(i, header))
			}
			row = append(row, payload.Label, member.SelectedResultID.String(), member.DecisionID.String())
			if err := writer.Write(row); err != nil {
				return nil, nil, 0, err
			}
			hash := member.SelectedResultSHA256
			index := outputIndex
			production.SelectedResultSHA256 = &hash
			production.OutputRowIndex = &index
			outputIndex++
		default:
			return nil, nil, 0, fmt.Errorf("%w: unsupported review outcome %s", ErrInvalidBuildRequest, member.Outcome)
		}
		productionMembers = append(productionMembers, production)
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, nil, 0, err
	}
	return buffer.Bytes(), productionMembers, outputIndex, nil
}

func buildProductionBinding(
	request workflowapp.ProcessingRequest,
	buildRequest goldinfra.BuildRequest,
	campaign annotationdomain.Campaign,
	inputVersion datasetdomain.DatasetVersion,
	outputVersion datasetdomain.DatasetVersion,
	members []goldinfra.ProductionMember,
) (goldinfra.ProductionBinding, error) {
	if outputVersion.RowCount == nil {
		return goldinfra.ProductionBinding{}, ErrInvalidBuildRequest
	}
	type manifestMember struct {
		TaskID               string  `json:"taskId"`
		SourceItemRef        string  `json:"sourceItemRef"`
		SourceContentSHA256  string  `json:"sourceContentSha256"`
		DecisionID           string  `json:"decisionId"`
		Outcome              string  `json:"outcome"`
		ReviewedResultID     *string `json:"reviewedResultId,omitempty"`
		SelectedResultID     *string `json:"selectedResultId,omitempty"`
		SelectedResultSHA256 *string `json:"selectedResultSha256,omitempty"`
		OutputRowIndex       *int    `json:"outputRowIndex,omitempty"`
	}
	type manifest struct {
		FormatVersion                    string           `json:"formatVersion"`
		WorkspaceID                      string           `json:"workspaceId"`
		ExecutionID                      string           `json:"executionId"`
		WorkflowVersionID                string           `json:"workflowVersionId"`
		InputDatasetVersionID            string           `json:"inputDatasetVersionId"`
		InputCertificationID             string           `json:"inputCertificationId"`
		AnnotationCampaignID             string           `json:"annotationCampaignId"`
		AnnotationSnapshotID             string           `json:"annotationSnapshotId"`
		AnnotationContributionResourceID string           `json:"annotationContributionResourceId"`
		OutputDatasetVersionID           string           `json:"outputDatasetVersionId"`
		InputChecksumSHA256              string           `json:"inputChecksumSha256"`
		SnapshotRootHash                 string           `json:"snapshotRootHash"`
		OutputChecksumSHA256             string           `json:"outputChecksumSha256"`
		OutputRowCount                   int64            `json:"outputRowCount"`
		Members                          []manifestMember `json:"members"`
	}
	orderedMembers := append([]goldinfra.ProductionMember(nil), members...)
	sort.Slice(orderedMembers, func(i, j int) bool {
		return orderedMembers[i].TaskID.String() < orderedMembers[j].TaskID.String()
	})
	items := make([]manifestMember, 0, len(orderedMembers))
	for _, member := range orderedMembers {
		var reviewed, selected *string
		if member.ReviewedResultID != nil {
			value := member.ReviewedResultID.String()
			reviewed = &value
		}
		if member.SelectedResultID != nil {
			value := member.SelectedResultID.String()
			selected = &value
		}
		items = append(items, manifestMember{
			TaskID:               member.TaskID.String(),
			SourceItemRef:        member.SourceItemRef,
			SourceContentSHA256:  member.SourceContentSHA256,
			DecisionID:           member.DecisionID.String(),
			Outcome:              member.Outcome,
			ReviewedResultID:     reviewed,
			SelectedResultID:     selected,
			SelectedResultSHA256: member.SelectedResultSHA256,
			OutputRowIndex:       member.OutputRowIndex,
		})
	}
	payload, err := json.Marshal(manifest{
		FormatVersion:                    "gold-production-binding-v1",
		WorkspaceID:                      request.WorkspaceID.String(),
		ExecutionID:                      request.ExecutionID.String(),
		WorkflowVersionID:                request.WorkflowVersion.ID.String(),
		InputDatasetVersionID:            buildRequest.InputDatasetVersionID.String(),
		InputCertificationID:             buildRequest.InputCertificationID.String(),
		AnnotationCampaignID:             buildRequest.AnnotationCampaignID.String(),
		AnnotationSnapshotID:             buildRequest.AnnotationSnapshotID.String(),
		AnnotationContributionResourceID: buildRequest.AnnotationContributionResourceID.String(),
		OutputDatasetVersionID:           outputVersion.ID.String(),
		InputChecksumSHA256:              strings.ToLower(inputVersion.ChecksumValue),
		SnapshotRootHash:                 buildRequest.SnapshotRootHash,
		OutputChecksumSHA256:             strings.ToLower(outputVersion.ChecksumValue),
		OutputRowCount:                   *outputVersion.RowCount,
		Members:                          items,
	})
	if err != nil {
		return goldinfra.ProductionBinding{}, err
	}
	rootHash := hashBytes(payload)
	now := time.Now().UTC().Truncate(time.Microsecond)
	return goldinfra.ProductionBinding{
		ID:                               uuid.NewSHA1(uuid.NameSpaceURL, []byte("gold-production-binding:"+request.ExecutionID.String())),
		WorkspaceID:                      request.WorkspaceID,
		ExecutionID:                      request.ExecutionID,
		WorkflowVersionID:                request.WorkflowVersion.ID,
		InputDatasetVersionID:            buildRequest.InputDatasetVersionID,
		InputCertificationID:             buildRequest.InputCertificationID,
		AnnotationCampaignID:             buildRequest.AnnotationCampaignID,
		AnnotationSnapshotID:             buildRequest.AnnotationSnapshotID,
		AnnotationContributionResourceID: buildRequest.AnnotationContributionResourceID,
		OutputDatasetVersionID:           outputVersion.ID,
		InputChecksumSHA256:              strings.ToLower(inputVersion.ChecksumValue),
		SnapshotRootHash:                 buildRequest.SnapshotRootHash,
		SchemaContentSHA256:              campaign.Schema.ContentSHA256,
		TaxonomyContentSHA256:            campaign.Taxonomy.ContentSHA256,
		RubricContentSHA256:              campaign.Rubric.ContentSHA256,
		RendererContentSHA256:            campaign.Renderer.ContentSHA256,
		ReviewPolicyContentSHA256:        campaign.ReviewPolicy.ContentSHA256,
		OutputChecksumSHA256:             strings.ToLower(outputVersion.ChecksumValue),
		OutputRowCount:                   *outputVersion.RowCount,
		Manifest:                         payload,
		ManifestHashPayload:              append([]byte(nil), payload...),
		RootHash:                         rootHash,
		Status:                           "BUILDING",
		CreatedAt:                        now,
	}, nil
}

func buildFingerprint(cmd CreateBuildCommand, snapshotRoot string) (string, error) {
	payload := struct {
		WorkspaceID                      uuid.UUID  `json:"workspaceId"`
		WorkflowVersionID                uuid.UUID  `json:"workflowVersionId"`
		OutputDatasetID                  uuid.UUID  `json:"outputDatasetId"`
		InputDatasetVersionID            uuid.UUID  `json:"inputDatasetVersionId"`
		InputCertificationID             uuid.UUID  `json:"inputCertificationId"`
		AnnotationCampaignID             uuid.UUID  `json:"annotationCampaignId"`
		AnnotationSnapshotID             uuid.UUID  `json:"annotationSnapshotId"`
		AnnotationContributionResourceID uuid.UUID  `json:"annotationContributionResourceId"`
		SnapshotRootHash                 string     `json:"snapshotRootHash"`
		ActorID                          *uuid.UUID `json:"actorId,omitempty"`
	}{
		WorkspaceID:                      cmd.WorkspaceID,
		WorkflowVersionID:                cmd.WorkflowVersionID,
		OutputDatasetID:                  cmd.OutputDatasetID,
		InputDatasetVersionID:            cmd.InputDatasetVersionID,
		InputCertificationID:             cmd.InputCertificationID,
		AnnotationCampaignID:             cmd.AnnotationCampaignID,
		AnnotationSnapshotID:             cmd.AnnotationSnapshotID,
		AnnotationContributionResourceID: cmd.AnnotationContributionResourceID,
		SnapshotRootHash:                 snapshotRoot,
		ActorID:                          cmd.ActorID,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return hashBytes(encoded), nil
}

func requiredInput(inputs []workflowdomain.InputBinding, name string) uuid.UUID {
	for _, input := range inputs {
		if input.Name == name {
			return input.DatasetVersionID
		}
	}
	return uuid.Nil
}

func goldProcessor(definition map[string]any) string {
	spec, ok := definition["spec"].(map[string]any)
	if !ok {
		return ""
	}
	value, _ := spec["processor"].(string)
	return strings.TrimSpace(value)
}

func workflowRequiresTargetPeriod(definition map[string]any) bool {
	spec, ok := definition["spec"].(map[string]any)
	if !ok {
		return true
	}
	execution, ok := spec["execution"].(map[string]any)
	if !ok {
		return true
	}
	required, ok := execution["requiresTargetPeriod"].(bool)
	if !ok {
		return true
	}
	return required
}

func hashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func actorType(actorID *uuid.UUID) string {
	if actorID == nil {
		return "SYSTEM"
	}
	return "USER"
}

func ParseRowReference(value string) (int, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "row:") {
		return 0, ErrInvalidBuildRequest
	}
	index, err := strconv.Atoi(strings.TrimPrefix(value, "row:"))
	if err != nil || index < 1 {
		return 0, ErrInvalidBuildRequest
	}
	return index, nil
}

func goldSourceRowHash(row map[string]string) (string, error) {
	encoded, err := json.Marshal(row)
	if err != nil {
		return "", fmt.Errorf("marshal Gold source row: %w", err)
	}
	return hashBytes(encoded), nil
}
