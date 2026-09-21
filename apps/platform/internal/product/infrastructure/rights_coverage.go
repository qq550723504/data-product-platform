package infrastructure

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

var releaseRequiredActions = []string{"READ", "AGGREGATE", "DERIVE", "PRODUCTIZE"}

type RightsCoverage struct {
	Known             bool
	Complete          bool
	RequiredResources []uuid.UUID
	MissingResources  []uuid.UUID
	MissingActions    map[string][]string
}

func (r *PostgresRepository) EvaluateRightsCoverage(ctx context.Context, snapshotID uuid.UUID, releaseDatasetVersionIDs []uuid.UUID, now time.Time) (RightsCoverage, error) {
	coverage := RightsCoverage{MissingActions: map[string][]string{}}
	if snapshotID == uuid.Nil || len(releaseDatasetVersionIDs) == 0 {
		return coverage, nil
	}

	rows, err := r.pool.Query(ctx, `
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

	grantRows, err := r.pool.Query(ctx, `
		SELECT ar.data_resource_id, ar.actions
		FROM rights_snapshot_authorization rsa
		JOIN rights_snapshot rs ON rs.id = rsa.rights_snapshot_id
		JOIN data_authorization da ON da.id = rsa.authorization_id
		JOIN authorization_resource ar ON ar.authorization_id = da.id
		WHERE rsa.rights_snapshot_id=$1
		  AND da.status='ACTIVE'
		  AND da.purpose=rs.purpose
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
			  AND NOT EXISTS (SELECT 1 FROM rights_declaration_disposition rdd WHERE rdd.declaration_id=rd.id AND rdd.effective_at <= $2)
			  AND NOT EXISTS (SELECT 1 FROM authorization_provenance_binding_disposition apbd WHERE apbd.binding_id=apb.id AND apbd.effective_at <= $2)
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
