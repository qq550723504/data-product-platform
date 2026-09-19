package infrastructure

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
)

// GetSuccessfulResolutionJobForOutput proves that the explicitly supplied
// STANDARDIZED version was produced from this workspace's enterprise RAW
// version by the declared source identity. It deliberately does not choose a
// job by recency.
func (r *PostgresRepository) GetSuccessfulResolutionJobForOutput(ctx context.Context, workspaceID, inputVersionID, outputVersionID uuid.UUID, sourceType, sourceRef string) (domain.MatchJob, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT j.id
		FROM entity_match_job j
		JOIN dataset d_in ON d_in.id=(SELECT dataset_id FROM dataset_version WHERE id=j.input_dataset_version_id)
		JOIN dataset d_out ON d_out.id=j.output_dataset_id
		WHERE j.workspace_id=$1
		  AND d_in.workspace_id=$1
		  AND d_out.workspace_id=$1
		  AND d_in.dataset_type='RAW'
		  AND d_out.dataset_type='STANDARDIZED'
		  AND j.input_dataset_version_id=$2
		  AND j.output_dataset_version_id=$3
		  AND j.source_type=$4
		  AND j.source_ref=$5
		  AND j.status='SUCCEEDED'
	`, workspaceID, inputVersionID, outputVersionID, sourceType, sourceRef)
	if err != nil {
		return domain.MatchJob{}, fmt.Errorf("prove entity-resolution output lineage: %w", err)
	}
	defer rows.Close()
	var jobID uuid.UUID
	count := 0
	for rows.Next() {
		if err := rows.Scan(&jobID); err != nil {
			return domain.MatchJob{}, fmt.Errorf("scan entity-resolution output lineage: %w", err)
		}
		count++
		if count > 1 {
			return domain.MatchJob{}, fmt.Errorf("enterprise_resolution has multiple successful resolution jobs for the same input, output and source")
		}
	}
	if err := rows.Err(); err != nil {
		return domain.MatchJob{}, fmt.Errorf("iterate entity-resolution output lineage: %w", err)
	}
	if count == 0 {
		return domain.MatchJob{}, ErrNotFound
	}
	return r.GetJob(ctx, jobID)
}

// ListResolutionOutputDecisions returns the exact decision associated with
// each source key in a frozen resolution output. A missing row is meaningful:
// the output cannot be used as a complete production dependency.
func (r *PostgresRepository) ListResolutionOutputDecisions(ctx context.Context, workspaceID, outputVersionID uuid.UUID) (map[string]domain.MappingDecision, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT d.id, d.workspace_id, d.mapping_id, d.entity_id, d.source_type, d.source_ref, d.source_key,
		       COALESCE(d.source_name,''), d.match_method, COALESCE(d.match_rule_id,''), d.match_policy_version,
		       d.match_engine_name, d.match_engine_version, d.match_model_version,
		       COALESCE(d.confidence,0), d.status, d.reviewed_by, d.reviewed_at,
		       COALESCE(d.reviewer_reason,''), d.evidence_id, d.decided_at, d.decided_by,
		       COALESCE(d.idempotency_key,''), d.source_origin, d.source_job_id, d.source_candidate_id
		FROM entity_resolution_output_decision rod
		JOIN entity_mapping_decision d
		  ON d.workspace_id=rod.workspace_id AND d.id=rod.decision_id
		WHERE rod.workspace_id=$1 AND rod.output_dataset_version_id=$2
		ORDER BY rod.source_key
	`, workspaceID, outputVersionID)
	if err != nil {
		return nil, fmt.Errorf("list resolution output decisions: %w", err)
	}
	defer rows.Close()
	result := make(map[string]domain.MappingDecision)
	for rows.Next() {
		decision, err := scanMappingDecision(rows)
		if err != nil {
			return nil, err
		}
		if _, exists := result[decision.SourceKey]; exists {
			return nil, fmt.Errorf("resolution output has multiple decisions for source key %s", decision.SourceKey)
		}
		result[decision.SourceKey] = decision
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate resolution output decisions: %w", err)
	}
	return result, nil
}

// GetMappingDecisionByCandidateTx is used during resolution finalization to
// bind the output row to the candidate decision that produced it.
func (r *PostgresRepository) GetMappingDecisionByCandidateTx(ctx context.Context, tx pgx.Tx, workspaceID, candidateID uuid.UUID) (domain.MappingDecision, error) {
	return scanMappingDecision(tx.QueryRow(ctx, `
		SELECT id, workspace_id, mapping_id, entity_id, source_type, source_ref, source_key,
		       COALESCE(source_name,''), match_method, COALESCE(match_rule_id,''), match_policy_version,
		       match_engine_name, match_engine_version, match_model_version,
		       COALESCE(confidence,0), status, reviewed_by, reviewed_at,
		       COALESCE(reviewer_reason,''), evidence_id, decided_at, decided_by,
		       COALESCE(idempotency_key,''), source_origin, source_job_id, source_candidate_id
		FROM entity_mapping_decision
		WHERE workspace_id=$1 AND source_candidate_id=$2
	`, workspaceID, candidateID))
}

func (r *PostgresRepository) InsertResolutionOutputDecision(ctx context.Context, tx pgx.Tx, outputVersionID uuid.UUID, job domain.MatchJob, candidate domain.MatchCandidate, decision domain.MappingDecision) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO entity_resolution_output_decision (
			output_dataset_version_id, workspace_id, source_job_id, source_type, source_ref,
			source_key, decision_id, entity_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, outputVersionID, job.WorkspaceID, job.ID, job.SourceType, job.SourceRef,
		candidate.SourceKey, decision.ID, decision.EntityID); err != nil {
		return fmt.Errorf("insert resolution output decision %s: %w", candidate.SourceKey, err)
	}
	return nil
}
