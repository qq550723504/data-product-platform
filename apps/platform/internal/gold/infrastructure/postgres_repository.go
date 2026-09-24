package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("gold production fact not found")

type BuildRequest struct {
	ExecutionID                      uuid.UUID
	WorkspaceID                      uuid.UUID
	InputDatasetVersionID            uuid.UUID
	InputCertificationID             uuid.UUID
	AnnotationCampaignID             uuid.UUID
	AnnotationSnapshotID             uuid.UUID
	AnnotationContributionResourceID uuid.UUID
	SnapshotRootHash                 string
	RequestFingerprint               string
	CreatedAt                        time.Time
	CreatedBy                        *uuid.UUID
}

type FrozenMember struct {
	TaskID               uuid.UUID
	SourceItemRef        string
	SourceContentSHA256  string
	DecisionID           uuid.UUID
	Outcome              string
	ReviewedResultID     *uuid.UUID
	SelectedResultID     *uuid.UUID
	SelectedResultSHA256 string
	SelectedPayload      []byte
}

type ProductionBinding struct {
	ID                               uuid.UUID
	WorkspaceID                      uuid.UUID
	ExecutionID                      uuid.UUID
	WorkflowVersionID                uuid.UUID
	InputDatasetVersionID            uuid.UUID
	InputCertificationID             uuid.UUID
	AnnotationCampaignID             uuid.UUID
	AnnotationSnapshotID             uuid.UUID
	AnnotationContributionResourceID uuid.UUID
	OutputDatasetVersionID           uuid.UUID
	InputChecksumSHA256              string
	SnapshotRootHash                 string
	SchemaContentSHA256              string
	TaxonomyContentSHA256            string
	RubricContentSHA256              string
	RendererContentSHA256            string
	ReviewPolicyContentSHA256        string
	OutputChecksumSHA256             string
	OutputRowCount                   int64
	Manifest                         []byte
	ManifestHashPayload              []byte
	RootHash                         string
	Status                           string
	CreatedAt                        time.Time
	CreatedBy                        *uuid.UUID
	FinalizedAt                      *time.Time
}

type ProductionMember struct {
	TaskID               uuid.UUID
	SourceItemRef        string
	SourceContentSHA256  string
	DecisionID           uuid.UUID
	Outcome              string
	ReviewedResultID     *uuid.UUID
	SelectedResultID     *uuid.UUID
	SelectedResultSHA256 *string
	OutputRowIndex       *int
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) InsertBuildRequest(ctx context.Context, tx pgx.Tx, request BuildRequest) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO gold_build_request(
			execution_id, workspace_id, input_dataset_version_id, input_certification_id,
			annotation_campaign_id, annotation_snapshot_id, annotation_contribution_resource_id,
			snapshot_root_hash, request_fingerprint, created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`, request.ExecutionID, request.WorkspaceID, request.InputDatasetVersionID,
		request.InputCertificationID, request.AnnotationCampaignID, request.AnnotationSnapshotID,
		request.AnnotationContributionResourceID, request.SnapshotRootHash,
		request.RequestFingerprint, request.CreatedAt, request.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert gold build request: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetBuildRequestTx(ctx context.Context, tx pgx.Tx, executionID uuid.UUID) (BuildRequest, error) {
	var request BuildRequest
	err := tx.QueryRow(ctx, `
		SELECT execution_id, workspace_id, input_dataset_version_id, input_certification_id,
		       annotation_campaign_id, annotation_snapshot_id, annotation_contribution_resource_id,
		       snapshot_root_hash, request_fingerprint, created_at, created_by
		  FROM gold_build_request
		 WHERE execution_id=$1
	`, executionID).Scan(
		&request.ExecutionID, &request.WorkspaceID, &request.InputDatasetVersionID,
		&request.InputCertificationID, &request.AnnotationCampaignID, &request.AnnotationSnapshotID,
		&request.AnnotationContributionResourceID, &request.SnapshotRootHash,
		&request.RequestFingerprint, &request.CreatedAt, &request.CreatedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return BuildRequest{}, ErrNotFound
	}
	if err != nil {
		return BuildRequest{}, fmt.Errorf("get Gold build request tx: %w", err)
	}
	return request, nil
}

func (r *PostgresRepository) GetBuildRequest(ctx context.Context, executionID uuid.UUID) (BuildRequest, error) {
	var request BuildRequest
	err := r.pool.QueryRow(ctx, `
		SELECT execution_id, workspace_id, input_dataset_version_id, input_certification_id,
		       annotation_campaign_id, annotation_snapshot_id, annotation_contribution_resource_id,
		       snapshot_root_hash, request_fingerprint, created_at, created_by
		  FROM gold_build_request
		 WHERE execution_id=$1
	`, executionID).Scan(
		&request.ExecutionID, &request.WorkspaceID, &request.InputDatasetVersionID,
		&request.InputCertificationID, &request.AnnotationCampaignID, &request.AnnotationSnapshotID,
		&request.AnnotationContributionResourceID, &request.SnapshotRootHash,
		&request.RequestFingerprint, &request.CreatedAt, &request.CreatedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return BuildRequest{}, ErrNotFound
	}
	if err != nil {
		return BuildRequest{}, fmt.Errorf("get gold build request: %w", err)
	}
	return request, nil
}

func (r *PostgresRepository) ListSnapshotMembers(ctx context.Context, snapshotID uuid.UUID) ([]FrozenMember, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT st.task_id, st.source_item_ref, st.source_content_sha256,
		       sd.decision_id, sd.outcome, sd.reviewed_result_id, sd.selected_result_id,
		       COALESCE(ar.canonical_payload_sha256,''), COALESCE(ar.canonical_payload,''::bytea)
		  FROM annotation_snapshot_task st
		  JOIN annotation_snapshot_decision sd
		    ON sd.snapshot_id=st.snapshot_id AND sd.task_id=st.task_id
		  LEFT JOIN annotation_result ar ON ar.id=sd.selected_result_id
		 WHERE st.snapshot_id=$1
		 ORDER BY st.source_item_ref, st.task_id
	`, snapshotID)
	if err != nil {
		return nil, fmt.Errorf("list frozen Gold snapshot members: %w", err)
	}
	defer rows.Close()

	members := make([]FrozenMember, 0)
	for rows.Next() {
		var member FrozenMember
		if err := rows.Scan(
			&member.TaskID, &member.SourceItemRef, &member.SourceContentSHA256,
			&member.DecisionID, &member.Outcome, &member.ReviewedResultID, &member.SelectedResultID,
			&member.SelectedResultSHA256, &member.SelectedPayload,
		); err != nil {
			return nil, fmt.Errorf("scan frozen Gold snapshot member: %w", err)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate frozen Gold snapshot members: %w", err)
	}
	return members, nil
}

