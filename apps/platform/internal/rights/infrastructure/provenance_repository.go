package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
)

func (r *PostgresRepository) InsertRightsDeclaration(ctx context.Context, tx pgx.Tx, declaration domain.RightsDeclaration) error {
	restrictions, err := json.Marshal(declaration.Restrictions)
	if err != nil {
		return fmt.Errorf("marshal rights declaration restrictions: %w", err)
	}
	var resourceWorkspace uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT workspace_id FROM data_resource WHERE id=$1`, declaration.DataResourceID).Scan(&resourceWorkspace); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("read rights declaration resource: %w", err)
	}
	if resourceWorkspace != declaration.WorkspaceID {
		return domain.ErrResourceWorkspace
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO rights_declaration (id,workspace_id,data_resource_id,claimant_ref,basis_type,basis_ref,consumer_scope_type,consumer_ref,effective_from,effective_to,restrictions,created_at,created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9,$10,$11,$12,$13)
	`, declaration.ID, declaration.WorkspaceID, declaration.DataResourceID, declaration.ClaimantRef, declaration.BasisType, declaration.BasisRef,
		declaration.ConsumerScopeType, declaration.ConsumerRef, declaration.EffectiveFrom, declaration.EffectiveTo, restrictions, declaration.CreatedAt, declaration.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert rights declaration: %w", err)
	}
	for _, party := range declaration.Parties {
		if _, err := tx.Exec(ctx, `INSERT INTO rights_declaration_party(id,declaration_id,party_ref,role) VALUES ($1,$2,$3,$4)`, uuid.New(), declaration.ID, party.PartyRef, party.Role); err != nil {
			return fmt.Errorf("insert declaration party: %w", err)
		}
	}
	for _, evidenceID := range declaration.EvidenceIDs {
		var evidenceWorkspace uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT workspace_id FROM evidence WHERE id=$1`, evidenceID).Scan(&evidenceWorkspace); err != nil || evidenceWorkspace != declaration.WorkspaceID {
			return domain.ErrInvalidRightsDeclaration
		}
		if _, err := tx.Exec(ctx, `INSERT INTO rights_declaration_evidence(declaration_id,evidence_id,relation_type) VALUES ($1,$2,'RIGHTS_BASIS')`, declaration.ID, evidenceID); err != nil {
			return fmt.Errorf("insert declaration evidence: %w", err)
		}
	}
	for _, permission := range declaration.Permissions {
		permissionID := uuid.New()
		if _, err := tx.Exec(ctx, `INSERT INTO rights_declaration_permission(id,declaration_id,permission_kind,action) VALUES ($1,$2,$3,$4)`, permissionID, declaration.ID, permission.Kind, permission.Action); err != nil {
			return fmt.Errorf("insert declaration action: %w", err)
		}
		if permission.Purpose != "" {
			if _, err := tx.Exec(ctx, `INSERT INTO rights_declaration_purpose(id,declaration_id,permission_id,permission_kind,purpose_code) VALUES ($1,$2,$3,$4,$5)`, uuid.New(), declaration.ID, permissionID, permission.Kind, permission.Purpose); err != nil {
				return fmt.Errorf("insert declaration purpose: %w", err)
			}
		}
		if permission.Scope.Ref != "" {
			if _, err := tx.Exec(ctx, `INSERT INTO rights_declaration_scope(id,declaration_id,permission_id,permission_kind,scope_type,scope_ref) VALUES ($1,$2,$3,$4,$5,$6)`, uuid.New(), declaration.ID, permissionID, permission.Kind, permission.Scope.Type, permission.Scope.Ref); err != nil {
				return fmt.Errorf("insert declaration scope: %w", err)
			}
		}
	}
	return nil
}

func (r *PostgresRepository) GetRightsDeclaration(ctx context.Context, id uuid.UUID) (domain.RightsDeclaration, error) {
	var d domain.RightsDeclaration
	var restrictions []byte
	err := r.pool.QueryRow(ctx, `SELECT id,workspace_id,data_resource_id,claimant_ref,basis_type,basis_ref,consumer_scope_type,COALESCE(consumer_ref,''),effective_from,effective_to,restrictions,created_at,created_by FROM rights_declaration WHERE id=$1`, id).
		Scan(&d.ID, &d.WorkspaceID, &d.DataResourceID, &d.ClaimantRef, &d.BasisType, &d.BasisRef, &d.ConsumerScopeType, &d.ConsumerRef, &d.EffectiveFrom, &d.EffectiveTo, &restrictions, &d.CreatedAt, &d.CreatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RightsDeclaration{}, ErrNotFound
	}
	if err != nil {
		return domain.RightsDeclaration{}, fmt.Errorf("get rights declaration: %w", err)
	}
	if err := json.Unmarshal(restrictions, &d.Restrictions); err != nil {
		return domain.RightsDeclaration{}, fmt.Errorf("decode declaration restrictions: %w", err)
	}
	rows, err := r.pool.Query(ctx, `SELECT evidence_id FROM rights_declaration_evidence WHERE declaration_id=$1 ORDER BY evidence_id`, id)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var evidenceID uuid.UUID
		if err := rows.Scan(&evidenceID); err != nil {
			rows.Close()
			return d, err
		}
		d.EvidenceIDs = append(d.EvidenceIDs, evidenceID)
	}
	rows.Close()
	rows, err = r.pool.Query(ctx, `SELECT party_ref,role FROM rights_declaration_party WHERE declaration_id=$1 ORDER BY party_ref,role`, id)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var p domain.RightsParty
		if err := rows.Scan(&p.PartyRef, &p.Role); err != nil {
			rows.Close()
			return d, err
		}
		d.Parties = append(d.Parties, p)
	}
	rows.Close()
	rows, err = r.pool.Query(ctx, `SELECT p.permission_kind,p.action,COALESCE(pu.purpose_code,''),COALESCE(sc.scope_type,''),COALESCE(sc.scope_ref,'') FROM rights_declaration_permission p LEFT JOIN rights_declaration_purpose pu ON pu.permission_id=p.id LEFT JOIN rights_declaration_scope sc ON sc.permission_id=p.id WHERE p.declaration_id=$1 ORDER BY p.permission_kind,p.action`, id)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var kind, action, purpose, scopeType, scopeRef string
		if err := rows.Scan(&kind, &action, &purpose, &scopeType, &scopeRef); err != nil {
			rows.Close()
			return d, err
		}
		var scope domain.NormalizedScope
		if scopeRef != "" {
			scope, _ = domain.NewNormalizedScope(scopeType, scopeRef)
		}
		d.Permissions = append(d.Permissions, domain.RightsPermission{Kind: kind, Action: action, Purpose: purpose, Scope: scope})
	}
	rows.Close()
	return d, nil
}

func (r *PostgresRepository) InsertDeclarationVerification(ctx context.Context, tx pgx.Tx, verification domain.RightsVerification) error {
	_, err := tx.Exec(ctx, `INSERT INTO rights_declaration_verification(id,declaration_id,outcome,reason,evidence_id,occurred_at,actor_id,activity_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, verification.ID, verification.DeclarationID, verification.Outcome, verification.Reason, verification.EvidenceID, verification.OccurredAt, verification.ActorID, verification.ActivityID)
	if err != nil {
		return fmt.Errorf("insert rights declaration verification: %w", err)
	}
	return nil
}

