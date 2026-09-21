package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
)

var ErrEffectiveRightsIdempotentReplay = errors.New("effective rights idempotent replay")

func (r *PostgresRepository) InsertEffectiveRightsHeader(ctx context.Context, tx pgx.Tx, snapshot domain.EffectiveRightsSnapshot) error {
	tag, err := tx.Exec(ctx, `INSERT INTO effective_rights_snapshot(id,workspace_id,target_dataset_version_id,calculation_as_of,consumer_ref,purpose,calculation_rule_version,calculation_rule_hash,required_input_hash,status,root_hash,created_at,created_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'DRAFT',$10,$11,$12) ON CONFLICT (id) DO NOTHING`, snapshot.ID, snapshot.WorkspaceID, snapshot.TargetDatasetVersionID, snapshot.CalculationAsOf, snapshot.ConsumerRef, snapshot.Purpose, snapshot.CalculationRuleVersion, snapshot.CalculationRuleHash, snapshot.RequiredInputHash, snapshot.RootHash, snapshot.CreatedAt, snapshot.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert effective rights snapshot: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEffectiveRightsIdempotentReplay
	}
	return nil
}

func (r *PostgresRepository) InsertEffectiveRightsInput(ctx context.Context, tx pgx.Tx, input domain.EffectiveRightsInput, snapshotID uuid.UUID) error {
	_, err := tx.Exec(ctx, `INSERT INTO effective_rights_input(id,snapshot_id,input_dataset_version_id,data_resource_id,rights_snapshot_id,declaration_id,binding_id,input_hash) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, input.ID, snapshotID, input.InputDatasetVersionID, input.DataResourceID, input.RightsSnapshotID, input.DeclarationID, input.BindingID, input.InputHash)
	if err != nil {
		return fmt.Errorf("insert effective rights input: %w", err)
	}
	return nil
}

func (r *PostgresRepository) InsertEffectiveRightsAction(ctx context.Context, tx pgx.Tx, action domain.EffectiveRightsAction, snapshotID uuid.UUID) error {
	_, err := tx.Exec(ctx, `INSERT INTO effective_rights_action(id,snapshot_id,action,decision,reason,blocking_input_id) VALUES ($1,$2,$3,$4,$5,$6)`, action.ID, snapshotID, action.Action, action.Decision, action.Reason, action.BlockingInputID)
	if err != nil {
		return fmt.Errorf("insert effective rights action: %w", err)
	}
	for _, provenance := range action.Provenance {
		if _, err := tx.Exec(ctx, `INSERT INTO effective_rights_action_provenance(snapshot_id,action_id,input_id,declaration_id,binding_id) VALUES ($1,$2,$3,$4,$5)`, snapshotID, action.ID, provenance.InputID, provenance.DeclarationID, provenance.BindingID); err != nil {
			return fmt.Errorf("insert effective rights action provenance: %w", err)
		}
	}
	return nil
}

func (r *PostgresRepository) FinalizeEffectiveRights(ctx context.Context, tx pgx.Tx, snapshot domain.EffectiveRightsSnapshot) error {
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM effective_rights_snapshot WHERE id=$1 FOR UPDATE`, snapshot.ID).Scan(&status); err != nil {
		return err
	}
	if status != "DRAFT" {
		return fmt.Errorf("effective rights snapshot %s is not draft", snapshot.ID)
	}
	if _, err := tx.Exec(ctx, `UPDATE effective_rights_snapshot SET status='FINALIZED',finalized_at=now() WHERE id=$1`, snapshot.ID); err != nil {
		return fmt.Errorf("finalize effective rights snapshot: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetEffectiveRights(ctx context.Context, id uuid.UUID) (domain.EffectiveRightsSnapshot, error) {
	var s domain.EffectiveRightsSnapshot
	var status string
	var consumer string
	err := r.pool.QueryRow(ctx, `SELECT id,workspace_id,target_dataset_version_id,calculation_as_of,COALESCE(consumer_ref,''),purpose,calculation_rule_version,calculation_rule_hash,required_input_hash,status,COALESCE(root_hash,''),created_at,created_by FROM effective_rights_snapshot WHERE id=$1`, id).Scan(&s.ID, &s.WorkspaceID, &s.TargetDatasetVersionID, &s.CalculationAsOf, &consumer, &s.Purpose, &s.CalculationRuleVersion, &s.CalculationRuleHash, &s.RequiredInputHash, &status, &s.RootHash, &s.CreatedAt, &s.CreatedBy)
	if err != nil {
		return s, err
	}
	s.ConsumerRef = consumer
	s.Status = status
	rows, err := r.pool.Query(ctx, `SELECT id,input_dataset_version_id,data_resource_id,rights_snapshot_id,declaration_id,binding_id,input_hash FROM effective_rights_input WHERE snapshot_id=$1 ORDER BY input_dataset_version_id`, id)
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var i domain.EffectiveRightsInput
		if err := rows.Scan(&i.ID, &i.InputDatasetVersionID, &i.DataResourceID, &i.RightsSnapshotID, &i.DeclarationID, &i.BindingID, &i.InputHash); err != nil {
			rows.Close()
			return s, err
		}
		s.Inputs = append(s.Inputs, i)
	}
	rows.Close()
	rows, err = r.pool.Query(ctx, `SELECT id,action,decision,reason,blocking_input_id FROM effective_rights_action WHERE snapshot_id=$1 ORDER BY action`, id)
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var a domain.EffectiveRightsAction
		if err := rows.Scan(&a.ID, &a.Action, &a.Decision, &a.Reason, &a.BlockingInputID); err != nil {
			rows.Close()
			return s, err
		}
		s.Actions = append(s.Actions, a)
	}
	rows.Close()
	actionByID := make(map[uuid.UUID]*domain.EffectiveRightsAction, len(s.Actions))
	for i := range s.Actions {
		actionByID[s.Actions[i].ID] = &s.Actions[i]
	}
	rows, err = r.pool.Query(ctx, `SELECT action_id,input_id,declaration_id,binding_id FROM effective_rights_action_provenance WHERE snapshot_id=$1 ORDER BY action_id,input_id`, id)
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var actionID, inputID, declarationID uuid.UUID
		var bindingID *uuid.UUID
		if err := rows.Scan(&actionID, &inputID, &declarationID, &bindingID); err != nil {
			rows.Close()
			return s, err
		}
		if action := actionByID[actionID]; action != nil {
			action.Provenance = append(action.Provenance, domain.EffectiveRightsProvenance{InputID: inputID, DeclarationID: declarationID, BindingID: bindingID})
		}
	}
	rows.Close()
	return s, nil
}

