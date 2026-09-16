package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
)

func (r *PostgresRepository) InsertJob(ctx context.Context, tx pgx.Tx, job domain.MatchJob) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO entity_match_job (
			id, workspace_id, entity_type_id, input_dataset_version_id, output_dataset_id,
			source_type, source_ref, source_role, policy_ref, policy_version,
			status, created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	`, job.ID, job.WorkspaceID, job.EntityTypeID, job.InputDatasetVersionID, job.OutputDatasetID,
		job.SourceType, job.SourceRef, job.SourceRole, job.PolicyRef, job.PolicyVersion,
		job.Status, job.CreatedAt, job.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert entity match job: %w", err)
	}
	return nil
}

func (r *PostgresRepository) MarkJobRunning(ctx context.Context, tx pgx.Tx, jobID uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE entity_match_job SET status='RUNNING', started_at=now() WHERE id=$1 AND status='QUEUED'`, jobID)
	if err != nil {
		return fmt.Errorf("mark entity match job running: %w", err)
	}
	return nil
}

func (r *PostgresRepository) InsertCandidate(ctx context.Context, tx pgx.Tx, candidate domain.MatchCandidate) error {
	sourcePayload, err := json.Marshal(candidate.SourcePayload)
	if err != nil {
		return fmt.Errorf("marshal source payload: %w", err)
	}
	normalizedPayload, err := json.Marshal(candidate.NormalizedPayload)
	if err != nil {
		return fmt.Errorf("marshal normalized payload: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO entity_match_candidate (
			id, job_id, source_key, source_name, source_payload, normalized_payload,
			candidate_entity_id, decision, status, match_method, match_rule_id,
			confidence, reviewed_by, reviewed_at, reviewer_reason, evidence_id, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
	`, candidate.ID, candidate.JobID, candidate.SourceKey, candidate.SourceName, sourcePayload, normalizedPayload,
		candidate.CandidateEntityID, candidate.Decision, candidate.Status, candidate.MatchMethod,
		candidate.MatchRuleID, candidate.Confidence, candidate.ReviewedBy, candidate.ReviewedAt,
		candidate.ReviewerReason, candidate.EvidenceID, candidate.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert entity match candidate: %w", err)
	}
	return nil
}

func (r *PostgresRepository) RefreshJobCountsAndStatus(ctx context.Context, tx pgx.Tx, jobID uuid.UUID) (domain.JobStatus, error) {
	var autoCount, reviewCount, pendingCount, unresolvedCount, rejectedCount int64
	if err := tx.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE status='AUTO_CONFIRMED'),
			count(*) FILTER (WHERE decision='REVIEW'),
			count(*) FILTER (WHERE status='PENDING'),
			count(*) FILTER (WHERE status='UNRESOLVED'),
			count(*) FILTER (WHERE status='REJECTED')
		FROM entity_match_candidate WHERE job_id=$1
	`, jobID).Scan(&autoCount, &reviewCount, &pendingCount, &unresolvedCount, &rejectedCount); err != nil {
		return "", fmt.Errorf("count entity match candidates: %w", err)
	}
	status := domain.JobRunning
	if pendingCount > 0 {
		status = domain.JobWaitingReview
	}
	_, err := tx.Exec(ctx, `
		UPDATE entity_match_job
		SET status=$2, auto_match_count=$3, review_count=$4,
		    unresolved_count=$5, rejected_count=$6, finished_at=NULL
		WHERE id=$1
	`, jobID, status, autoCount, reviewCount, unresolvedCount, rejectedCount)
	if err != nil {
		return "", fmt.Errorf("update entity match job counts: %w", err)
	}
	return status, nil
}

func (r *PostgresRepository) GetJob(ctx context.Context, jobID uuid.UUID) (domain.MatchJob, error) {
	var job domain.MatchJob
	err := r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, entity_type_id, input_dataset_version_id, output_dataset_id,
		       source_type, source_ref, source_role, policy_ref, policy_version, status,
		       auto_match_count, review_count, unresolved_count, rejected_count,
		       output_dataset_version_id, COALESCE(error_message,''), created_at, created_by,
		       started_at, finished_at
		FROM entity_match_job WHERE id=$1
	`, jobID).Scan(
		&job.ID, &job.WorkspaceID, &job.EntityTypeID, &job.InputDatasetVersionID, &job.OutputDatasetID,
		&job.SourceType, &job.SourceRef, &job.SourceRole, &job.PolicyRef, &job.PolicyVersion, &job.Status,
		&job.AutoMatchCount, &job.ReviewCount, &job.UnresolvedCount, &job.RejectedCount,
		&job.OutputDatasetVersionID, &job.ErrorMessage, &job.CreatedAt, &job.CreatedBy,
		&job.StartedAt, &job.FinishedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.MatchJob{}, ErrNotFound
	}
	if err != nil {
		return domain.MatchJob{}, fmt.Errorf("get entity match job: %w", err)
	}
	return job, nil
}

func (r *PostgresRepository) GetCandidate(ctx context.Context, candidateID uuid.UUID) (domain.MatchCandidate, error) {
	var candidate domain.MatchCandidate
	var sourcePayload, normalizedPayload []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, job_id, source_key, COALESCE(source_name,''), source_payload, normalized_payload,
		       candidate_entity_id, decision, status, match_method, COALESCE(match_rule_id,''),
		       COALESCE(confidence,0), reviewed_by, reviewed_at, COALESCE(reviewer_reason,''), evidence_id, created_at
		FROM entity_match_candidate WHERE id=$1
	`, candidateID).Scan(
		&candidate.ID, &candidate.JobID, &candidate.SourceKey, &candidate.SourceName,
		&sourcePayload, &normalizedPayload, &candidate.CandidateEntityID, &candidate.Decision,
		&candidate.Status, &candidate.MatchMethod, &candidate.MatchRuleID, &candidate.Confidence,
		&candidate.ReviewedBy, &candidate.ReviewedAt, &candidate.ReviewerReason, &candidate.EvidenceID, &candidate.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.MatchCandidate{}, ErrNotFound
	}
	if err != nil {
		return domain.MatchCandidate{}, fmt.Errorf("get entity match candidate: %w", err)
	}
	if err := json.Unmarshal(sourcePayload, &candidate.SourcePayload); err != nil {
		return domain.MatchCandidate{}, fmt.Errorf("decode source payload: %w", err)
	}
	if err := json.Unmarshal(normalizedPayload, &candidate.NormalizedPayload); err != nil {
		return domain.MatchCandidate{}, fmt.Errorf("decode normalized payload: %w", err)
	}
	return candidate, nil
}

