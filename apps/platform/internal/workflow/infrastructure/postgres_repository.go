package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
)

var ErrNotFound = errors.New("workflow object not found")

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) EnsureWorkflow(ctx context.Context, tx pgx.Tx, workflow domain.Workflow) (domain.Workflow, error) {
	var existing domain.Workflow
	err := tx.QueryRow(ctx, `
		SELECT id, workspace_id, code, name, COALESCE(description,''), status, created_at, created_by
		FROM workflow WHERE workspace_id=$1 AND code=$2
	`, workflow.WorkspaceID, workflow.Code).Scan(
		&existing.ID, &existing.WorkspaceID, &existing.Code, &existing.Name, &existing.Description,
		&existing.Status, &existing.CreatedAt, &existing.CreatedBy,
	)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Workflow{}, fmt.Errorf("find workflow: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO workflow (id, workspace_id, code, name, description, status, created_at, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, workflow.ID, workflow.WorkspaceID, workflow.Code, workflow.Name, workflow.Description, workflow.Status, workflow.CreatedAt, workflow.CreatedBy)
	if err != nil {
		return domain.Workflow{}, fmt.Errorf("insert workflow: %w", err)
	}
	return workflow, nil
}

func (r *PostgresRepository) InsertVersion(ctx context.Context, tx pgx.Tx, version domain.WorkflowVersion) error {
	definition, err := json.Marshal(version.Definition)
	if err != nil {
		return fmt.Errorf("marshal workflow definition: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO workflow_version (
			id, workflow_id, version, definition_ref, definition_sha256, definition, created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, version.ID, version.WorkflowID, version.Version, version.DefinitionRef, version.DefinitionSHA256, definition, version.CreatedAt, version.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert workflow version: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetVersion(ctx context.Context, versionID uuid.UUID) (domain.WorkflowVersion, error) {
	var version domain.WorkflowVersion
	var definition []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, workflow_id, version, definition_ref, definition_sha256, definition, created_at, created_by
		FROM workflow_version WHERE id=$1
	`, versionID).Scan(
		&version.ID, &version.WorkflowID, &version.Version, &version.DefinitionRef,
		&version.DefinitionSHA256, &definition, &version.CreatedAt, &version.CreatedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.WorkflowVersion{}, ErrNotFound
	}
	if err != nil {
		return domain.WorkflowVersion{}, fmt.Errorf("get workflow version: %w", err)
	}
	if err := json.Unmarshal(definition, &version.Definition); err != nil {
		return domain.WorkflowVersion{}, fmt.Errorf("decode workflow definition: %w", err)
	}
	return version, nil
}

func (r *PostgresRepository) ValidateExecutionReferences(ctx context.Context, tx pgx.Tx, workspaceID, workflowVersionID, outputDatasetID uuid.UUID, inputs []domain.InputBinding) error {
	// A reference existing is not proof it belongs to the declared workspace. Every
	// reference is resolved through its owning scope and compared with workspaceID
	// before the caller persists the Execution or dispatches a queue task.
	var workflowWorkspace uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT w.workspace_id
		FROM workflow_version wv
		JOIN workflow w ON w.id = wv.workflow_id
		WHERE wv.id=$1
	`, workflowVersionID).Scan(&workflowWorkspace); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("workflow version: %w", ErrNotFound)
		}
		return fmt.Errorf("validate workflow version workspace: %w", err)
	}
	if workflowWorkspace != workspaceID {
		return fmt.Errorf("workflow version: %w", domain.ErrWorkspaceMismatch)
	}

	var outputWorkspace uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT workspace_id FROM dataset WHERE id=$1 AND deleted_at IS NULL`, outputDatasetID).Scan(&outputWorkspace); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("output dataset: %w", ErrNotFound)
		}
		return fmt.Errorf("validate output dataset workspace: %w", err)
	}
	if outputWorkspace != workspaceID {
		return fmt.Errorf("output dataset: %w", domain.ErrWorkspaceMismatch)
	}

	for _, input := range inputs {
		var inputWorkspace uuid.UUID
		var status string
		if err := tx.QueryRow(ctx, `
			SELECT d.workspace_id, v.status
			FROM dataset_version v
			JOIN dataset d ON d.id = v.dataset_id
			WHERE v.id=$1
		`, input.DatasetVersionID).Scan(&inputWorkspace, &status); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("input %s: %w", input.Name, ErrNotFound)
			}
			return fmt.Errorf("validate input %s workspace: %w", input.Name, err)
		}
		if inputWorkspace != workspaceID {
			return fmt.Errorf("input %s: %w", input.Name, domain.ErrWorkspaceMismatch)
		}
		if status != "READY" && status != "SUPERSEDED" {
			return fmt.Errorf("input %s: %w (must be READY or SUPERSEDED, got %s)", input.Name, domain.ErrExecutionReferenceUnusable, status)
		}
	}
	return nil
}

// ValidateExecutionOwnership re-checks a persisted Execution right before a worker
// dispatches it. Executions queued before the ownership rule existed can still bind a
// foreign workflow, input or output, so delivery must not rely only on the check made by
// Create/Retry.
func (r *PostgresRepository) ValidateExecutionOwnership(ctx context.Context, execution domain.Execution) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin execution ownership validation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := r.ValidateExecutionReferences(ctx, tx, execution.WorkspaceID, execution.WorkflowVersionID, execution.OutputDatasetID, execution.Inputs); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) ValidateOutputVersion(ctx context.Context, tx pgx.Tx, outputDatasetID, outputVersionID uuid.UUID) error {
	var datasetID uuid.UUID
	var status string
	if err := tx.QueryRow(ctx, `SELECT dataset_id, status FROM dataset_version WHERE id=$1`, outputVersionID).Scan(&datasetID, &status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("output DatasetVersion: %w", ErrNotFound)
		}
		return fmt.Errorf("validate output DatasetVersion: %w", err)
	}
	if datasetID != outputDatasetID {
		return fmt.Errorf("output DatasetVersion belongs to dataset %s, expected %s", datasetID, outputDatasetID)
	}
	if status != "READY" && status != "SUPERSEDED" {
		return fmt.Errorf("output DatasetVersion must be immutable and usable (READY or SUPERSEDED), got %s", status)
	}
	return nil
}

func (r *PostgresRepository) InsertExecution(ctx context.Context, tx pgx.Tx, execution domain.Execution) error {
	metrics, err := json.Marshal(execution.Metrics)
	if err != nil {
		return fmt.Errorf("marshal execution metrics: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO execution (
			id, workspace_id, workflow_version_id, output_dataset_id, output_dataset_version_id,
			target_period, status, attempt, retry_of_execution_id, engine_type, engine_execution_id,
			error_code, error_message, metrics, created_at, created_by, started_at, finished_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
	`, execution.ID, execution.WorkspaceID, execution.WorkflowVersionID, execution.OutputDatasetID,
		execution.OutputDatasetVersionID, execution.TargetPeriod, execution.Status, execution.Attempt,
		execution.RetryOfExecutionID, execution.EngineType, nullableString(execution.EngineExecutionID),
		nullableString(execution.ErrorCode), nullableString(execution.ErrorMessage), metrics,
		execution.CreatedAt, execution.CreatedBy, execution.StartedAt, execution.FinishedAt)
	if err != nil {
		return fmt.Errorf("insert execution: %w", err)
	}
	for _, input := range execution.Inputs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO execution_input (execution_id, input_name, dataset_version_id)
			VALUES ($1,$2,$3)
		`, execution.ID, input.Name, input.DatasetVersionID); err != nil {
			return fmt.Errorf("insert execution input %s: %w", input.Name, err)
		}
	}
	return nil
}

func (r *PostgresRepository) GetExecution(ctx context.Context, executionID uuid.UUID) (domain.Execution, error) {
	var execution domain.Execution
	var metrics []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, workflow_version_id, output_dataset_id, output_dataset_version_id,
		       target_period, status, attempt, retry_of_execution_id, engine_type,
		       COALESCE(engine_execution_id,''), COALESCE(error_code,''), COALESCE(error_message,''),
		       metrics, created_at, created_by, started_at, finished_at
		FROM execution WHERE id=$1
	`, executionID).Scan(
		&execution.ID, &execution.WorkspaceID, &execution.WorkflowVersionID, &execution.OutputDatasetID,
		&execution.OutputDatasetVersionID, &execution.TargetPeriod, &execution.Status, &execution.Attempt,
		&execution.RetryOfExecutionID, &execution.EngineType, &execution.EngineExecutionID,
		&execution.ErrorCode, &execution.ErrorMessage, &metrics, &execution.CreatedAt, &execution.CreatedBy,
		&execution.StartedAt, &execution.FinishedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Execution{}, ErrNotFound
	}
	if err != nil {
		return domain.Execution{}, fmt.Errorf("get execution: %w", err)
	}
	if len(metrics) > 0 {
		if err := json.Unmarshal(metrics, &execution.Metrics); err != nil {
			return domain.Execution{}, fmt.Errorf("decode execution metrics: %w", err)
		}
	}
	rows, err := r.pool.Query(ctx, `SELECT input_name, dataset_version_id FROM execution_input WHERE execution_id=$1 ORDER BY input_name`, executionID)
	if err != nil {
		return domain.Execution{}, fmt.Errorf("list execution inputs: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var input domain.InputBinding
		if err := rows.Scan(&input.Name, &input.DatasetVersionID); err != nil {
			return domain.Execution{}, fmt.Errorf("scan execution input: %w", err)
		}
		execution.Inputs = append(execution.Inputs, input)
	}
	if err := rows.Err(); err != nil {
		return domain.Execution{}, fmt.Errorf("iterate execution inputs: %w", err)
	}
	return execution, nil
}

