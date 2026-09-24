package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
)

var ErrEffectiveRightsIdempotentReplay = errors.New("effective rights idempotent replay")

type effectiveRightsQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

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
	var datasetVersionID any
	if input.InputDatasetVersionID != uuid.Nil {
		datasetVersionID = input.InputDatasetVersionID
	}
	_, err := tx.Exec(ctx, `INSERT INTO effective_rights_input(id,snapshot_id,dependency_kind,input_dataset_version_id,data_resource_id,rights_snapshot_id,declaration_id,binding_id,input_hash) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, input.ID, snapshotID, input.DependencyKind, datasetVersionID, input.DataResourceID, input.RightsSnapshotID, input.DeclarationID, input.BindingID, input.InputHash)
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
	var mismatchedProvenance bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM effective_rights_action_provenance p
			JOIN effective_rights_action a ON a.id=p.action_id
			JOIN effective_rights_input i ON i.id=p.input_id
			WHERE p.snapshot_id=$1
			  AND (a.snapshot_id<>p.snapshot_id OR i.snapshot_id<>p.snapshot_id)
		)
	`, snapshot.ID).Scan(&mismatchedProvenance); err != nil {
		return err
	}
	if mismatchedProvenance {
		return fmt.Errorf("effective rights snapshot %s has cross-snapshot provenance", snapshot.ID)
	}
	persisted, err := loadEffectiveRights(ctx, tx, snapshot.ID)
	if err != nil {
		return err
	}
	if persisted.RequiredInputHash != HashEffectiveInputs(persisted.Inputs) {
		return fmt.Errorf("effective rights snapshot %s input membership hash mismatch", snapshot.ID)
	}
	computedRoot := EffectiveRightsRootHash(persisted)
	if persisted.RootHash == "" || persisted.RootHash != computedRoot || persisted.RootHash != snapshot.RootHash {
		return fmt.Errorf("effective rights snapshot %s root hash mismatch", snapshot.ID)
	}
	if _, err := tx.Exec(ctx, `UPDATE effective_rights_snapshot SET status='FINALIZED',finalized_at=now() WHERE id=$1`, snapshot.ID); err != nil {
		return fmt.Errorf("finalize effective rights snapshot: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetEffectiveRights(ctx context.Context, id uuid.UUID) (domain.EffectiveRightsSnapshot, error) {
	return loadEffectiveRights(ctx, r.pool, id)
}

func (r *PostgresRepository) GetEffectiveRightsTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (domain.EffectiveRightsSnapshot, error) {
	return loadEffectiveRights(ctx, tx, id)
}

func loadEffectiveRights(ctx context.Context, q effectiveRightsQueryer, id uuid.UUID) (domain.EffectiveRightsSnapshot, error) {
	var s domain.EffectiveRightsSnapshot
	var status string
	var consumer string
	err := q.QueryRow(ctx, `SELECT id,workspace_id,target_dataset_version_id,calculation_as_of,COALESCE(consumer_ref,''),purpose,calculation_rule_version,calculation_rule_hash,required_input_hash,status,COALESCE(root_hash,''),created_at,created_by FROM effective_rights_snapshot WHERE id=$1`, id).Scan(&s.ID, &s.WorkspaceID, &s.TargetDatasetVersionID, &s.CalculationAsOf, &consumer, &s.Purpose, &s.CalculationRuleVersion, &s.CalculationRuleHash, &s.RequiredInputHash, &status, &s.RootHash, &s.CreatedAt, &s.CreatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, ErrNotFound
	}
	if err != nil {
		return s, err
	}
	s.ConsumerRef = consumer
	s.Status = status
	rows, err := q.Query(ctx, `SELECT id,dependency_kind,input_dataset_version_id,data_resource_id,rights_snapshot_id,declaration_id,binding_id,input_hash FROM effective_rights_input WHERE snapshot_id=$1 ORDER BY dependency_kind,COALESCE(input_dataset_version_id::text,''),data_resource_id`, id)
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var i domain.EffectiveRightsInput
		var datasetVersionID *uuid.UUID
		if err := rows.Scan(&i.ID, &i.DependencyKind, &datasetVersionID, &i.DataResourceID, &i.RightsSnapshotID, &i.DeclarationID, &i.BindingID, &i.InputHash); err != nil {
			rows.Close()
			return s, err
		}
		if datasetVersionID != nil {
			i.InputDatasetVersionID = *datasetVersionID
		}
		s.Inputs = append(s.Inputs, i)
	}
	rows.Close()
	rows, err = q.Query(ctx, `SELECT id,action,decision,reason,blocking_input_id FROM effective_rights_action WHERE snapshot_id=$1 ORDER BY action`, id)
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
	rows, err = q.Query(ctx, `SELECT action_id,input_id,declaration_id,binding_id FROM effective_rights_action_provenance WHERE snapshot_id=$1 ORDER BY action_id,input_id`, id)
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
	inputs := append([]domain.EffectiveRightsInput(nil), snapshot.Inputs...)
	sort.Slice(inputs, func(i, j int) bool {
		left := inputs[i].DependencyKind + "|" + inputs[i].InputDatasetVersionID.String() + "|" + inputs[i].DataResourceID.String()
		right := inputs[j].DependencyKind + "|" + inputs[j].InputDatasetVersionID.String() + "|" + inputs[j].DataResourceID.String()
		return left < right
	})
	for _, i := range inputs {
		parts = append(parts, "I|"+i.DependencyKind+"|"+i.InputDatasetVersionID.String()+"|"+i.DataResourceID.String()+"|"+i.InputHash)
	}
	actions := append([]domain.EffectiveRightsAction(nil), snapshot.Actions...)
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].Action < actions[j].Action
	})
	for _, a := range actions {
		blockingInputID := ""
		if a.BlockingInputID != nil {
			blockingInputID = a.BlockingInputID.String()
		}
		parts = append(parts, "A|"+a.Action+"|"+a.Decision+"|"+a.Reason+"|"+blockingInputID)
		provenanceEntries := append([]domain.EffectiveRightsProvenance(nil), a.Provenance...)
		sort.Slice(provenanceEntries, func(i, j int) bool {
			leftBinding, rightBinding := "", ""
			if provenanceEntries[i].BindingID != nil {
				leftBinding = provenanceEntries[i].BindingID.String()
			}
			if provenanceEntries[j].BindingID != nil {
				rightBinding = provenanceEntries[j].BindingID.String()
			}
			left := provenanceEntries[i].InputID.String() + "|" + provenanceEntries[i].DeclarationID.String() + "|" + leftBinding
			right := provenanceEntries[j].InputID.String() + "|" + provenanceEntries[j].DeclarationID.String() + "|" + rightBinding
			return left < right
		})
		for _, provenance := range provenanceEntries {
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
