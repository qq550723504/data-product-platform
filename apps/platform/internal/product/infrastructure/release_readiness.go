package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/domain"
)

type ReadinessFacts struct {
	TargetDatasetVersionID              *uuid.UUID
	TargetDatasetID                     *uuid.UUID
	AllDatasetsUsable                   bool
	ProductionExecutionPresent          bool
	ProductionDependencyBindingRequired bool
	ProductionDependencyBindingComplete bool
	RightsSnapshotExists                bool
	RightsSnapshotWorkspaceMatch        bool
	RightsCurrentlyValid                bool
	RightsCoverageKnown                 bool
	RightsCoverageComplete              bool
	RequiredResourceIDs                 []uuid.UUID
	MissingResourceIDs                  []uuid.UUID
	MissingActions                      map[string][]string
	ContractExists                      bool
	ContractPublished                   bool
	ContractMatchesProduct              bool
	QualityResultExists                 bool
	QualityDatasetMatches               bool
	QualityDecision                     string
	ComplianceResultExists              bool
	ComplianceDatasetMatches            bool
	ComplianceDecision                  string
	EvidenceCount                       int
	DeliveryAvailable                   bool
}

func (r *PostgresRepository) SaveReleaseValidation(ctx context.Context, tx pgx.Tx, release domain.ProductRelease) error {
	commandTag, err := tx.Exec(ctx, `
		UPDATE product_release
		SET status=$2,
		    contract_version_id=$3,
		    rights_snapshot_id=$4,
		    quality_result_id=$5,
		    compliance_result_id=$6
		WHERE id=$1 AND status IN ('DRAFT','VALIDATING')
	`, release.ID, release.Status, release.ContractVersionID, release.RightsSnapshotID,
		release.QualityResultID, release.ComplianceResultID)
	if err != nil {
		return fmt.Errorf("save release validation bindings: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return domain.ErrInvalidReleaseTransition
	}
	return nil
}

func (r *PostgresRepository) SaveReleaseStatus(ctx context.Context, tx pgx.Tx, releaseID uuid.UUID, from, to domain.ReleaseStatus) error {
	commandTag, err := tx.Exec(ctx, `
		UPDATE product_release SET status=$3 WHERE id=$1 AND status=$2
	`, releaseID, from, to)
	if err != nil {
		return fmt.Errorf("save ProductRelease status: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return domain.ErrInvalidReleaseTransition
	}
	return nil
}

type readinessQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (r *PostgresRepository) ReadinessFacts(ctx context.Context, release domain.ProductRelease, product domain.DataProduct, version domain.ProductVersion, now time.Time) (ReadinessFacts, error) {
	return r.readinessFacts(ctx, r.pool, release, product, version, now)
}

func (r *PostgresRepository) ReadinessFactsTx(ctx context.Context, tx pgx.Tx, release domain.ProductRelease, product domain.DataProduct, version domain.ProductVersion, now time.Time) (ReadinessFacts, error) {
	return r.readinessFacts(ctx, tx, release, product, version, now)
}

func (r *PostgresRepository) readinessFacts(ctx context.Context, q readinessQueryer, release domain.ProductRelease, product domain.DataProduct, version domain.ProductVersion, now time.Time) (ReadinessFacts, error) {
	facts := ReadinessFacts{AllDatasetsUsable: len(release.Datasets) > 0}
	releaseDatasetVersionIDs := make([]uuid.UUID, 0, len(release.Datasets))

	for _, binding := range release.Datasets {
		releaseDatasetVersionIDs = append(releaseDatasetVersionIDs, binding.DatasetVersionID)
		var datasetID uuid.UUID
		var datasetWorkspace uuid.UUID
		var status string
		var generatedBy *uuid.UUID
		err := q.QueryRow(ctx, `
			SELECT v.dataset_id, d.workspace_id, v.status, v.generated_by_execution_id
			FROM dataset_version v
			JOIN dataset d ON d.id = v.dataset_id
			WHERE v.id=$1
		`, binding.DatasetVersionID).Scan(&datasetID, &datasetWorkspace, &status, &generatedBy)
		if errors.Is(err, pgx.ErrNoRows) {
			facts.AllDatasetsUsable = false
			continue
		}
		if err != nil {
			return ReadinessFacts{}, fmt.Errorf("read release DatasetVersion %s: %w", binding.DatasetVersionID, err)
		}
		if datasetWorkspace != product.WorkspaceID {
			// A ProductRelease may only bind DatasetVersions owned by its own workspace.
			// The foreign key proves the version exists, not who owns it.
			facts.AllDatasetsUsable = false
			continue
		}
		if status != "READY" && status != "SUPERSEDED" {
			facts.AllDatasetsUsable = false
		}
		if facts.TargetDatasetVersionID == nil && (binding.Role == domain.DatasetPrimary || binding.Role == domain.DatasetOutput) {
			targetVersionID := binding.DatasetVersionID
			targetDatasetID := datasetID
			facts.TargetDatasetVersionID = &targetVersionID
			facts.TargetDatasetID = &targetDatasetID
			facts.ProductionExecutionPresent = generatedBy != nil
		}
	}
	if facts.TargetDatasetVersionID == nil && len(release.Datasets) > 0 {
		binding := release.Datasets[0]
		var datasetID uuid.UUID
		var datasetWorkspace uuid.UUID
		var generatedBy *uuid.UUID
		if err := q.QueryRow(ctx, `
			SELECT v.dataset_id, d.workspace_id, v.generated_by_execution_id
			FROM dataset_version v
			JOIN dataset d ON d.id = v.dataset_id
			WHERE v.id=$1
		`, binding.DatasetVersionID).Scan(&datasetID, &datasetWorkspace, &generatedBy); err == nil && datasetWorkspace == product.WorkspaceID {
			targetVersionID := binding.DatasetVersionID
			facts.TargetDatasetVersionID = &targetVersionID
			facts.TargetDatasetID = &datasetID
			facts.ProductionExecutionPresent = generatedBy != nil
		}
	}

	if release.RightsSnapshotID != nil {
		var snapshotWorkspace uuid.UUID
		var purpose string
		var authorizationCount int
		var currentlyValid bool
		err := q.QueryRow(ctx, `
			SELECT rs.workspace_id,
			       rs.purpose,
			       count(rsa.authorization_id),
			       COALESCE(bool_and(
			           da.status='ACTIVE'
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
						  AND apb.data_resource_id IS NOT NULL
						  AND (rd.effective_from IS NULL OR rd.effective_from <= $2)
						  AND (rd.effective_to IS NULL OR rd.effective_to > $2)
						  AND NOT EXISTS (SELECT 1 FROM rights_declaration_disposition rdd WHERE rdd.declaration_id=rd.id AND rdd.effective_at <= $2)
						  AND NOT EXISTS (SELECT 1 FROM authorization_provenance_binding_disposition apbd WHERE apbd.binding_id=apb.id AND apbd.effective_at <= $2)
						  AND (apb.grantor_authority_mode='DIRECT_DECLARATION_PARTY' OR (
							apb.delegation_chain_id IS NOT NULL
							AND EXISTS (SELECT 1 FROM grantor_authority_delegation_chain c WHERE c.id=apb.delegation_chain_id AND c.source_declaration_id=apb.rights_declaration_id AND c.status='FINALIZED' AND c.chain_hash=apb.delegation_chain_hash)
							AND NOT EXISTS (SELECT 1 FROM grantor_authority_delegation_disposition x WHERE x.chain_id=apb.delegation_chain_id AND x.effective_at <= $2)
							AND NOT EXISTS (SELECT 1 FROM grantor_authority_delegation_disposition x JOIN grantor_authority_delegation_edge e ON e.id=x.edge_id WHERE e.chain_id=apb.delegation_chain_id AND x.effective_at <= $2)
							AND NOT EXISTS (SELECT 1 FROM grantor_authority_delegation_edge e WHERE e.chain_id=apb.delegation_chain_id AND ((e.valid_from IS NOT NULL AND e.valid_from > $2) OR (e.valid_to IS NOT NULL AND e.valid_to <= $2)))
						  ))
					   )
			       ), false)
			FROM rights_snapshot rs
			LEFT JOIN rights_snapshot_authorization rsa ON rsa.rights_snapshot_id=rs.id
			LEFT JOIN data_authorization da ON da.id=rsa.authorization_id
			WHERE rs.id=$1
			GROUP BY rs.workspace_id, rs.purpose
		`, *release.RightsSnapshotID, now.UTC()).Scan(&snapshotWorkspace, &purpose, &authorizationCount, &currentlyValid)
		if err == nil {
			facts.RightsSnapshotExists = true
			facts.RightsSnapshotWorkspaceMatch = snapshotWorkspace == product.WorkspaceID
			facts.RightsCurrentlyValid = authorizationCount > 0 && currentlyValid
			coverage, err := evaluateRightsCoverage(ctx, q, *release.RightsSnapshotID, releaseDatasetVersionIDs, now)
			if err != nil {
				return ReadinessFacts{}, err
			}
			facts.RightsCoverageKnown = coverage.Known
			facts.RightsCoverageComplete = coverage.Complete
			facts.RequiredResourceIDs = coverage.RequiredResources
			facts.MissingResourceIDs = coverage.MissingResources
			facts.MissingActions = coverage.MissingActions
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return ReadinessFacts{}, fmt.Errorf("read rights snapshot readiness: %w", err)
		}
	}

	if release.ContractVersionID != nil {
		var status string
		err := q.QueryRow(ctx, `SELECT status FROM contract_version WHERE id=$1`, *release.ContractVersionID).Scan(&status)
		if err == nil {
			facts.ContractExists = true
			facts.ContractPublished = status == "PUBLISHED"
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return ReadinessFacts{}, fmt.Errorf("read contract readiness: %w", err)
		}
		facts.ContractMatchesProduct = version.ContractVersionID != nil && *version.ContractVersionID == *release.ContractVersionID
	}

	if release.QualityResultID != nil {
		var datasetVersionID uuid.UUID
		if err := q.QueryRow(ctx, `SELECT dataset_version_id, gate_decision FROM quality_result WHERE id=$1`, *release.QualityResultID).Scan(&datasetVersionID, &facts.QualityDecision); err == nil {
			facts.QualityResultExists = true
			facts.QualityDatasetMatches = facts.TargetDatasetVersionID != nil && datasetVersionID == *facts.TargetDatasetVersionID
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return ReadinessFacts{}, fmt.Errorf("read quality readiness: %w", err)
		}
	}

	if release.ComplianceResultID != nil {
		var datasetVersionID uuid.UUID
		if err := q.QueryRow(ctx, `SELECT dataset_version_id, gate_decision FROM compliance_result WHERE id=$1`, *release.ComplianceResultID).Scan(&datasetVersionID, &facts.ComplianceDecision); err == nil {
			facts.ComplianceResultExists = true
			facts.ComplianceDatasetMatches = facts.TargetDatasetVersionID != nil && datasetVersionID == *facts.TargetDatasetVersionID
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return ReadinessFacts{}, fmt.Errorf("read compliance readiness: %w", err)
		}
	}

	if facts.TargetDatasetVersionID != nil {
		if facts.ProductionExecutionPresent {
			// A DatasetVersion that claims a producing Execution always requires
			// an explicit dependency proof. Missing or legacy proof is a visible
			// readiness gap, never an implicit pass.
			facts.ProductionDependencyBindingRequired = true
			var executionID uuid.UUID
			var executionStatus string
			var executionOutputVersionID *uuid.UUID
			if err := q.QueryRow(ctx, `
				SELECT e.id, e.status, e.output_dataset_version_id
				FROM dataset_version v
				JOIN execution e ON e.id=v.generated_by_execution_id
				WHERE v.id=$1
			`, *facts.TargetDatasetVersionID).Scan(&executionID, &executionStatus, &executionOutputVersionID); err != nil {
				return ReadinessFacts{}, fmt.Errorf("read producing execution for dependency readiness: %w", err)
			}
			if executionStatus != "SUCCEEDED" || executionOutputVersionID == nil || *executionOutputVersionID != *facts.TargetDatasetVersionID {
				facts.ProductionDependencyBindingRequired = true
				facts.ProductionDependencyBindingComplete = false
			} else if err := q.QueryRow(ctx, `
					WITH execution_inputs AS (
						SELECT input_name, dataset_version_id
						FROM execution_input
						WHERE execution_id=$1
					), required_resolution AS (
						SELECT dataset_version_id
						FROM execution_inputs
						WHERE input_name='enterprise_resolution'
					), preparation AS (
						SELECT p.*
						FROM execution_dependency_preparation p
						WHERE p.execution_id=$1
						  AND p.workspace_id=$2
					)
					SELECT
						EXISTS(
							SELECT 1
							FROM preparation p
							WHERE p.status='PREPARED'
							  AND EXISTS(
								  SELECT 1 FROM required_resolution
							  )
							  AND (SELECT count(*) FROM execution_inputs
							       WHERE input_name IN ('enterprise_raw','lease_raw','energy_raw')) = 3
							  AND (SELECT count(*) FROM execution_dependency_binding b
							       WHERE b.execution_id=$1 AND b.workspace_id=$2) = 3
							  AND (SELECT count(*) FROM execution_dependency_binding b
							       WHERE b.execution_id=$1 AND b.workspace_id=$2
							         AND b.dependency_name IN ('enterprise_resolution','company_match_policy','indicator_policy')) = 3
							  AND (SELECT count(*) FROM execution_dependency_binding b
							       WHERE b.execution_id=$1 AND b.workspace_id=$2
							         AND b.dependency_name='enterprise_resolution'
							         AND b.dataset_version_id=(SELECT dataset_version_id FROM required_resolution)
							         AND octet_length(b.content) > 0
							         AND encode(digest(b.content, 'sha256'), 'hex')=b.content_sha256) = 1
							  AND (SELECT count(*) FROM execution_dependency_binding b
							       WHERE b.execution_id=$1 AND b.workspace_id=$2
							         AND b.dependency_name IN ('company_match_policy','indicator_policy')
							         AND octet_length(b.content) > 0
							         AND encode(digest(b.content, 'sha256'), 'hex')=b.content_sha256) = 2
							  AND (SELECT count(*) FROM execution_mapping_usage u
							       WHERE u.execution_id=$1 AND u.workspace_id=$2) = p.mapping_usage_count
							  AND NOT EXISTS(
								  SELECT 1
								  FROM execution_mapping_usage u
								  LEFT JOIN entity_mapping_decision d
								    ON d.workspace_id=u.workspace_id AND d.id=u.decision_id AND d.entity_id=u.entity_id
								  LEFT JOIN entity e
								    ON e.workspace_id=u.workspace_id AND e.id=u.entity_id
								  WHERE u.execution_id=$1 AND u.workspace_id=$2
								    AND (u.input_name NOT IN ('enterprise_raw','lease_raw','energy_raw')
								      OR NOT EXISTS(
									      SELECT 1 FROM execution_inputs i
									      WHERE i.input_name=u.input_name AND i.dataset_version_id=u.input_dataset_version_id
									    )
								      OR u.resolution_dataset_version_id IS DISTINCT FROM (SELECT dataset_version_id FROM required_resolution)
								      OR d.id IS NULL
								      OR e.id IS NULL)
							  )
							)
			`, executionID, product.WorkspaceID).Scan(&facts.ProductionDependencyBindingComplete); err != nil {
				return ReadinessFacts{}, fmt.Errorf("read production dependency readiness: %w", err)
			}
		}
		if err := q.QueryRow(ctx, `
			SELECT count(*) FROM evidence_relation
			WHERE object_type='DATASET_VERSION' AND object_id=$1
		`, *facts.TargetDatasetVersionID).Scan(&facts.EvidenceCount); err != nil {
			return ReadinessFacts{}, fmt.Errorf("read release evidence readiness: %w", err)
		}
	}

	if facts.TargetDatasetID != nil {
		if err := q.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM product_asset
				WHERE product_version_id=$1
				  AND (asset_type <> 'DATASET' OR dataset_id=$2)
			)
		`, version.ID, *facts.TargetDatasetID).Scan(&facts.DeliveryAvailable); err != nil {
			return ReadinessFacts{}, fmt.Errorf("read delivery readiness: %w", err)
		}
	}
	return facts, nil
}
