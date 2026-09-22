package cost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const QualityEngineInvocation = "QUALITY_ENGINE_INVOCATION"\nconst NativeEngineInvocation = "NATIVE_ENGINE_INVOCATION"

const (
	CertificationEvaluationActivity  = "CERTIFICATION_EVALUATION"
	CertificationDispositionActivity = "CERTIFICATION_DISPOSITION"
)

type Event struct {
	ID          uuid.UUID
	WorkspaceID uuid.UUID
	ExecutionID *uuid.UUID
	ActivityID  uuid.UUID
	CostType    string
	Quantity    float64
	Unit        string
	Amount      *float64
	Currency    string
	PricingMode string
	Metadata    map[string]any
	OccurredAt  time.Time
}

func Append(ctx context.Context, tx pgx.Tx, event Event) error {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	if event.PricingMode == "" {
		event.PricingMode = "POC_ESTIMATE"
	}
	if event.Metadata == nil {
		event.Metadata = map[string]any{}
	}
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("marshal cost metadata: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO cost_event (
			id, workspace_id, execution_id, activity_id, cost_type, quantity, unit,
			amount, currency, pricing_mode, metadata, occurred_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, event.ID, event.WorkspaceID, event.ExecutionID, nullableUUID(event.ActivityID), event.CostType, event.Quantity, event.Unit,
		event.Amount, nullable(event.Currency), event.PricingMode, metadata, event.OccurredAt)
	if err != nil {
		return fmt.Errorf("append cost event: %w", err)
	}
	return nil
}

// QualityAssessmentActivity is one physical quality-evaluation activity. The
// attempt ID is deliberately independent from the assessment ID: a retry that
// performs real work must be chargeable even when it belongs to the same
// top-level assessment command.
type QualityAssessmentActivity struct {
	WorkspaceID  uuid.UUID
	AssessmentID uuid.UUID
	AttemptID    uuid.UUID
	CostType     string
	Quantity     float64
	Unit         string
	Amount       *float64
	Currency     string
	PricingMode  string
	Metadata     map[string]any
	OccurredAt   time.Time
}

// QualityAssessmentAttemptActivity records the cost of a physical quality
// evaluation before a QualityAssessment exists. The attempt is a typed
// subject, so failed evaluations remain allocated and auditable.
type QualityAssessmentAttemptActivity struct {
	WorkspaceID uuid.UUID
	AttemptID   uuid.UUID
	CostType    string
	Quantity    float64
	Unit        string
	Amount      *float64
	Currency    string
	PricingMode string
	Metadata    map[string]any
	OccurredAt  time.Time
}

// CertificationActivity is an optional physical evaluation/approval activity.
// Exactly one typed subject is required. CostType is the component key in the
// existing (workspace, activity_id, cost_type) idempotency boundary.
type CertificationActivity struct {
	WorkspaceID     uuid.UUID
	CertificationID *uuid.UUID
	DispositionID   *uuid.UUID
	ActivityID      uuid.UUID
	CostType        string
	Quantity        float64
	Unit            string
	Amount          *float64
	Currency        string
	PricingMode     string
	Metadata        map[string]any
	OccurredAt      time.Time
}

