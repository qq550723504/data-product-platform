package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/domain"
)

var ErrNotFound = errors.New("compliance result not found")
var ErrAssessmentAttemptConflict = errors.New("compliance assessment attempt already exists")

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) InsertResult(ctx context.Context, tx pgx.Tx, result domain.Result) error {
	summary, err := json.Marshal(result.Summary)
	if err != nil {
		return fmt.Errorf("marshal compliance summary: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO compliance_result (
			id, assessment_attempt_id, workspace_id, dataset_version_id, policy_ref, policy_version,
			gate_decision, summary, created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`, result.ID, result.AssessmentAttemptID, result.WorkspaceID, result.DatasetVersionID, result.PolicyRef,
		result.PolicyVersion, result.GateDecision, summary, result.CreatedAt, result.CreatedBy)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_compliance_result_assessment_attempt" {
			return ErrAssessmentAttemptConflict
		}
		return fmt.Errorf("insert compliance result: %w", err)
	}
	for _, finding := range result.Findings {
		if _, err := tx.Exec(ctx, `
			INSERT INTO compliance_finding (
				id, result_id, field_name, category, action, status, message, created_at
			) VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,NULLIF($7,''),$8)
		`, finding.ID, result.ID, finding.FieldName, finding.Category, finding.Action,
			finding.Status, finding.Message, finding.CreatedAt); err != nil {
			return fmt.Errorf("insert compliance finding %s: %w", finding.FieldName, err)
		}
	}
	return nil
}

func (r *PostgresRepository) GetResultByAssessmentAttempt(ctx context.Context, attemptID uuid.UUID) (domain.Result, error) {
	var result domain.Result
	var summary []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, assessment_attempt_id, workspace_id, dataset_version_id, policy_ref, policy_version,
		       gate_decision, summary, created_at, created_by
		FROM compliance_result
		WHERE assessment_attempt_id=$1
	`, attemptID).Scan(&result.ID, &result.AssessmentAttemptID, &result.WorkspaceID, &result.DatasetVersionID,
		&result.PolicyRef, &result.PolicyVersion, &result.GateDecision, &summary, &result.CreatedAt, &result.CreatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Result{}, ErrNotFound
	}
	if err != nil {
		return domain.Result{}, fmt.Errorf("get compliance result by assessment attempt: %w", err)
	}
	if err := json.Unmarshal(summary, &result.Summary); err != nil {
		return domain.Result{}, fmt.Errorf("decode compliance summary: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, result_id, field_name, COALESCE(category,''), action, status,
		       COALESCE(message,''), created_at
		FROM compliance_finding WHERE result_id=$1 ORDER BY created_at, field_name
	`, result.ID)
	if err != nil {
		return domain.Result{}, fmt.Errorf("list compliance findings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var finding domain.Finding
		if err := rows.Scan(&finding.ID, &finding.ResultID, &finding.FieldName, &finding.Category,
			&finding.Action, &finding.Status, &finding.Message, &finding.CreatedAt); err != nil {
			return domain.Result{}, fmt.Errorf("scan compliance finding: %w", err)
		}
		result.Findings = append(result.Findings, finding)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) GetResult(ctx context.Context, resultID uuid.UUID) (domain.Result, error) {
	var result domain.Result
	var summary []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, COALESCE(assessment_attempt_id,'00000000-0000-0000-0000-000000000000'::uuid),
		       workspace_id, dataset_version_id, policy_ref, policy_version,
		       gate_decision, summary, created_at, created_by
		FROM compliance_result WHERE id=$1
	`, resultID).Scan(&result.ID, &result.AssessmentAttemptID, &result.WorkspaceID, &result.DatasetVersionID, &result.PolicyRef,
		&result.PolicyVersion, &result.GateDecision, &summary, &result.CreatedAt, &result.CreatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Result{}, ErrNotFound
	}
	if err != nil {
		return domain.Result{}, fmt.Errorf("get compliance result: %w", err)
	}
	if err := json.Unmarshal(summary, &result.Summary); err != nil {
		return domain.Result{}, fmt.Errorf("decode compliance summary: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, result_id, field_name, COALESCE(category,''), action, status,
		       COALESCE(message,''), created_at
		FROM compliance_finding WHERE result_id=$1 ORDER BY created_at, field_name
	`, resultID)
	if err != nil {
		return domain.Result{}, fmt.Errorf("list compliance findings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var finding domain.Finding
		if err := rows.Scan(&finding.ID, &finding.ResultID, &finding.FieldName, &finding.Category,
			&finding.Action, &finding.Status, &finding.Message, &finding.CreatedAt); err != nil {
			return domain.Result{}, fmt.Errorf("scan compliance finding: %w", err)
		}
		result.Findings = append(result.Findings, finding)
	}
	return result, rows.Err()
}