func (r *PostgresRepository) GetBindingByOutput(ctx context.Context, outputDatasetVersionID uuid.UUID) (ProductionBinding, error) {
	var binding ProductionBinding
	var manifest []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, execution_id, workflow_version_id,
		       input_dataset_version_id, input_certification_id,
		       annotation_campaign_id, annotation_snapshot_id, annotation_contribution_resource_id,
		       output_dataset_version_id, input_checksum_sha256, snapshot_root_hash,
		       schema_content_sha256, taxonomy_content_sha256, rubric_content_sha256,
		       renderer_content_sha256, review_policy_content_sha256,
		       output_checksum_sha256, output_row_count, manifest, manifest_hash_payload,
		       root_hash, status, created_at, created_by, finalized_at
		  FROM gold_production_binding
		 WHERE output_dataset_version_id=$1
	`, outputDatasetVersionID).Scan(
		&binding.ID, &binding.WorkspaceID, &binding.ExecutionID, &binding.WorkflowVersionID,
		&binding.InputDatasetVersionID, &binding.InputCertificationID,
		&binding.AnnotationCampaignID, &binding.AnnotationSnapshotID,
		&binding.AnnotationContributionResourceID, &binding.OutputDatasetVersionID,
		&binding.InputChecksumSHA256, &binding.SnapshotRootHash,
		&binding.SchemaContentSHA256, &binding.TaxonomyContentSHA256,
		&binding.RubricContentSHA256, &binding.RendererContentSHA256,
		&binding.ReviewPolicyContentSHA256, &binding.OutputChecksumSHA256,
		&binding.OutputRowCount, &manifest, &binding.ManifestHashPayload,
		&binding.RootHash, &binding.Status, &binding.CreatedAt, &binding.CreatedBy,
		&binding.FinalizedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProductionBinding{}, ErrNotFound
	}
	if err != nil {
		return ProductionBinding{}, fmt.Errorf("get Gold production binding by output: %w", err)
	}
	binding.Manifest = append([]byte(nil), manifest...)
	return binding, nil
}

func (r *PostgresRepository) GetBindingByExecution(ctx context.Context, executionID uuid.UUID) (ProductionBinding, error) {
	var binding ProductionBinding
	var manifest []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, execution_id, workflow_version_id,
		       input_dataset_version_id, input_certification_id,
		       annotation_campaign_id, annotation_snapshot_id, annotation_contribution_resource_id,
		       output_dataset_version_id, input_checksum_sha256, snapshot_root_hash,
		       schema_content_sha256, taxonomy_content_sha256, rubric_content_sha256,
		       renderer_content_sha256, review_policy_content_sha256,
		       output_checksum_sha256, output_row_count, manifest, manifest_hash_payload,
		       root_hash, status, created_at, created_by, finalized_at
		  FROM gold_production_binding
		 WHERE execution_id=$1
	`, executionID).Scan(
		&binding.ID, &binding.WorkspaceID, &binding.ExecutionID, &binding.WorkflowVersionID,
		&binding.InputDatasetVersionID, &binding.InputCertificationID,
		&binding.AnnotationCampaignID, &binding.AnnotationSnapshotID,
		&binding.AnnotationContributionResourceID, &binding.OutputDatasetVersionID,
		&binding.InputChecksumSHA256, &binding.SnapshotRootHash,
		&binding.SchemaContentSHA256, &binding.TaxonomyContentSHA256,
		&binding.RubricContentSHA256, &binding.RendererContentSHA256,
		&binding.ReviewPolicyContentSHA256, &binding.OutputChecksumSHA256,
		&binding.OutputRowCount, &manifest, &binding.ManifestHashPayload,
		&binding.RootHash, &binding.Status, &binding.CreatedAt, &binding.CreatedBy,
		&binding.FinalizedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProductionBinding{}, ErrNotFound
	}
	if err != nil {
		return ProductionBinding{}, fmt.Errorf("get Gold production binding: %w", err)
	}
	binding.Manifest = append([]byte(nil), manifest...)
	return binding, nil
}