func (r *PostgresRepository) InsertDeclarationDisposition(ctx context.Context, tx pgx.Tx, disposition domain.RightsDisposition) error {
	_, err := tx.Exec(ctx, `INSERT INTO rights_declaration_disposition(id,declaration_id,disposition,effective_at,reason,superseded_by_declaration_id,evidence_id,activity_id,actor_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, disposition.ID, disposition.DeclarationID, disposition.Disposition, disposition.EffectiveAt, disposition.Reason, disposition.SupersededBy, disposition.EvidenceID, disposition.ActivityID, disposition.ActorID)
	if err != nil {
		return fmt.Errorf("insert rights declaration disposition: %w", err)
	}
	return nil
}

func (r *PostgresRepository) InsertBinding(ctx context.Context, tx pgx.Tx, binding domain.AuthorizationProvenanceBinding, asOf time.Time) error {
	if binding.AuthorityMode != domain.AuthorityDirect && binding.AuthorityMode != domain.AuthorityDelegated {
		return domain.ErrInvalidBinding
	}
	var authWorkspace, resourceID uuid.UUID
	var grantor, grantee, purpose string
	var status domain.AuthorizationStatus
	var validFrom, validTo *time.Time
	if err := tx.QueryRow(ctx, `SELECT workspace_id,grantor_ref,grantee_ref,purpose,status,valid_from,valid_to FROM data_authorization WHERE id=$1 FOR SHARE`, binding.AuthorizationID).Scan(&authWorkspace, &grantor, &grantee, &purpose, &status, &validFrom, &validTo); err != nil {
		return fmt.Errorf("read binding authorization: %w", err)
	}
	if authWorkspace != binding.WorkspaceID || strings.TrimSpace(grantor) != strings.TrimSpace(binding.GrantorRef) {
		return domain.ErrInvalidBinding
	}
	if status == domain.StatusRevoked || status == domain.StatusSuspended || status == domain.StatusExpired || status == domain.StatusRejected {
		return domain.ErrInvalidBinding
	}
	if validFrom != nil && asOf.Before(*validFrom) || validTo != nil && !asOf.Before(*validTo) {
		return domain.ErrInvalidBinding
	}
	var actions []string
	var scopeType, scopeRef string
	if err := tx.QueryRow(ctx, `SELECT data_resource_id,actions,scope_type,scope_ref FROM authorization_resource WHERE authorization_id=$1 AND data_resource_id=$2 AND scope_type IS NOT NULL`, binding.AuthorizationID, binding.DataResourceID).Scan(&resourceID, &actions, &scopeType, &scopeRef); err != nil {
		return domain.ErrInvalidBinding
	}
	if resourceID != binding.DataResourceID {
		return domain.ErrInvalidBinding
	}
	var declarationWorkspace, declarationResource uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT workspace_id,data_resource_id FROM rights_declaration WHERE id=$1`, binding.DeclarationID).Scan(&declarationWorkspace, &declarationResource); err != nil {
		return domain.ErrInvalidBinding
	}
	if declarationWorkspace != binding.WorkspaceID || declarationResource != binding.DataResourceID {
		return domain.ErrInvalidBinding
	}
	var verified bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM rights_declaration_verification v WHERE v.declaration_id=$1 AND v.outcome='VERIFIED') AND NOT EXISTS(SELECT 1 FROM rights_declaration_disposition x WHERE x.declaration_id=$1 AND x.effective_at <= $2) AND EXISTS(SELECT 1 FROM rights_declaration d WHERE d.id=$1 AND (d.effective_from IS NULL OR d.effective_from <= $2) AND (d.effective_to IS NULL OR d.effective_to > $2))`, binding.DeclarationID, asOf).Scan(&verified); err != nil || !verified {
		return domain.ErrDeclarationNotVerified
	}
	var supported bool
	if err := tx.QueryRow(ctx, `
		SELECT NOT EXISTS(
			SELECT 1
			FROM unnest($2::text[]) requested_action
			WHERE NOT EXISTS (
				SELECT 1 FROM rights_declaration_permission p
				WHERE p.declaration_id=$1 AND p.permission_kind='GRANT' AND p.action=requested_action
				  AND EXISTS (SELECT 1 FROM rights_declaration_purpose pu WHERE pu.permission_id=p.id AND pu.purpose_code=$3)
				  AND EXISTS (SELECT 1 FROM rights_declaration_scope sc WHERE sc.permission_id=p.id AND sc.scope_type=$4 AND sc.scope_ref=$5)
			)
		)`, binding.DeclarationID, actions, purpose, scopeType, scopeRef).Scan(&supported); err != nil || !supported {
		return domain.ErrInvalidBinding
	}
	if binding.AuthorityMode == domain.AuthorityDirect {
		var partySupported bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM rights_declaration_party WHERE declaration_id=$1 AND party_ref=$2 AND role IN ('RIGHTS_HOLDER','PROVIDER','CONTROLLER'))`, binding.DeclarationID, binding.GrantorRef).Scan(&partySupported); err != nil || !partySupported {
			return domain.ErrInvalidBinding
		}
	} else {
		if binding.DelegationChainID == nil || strings.TrimSpace(binding.DelegationChainHash) == "" {
			return domain.ErrInvalidBinding
		}
		var chainWorkspace uuid.UUID
		var chainStatus, chainHash string
		var sourceDeclaration uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT workspace_id,status,COALESCE(chain_hash,''),source_declaration_id FROM grantor_authority_delegation_chain WHERE id=$1`, *binding.DelegationChainID).Scan(&chainWorkspace, &chainStatus, &chainHash, &sourceDeclaration); err != nil {
			return domain.ErrInvalidBinding
		}
		if chainWorkspace != binding.WorkspaceID || chainStatus != "FINALIZED" || chainHash != binding.DelegationChainHash || sourceDeclaration != binding.DeclarationID {
			return domain.ErrInvalidBinding
		}
		var sourceResource uuid.UUID
		var sourceClaimant string
		var sourceCurrent bool
		if err := tx.QueryRow(ctx, `
			SELECT d.data_resource_id,d.claimant_ref,
			       EXISTS(
					SELECT 1 FROM rights_declaration_verification v
					WHERE v.declaration_id=d.id AND v.outcome='VERIFIED'
					  AND (d.effective_from IS NULL OR d.effective_from <= $2)
					  AND (d.effective_to IS NULL OR d.effective_to > $2)
					  AND NOT EXISTS (SELECT 1 FROM rights_declaration_disposition x WHERE x.declaration_id=d.id AND x.effective_at <= $2)
				)
			FROM rights_declaration d WHERE d.id=$1`, sourceDeclaration, asOf).Scan(&sourceResource, &sourceClaimant, &sourceCurrent); err != nil || !sourceCurrent || sourceResource != binding.DataResourceID {
			return domain.ErrInvalidBinding
		}
		var sourceParty bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM rights_declaration_party WHERE declaration_id=$1 AND party_ref=$2 AND role IN ('RIGHTS_HOLDER','PROVIDER','CONTROLLER'))`, sourceDeclaration, sourceClaimant).Scan(&sourceParty); err != nil || !sourceParty {
			return domain.ErrInvalidBinding
		}
		var lastDelegate string
		rows, err := tx.Query(ctx, `SELECT delegator_ref,delegate_ref,data_resource_id,grantable_actions,grantable_purposes,scope_type,scope_ref FROM grantor_authority_delegation_edge WHERE chain_id=$1 ORDER BY ordinal`, *binding.DelegationChainID)
		if err != nil {
			return domain.ErrInvalidBinding
		}
		deferred := false
		var previous string
		var firstDelegator string
		for rows.Next() {
			var delegator, delegate, typ, ref string
			var edgeResource uuid.UUID
			var edgeActions, edgePurposes []string
			if err := rows.Scan(&delegator, &delegate, &edgeResource, &edgeActions, &edgePurposes, &typ, &ref); err != nil {
				rows.Close()
				return domain.ErrInvalidBinding
			}
			if previous != "" && previous != delegator {
				rows.Close()
				return domain.ErrInvalidBinding
			}
			if firstDelegator == "" {
				firstDelegator = delegator
			}
			if edgeResource != binding.DataResourceID || delegate == "" || delegator == delegate || typ != scopeType || ref != scopeRef {
				rows.Close()
				return domain.ErrInvalidBinding
			}
			for _, action := range actions {
				found := false
				for _, allowed := range edgeActions {
					if allowed == action {
						found = true
						break
					}
				}
				if !found {
					rows.Close()
					return domain.ErrInvalidBinding
				}
			}
			foundPurpose := false
			for _, allowed := range edgePurposes {
				if allowed == purpose {
					foundPurpose = true
					break
				}
			}
			if !foundPurpose {
				rows.Close()
				return domain.ErrInvalidBinding
			}
			previous = delegate
			lastDelegate = delegate
			deferred = true
		}
		rows.Close()
		if !deferred || firstDelegator != sourceClaimant || lastDelegate != binding.GrantorRef {
			return domain.ErrInvalidBinding
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO authorization_provenance_binding(id,workspace_id,authorization_id,data_resource_id,rights_declaration_id,grantor_ref,grantor_authority_mode,delegation_chain_id,delegation_chain_hash,created_at,created_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, binding.ID, binding.WorkspaceID, binding.AuthorizationID, binding.DataResourceID, binding.DeclarationID, binding.GrantorRef, binding.AuthorityMode, binding.DelegationChainID, nullableString(binding.DelegationChainHash), binding.CreatedAt, binding.CreatedBy); err != nil {
		return fmt.Errorf("insert authorization provenance binding: %w", err)
	}
	return nil
}

