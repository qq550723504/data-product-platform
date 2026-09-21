package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/certification/domain"
)

var ErrProfileNotFound = errors.New("certification profile not found")

type ProfileRepository struct {
	pool *pgxpool.Pool
}

func NewProfileRepository(pool *pgxpool.Pool) *ProfileRepository {
	return &ProfileRepository{pool: pool}
}

// InsertProfile creates and locks the parent in DRAFT state before writing its
// normalized membership rows. The parent is finalized only after all rows are
// present; child triggers lock the same parent and reject mutations after that
// transition.
func (r *ProfileRepository) InsertProfile(ctx context.Context, tx pgx.Tx, profile domain.ProfileSnapshot) error {
	if err := profile.Validate(); err != nil {
		return err
	}
	if profile.WorkspaceID == uuid.Nil {
		return fmt.Errorf("%w: workspace id is required", domain.ErrInvalidProfileSnapshot)
	}
	createdAt := profile.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	var rightsPurposeMode, rightsActionMode, rightsConsumerMode, rightsScopeMode any
	if profile.Rights.Required {
		rightsPurposeMode = string(profile.Rights.Purpose.Mode)
		rightsActionMode = string(profile.Rights.Actions.Mode)
		rightsConsumerMode = string(profile.Rights.Consumers.Mode)
		rightsScopeMode = string(profile.Rights.Scopes.Mode)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO certification_profile (
			id, workspace_id, profile_ref, code, name, version, content_sha256, content_snapshot,
			purpose_mode, action_mode, consumer_mode, delivery_mode,
			quality_gate_required, rights_required, compliance_required, contract_required,
			traceability_required, evidence_required, rights_purpose_mode, rights_action_mode,
			rights_consumer_mode, rights_scope_mode, membership_state, created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,'DRAFT',$23,$24)
	`, profile.ID, profile.WorkspaceID, profile.ProfileRef, profile.Code, profile.Name, profile.Version,
		profile.ContentSHA256, string(profile.Content), profile.Purpose.Mode, profile.Actions.Mode,
		profile.Consumers.Mode, profile.Delivery.Mode, profile.QualityGateRequired, profile.Rights.Required,
		profile.ComplianceRequired, profile.ContractRequired, profile.TraceabilityRequired, profile.EvidenceRequired,
		rightsPurposeMode, rightsActionMode, rightsConsumerMode, rightsScopeMode, createdAt, profile.CreatedBy); err != nil {
		return fmt.Errorf("insert certification profile: %w", err)
	}
	if _, err := tx.Exec(ctx, `SELECT id FROM certification_profile WHERE id=$1 FOR UPDATE`, profile.ID); err != nil {
		return fmt.Errorf("lock certification profile before membership insert: %w", err)
	}

	insert := func(query string, args ...any) error {
		if _, err := tx.Exec(ctx, query, args...); err != nil {
			return err
		}
		return nil
	}
	for _, value := range profile.Purpose.Values {
		if err := insert(`INSERT INTO certification_profile_purpose(profile_id, purpose_code) VALUES ($1,$2)`, profile.ID, value); err != nil {
			return fmt.Errorf("insert certification profile purpose: %w", err)
		}
	}
	for _, value := range profile.Actions.Values {
		if err := insert(`INSERT INTO certification_profile_action(profile_id, action) VALUES ($1,$2)`, profile.ID, value); err != nil {
			return fmt.Errorf("insert certification profile action: %w", err)
		}
	}
	for _, value := range profile.Consumers.Values {
		if err := insert(`INSERT INTO certification_profile_consumer(profile_id, consumer_ref) VALUES ($1,$2)`, profile.ID, value); err != nil {
			return fmt.Errorf("insert certification profile consumer: %w", err)
		}
	}
	for _, value := range profile.Delivery.Values {
		if err := insert(`INSERT INTO certification_profile_delivery(profile_id, delivery_channel) VALUES ($1,$2)`, profile.ID, value); err != nil {
			return fmt.Errorf("insert certification profile delivery: %w", err)
		}
	}
	for _, value := range profile.RequiredQualityDimensions {
		if err := insert(`INSERT INTO certification_profile_quality_dimension(profile_id, dimension) VALUES ($1,$2)`, profile.ID, value); err != nil {
			return fmt.Errorf("insert certification profile quality dimension: %w", err)
		}
	}
	for _, value := range profile.RequiredCriticalRules {
		if err := insert(`INSERT INTO certification_profile_critical_rule(profile_id, rule_id) VALUES ($1,$2)`, profile.ID, value); err != nil {
			return fmt.Errorf("insert certification profile critical rule: %w", err)
		}
	}
	if profile.Rights.Required {
		for _, value := range profile.Rights.Purpose.Values {
			if err := insert(`INSERT INTO certification_profile_rights_purpose(profile_id, purpose_code) VALUES ($1,$2)`, profile.ID, value); err != nil {
				return fmt.Errorf("insert certification profile rights purpose: %w", err)
			}
		}
		for _, value := range profile.Rights.Actions.Values {
			if err := insert(`INSERT INTO certification_profile_rights_action(profile_id, action) VALUES ($1,$2)`, profile.ID, value); err != nil {
				return fmt.Errorf("insert certification profile rights action: %w", err)
			}
		}
		for _, value := range profile.Rights.Consumers.Values {
			if err := insert(`INSERT INTO certification_profile_rights_consumer(profile_id, consumer_ref) VALUES ($1,$2)`, profile.ID, value); err != nil {
				return fmt.Errorf("insert certification profile rights consumer: %w", err)
			}
		}
		for _, value := range profile.Rights.Scopes.Values {
			if err := insert(`INSERT INTO certification_profile_rights_scope(profile_id, scope_type, scope_ref) VALUES ($1,$2,$3)`, profile.ID, value.Type, value.Ref); err != nil {
				return fmt.Errorf("insert certification profile rights scope: %w", err)
			}
		}
	}

	// The membership trigger serializes these writes on the same parent row.
	// Finalization is the only permitted parent mutation and the deferred
	// membership-completeness constraint validates the complete snapshot at commit.
	if _, err := tx.Exec(ctx, `UPDATE certification_profile SET membership_state='FINALIZED' WHERE id=$1`, profile.ID); err != nil {
		return fmt.Errorf("finalize certification profile membership: %w", err)
	}
	return nil
}

func (r *ProfileRepository) GetProfile(ctx context.Context, profileID uuid.UUID) (domain.ProfileSnapshot, error) {
	var snapshot domain.ProfileSnapshot
	var purposeMode, actionMode, consumerMode, deliveryMode string
	var rightsPurposeMode, rightsActionMode, rightsConsumerMode, rightsScopeMode *string
	var content string
	err := r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, profile_ref, code, name, version, content_sha256, content_snapshot,
		       purpose_mode, action_mode, consumer_mode, delivery_mode,
		       quality_gate_required, rights_required, compliance_required, contract_required,
		       traceability_required, evidence_required, rights_purpose_mode, rights_action_mode,
		       rights_consumer_mode, rights_scope_mode, created_at, created_by
		FROM certification_profile WHERE id=$1
	`, profileID).Scan(
		&snapshot.ID, &snapshot.WorkspaceID, &snapshot.ProfileRef, &snapshot.Code, &snapshot.Name, &snapshot.Version,
		&snapshot.ContentSHA256, &content, &purposeMode, &actionMode, &consumerMode, &deliveryMode,
		&snapshot.QualityGateRequired, &snapshot.Rights.Required, &snapshot.ComplianceRequired,
		&snapshot.ContractRequired, &snapshot.TraceabilityRequired, &snapshot.EvidenceRequired,
		&rightsPurposeMode, &rightsActionMode, &rightsConsumerMode, &rightsScopeMode, &snapshot.CreatedAt, &snapshot.CreatedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ProfileSnapshot{}, ErrProfileNotFound
	}
	if err != nil {
		return domain.ProfileSnapshot{}, fmt.Errorf("get certification profile: %w", err)
	}
	snapshot.Content = []byte(content)
	snapshot.Purpose.Mode = domain.ApplicabilityMode(purposeMode)
	snapshot.Actions.Mode = domain.ApplicabilityMode(actionMode)
	snapshot.Consumers.Mode = domain.ApplicabilityMode(consumerMode)
	snapshot.Delivery.Mode = domain.ApplicabilityMode(deliveryMode)
	if snapshot.Rights.Required {
		if rightsPurposeMode == nil || rightsActionMode == nil || rightsConsumerMode == nil || rightsScopeMode == nil {
			return domain.ProfileSnapshot{}, fmt.Errorf("%w: required rights applicability modes are incomplete", domain.ErrInvalidProfileSnapshot)
		}
		snapshot.Rights.Purpose.Mode = domain.ApplicabilityMode(*rightsPurposeMode)
		snapshot.Rights.Actions.Mode = domain.ApplicabilityMode(*rightsActionMode)
		snapshot.Rights.Consumers.Mode = domain.ApplicabilityMode(*rightsConsumerMode)
		snapshot.Rights.Scopes.Mode = domain.ApplicabilityMode(*rightsScopeMode)
	}

	if err := r.loadStrings(ctx, "certification_profile_purpose", "purpose_code", profileID, &snapshot.Purpose.Values); err != nil {
		return domain.ProfileSnapshot{}, err
	}
	if err := r.loadStrings(ctx, "certification_profile_action", "action", profileID, &snapshot.Actions.Values); err != nil {
		return domain.ProfileSnapshot{}, err
	}
	if err := r.loadStrings(ctx, "certification_profile_consumer", "consumer_ref", profileID, &snapshot.Consumers.Values); err != nil {
		return domain.ProfileSnapshot{}, err
	}
	if err := r.loadStrings(ctx, "certification_profile_delivery", "delivery_channel", profileID, &snapshot.Delivery.Values); err != nil {
		return domain.ProfileSnapshot{}, err
	}
	if err := r.loadStrings(ctx, "certification_profile_quality_dimension", "dimension", profileID, &snapshot.RequiredQualityDimensions); err != nil {
		return domain.ProfileSnapshot{}, err
	}
	if err := r.loadStrings(ctx, "certification_profile_critical_rule", "rule_id", profileID, &snapshot.RequiredCriticalRules); err != nil {
		return domain.ProfileSnapshot{}, err
	}
	if snapshot.Rights.Required {
		if err := r.loadStrings(ctx, "certification_profile_rights_purpose", "purpose_code", profileID, &snapshot.Rights.Purpose.Values); err != nil {
			return domain.ProfileSnapshot{}, err
		}
		if err := r.loadStrings(ctx, "certification_profile_rights_action", "action", profileID, &snapshot.Rights.Actions.Values); err != nil {
			return domain.ProfileSnapshot{}, err
		}
		if err := r.loadStrings(ctx, "certification_profile_rights_consumer", "consumer_ref", profileID, &snapshot.Rights.Consumers.Values); err != nil {
			return domain.ProfileSnapshot{}, err
		}
		rows, err := r.pool.Query(ctx, `SELECT scope_type, scope_ref FROM certification_profile_rights_scope WHERE profile_id=$1 ORDER BY scope_type, scope_ref`, profileID)
		if err != nil {
			return domain.ProfileSnapshot{}, fmt.Errorf("load certification profile rights scopes: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var scope domain.ScopeRef
			if err := rows.Scan(&scope.Type, &scope.Ref); err != nil {
				return domain.ProfileSnapshot{}, fmt.Errorf("scan certification profile rights scope: %w", err)
			}
			snapshot.Rights.Scopes.Values = append(snapshot.Rights.Scopes.Values, scope)
		}
		if err := rows.Err(); err != nil {
			return domain.ProfileSnapshot{}, fmt.Errorf("iterate certification profile rights scopes: %w", err)
		}
	}
	if err := snapshot.Validate(); err != nil {
		return domain.ProfileSnapshot{}, fmt.Errorf("validate stored certification profile: %w", err)
	}
	return snapshot, nil
}

func (r *ProfileRepository) loadStrings(ctx context.Context, table, column string, profileID uuid.UUID, target *[]string) error {
	query := fmt.Sprintf("SELECT %s FROM %s WHERE profile_id=$1 ORDER BY %s", column, table, column)
	rows, err := r.pool.Query(ctx, query, profileID)
	if err != nil {
		return fmt.Errorf("load certification profile %s: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return fmt.Errorf("scan certification profile %s: %w", table, err)
		}
		*target = append(*target, value)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate certification profile %s: %w", table, err)
	}
	return nil
}