func (r *PostgresRepository) GetEffectiveRightsSupportingEvidence(ctx context.Context, snapshotID uuid.UUID) (*uuid.UUID, error) {
	var evidenceID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT esi.evidence_id
		FROM evidence_snapshot es
		JOIN evidence_snapshot_item esi ON esi.snapshot_id=es.id AND esi.category='SUPPORTING_EVIDENCE'
		WHERE es.object_type='EFFECTIVE_RIGHTS_SNAPSHOT' AND es.object_id=$1
		ORDER BY esi.evidence_id
		LIMIT 1
	`, snapshotID).Scan(&evidenceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get effective rights supporting evidence: %w", err)
	}
	return &evidenceID, nil
}

func EffectiveRightsRootHash(snapshot domain.EffectiveRightsSnapshot) string {
	parts := []string{snapshot.TargetDatasetVersionID.String(), snapshot.CalculationAsOf.UTC().String(), snapshot.ConsumerRef, snapshot.Purpose, snapshot.CalculationRuleVersion, snapshot.CalculationRuleHash, snapshot.RequiredInputHash}
	for _, i := range snapshot.Inputs {
		parts = append(parts, "I|"+i.InputDatasetVersionID.String()+"|"+i.DataResourceID.String()+"|"+i.InputHash)
	}
	for _, a := range snapshot.Actions {
		blockingInputID := ""
		if a.BlockingInputID != nil {
			blockingInputID = a.BlockingInputID.String()
		}
		parts = append(parts, "A|"+a.Action+"|"+a.Decision+"|"+a.Reason+"|"+blockingInputID)
		for _, provenance := range a.Provenance {
			bindingID := ""
			if provenance.BindingID != nil {
				bindingID = provenance.BindingID.String()
			}
			parts = append(parts, "P|"+a.Action+"|"+provenance.InputID.String()+"|"+provenance.DeclarationID.String()+"|"+bindingID)
		}
	}
	b, _ := json.Marshal(parts)
	sum := sha256Sum(b)
	return sum
}

func sha256Sum(value []byte) string { h := sha256.Sum256(value); return fmt.Sprintf("%x", h[:]) }
