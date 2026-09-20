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
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
)

var ErrNotFound = errors.New("quality assessment not found")

type PostgresRepository struct {
	pool *pgxpool.Pool
}

type AuditEvent struct {
	ID          uuid.UUID  `json:"id"`
	Action      string     `json:"action"`
	ObjectType  string     `json:"objectType"`
	ObjectID    uuid.UUID  `json:"objectId"`
	ActorType   string     `json:"actorType"`
	ActorID     *uuid.UUID `json:"actorId,omitempty"`
	BeforeState any        `json:"beforeState,omitempty"`
	AfterState  any        `json:"afterState,omitempty"`
	Reason      string     `json:"reason,omitempty"`
	TraceID     string     `json:"traceId,omitempty"`
	Metadata    any        `json:"metadata"`
	OccurredAt  time.Time  `json:"occurredAt"`
}

type AssessmentPage struct {
	Items  []domain.Assessment
	Limit  int
	Offset int
	Total  int
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) InsertResult(ctx context.Context, tx pgx.Tx, result domain.Assessment) error {
	metrics, err := json.Marshal(result.Metrics)
	if err != nil {
		return fmt.Errorf("marshal quality metrics: %w", err)
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
	// Findings are inserted before their parent assessment. Migration 019 makes
	// the FK deferred and rejects child inserts when the parent already exists;
	// this permits only the initial creation transaction and prevents later
	// append-only rewrites of an assessment's meaning.
	_, err = tx.Exec(ctx, `
		INSERT INTO quality_result (
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
			rule_set_content_sha256, rule_set_content, evaluator_name, evaluator_version,
			gate_decision, metrics, created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	`, result.ID, result.WorkspaceID, result.DatasetVersionID, result.RuleSetRef,
		result.RuleSetVersion, result.RuleSetContentSHA256, result.RuleSetContent,
		result.EvaluatorName, result.EvaluatorVersion, result.GateDecision, metrics,
		result.CreatedAt, result.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert quality result: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetResult(ctx context.Context, resultID uuid.UUID) (domain.Assessment, error) {
	return r.GetAssessment(ctx, resultID)
}

func (r *PostgresRepository) FindAssessmentIDByAttempt(ctx context.Context, workspaceID, attemptID uuid.UUID) (uuid.UUID, bool, error) {
	var assessmentID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT a.quality_assessment_id
		FROM cost_event e
		JOIN cost_allocation a ON a.cost_event_id=e.id
		WHERE e.workspace_id=$1
		  AND e.activity_id=$2
		  AND e.cost_type=$3
	`, workspaceID, attemptID, cost.QualityEngineInvocation).Scan(&assessmentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("find quality assessment by attempt: %w", err)
	}
	return assessmentID, true, nil
}

func (r *PostgresRepository) GetAssessment(ctx context.Context, assessmentID uuid.UUID) (domain.Assessment, error) {
	var result domain.Assessment
	var metrics []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
		       COALESCE(rule_set_content_sha256,''), COALESCE(rule_set_content,''),
		       COALESCE(evaluator_name,''), COALESCE(evaluator_version,''),
		       gate_decision, metrics, created_at, created_by
		FROM quality_result WHERE id=$1
	`, assessmentID).Scan(&result.ID, &result.WorkspaceID, &result.DatasetVersionID, &result.RuleSetRef,
		&result.RuleSetVersion, &result.RuleSetContentSHA256, &result.RuleSetContent,
		&result.EvaluatorName, &result.EvaluatorVersion, &result.GateDecision, &metrics,
		&result.CreatedAt, &result.CreatedBy)
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
	`, assessmentID)
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

func (r *PostgresRepository) ListAssessments(ctx context.Context, datasetVersionID uuid.UUID, limit, offset int) (AssessmentPage, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		return AssessmentPage{}, fmt.Errorf("assessment limit must be between 1 and 100")
	}
	if offset < 0 {
		return AssessmentPage{}, fmt.Errorf("assessment offset must be zero or greater")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
		       COALESCE(rule_set_content_sha256,''), COALESCE(rule_set_content,''),
		       COALESCE(evaluator_name,''), COALESCE(evaluator_version,''),
		       gate_decision, metrics, created_at, created_by,
		       count(*) OVER() AS total
		FROM quality_result
		WHERE dataset_version_id=$1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, datasetVersionID, limit, offset)
	if err != nil {
		return AssessmentPage{}, fmt.Errorf("list quality assessments: %w", err)
	}
	results := make([]domain.Assessment, 0)
	total := 0
	for rows.Next() {
		var result domain.Assessment
		var metrics []byte
		var rowTotal int
		if err := rows.Scan(&result.ID, &result.WorkspaceID, &result.DatasetVersionID, &result.RuleSetRef,
			&result.RuleSetVersion, &result.RuleSetContentSHA256, &result.RuleSetContent,
			&result.EvaluatorName, &result.EvaluatorVersion, &result.GateDecision, &metrics,
			&result.CreatedAt, &result.CreatedBy, &rowTotal); err != nil {
			return AssessmentPage{}, fmt.Errorf("scan quality assessment: %w", err)
		}
		if err := json.Unmarshal(metrics, &result.Metrics); err != nil {
			return AssessmentPage{}, fmt.Errorf("decode quality assessment metrics: %w", err)
		}
		total = rowTotal
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return AssessmentPage{}, fmt.Errorf("iterate quality assessments: %w", err)
	}
	rows.Close()
	if len(results) == 0 {
		if err := r.pool.QueryRow(ctx, `
			SELECT count(*) FROM quality_result WHERE dataset_version_id=$1
		`, datasetVersionID).Scan(&total); err != nil {
			return AssessmentPage{}, fmt.Errorf("count quality assessments: %w", err)
		}
	}
	if err := r.loadFindingsBatch(ctx, results); err != nil {
		return AssessmentPage{}, err
	}
	return AssessmentPage{Items: results, Limit: limit, Offset: offset, Total: total}, nil
}

func (r *PostgresRepository) LatestAssessment(ctx context.Context, datasetVersionID uuid.UUID) (domain.Assessment, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT id FROM quality_result
		WHERE dataset_version_id=$1
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, datasetVersionID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Assessment{}, ErrNotFound
	}
	if err != nil {
		return domain.Assessment{}, fmt.Errorf("find latest quality assessment: %w", err)
	}
	return r.GetAssessment(ctx, id)
}

func (r *PostgresRepository) ListAuditEvents(ctx context.Context, assessmentID uuid.UUID) ([]AuditEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, action, object_type, object_id, actor_type, actor_id,
		       before_state, after_state, COALESCE(reason,''), COALESCE(trace_id,''), metadata, occurred_at
		FROM audit_event
		WHERE object_type='QUALITY_RESULT' AND object_id=$1
		ORDER BY occurred_at, id
	`, assessmentID)
	if err != nil {
		return nil, fmt.Errorf("list quality assessment audit events: %w", err)
	}
	defer rows.Close()
	events := make([]AuditEvent, 0)
	for rows.Next() {
		var event AuditEvent
		var before, after, metadata []byte
		if err := rows.Scan(&event.ID, &event.Action, &event.ObjectType, &event.ObjectID,
			&event.ActorType, &event.ActorID, &before, &after, &event.Reason, &event.TraceID,
			&metadata, &event.OccurredAt); err != nil {
			return nil, fmt.Errorf("scan quality assessment audit event: %w", err)
		}
		if len(before) > 0 {
			if err := json.Unmarshal(before, &event.BeforeState); err != nil {
				return nil, fmt.Errorf("decode quality assessment audit before state: %w", err)
			}
		}
		if len(after) > 0 {
			if err := json.Unmarshal(after, &event.AfterState); err != nil {
				return nil, fmt.Errorf("decode quality assessment audit after state: %w", err)
			}
		}
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &event.Metadata); err != nil {
				return nil, fmt.Errorf("decode quality assessment audit metadata: %w", err)
			}
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate quality assessment audit events: %w", err)
	}
	return events, nil
}