func (r *PostgresRepository) InsertAndFinalizeBinding(
	ctx context.Context,
	tx pgx.Tx,
	binding ProductionBinding,
	members []ProductionMember,
	finalizedAt time.Time,
) error {
	if !json.Valid(binding.Manifest) || len(binding.ManifestHashPayload) == 0 {
		return fmt.Errorf("gold production binding manifest is invalid")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO gold_production_binding(
			id, workspace_id, execution_id, workflow_version_id,
			input_dataset_version_id, input_certification_id,
			annotation_campaign_id, annotation_snapshot_id, annotation_contribution_resource_id,
			output_dataset_version_id, input_checksum_sha256, snapshot_root_hash,
			schema_content_sha256, taxonomy_content_sha256, rubric_content_sha256,
			renderer_content_sha256, review_policy_content_sha256,
			output_checksum_sha256, output_row_count, manifest, manifest_hash_payload,
			root_hash, status, created_at, created_by
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,
			$20,$21,$22,'BUILDING',$23,$24
		)
	`, binding.ID, binding.WorkspaceID, binding.ExecutionID, binding.WorkflowVersionID,
		binding.InputDatasetVersionID, binding.InputCertificationID,
		binding.AnnotationCampaignID, binding.AnnotationSnapshotID,
		binding.AnnotationContributionResourceID, binding.OutputDatasetVersionID,
		binding.InputChecksumSHA256, binding.SnapshotRootHash,
		binding.SchemaContentSHA256, binding.TaxonomyContentSHA256,
		binding.RubricContentSHA256, binding.RendererContentSHA256,
		binding.ReviewPolicyContentSHA256, binding.OutputChecksumSHA256,
		binding.OutputRowCount, binding.Manifest, binding.ManifestHashPayload,
		binding.RootHash, binding.CreatedAt, binding.CreatedBy); err != nil {
		return fmt.Errorf("insert Gold production binding: %w", err)
	}

	for _, member := range members {
		if _, err := tx.Exec(ctx, `
			INSERT INTO gold_production_member(
				binding_id, task_id, source_item_ref, source_content_sha256,
				decision_id, outcome, reviewed_result_id, selected_result_id,
				selected_result_sha256, output_row_index
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		`, binding.ID, member.TaskID, member.SourceItemRef, member.SourceContentSHA256,
			member.DecisionID, member.Outcome, member.ReviewedResultID,
			member.SelectedResultID, member.SelectedResultSHA256, member.OutputRowIndex); err != nil {
			return fmt.Errorf("insert Gold production member: %w", err)
		}
	}

	tag, err := tx.Exec(ctx, `
		UPDATE gold_production_binding
		   SET status='FINALIZED', finalized_at=$2
		 WHERE id=$1 AND status='BUILDING'
	`, binding.ID, finalizedAt)
	if err != nil {
		return fmt.Errorf("finalize Gold production binding: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("finalize Gold production binding: stale state")
	}
	return nil
}