func AppendCertificationActivity(ctx context.Context, tx pgx.Tx, activity CertificationActivity) error {
	if activity.WorkspaceID == uuid.Nil || activity.ActivityID == uuid.Nil {
		return errors.New("certification cost activity requires workspace and activity IDs")
	}
	if (activity.CertificationID == nil) == (activity.DispositionID == nil) {
		return errors.New("certification cost activity requires exactly one typed subject")
	}
	if activity.CostType == "" {
		activity.CostType = CertificationEvaluationActivity
		if activity.DispositionID != nil {
			activity.CostType = CertificationDispositionActivity
		}
	}
	if activity.Quantity <= 0 {
		return errors.New("certification cost activity quantity must be positive")
	}
	if activity.Unit == "" {
		activity.Unit = "certification"
	}
	if activity.PricingMode == "" {
		activity.PricingMode = "ACTUAL"
	}
	if activity.OccurredAt.IsZero() {
		activity.OccurredAt = time.Now().UTC()
	}
	if activity.Metadata == nil {
		activity.Metadata = map[string]any{}
	}
	metadata, err := json.Marshal(activity.Metadata)
	if err != nil {
		return fmt.Errorf("marshal certification cost metadata: %w", err)
	}

	var costEventID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO cost_event (
			id, workspace_id, execution_id, activity_id, cost_type, quantity, unit,
			amount, currency, pricing_mode, metadata, occurred_at
		) VALUES ($1,$2,NULL,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (workspace_id, activity_id, cost_type)
		WHERE activity_id IS NOT NULL DO NOTHING
		RETURNING id
	`, uuid.New(), activity.WorkspaceID, activity.ActivityID, activity.CostType,
		activity.Quantity, activity.Unit, activity.Amount, nullable(activity.Currency),
		activity.PricingMode, metadata, activity.OccurredAt).Scan(&costEventID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `
			SELECT id FROM cost_event WHERE workspace_id=$1 AND activity_id=$2 AND cost_type=$3
		`, activity.WorkspaceID, activity.ActivityID, activity.CostType).Scan(&costEventID)
	}
	if err != nil {
		return fmt.Errorf("append certification cost event: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO cost_allocation(id, cost_event_id, dataset_certification_id, certification_disposition_id)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (cost_event_id) DO NOTHING
	`, uuid.New(), costEventID, activity.CertificationID, activity.DispositionID); err != nil {
		return fmt.Errorf("allocate certification cost event: %w", err)
	}
	var allocatedCertificationID, allocatedDispositionID *uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT dataset_certification_id, certification_disposition_id
		FROM cost_allocation WHERE cost_event_id=$1
	`, costEventID).Scan(&allocatedCertificationID, &allocatedDispositionID); err != nil {
		return fmt.Errorf("verify certification cost allocation: %w", err)
	}
	if activity.CertificationID != nil && (allocatedCertificationID == nil || *allocatedCertificationID != *activity.CertificationID) {
		return fmt.Errorf("cost activity %s is allocated to a different certification", activity.ActivityID)
	}
	if activity.DispositionID != nil && (allocatedDispositionID == nil || *allocatedDispositionID != *activity.DispositionID) {
		return fmt.Errorf("cost activity %s is allocated to a different certification disposition", activity.ActivityID)
	}
	return nil
}

// AppendQualityAssessmentActivity records the cost and its typed assessment
// allocation in one transaction. A replay of the same physical attempt and
// component is a no-op; a new attempt gets a new activity_id and therefore a
// new cost fact.
func AppendQualityAssessmentActivity(ctx context.Context, tx pgx.Tx, activity QualityAssessmentActivity) error {
	if activity.WorkspaceID == uuid.Nil || activity.AssessmentID == uuid.Nil || activity.AttemptID == uuid.Nil {
		return errors.New("quality assessment cost activity requires workspace, assessment, and attempt IDs")
	}
	if activity.CostType == "" {
		activity.CostType = QualityEngineInvocation
	}
	if activity.Quantity <= 0 {
		return errors.New("quality assessment cost activity quantity must be positive")
	}
	if activity.Unit == "" {
		activity.Unit = "assessment"
	}
	if activity.PricingMode == "" {
		activity.PricingMode = "ACTUAL"
	}
	if activity.OccurredAt.IsZero() {
		activity.OccurredAt = time.Now().UTC()
	}
	if activity.Metadata == nil {
		activity.Metadata = map[string]any{}
	}
	metadata, err := json.Marshal(activity.Metadata)
	if err != nil {
		return fmt.Errorf("marshal quality assessment cost metadata: %w", err)
	}

	var costEventID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO cost_event (
			id, workspace_id, execution_id, activity_id, cost_type, quantity, unit,
			amount, currency, pricing_mode, metadata, occurred_at
		) VALUES ($1,$2,NULL,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (workspace_id, activity_id, cost_type)
		WHERE activity_id IS NOT NULL DO NOTHING
		RETURNING id
	`, uuid.New(), activity.WorkspaceID, activity.AttemptID, activity.CostType,
		activity.Quantity, activity.Unit, activity.Amount, nullable(activity.Currency),
		activity.PricingMode, metadata, activity.OccurredAt).Scan(&costEventID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `
			SELECT id FROM cost_event
			WHERE workspace_id=$1 AND activity_id=$2 AND cost_type=$3
		`, activity.WorkspaceID, activity.AttemptID, activity.CostType).Scan(&costEventID)
	}
	if err != nil {
		return fmt.Errorf("append quality assessment cost event: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO cost_allocation(id, cost_event_id, quality_assessment_id)
		VALUES ($1,$2,$3)
		ON CONFLICT (cost_event_id) DO NOTHING
	`, uuid.New(), costEventID, activity.AssessmentID)
	if err != nil {
		return fmt.Errorf("allocate quality assessment cost event: %w", err)
	}

	var allocatedAssessmentID *uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT quality_assessment_id
		FROM cost_allocation
		WHERE cost_event_id=$1
	`, costEventID).Scan(&allocatedAssessmentID); err != nil {
		return fmt.Errorf("verify quality assessment cost allocation: %w", err)
	}
	if allocatedAssessmentID == nil {
		return fmt.Errorf("cost activity %s is already allocated to a non-quality subject", activity.AttemptID)
	}
	if *allocatedAssessmentID != activity.AssessmentID {
		return fmt.Errorf("cost activity %s is already allocated to assessment %s", activity.AttemptID, *allocatedAssessmentID)
	}
	return nil
}