func (r *PostgresRepository) loadFindings(ctx context.Context, result *domain.Assessment) error {
	rows, err := r.pool.Query(ctx, `
		SELECT id, result_id, rule_id, COALESCE(dimension,''), severity, status,
		       observed, COALESCE(message,''), created_at
		FROM quality_finding WHERE result_id=$1 ORDER BY created_at, rule_id
	`, result.ID)
	if err != nil {
		return fmt.Errorf("list quality findings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var finding domain.Finding
		var observed []byte
		if err := rows.Scan(&finding.ID, &finding.ResultID, &finding.RuleID, &finding.Dimension,
			&finding.Severity, &finding.Status, &observed, &finding.Message, &finding.CreatedAt); err != nil {
			return fmt.Errorf("scan quality finding: %w", err)
		}
		if err := json.Unmarshal(observed, &finding.Observed); err != nil {
			return fmt.Errorf("decode quality finding observation: %w", err)
		}
		result.Findings = append(result.Findings, finding)
	}
	return rows.Err()
}

func (r *PostgresRepository) loadFindingsBatch(ctx context.Context, results []domain.Assessment) error {
	if len(results) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(results))
	byResult := make(map[uuid.UUID][]domain.Finding, len(results))
	for _, result := range results {
		ids = append(ids, result.ID)
		byResult[result.ID] = nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, result_id, rule_id, COALESCE(dimension,''), severity, status,
		       observed, COALESCE(message,''), created_at
		FROM quality_finding
		WHERE result_id = ANY($1::uuid[])
		ORDER BY result_id, created_at, rule_id
	`, ids)
	if err != nil {
		return fmt.Errorf("list quality assessment findings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var finding domain.Finding
		var observed []byte
		if err := rows.Scan(&finding.ID, &finding.ResultID, &finding.RuleID, &finding.Dimension,
			&finding.Severity, &finding.Status, &observed, &finding.Message, &finding.CreatedAt); err != nil {
			return fmt.Errorf("scan quality assessment finding: %w", err)
		}
		if err := json.Unmarshal(observed, &finding.Observed); err != nil {
			return fmt.Errorf("decode quality assessment finding observation: %w", err)
		}
		byResult[finding.ResultID] = append(byResult[finding.ResultID], finding)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate quality assessment findings: %w", err)
	}
	for i := range results {
		results[i].Findings = byResult[results[i].ID]
	}
	return nil
}
