package infrastructure

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var releaseRequiredActions = []string{"READ", "AGGREGATE", "DERIVE", "PRODUCTIZE"}

type RightsCoverage struct {
	Known             bool
	Complete          bool
	RequiredResources []uuid.UUID
	MissingResources  []uuid.UUID
	MissingActions    map[string][]string
}

type rightsCoverageQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func (r *PostgresRepository) EvaluateRightsCoverage(ctx context.Context, snapshotID uuid.UUID, releaseDatasetVersionIDs []uuid.UUID, now time.Time) (RightsCoverage, error) {
	return evaluateRightsCoverage(ctx, r.pool, snapshotID, releaseDatasetVersionIDs, now)
}

func evaluateRightsCoverage(ctx context.Context, q rightsCoverageQueryer, snapshotID uuid.UUID, releaseDatasetVersionIDs []uuid.UUID, now time.Time) (RightsCoverage, error) {
	coverage := RightsCoverage{MissingActions: map[string][]string{}}
	if snapshotID == uuid.Nil || len(releaseDatasetVersionIDs) == 0 {
		return coverage, nil
	}

	rows, err := q.Query(ctx, `
		WITH RECURSIVE lineage(version_id) AS (
			SELECT unnest($1::uuid[])
			UNION
			SELECT dvl.input_version_id
			FROM dataset_version_lineage dvl
			JOIN lineage l ON dvl.output_version_id = l.version_id
		)
		SELECT DISTINCT d.source_resource_id
		FROM lineage l
		JOIN dataset_version dv ON dv.id = l.version_id
		JOIN dataset d ON d.id = dv.dataset_id
		WHERE d.source_resource_id IS NOT NULL
		ORDER BY d.source_resource_id
	`, releaseDatasetVersionIDs)
	if err != nil {
		return RightsCoverage{}, fmt.Errorf("resolve upstream DataResources for rights coverage: %w", err)
	}
	for rows.Next() {
		var resourceID uuid.UUID
		if err := rows.Scan(&resourceID); err != nil {
			rows.Close()
			return RightsCoverage{}, fmt.Errorf("scan upstream DataResource: %w", err)
		}
		coverage.RequiredResources = append(coverage.RequiredResources, resourceID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return RightsCoverage{}, fmt.Errorf("iterate upstream DataResources: %w", err)
	}
	rows.Close()
	if len(coverage.RequiredResources) == 0 {
		return coverage, nil
	}
	coverage.Known = true

	grantRows, err := q.Query(ctx, `
		SELECT ar.data_resource_id, ar.actions
		FROM rights_snapshot_authorization rsa
		JOIN rights_snapshot rs ON rs.id = rsa.rights_snapshot_id
		JOIN data_authorization da ON da.id = rsa.authorization_id
		JOIN authorization_resource ar ON ar.authorization_id = da.id
		WHERE rsa.rights_snapshot_id=$1
		  AND da.status='ACTIVE'
		  AND da.purpose=rs.purpose
		  AND ar.scope_type='ALL_RESOURCE'
		  AND ar.scope_ref=ar.data_resource_id::text
		  AND (da.valid_from IS NULL OR da.valid_from <= $2)
		  AND (da.valid_to IS NULL OR da.valid_to > $2)
		  AND EXISTS (
			SELECT 1
			FROM rights_snapshot_provenance_binding rspb
			JOIN authorization_provenance_binding apb ON apb.id=rspb.binding_id
			JOIN rights_declaration rd ON rd.id=apb.rights_declaration_id
			JOIN rights_declaration_verification rdv ON rdv.declaration_id=rd.id AND rdv.outcome='VERIFIED'
			WHERE rspb.rights_snapshot_id=rs.id
			  AND apb.authorization_id=da.id
			  AND apb.data_resource_id=ar.data_resource_id
			  AND (rd.effective_from IS NULL OR rd.effective_from <= $2)
			  AND (rd.effective_to IS NULL OR rd.effective_to > $2)
			  AND NOT EXISTS (SELECT 1 FROM rights_declaration_disposition rdd WHERE rdd.declaration_id=rd.id AND rdd.effective_at <= $2)
			  AND NOT EXISTS (SELECT 1 FROM authorization_provenance_binding_disposition apbd WHERE apbd.binding_id=apb.id AND apbd.effective_at <= $2)
			  AND (apb.grantor_authority_mode='DIRECT_DECLARATION_PARTY' OR (
				apb.delegation_chain_id IS NOT NULL
				AND EXISTS (SELECT 1 FROM grantor_authority_delegation_chain c WHERE c.id=apb.delegation_chain_id AND c.source_declaration_id=apb.rights_declaration_id AND c.status='FINALIZED' AND c.chain_hash=apb.delegation_chain_hash)
				AND EXISTS (
					SELECT 1 FROM rights_declaration_party rp
					WHERE rp.declaration_id=rd.id
					  AND rp.party_ref=(SELECT e.delegator_ref FROM grantor_authority_delegation_edge e WHERE e.chain_id=apb.delegation_chain_id ORDER BY e.ordinal LIMIT 1)
					  AND rp.role IN ('RIGHTS_HOLDER','PROVIDER','CONTROLLER')
				)
				AND NOT EXISTS (SELECT 1 FROM grantor_authority_delegation_disposition x WHERE x.chain_id=apb.delegation_chain_id AND x.effective_at <= $2)
				AND NOT EXISTS (SELECT 1 FROM grantor_authority_delegation_disposition x JOIN grantor_authority_delegation_edge e ON e.id=x.edge_id WHERE e.chain_id=apb.delegation_chain_id AND x.effective_at <= $2)
				AND NOT EXISTS (SELECT 1 FROM grantor_authority_delegation_edge e WHERE e.chain_id=apb.delegation_chain_id AND ((e.valid_from IS NOT NULL AND e.valid_from > $2) OR (e.valid_to IS NOT NULL AND e.valid_to <= $2)))
				AND NOT EXISTS (
					SELECT 1 FROM grantor_authority_delegation_edge e
					WHERE e.chain_id=apb.delegation_chain_id
					  AND (e.data_resource_id<>ar.data_resource_id OR NOT (ar.actions <@ e.grantable_actions) OR NOT (rs.purpose=ANY(e.grantable_purposes)) OR e.scope_type<>'ALL_RESOURCE' OR e.scope_ref<>ar.data_resource_id::text)
				)
				AND (SELECT e.delegate_ref FROM grantor_authority_delegation_edge e WHERE e.chain_id=apb.delegation_chain_id ORDER BY e.ordinal DESC LIMIT 1)=apb.grantor_ref
				AND NOT EXISTS (
					SELECT 1
					FROM (
						SELECT e.ordinal,e.delegator_ref,lag(e.delegate_ref) OVER (ORDER BY e.ordinal) previous_delegate,row_number() OVER (ORDER BY e.ordinal)-1 expected_ordinal
						FROM grantor_authority_delegation_edge e
						WHERE e.chain_id=apb.delegation_chain_id
					) ordered_edges
					WHERE ordered_edges.ordinal<>ordered_edges.expected_ordinal
					   OR (ordered_edges.previous_delegate IS NOT NULL AND ordered_edges.previous_delegate<>ordered_edges.delegator_ref)
				)
			  ))
		  )
	`, snapshotID, now.UTC())
	if err != nil {
		return RightsCoverage{}, fmt.Errorf("read rights resource grants: %w", err)
	}
	grantedActions := map[uuid.UUID]map[string]struct{}{}
	for grantRows.Next() {
		var resourceID uuid.UUID
		var actions []string
		if err := grantRows.Scan(&resourceID, &actions); err != nil {
			grantRows.Close()
			return RightsCoverage{}, fmt.Errorf("scan rights resource grant: %w", err)
		}
		set := grantedActions[resourceID]
		if set == nil {
			set = map[string]struct{}{}
			grantedActions[resourceID] = set
		}
		for _, action := range actions {
			set[strings.ToUpper(strings.TrimSpace(action))] = struct{}{}
		}
	}
	if err := grantRows.Err(); err != nil {
		grantRows.Close()
		return RightsCoverage{}, fmt.Errorf("iterate rights resource grants: %w", err)
	}
	grantRows.Close()

	for _, resourceID := range coverage.RequiredResources {
		actions, ok := grantedActions[resourceID]
		if !ok {
			coverage.MissingResources = append(coverage.MissingResources, resourceID)
			continue
		}
		missing := make([]string, 0)
		for _, requiredAction := range releaseRequiredActions {
			if _, exists := actions[requiredAction]; !exists {
				missing = append(missing, requiredAction)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			coverage.MissingActions[resourceID.String()] = missing
		}
	}
	coverage.Complete = len(coverage.MissingResources) == 0 && len(coverage.MissingActions) == 0
	return coverage, nil
}
