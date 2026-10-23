package infrastructure

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

func (r *PostgresRepository) DatasetVersionBelongsToWorkspace(ctx context.Context, workspaceID, datasetVersionID uuid.UUID) (bool, error) {
	var matches bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM dataset_version v
			JOIN dataset d ON d.id=v.dataset_id
			WHERE v.id=$1 AND d.workspace_id=$2 AND d.deleted_at IS NULL
		)
	`, datasetVersionID, workspaceID).Scan(&matches); err != nil {
		return false, fmt.Errorf("check quality DatasetVersion workspace: %w", err)
	}
	return matches, nil
}

// Validate the real DatasetVersion owner as well as the Quality row's scope
// before hydrating findings, evidence, audit, costs or an attempt outcome.
func (r *PostgresRepository) AssessmentBelongsToWorkspace(ctx context.Context, workspaceID, assessmentID uuid.UUID) (bool, error) {
	var matches bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM quality_result q
			JOIN dataset_version v ON v.id=q.dataset_version_id
			JOIN dataset d ON d.id=v.dataset_id
			WHERE q.id=$1 AND q.workspace_id=$2 AND d.workspace_id=$2 AND d.deleted_at IS NULL
		)
	`, assessmentID, workspaceID).Scan(&matches)
	return matches, err
}

func (r *PostgresRepository) AssessmentAttemptBelongsToWorkspace(ctx context.Context, workspaceID, attemptID uuid.UUID) (bool, error) {
	var matches bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM quality_assessment_attempt a
			JOIN dataset_version v ON v.id=a.dataset_version_id
			JOIN dataset d ON d.id=v.dataset_id
			LEFT JOIN quality_assessment_attempt_outcome o ON o.attempt_id=a.id
			LEFT JOIN quality_result q ON q.id=o.assessment_id
			WHERE a.id=$1 AND a.workspace_id=$2 AND d.workspace_id=$2 AND d.deleted_at IS NULL
			AND (o.assessment_id IS NULL OR (q.workspace_id=$2 AND q.dataset_version_id=a.dataset_version_id))
		)
	`, attemptID, workspaceID).Scan(&matches)
	return matches, err
}