// SaveExecutionState is a compare-and-set transition. The expected status is
// part of the UPDATE predicate so stale workers cannot emit duplicate terminal
// facts after another worker has already completed the same Execution.
func (r *PostgresRepository) SaveExecutionState(ctx context.Context, tx pgx.Tx, execution domain.Execution, expectedStatus domain.ExecutionStatus) error {
	metrics, err := json.Marshal(execution.Metrics)
	if err != nil {
		return fmt.Errorf("marshal execution metrics: %w", err)
	}
	commandTag, err := tx.Exec(ctx, `
		UPDATE execution
		SET status=$2,
		    output_dataset_version_id=$3,
		    engine_type=$4,
		    engine_execution_id=$5,
		    error_code=$6,
		    error_message=$7,
		    metrics=$8,
		    started_at=$9,
		    finished_at=$10
		WHERE id=$1 AND status=$11
	`, execution.ID, execution.Status, execution.OutputDatasetVersionID, execution.EngineType,
		nullableString(execution.EngineExecutionID), nullableString(execution.ErrorCode), nullableString(execution.ErrorMessage),
		metrics, execution.StartedAt, execution.FinishedAt, expectedStatus)
	if err != nil {
		return fmt.Errorf("save execution state: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return domain.ErrInvalidTransition
	}
	return nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