func (r *PostgresRepository) InsertBindingDisposition(ctx context.Context, tx pgx.Tx, disposition domain.BindingDisposition) error {
	_, err := tx.Exec(ctx, `INSERT INTO authorization_provenance_binding_disposition(id,binding_id,disposition,effective_at,reason,superseded_by_binding_id,evidence_id,activity_id,actor_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, disposition.ID, disposition.BindingID, disposition.Disposition, disposition.EffectiveAt, disposition.Reason, disposition.SupersededBy, disposition.EvidenceID, disposition.ActivityID, disposition.ActorID)
	if err != nil {
		return fmt.Errorf("insert binding disposition: %w", err)
	}
	return nil
}

func (r *PostgresRepository) CheckCurrentEntitlement(ctx context.Context, request domain.EntitlementRequest) (domain.EntitlementDecision, error) {
	if request.AsOf.IsZero() {
		request.AsOf = time.Now().UTC()
	}
	request.Action = strings.ToUpper(strings.TrimSpace(request.Action))
	request.Purpose = strings.TrimSpace(request.Purpose)
	var decision domain.EntitlementDecision
	decision.AuthorizationID = request.AuthorizationID
	decision.DataResourceID = request.DataResourceID
	if request.Path == domain.EntitlementDirectUse {
		var declarationID uuid.UUID
		err := r.pool.QueryRow(ctx, `
			SELECT d.id
			FROM rights_declaration d
			JOIN rights_declaration_verification v ON v.declaration_id=d.id AND v.outcome='VERIFIED'
			JOIN rights_declaration_permission p ON p.declaration_id=d.id AND p.permission_kind='USE' AND p.action=$5
			JOIN rights_declaration_purpose pu ON pu.permission_id=p.id AND pu.purpose_code=$4
			JOIN rights_declaration_scope sc ON sc.permission_id=p.id AND sc.scope_type=$6 AND sc.scope_ref=$7
			WHERE d.workspace_id=$1 AND d.data_resource_id=$2
			  AND (d.consumer_scope_type='ANY' OR (d.consumer_scope_type='EXPLICIT' AND d.consumer_ref=$3))
			  AND (d.effective_from IS NULL OR d.effective_from <= $8) AND (d.effective_to IS NULL OR d.effective_to > $8)
			  AND NOT EXISTS (SELECT 1 FROM rights_declaration_disposition x WHERE x.declaration_id=d.id AND x.effective_at <= $8)
			ORDER BY d.created_at,d.id LIMIT 1
		`, request.WorkspaceID, request.DataResourceID, request.ConsumerRef, request.Purpose, request.Action, request.Scope.Type, request.Scope.Ref, request.AsOf).Scan(&declarationID)
		if errors.Is(err, pgx.ErrNoRows) {
			decision.Decision = domain.DecisionNotAllowed
			decision.Reason = "no current direct-use declaration covers the requested context"
			return decision, nil
		}
		if err != nil {
			return decision, fmt.Errorf("check direct-use entitlement: %w", err)
		}
		decision.Decision = domain.DecisionAllowed
		decision.DeclarationID = &declarationID
		decision.Reason = "current verified declaration covers direct use"
		return decision, nil
	}
	query := `
		SELECT b.id,d.id
		FROM authorization_provenance_binding b
		JOIN data_authorization a ON a.id=b.authorization_id
		JOIN authorization_resource ar ON ar.authorization_id=a.id AND ar.data_resource_id=b.data_resource_id
		JOIN rights_declaration d ON d.id=b.rights_declaration_id
		JOIN rights_declaration_verification v ON v.declaration_id=d.id AND v.outcome='VERIFIED'
		JOIN rights_declaration_permission gp ON gp.declaration_id=d.id AND gp.permission_kind='GRANT' AND gp.action=$9
		JOIN rights_declaration_purpose gpurpose ON gpurpose.permission_id=gp.id AND gpurpose.purpose_code=$5
		JOIN rights_declaration_scope gscope ON gscope.permission_id=gp.id AND gscope.scope_type=$7 AND gscope.scope_ref=$8
		WHERE b.workspace_id=$1 AND b.data_resource_id=$2 AND ($3::uuid='00000000-0000-0000-0000-000000000000' OR b.authorization_id=$3)
		  AND a.status='ACTIVE' AND a.grantee_ref=$4 AND a.purpose=$5
		  AND (a.valid_from IS NULL OR a.valid_from <= $6) AND (a.valid_to IS NULL OR a.valid_to > $6)
		  AND ar.scope_type=$7 AND ar.scope_ref=$8 AND $9 = ANY(ar.actions)
		  AND (d.effective_from IS NULL OR d.effective_from <= $6) AND (d.effective_to IS NULL OR d.effective_to > $6)
		  AND NOT EXISTS (SELECT 1 FROM rights_declaration_disposition x WHERE x.declaration_id=d.id AND x.effective_at <= $6)
		  AND NOT EXISTS (SELECT 1 FROM authorization_provenance_binding_disposition x WHERE x.binding_id=b.id AND x.effective_at <= $6)
		  AND (b.grantor_authority_mode='DIRECT_DECLARATION_PARTY' OR (
			b.delegation_chain_id IS NOT NULL
			AND EXISTS (
				SELECT 1
				FROM grantor_authority_delegation_chain c
				JOIN rights_declaration sd ON sd.id=c.source_declaration_id
				JOIN rights_declaration_verification sv ON sv.declaration_id=sd.id AND sv.outcome='VERIFIED'
				WHERE c.id=b.delegation_chain_id AND c.source_declaration_id=b.rights_declaration_id AND c.status='FINALIZED' AND c.chain_hash=b.delegation_chain_hash
				  AND sd.workspace_id=$1 AND sd.data_resource_id=$2
				  AND (sd.effective_from IS NULL OR sd.effective_from <= $6) AND (sd.effective_to IS NULL OR sd.effective_to > $6)
				  AND NOT EXISTS (SELECT 1 FROM rights_declaration_disposition sx WHERE sx.declaration_id=sd.id AND sx.effective_at <= $6)
			)
			AND NOT EXISTS (SELECT 1 FROM grantor_authority_delegation_disposition x WHERE x.chain_id=b.delegation_chain_id AND x.effective_at <= $6)
			AND NOT EXISTS (SELECT 1 FROM grantor_authority_delegation_disposition x JOIN grantor_authority_delegation_edge e ON e.id=x.edge_id WHERE e.chain_id=b.delegation_chain_id AND x.effective_at <= $6)
			AND NOT EXISTS (SELECT 1 FROM grantor_authority_delegation_edge e WHERE e.chain_id=b.delegation_chain_id AND ((e.valid_from IS NOT NULL AND e.valid_from > $6) OR (e.valid_to IS NOT NULL AND e.valid_to <= $6)))
			AND NOT EXISTS (
				SELECT 1 FROM grantor_authority_delegation_edge e
				WHERE e.chain_id=b.delegation_chain_id
				  AND (e.data_resource_id<>$2 OR NOT ($9=ANY(e.grantable_actions)) OR NOT ($5=ANY(e.grantable_purposes)) OR e.scope_type<>$7 OR e.scope_ref<>$8)
			)
		  ))`
	args := []any{request.WorkspaceID, request.DataResourceID, request.AuthorizationID, request.ConsumerRef, request.Purpose, request.AsOf, request.Scope.Type, request.Scope.Ref, request.Action}
	err := r.pool.QueryRow(ctx, query, args...).Scan(&decision.BindingID, &decision.DeclarationID)
	if errors.Is(err, pgx.ErrNoRows) {
		decision.Decision = domain.DecisionNotAllowed
		decision.Reason = "no current provenance binding covers the requested context"
		return decision, nil
	}
	if err != nil {
		return decision, fmt.Errorf("check current entitlement: %w", err)
	}
	decision.Decision = domain.DecisionAllowed
	decision.Reason = "current declaration, binding, authorization, scope, and validity all cover the request"
	return decision, nil
}

type LineageInput struct {
	DatasetVersionID uuid.UUID
	DataResourceID   uuid.UUID
}

func (r *PostgresRepository) RequiredLineageInputs(ctx context.Context, target uuid.UUID) ([]LineageInput, error) {
	rows, err := r.pool.Query(ctx, `WITH RECURSIVE lineage(version_id) AS (SELECT $1::uuid UNION SELECT l.input_version_id FROM dataset_version_lineage l JOIN lineage x ON x.version_id=l.output_version_id) SELECT DISTINCT l.version_id,d.source_resource_id FROM lineage l JOIN dataset_version v ON v.id=l.version_id JOIN dataset d ON d.id=v.dataset_id WHERE d.source_resource_id IS NOT NULL ORDER BY l.version_id`, target)
	if err != nil {
		return nil, fmt.Errorf("resolve effective rights lineage: %w", err)
	}
	defer rows.Close()
	var inputs []LineageInput
	for rows.Next() {
		var i LineageInput
		if err := rows.Scan(&i.DatasetVersionID, &i.DataResourceID); err != nil {
			return nil, err
		}
		inputs = append(inputs, i)
	}
	return inputs, rows.Err()
}

func (r *PostgresRepository) CurrentDirectDeclaration(ctx context.Context, workspaceID, resourceID uuid.UUID, consumer, purpose, action string, scope domain.NormalizedScope, asOf time.Time) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `SELECT d.id FROM rights_declaration d JOIN rights_declaration_verification v ON v.declaration_id=d.id AND v.outcome='VERIFIED' JOIN rights_declaration_permission p ON p.declaration_id=d.id AND p.permission_kind='USE' AND p.action=$5 JOIN rights_declaration_purpose q ON q.permission_id=p.id AND q.purpose_code=$4 JOIN rights_declaration_scope s ON s.permission_id=p.id AND s.scope_type=$6 AND s.scope_ref=$7 WHERE d.workspace_id=$1 AND d.data_resource_id=$2 AND (d.consumer_scope_type='ANY' OR (d.consumer_scope_type='EXPLICIT' AND d.consumer_ref=$3)) AND (d.effective_from IS NULL OR d.effective_from <= $8) AND (d.effective_to IS NULL OR d.effective_to > $8) AND NOT EXISTS(SELECT 1 FROM rights_declaration_disposition x WHERE x.declaration_id=d.id AND x.effective_at <= $8) ORDER BY d.created_at,d.id LIMIT 1`, workspaceID, resourceID, consumer, purpose, action, scope.Type, scope.Ref, asOf).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, domain.ErrDeclarationNotVerified
	}
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func HashEffectiveInputs(inputs []domain.EffectiveRightsInput) string {
	parts := make([]string, 0, len(inputs))
	for _, i := range inputs {
		parts = append(parts, strings.Join([]string{i.InputDatasetVersionID.String(), i.DataResourceID.String(), i.InputHash}, "|"))
	}
	sort.Strings(parts)
	h := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(h[:])
}