// AppendQualityAssessmentAttemptActivity persists one physical quality
// attempt's cost and its typed attempt allocation before evaluation starts.
// Replaying the same attempt and component is a no-op.
func AppendQualityAssessmentAttemptActivity(ctx context.Context, tx pgx.Tx, activity QualityAssessmentAttemptActivity) error {
	if activity.WorkspaceID == uuid.Nil || activity.AttemptID == uuid.Nil {
		return errors.New("quality assessment attempt cost activity requires workspace and attempt IDs")
	}
	if activity.CostType == "" {
		activity.CostType = QualityEngineInvocation
	}
	if activity.Quantity <= 0 {
		return errors.New("quality assessment attempt cost activity quantity must be positive")
	}
	if activity.Unit == "" {
		activity.Unit = "assessment"
	}
	if activity.PricingMode == "" {
		activity.PricingMode = "ACTUAL"
	}
	if activity.OccurredAt.IsZero() {
		activity.OccurredAt = time.Now().UTC()
	}
	if activity.Metadata == nil {
		activity.Metadata = map[string]any{}
	}
	metadata, err := json.Marshal(activity.Metadata)
	if err != nil {
		return fmt.Errorf("marshal quality assessment attempt cost metadata: %w", err)
	}

	var costEventID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO cost_event (
			id, workspace_id, execution_id, activity_id, cost_type, quantity, unit,
			amount, currency, pricing_mode, metadata, occurred_at
		) VALUES ($1,$2,NULL,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (workspace_id, activity_id, cost_type)
		WHERE activity_id IS NOT NULL DO NOTHING
		RETURNING id
	`, uuid.New(), activity.WorkspaceID, activity.AttemptID, activity.CostType,
		activity.Quantity, activity.Unit, activity.Amount, nullable(activity.Currency),
		activity.PricingMode, metadata, activity.OccurredAt).Scan(&costEventID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `
			SELECT id FROM cost_event
			WHERE workspace_id=$1 AND activity_id=$2 AND cost_type=$3
		`, activity.WorkspaceID, activity.AttemptID, activity.CostType).Scan(&costEventID)
	}
	if err != nil {
		return fmt.Errorf("append quality assessment attempt cost event: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO cost_allocation(id, cost_event_id, quality_assessment_attempt_id)
		VALUES ($1,$2,$3)
		ON CONFLICT (cost_event_id) DO NOTHING
	`, uuid.New(), costEventID, activity.AttemptID)
	if err != nil {
		return fmt.Errorf("allocate quality assessment attempt cost event: %w", err)
	}

	var allocatedAttemptID *uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT quality_assessment_attempt_id
		FROM cost_allocation
		WHERE cost_event_id=$1
	`, costEventID).Scan(&allocatedAttemptID); err != nil {
		return fmt.Errorf("verify quality assessment attempt cost allocation: %w", err)
	}
	if allocatedAttemptID == nil {
		return fmt.Errorf("cost activity %s is already allocated to a non-quality-attempt subject", activity.AttemptID)
	}
	if *allocatedAttemptID != activity.AttemptID {
		return fmt.Errorf("cost activity %s is already allocated to attempt %s", activity.AttemptID, *allocatedAttemptID)
	}
	return nil
}

func nullableUUID(value uuid.UUID) any {
	if value == uuid.Nil {
		return nil
	}
	return value
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
