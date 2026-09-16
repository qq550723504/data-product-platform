package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
)

var ErrNotFound = errors.New("quality result not found")

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) InsertResult(ctx context.Context, tx pgx.Tx, result domain.Result) error {
	metrics, err := json.Marshal(result.Metrics)
	if err != nil {
		return fmt.Errorf("marshal quality metrics: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO quality_result (
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
			gate_decision, metrics, created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, result.ID, result.WorkspaceID, result.DatasetVersionID, result.RuleSetRef,
		result.RuleSetVersion, result.GateDecision, metrics, result.CreatedAt, result.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert quality result: %w", err)
	}
	for _, finding := range result.Findings {
		observed, err := json.Marshal(finding.Observed)
		if err != nil {
			return fmt.Errorf("marshal quality finding observation: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO quality_finding (
				id, result_id, rule_id, dimension, severity, status, observed, message, created_at
			) VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,$7,NULLIF($8,''),$9)
		`, finding.ID, result.ID, finding.RuleID, finding.Dimension, finding.Severity,
			finding.Status, observed, finding.Message, finding.CreatedAt); err != nil {
			return fmt.Errorf("insert quality finding %s: %w", finding.RuleID, err)
		}
	}
	return nil
}

func (r *PostgresRepository) GetResult(ctx context.Context, resultID uuid.UUID) (domain.Result, error) {
	var result domain.Result
	var metrics []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
		       gate_decision, metrics, created_at, created_by
		FROM quality_result WHERE id=$1
	`, resultID).Scan(&result.ID, &result.WorkspaceID, &result.DatasetVersionID, &result.RuleSetRef,
		&result.RuleSetVersion, &result.GateDecision, &metrics, &result.CreatedAt, &result.CreatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Result{}, ErrNotFound
	}
	if err != nil {
		return domain.Result{}, fmt.Errorf("get quality result: %w", err)
	}
	if err := json.Unmarshal(metrics, &result.Metrics); err != nil {
		return domain.Result{}, fmt.Errorf("decode quality metrics: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, result_id, rule_id, COALESCE(dimension,''), severity, status,
		       observed, COALESCE(message,''), created_at
		FROM quality_finding WHERE result_id=$1 ORDER BY created_at, rule_id
	`, resultID)
	if err != nil {
		return domain.Result{}, fmt.Errorf("list quality findings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var finding domain.Finding
		var observed []byte
		if err := rows.Scan(&finding.ID, &finding.ResultID, &finding.RuleID, &finding.Dimension,
			&finding.Severity, &finding.Status, &observed, &finding.Message, &finding.CreatedAt); err != nil {
			return domain.Result{}, fmt.Errorf("scan quality finding: %w", err)
		}
		if err := json.Unmarshal(observed, &finding.Observed); err != nil {
			return domain.Result{}, fmt.Errorf("decode quality finding observation: %w", err)
		}
		result.Findings = append(result.Findings, finding)
	}
	return result, rows.Err()
}
