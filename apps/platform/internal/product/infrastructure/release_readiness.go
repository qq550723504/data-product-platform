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
	TargetDatasetVersionID       *uuid.UUID
	TargetDatasetID              *uuid.UUID
	AllDatasetsUsable            bool
	ProductionExecutionPresent   bool
	RightsSnapshotExists         bool
	RightsSnapshotWorkspaceMatch bool
	RightsCurrentlyValid         bool
	RightsCoverageKnown          bool
	RightsCoverageComplete       bool
	RequiredResourceIDs          []uuid.UUID
	MissingResourceIDs           []uuid.UUID
	MissingActions               map[string][]string
	ContractExists               bool
	ContractPublished            bool
	ContractMatchesProduct       bool
	QualityResultExists          bool
	QualityDatasetMatches        bool
	QualityDecision              string
	ComplianceResultExists       bool
	ComplianceDatasetMatches     bool
	ComplianceDecision           string
	EvidenceCount                int
	DeliveryAvailable            bool
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

func (r *PostgresRepository) ReadinessFacts(ctx context.Context, release domain.ProductRelease, product domain.DataProduct, version domain.ProductVersion, now time.Time) (ReadinessFacts, error) {
	facts := ReadinessFacts{AllDatasetsUsable: len(release.Datasets) > 0}
	releaseDatasetVersionIDs := make([]uuid.UUID, 0, len(release.Datasets))

	for _, binding := range release.Datasets {
		releaseDatasetVersionIDs = append(releaseDatasetVersionIDs, binding.DatasetVersionID)
		var datasetID uuid.UUID
		var status string
		var generatedBy *uuid.UUID
		err := r.pool.QueryRow(ctx, `
			SELECT dataset_id, status, generated_by_execution_id
			FROM dataset_version WHERE id=$1
		`, binding.DatasetVersionID).Scan(&datasetID, &status, &generatedBy)
		if errors.Is(err, pgx.ErrNoRows) {
			facts.AllDatasetsUsable = false
			continue
		}
		if err != nil {
			return ReadinessFacts{}, fmt.Errorf("read release DatasetVersion %s: %w", binding.DatasetVersionID, err)
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
		var generatedBy *uuid.UUID
		if err := r.pool.QueryRow(ctx, `SELECT dataset_id, generated_by_execution_id FROM dataset_version WHERE id=$1`, binding.DatasetVersionID).Scan(&datasetID, &generatedBy); err == nil {
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
		err := r.pool.QueryRow(ctx, `
			SELECT rs.workspace_id,
			       rs.purpose,
			       count(rsa.authorization_id),
			       COALESCE(bool_and(
			           da.status='ACTIVE'
			           AND da.purpose=rs.purpose
			           AND (da.valid_from IS NULL OR da.valid_from <= $2)
			           AND (da.valid_to IS NULL OR da.valid_to > $2)
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
			coverage, err := r.EvaluateRightsCoverage(ctx, *release.RightsSnapshotID, releaseDatasetVersionIDs, now)
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
		err := r.pool.QueryRow(ctx, `SELECT status FROM contract_version WHERE id=$1`, *release.ContractVersionID).Scan(&status)
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
		if err := r.pool.QueryRow(ctx, `SELECT dataset_version_id, gate_decision FROM quality_result WHERE id=$1`, *release.QualityResultID).Scan(&datasetVersionID, &facts.QualityDecision); err == nil {
			facts.QualityResultExists = true
			facts.QualityDatasetMatches = facts.TargetDatasetVersionID != nil && datasetVersionID == *facts.TargetDatasetVersionID
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return ReadinessFacts{}, fmt.Errorf("read quality readiness: %w", err)
		}
	}

	if release.ComplianceResultID != nil {
		var datasetVersionID uuid.UUID
		if err := r.pool.QueryRow(ctx, `SELECT dataset_version_id, gate_decision FROM compliance_result WHERE id=$1`, *release.ComplianceResultID).Scan(&datasetVersionID, &facts.ComplianceDecision); err == nil {
			facts.ComplianceResultExists = true
			facts.ComplianceDatasetMatches = facts.TargetDatasetVersionID != nil && datasetVersionID == *facts.TargetDatasetVersionID
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return ReadinessFacts{}, fmt.Errorf("read compliance readiness: %w", err)
		}
	}

	if facts.TargetDatasetVersionID != nil {
		if err := r.pool.QueryRow(ctx, `
			SELECT count(*) FROM evidence_relation
			WHERE object_type='DATASET_VERSION' AND object_id=$1
		`, *facts.TargetDatasetVersionID).Scan(&facts.EvidenceCount); err != nil {
			return ReadinessFacts{}, fmt.Errorf("read release evidence readiness: %w", err)
		}
	}

	if facts.TargetDatasetID != nil {
		if err := r.pool.QueryRow(ctx, `
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