func (r *PostgresRepository) UpdateCandidateReview(ctx context.Context, tx pgx.Tx, candidate domain.MatchCandidate) error {
	commandTag, err := tx.Exec(ctx, `
		UPDATE entity_match_candidate
		SET status=$2, reviewed_by=$3, reviewed_at=$4, reviewer_reason=$5, evidence_id=$6
		WHERE id=$1 AND status='PENDING'
	`, candidate.ID, candidate.Status, candidate.ReviewedBy, candidate.ReviewedAt, candidate.ReviewerReason, candidate.EvidenceID)
	if err != nil {
		return fmt.Errorf("update entity match candidate review: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return domain.ErrCandidateNotReviewable
	}
	return nil
}

func (r *PostgresRepository) ListCandidates(ctx context.Context, jobID uuid.UUID) ([]domain.MatchCandidate, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, job_id, source_key, COALESCE(source_name,''), source_payload, normalized_payload,
		       candidate_entity_id, decision, status, match_method, COALESCE(match_rule_id,''),
		       COALESCE(confidence,0), reviewed_by, reviewed_at, COALESCE(reviewer_reason,''), evidence_id, created_at
		FROM entity_match_candidate WHERE job_id=$1 ORDER BY created_at, source_key
	`, jobID)
	if err != nil {
		return nil, fmt.Errorf("list entity match candidates: %w", err)
	}
	defer rows.Close()

	result := make([]domain.MatchCandidate, 0)
	for rows.Next() {
		var c domain.MatchCandidate
		var sourcePayload, normalizedPayload []byte
		if err := rows.Scan(&c.ID, &c.JobID, &c.SourceKey, &c.SourceName, &sourcePayload, &normalizedPayload,
			&c.CandidateEntityID, &c.Decision, &c.Status, &c.MatchMethod, &c.MatchRuleID, &c.Confidence,
			&c.ReviewedBy, &c.ReviewedAt, &c.ReviewerReason, &c.EvidenceID, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan entity match candidate: %w", err)
		}
		if err := json.Unmarshal(sourcePayload, &c.SourcePayload); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(normalizedPayload, &c.NormalizedPayload); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) CompleteJob(ctx context.Context, tx pgx.Tx, jobID, outputVersionID uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		UPDATE entity_match_job
		SET status='SUCCEEDED', output_dataset_version_id=$2, finished_at=now()
		WHERE id=$1 AND status='RUNNING'
	`, jobID, outputVersionID)
	if err != nil {
		return fmt.Errorf("complete entity match job: %w", err)
	}
	return nil
}
