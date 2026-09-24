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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/certification/domain"
	rightsinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
)

var ErrCertificationNotFound = errors.New("dataset certification not found")

type CertificationRepository struct {
	pool *pgxpool.Pool
}

type certificationQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type CertificationTarget struct {
	WorkspaceID      uuid.UUID
	DatasetVersionID uuid.UUID
	ProfileID        uuid.UUID
	Decision         domain.Decision
}

type CertificationHistory struct {
	Certifications []domain.DatasetCertification
	Dispositions   []domain.CertificationDisposition
}

func NewCertificationRepository(pool *pgxpool.Pool) *CertificationRepository {
	return &CertificationRepository{pool: pool}
}

func (r *CertificationRepository) InsertCertification(ctx context.Context, tx pgx.Tx, certification domain.DatasetCertification) error {
	blockers, err := json.Marshal(certification.Blockers)
	if err != nil {
		return fmt.Errorf("marshal certification blockers: %w", err)
	}
	if certification.Profile.ID == uuid.Nil {
		return fmt.Errorf("certification profile id is required")
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO dataset_certification (
			id, workspace_id, dataset_version_id, quality_assessment_id, certification_profile_id,
			profile_ref, profile_version, profile_content_sha256, profile_content_snapshot,
			rights_snapshot_id, effective_rights_snapshot_id, effective_rights_snapshot_hash,
			frozen_rights_context_hash, compliance_result_id, contract_version_id,
			traceability_evidence_id, evidence_snapshot_id,
			gold_production_binding_id, annotation_snapshot_id, annotation_snapshot_root_hash,
			annotation_schema_content_sha256, annotation_taxonomy_content_sha256,
			gold_production_binding_root_hash,
			decision, blockers, reason, issued_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28)
	`, certification.ID, certification.WorkspaceID, certification.DatasetVersionID,
		certification.QualityAssessmentID, certification.Profile.ID, certification.Profile.ProfileRef,
		certification.Profile.Version, certification.Profile.ContentSHA256, string(certification.Profile.Content),
		certification.RightsSnapshotID, certification.EffectiveRightsSnapshotID,
		nullableString(certification.EffectiveRightsSnapshotHash), nullableString(certification.FrozenRightsContextHash),
		certification.ComplianceResultID, certification.ContractVersionID, certification.TraceabilityEvidenceID,
		certification.EvidenceSnapshotID, certification.GoldProductionBindingID, certification.AnnotationSnapshotID,
		nullableString(certification.AnnotationSnapshotRootHash), nullableString(certification.AnnotationSchemaSHA256),
		nullableString(certification.AnnotationTaxonomySHA256), nullableString(certification.GoldProductionBindingRootHash),
		certification.Decision, blockers, certification.Reason, certification.IssuedAt, certification.ActorID)
	if err != nil {
		return fmt.Errorf("insert dataset certification: %w", err)
	}
	return nil
}

func (r *CertificationRepository) GetCertificationTx(ctx context.Context, tx pgx.Tx, certificationID uuid.UUID, profile domain.ProfileSnapshot) (domain.DatasetCertification, error) {
	var certification domain.DatasetCertification
	var blockers []byte
	var decision string
	var effectiveHash, contextHash *string
	var annotationRootHash, annotationSchemaHash, annotationTaxonomyHash, goldBindingRootHash *string
	if err := tx.QueryRow(ctx, `
		SELECT id, workspace_id, dataset_version_id, quality_assessment_id,
		       rights_snapshot_id, effective_rights_snapshot_id, effective_rights_snapshot_hash,
		       frozen_rights_context_hash, compliance_result_id, contract_version_id,
		       traceability_evidence_id, evidence_snapshot_id,
		       gold_production_binding_id, annotation_snapshot_id, annotation_snapshot_root_hash,
		       annotation_schema_content_sha256, annotation_taxonomy_content_sha256,
		       gold_production_binding_root_hash,
		       decision, blockers, reason, issued_at, created_by
		FROM dataset_certification WHERE id=$1
	`, certificationID).Scan(
		&certification.ID, &certification.WorkspaceID, &certification.DatasetVersionID,
		&certification.QualityAssessmentID, &certification.RightsSnapshotID,
		&certification.EffectiveRightsSnapshotID, &effectiveHash, &contextHash,
		&certification.ComplianceResultID, &certification.ContractVersionID,
		&certification.TraceabilityEvidenceID, &certification.EvidenceSnapshotID,
		&certification.GoldProductionBindingID, &certification.AnnotationSnapshotID,
		&annotationRootHash, &annotationSchemaHash, &annotationTaxonomyHash, &goldBindingRootHash,
		&decision, &blockers, &certification.Reason, &certification.IssuedAt, &certification.ActorID,
	); errors.Is(err, pgx.ErrNoRows) {
		return domain.DatasetCertification{}, ErrCertificationNotFound
	} else if err != nil {
		return domain.DatasetCertification{}, fmt.Errorf("get dataset certification: %w", err)
	}
	if err := json.Unmarshal(blockers, &certification.Blockers); err != nil {
		return domain.DatasetCertification{}, fmt.Errorf("decode dataset certification blockers: %w", err)
	}
	certification.Decision = domain.Decision(decision)
	certification.EffectiveRightsSnapshotHash = dereferenceString(effectiveHash)
	certification.FrozenRightsContextHash = dereferenceString(contextHash)
	certification.AnnotationSnapshotRootHash = dereferenceString(annotationRootHash)
	certification.AnnotationSchemaSHA256 = dereferenceString(annotationSchemaHash)
	certification.AnnotationTaxonomySHA256 = dereferenceString(annotationTaxonomyHash)
	certification.GoldProductionBindingRootHash = dereferenceString(goldBindingRootHash)
	certification.Profile = profile
	return certification, nil
}

// ListCertificationHistory intentionally returns every matching historical
// fact. Current selection belongs to the domain, where ambiguity can fail
// closed instead of being hidden by an ORDER BY created_at.
func (r *CertificationRepository) ListCertificationHistory(ctx context.Context, workspaceID, datasetVersionID, profileID uuid.UUID, asOf time.Time, profile domain.ProfileSnapshot) (CertificationHistory, error) {
	return listCertificationHistory(ctx, r.pool, workspaceID, datasetVersionID, profileID, asOf, profile)
}

func (r *CertificationRepository) ListCertificationHistoryTx(ctx context.Context, tx pgx.Tx, workspaceID, datasetVersionID, profileID uuid.UUID, asOf time.Time, profile domain.ProfileSnapshot) (CertificationHistory, error) {
	return listCertificationHistory(ctx, tx, workspaceID, datasetVersionID, profileID, asOf, profile)
}

func listCertificationHistory(ctx context.Context, q certificationQueryer, workspaceID, datasetVersionID, profileID uuid.UUID, asOf time.Time, profile domain.ProfileSnapshot) (CertificationHistory, error) {
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	rows, err := q.Query(ctx, `
		SELECT id, workspace_id, dataset_version_id, quality_assessment_id,
		       rights_snapshot_id, effective_rights_snapshot_id, effective_rights_snapshot_hash,
		       frozen_rights_context_hash, compliance_result_id, contract_version_id,
		       traceability_evidence_id, evidence_snapshot_id,
		       gold_production_binding_id, annotation_snapshot_id, annotation_snapshot_root_hash,
		       annotation_schema_content_sha256, annotation_taxonomy_content_sha256,
		       gold_production_binding_root_hash,
		       decision, blockers, reason, issued_at, created_by
		FROM dataset_certification
		WHERE workspace_id=$1 AND dataset_version_id=$2 AND certification_profile_id=$3
		  AND issued_at <= $4
		ORDER BY id
	`, workspaceID, datasetVersionID, profileID, asOf.UTC())
	if err != nil {
		return CertificationHistory{}, fmt.Errorf("list dataset certification history: %w", err)
	}
	defer rows.Close()
	history := CertificationHistory{Certifications: make([]domain.DatasetCertification, 0)}
	for rows.Next() {
		certification, err := scanCertification(rows, profile)
		if err != nil {
			return CertificationHistory{}, err
		}
		history.Certifications = append(history.Certifications, certification)
	}
	if err := rows.Err(); err != nil {
		return CertificationHistory{}, fmt.Errorf("iterate dataset certification history: %w", err)
	}

	dispositionRows, err := q.Query(ctx, `
		SELECT d.id, d.workspace_id, d.certification_id, d.disposition, d.effective_at,
		       d.reason, d.superseded_by_certification_id, d.evidence_snapshot_id, d.created_by
		FROM certification_disposition d
		JOIN dataset_certification c ON c.id=d.certification_id
		WHERE c.workspace_id=$1 AND c.dataset_version_id=$2 AND c.certification_profile_id=$3
		  AND d.effective_at <= $4
		ORDER BY d.certification_id, d.effective_at, d.id
	`, workspaceID, datasetVersionID, profileID, asOf.UTC())
	if err != nil {
		return CertificationHistory{}, fmt.Errorf("list certification dispositions: %w", err)
	}
	defer dispositionRows.Close()
	history.Dispositions = make([]domain.CertificationDisposition, 0)
	for dispositionRows.Next() {
		var disposition domain.CertificationDisposition
		var kind string
		if err := dispositionRows.Scan(
			&disposition.ID, &disposition.WorkspaceID, &disposition.CertificationID, &kind,
			&disposition.EffectiveAt, &disposition.Reason, &disposition.SupersededByCertificationID,
			&disposition.EvidenceSnapshotID, &disposition.ActorID,
		); err != nil {
			return CertificationHistory{}, fmt.Errorf("scan certification disposition: %w", err)
		}
		disposition.Disposition = domain.Disposition(kind)
		history.Dispositions = append(history.Dispositions, disposition)
	}
	if err := dispositionRows.Err(); err != nil {
		return CertificationHistory{}, fmt.Errorf("iterate certification dispositions: %w", err)
	}
	return history, nil
}

type certificationScanner interface {
	Scan(dest ...any) error
}

func scanCertification(row certificationScanner, profile domain.ProfileSnapshot) (domain.DatasetCertification, error) {
	var certification domain.DatasetCertification
	var blockers []byte
	var decision string
	var effectiveHash, contextHash *string
	var annotationRootHash, annotationSchemaHash, annotationTaxonomyHash, goldBindingRootHash *string
	if err := row.Scan(
		&certification.ID, &certification.WorkspaceID, &certification.DatasetVersionID,
		&certification.QualityAssessmentID, &certification.RightsSnapshotID,
		&certification.EffectiveRightsSnapshotID, &effectiveHash, &contextHash,
		&certification.ComplianceResultID, &certification.ContractVersionID,
		&certification.TraceabilityEvidenceID, &certification.EvidenceSnapshotID,
		&certification.GoldProductionBindingID, &certification.AnnotationSnapshotID,
		&annotationRootHash, &annotationSchemaHash, &annotationTaxonomyHash, &goldBindingRootHash,
		&decision, &blockers, &certification.Reason, &certification.IssuedAt, &certification.ActorID,
	); err != nil {
		return domain.DatasetCertification{}, fmt.Errorf("scan dataset certification: %w", err)
	}
	if err := json.Unmarshal(blockers, &certification.Blockers); err != nil {
		return domain.DatasetCertification{}, fmt.Errorf("decode dataset certification blockers: %w", err)
	}
	certification.Decision = domain.Decision(decision)
	certification.EffectiveRightsSnapshotHash = dereferenceString(effectiveHash)
	certification.FrozenRightsContextHash = dereferenceString(contextHash)
	certification.AnnotationSnapshotRootHash = dereferenceString(annotationRootHash)
	certification.AnnotationSchemaSHA256 = dereferenceString(annotationSchemaHash)
	certification.AnnotationTaxonomySHA256 = dereferenceString(annotationTaxonomyHash)
	certification.GoldProductionBindingRootHash = dereferenceString(goldBindingRootHash)
	certification.Profile = profile
	return certification, nil
}

func (r *CertificationRepository) GetCertificationProfileID(ctx context.Context, certificationID uuid.UUID) (uuid.UUID, error) {
	var profileID uuid.UUID
	if err := r.pool.QueryRow(ctx, `SELECT certification_profile_id FROM dataset_certification WHERE id=$1`, certificationID).Scan(&profileID); errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrCertificationNotFound
	} else if err != nil {
		return uuid.Nil, fmt.Errorf("get dataset certification profile: %w", err)
	}
	return profileID, nil
}

func (r *CertificationRepository) GetCertificationTargetTx(ctx context.Context, tx pgx.Tx, certificationID uuid.UUID) (CertificationTarget, error) {
	var target CertificationTarget
	var decision string
	if err := tx.QueryRow(ctx, `
		SELECT workspace_id, dataset_version_id, certification_profile_id, decision
		FROM dataset_certification WHERE id=$1
	`, certificationID).Scan(&target.WorkspaceID, &target.DatasetVersionID, &target.ProfileID, &decision); errors.Is(err, pgx.ErrNoRows) {
		return CertificationTarget{}, ErrCertificationNotFound
	} else if err != nil {
		return CertificationTarget{}, fmt.Errorf("get certification target: %w", err)
	}
	target.Decision = domain.Decision(decision)
	return target, nil
}

func (r *CertificationRepository) InsertDisposition(ctx context.Context, tx pgx.Tx, disposition domain.CertificationDisposition) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO certification_disposition (
			id, workspace_id, certification_id, disposition, effective_at, reason,
			superseded_by_certification_id, evidence_snapshot_id, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, disposition.ID, disposition.WorkspaceID, disposition.CertificationID, disposition.Disposition,
		disposition.EffectiveAt, disposition.Reason, disposition.SupersededByCertificationID,
		disposition.EvidenceSnapshotID, disposition.ActorID)
	if err != nil {
		return fmt.Errorf("insert certification disposition: %w", err)
	}
	return nil
}

func (r *CertificationRepository) GetDispositionTx(ctx context.Context, tx pgx.Tx, dispositionID uuid.UUID) (domain.CertificationDisposition, error) {
	var disposition domain.CertificationDisposition
	var kind string
	if err := tx.QueryRow(ctx, `
		SELECT id, workspace_id, certification_id, disposition, effective_at, reason,
		       superseded_by_certification_id, evidence_snapshot_id, created_by
		FROM certification_disposition WHERE id=$1
	`, dispositionID).Scan(
		&disposition.ID, &disposition.WorkspaceID, &disposition.CertificationID, &kind,
		&disposition.EffectiveAt, &disposition.Reason, &disposition.SupersededByCertificationID,
		&disposition.EvidenceSnapshotID, &disposition.ActorID,
	); errors.Is(err, pgx.ErrNoRows) {
		return domain.CertificationDisposition{}, fmt.Errorf("certification disposition %w", ErrCertificationNotFound)
	} else if err != nil {
		return domain.CertificationDisposition{}, fmt.Errorf("get certification disposition: %w", err)
	}
	disposition.Disposition = domain.Disposition(kind)
	return disposition, nil
}

type IdempotencyRecord struct {
	ObjectID           uuid.UUID
	ResultRef          *uuid.UUID
	RequestFingerprint string
}

func (r *CertificationRepository) FindIdempotencyTx(ctx context.Context, tx pgx.Tx, workspaceID uuid.UUID, commandType, key string) (IdempotencyRecord, bool, error) {
	var record IdempotencyRecord
	err := tx.QueryRow(ctx, `
		SELECT object_id, result_ref, COALESCE(request_fingerprint, '')
		FROM command_idempotency
		WHERE workspace_id=$1 AND command_type=$2 AND idempotency_key=$3
		FOR UPDATE
	`, workspaceID, commandType, key).Scan(&record.ObjectID, &record.ResultRef, &record.RequestFingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return IdempotencyRecord{}, false, nil
	}
	if err != nil {
		return IdempotencyRecord{}, false, fmt.Errorf("find certification idempotency record: %w", err)
	}
	return record, true, nil
}

func (r *CertificationRepository) TryInsertIdempotency(ctx context.Context, tx pgx.Tx, workspaceID uuid.UUID, commandType, key string, objectID uuid.UUID, fingerprint string) (bool, error) {
	var inserted uuid.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO command_idempotency (
			workspace_id, command_type, idempotency_key, object_id, result_ref, request_fingerprint
		) VALUES ($1,$2,$3,$4,$4,$5)
		ON CONFLICT (workspace_id, command_type, idempotency_key) DO NOTHING
		RETURNING id
	`, workspaceID, commandType, key, objectID, fingerprint).Scan(&inserted)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("insert certification idempotency record: %w", err)
	}
	return inserted != uuid.Nil, nil
}

func (r *CertificationRepository) BindTrustedEvaluationFactsTx(ctx context.Context, tx pgx.Tx, profile domain.ProfileSnapshot, input *domain.EvaluationInput) error {
	if input == nil {
		return fmt.Errorf("evaluation input is required")
	}
	var workspaceID uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT d.workspace_id, v.status
		FROM dataset_version v
		JOIN dataset d ON d.id=v.dataset_id
		WHERE v.id=$1
		FOR SHARE OF v
	`, input.DatasetVersionID).Scan(&workspaceID, &input.DatasetVersionStatus); err != nil {
		return fmt.Errorf("load DatasetVersion certification status: %w", err)
	}
	if workspaceID != input.WorkspaceID {
		return fmt.Errorf("DatasetVersion crosses certification workspace boundary")
	}

	// Rebind QualityAssessment from immutable stored facts. Resolver fields are
	// only candidate references and must not decide certification.
	var qualityWorkspaceID, qualityDatasetVersionID uuid.UUID
	var qualityGate, qualityRuleSetRef, qualityEvaluatorName string
	if err := tx.QueryRow(ctx, `
		SELECT workspace_id, dataset_version_id, gate_decision, rule_set_ref, evaluator_name
		FROM quality_result
		WHERE id=$1
		FOR SHARE
	`, input.Quality.ID).Scan(
		&qualityWorkspaceID, &qualityDatasetVersionID, &qualityGate, &qualityRuleSetRef, &qualityEvaluatorName,
	); err != nil {
		return fmt.Errorf("load QualityAssessment for certification: %w", err)
	}
	input.Quality.WorkspaceID = qualityWorkspaceID
	input.Quality.DatasetVersionID = qualityDatasetVersionID
	input.Quality.GateDecision = qualityGate
	input.Quality.Dimensions = map[string]string{}
	rows, err := tx.Query(ctx, `
		SELECT upper(key), value->>'status'
		FROM quality_result q,
		     jsonb_each(COALESCE(q.metrics->'dimensions', '{}'::jsonb))
		WHERE q.id=$1
		ORDER BY key
	`, input.Quality.ID)
	if err != nil {
		return fmt.Errorf("load QualityAssessment dimensions: %w", err)
	}
	for rows.Next() {
		var dimension, status string
		if err := rows.Scan(&dimension, &status); err != nil {
			rows.Close()
			return fmt.Errorf("scan QualityAssessment dimension: %w", err)
		}
		input.Quality.Dimensions[dimension] = status
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate QualityAssessment dimensions: %w", err)
	}
	rows.Close()

	input.Quality.RuleStatuses = map[string]string{}
	rows, err = tx.Query(ctx, `
		SELECT rule_id, status
		FROM quality_finding
		WHERE result_id=$1
		ORDER BY rule_id, id
	`, input.Quality.ID)
	if err != nil {
		return fmt.Errorf("load QualityAssessment findings: %w", err)
	}
	for rows.Next() {
		var ruleID, status string
		if err := rows.Scan(&ruleID, &status); err != nil {
			rows.Close()
			return fmt.Errorf("scan QualityAssessment finding: %w", err)
		}
		input.Quality.RuleStatuses[ruleID] = status
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate QualityAssessment findings: %w", err)
	}
	rows.Close()

	if input.Compliance != nil && input.Compliance.ID != uuid.Nil {
		var complianceWorkspaceID, complianceDatasetVersionID uuid.UUID
		var complianceDecision string
		if err := tx.QueryRow(ctx, `
			SELECT workspace_id, dataset_version_id, gate_decision
			FROM compliance_result
			WHERE id=$1
			FOR SHARE
		`, input.Compliance.ID).Scan(&complianceWorkspaceID, &complianceDatasetVersionID, &complianceDecision); err != nil {
			return fmt.Errorf("load ComplianceResult for certification: %w", err)
		}
		input.Compliance.WorkspaceID = complianceWorkspaceID
		input.Compliance.DatasetVersionID = complianceDatasetVersionID
		input.Compliance.Decision = complianceDecision
	}

	if input.Contract != nil && input.Contract.ID != uuid.Nil {
		var contractWorkspaceID uuid.UUID
		var contractCode, contractStatus string
		if err := tx.QueryRow(ctx, `
			SELECT c.workspace_id, c.code, cv.status
			FROM contract_version cv
			JOIN data_contract c ON c.id=cv.contract_id
			WHERE cv.id=$1
			FOR SHARE OF cv, c
		`, input.Contract.ID).Scan(&contractWorkspaceID, &contractCode, &contractStatus); err != nil {
			return fmt.Errorf("load ContractVersion for certification: %w", err)
		}
		input.Contract.WorkspaceID = contractWorkspaceID
		input.Contract.DatasetVersionID = input.DatasetVersionID
		input.Contract.MatchesProfile =
			contractWorkspaceID == input.WorkspaceID &&
				contractStatus == "PUBLISHED" &&
				strings.TrimSpace(contractCode) == strings.TrimSpace(profile.ContractCode)
	}

	// Lineage is append-only but can still grow concurrently. Hold a table SHARE
	// lock through certification so a new edge cannot cross the lineage evidence
	// read and the immutable certification insert.
	if _, err := tx.Exec(ctx, "LOCK TABLE dataset_version_lineage IN SHARE MODE"); err != nil {
		return fmt.Errorf("serialize DatasetVersion lineage for certification: %w", err)
	}

	if input.Rights != nil && input.Rights.EffectiveRightsSnapshotID != uuid.Nil {
		rights := input.Rights
		var status, rootHash, consumerRef, purpose string
		if err := tx.QueryRow(ctx, `
			SELECT workspace_id, target_dataset_version_id, status, COALESCE(root_hash,''), consumer_ref, purpose
			FROM effective_rights_snapshot
			WHERE id=$1
			FOR SHARE
		`, rights.EffectiveRightsSnapshotID).Scan(
			&rights.WorkspaceID, &rights.DatasetVersionID, &status, &rootHash, &consumerRef, &purpose,
		); err != nil {
			return fmt.Errorf("load EffectiveRightsSnapshot for certification: %w", err)
		}
		rights.EffectiveRightsFinalized = status == "FINALIZED"
		rights.EffectiveRightsSnapshotHash = rootHash
		rights.Coverage = domain.RightsCoverage{
			Purpose:   domain.Applicability{Mode: domain.ApplicabilityExplicit, Values: []string{purpose}},
			Actions:   domain.Applicability{Mode: domain.ApplicabilityExplicit},
			Consumers: domain.Applicability{Mode: domain.ApplicabilityExplicit, Values: []string{consumerRef}},
			Scopes:    domain.ScopeApplicability{Mode: domain.ApplicabilityExplicit},
		}
		rights.ActionDecisions = map[string]string{}

		rows, err := tx.Query(ctx, `
			SELECT action, decision
			FROM effective_rights_action
			WHERE snapshot_id=$1
			ORDER BY action
		`, rights.EffectiveRightsSnapshotID)
		if err != nil {
			return fmt.Errorf("load EffectiveRightsSnapshot actions: %w", err)
		}
		for rows.Next() {
			var action, decision string
			if err := rows.Scan(&action, &decision); err != nil {
				rows.Close()
				return fmt.Errorf("scan EffectiveRightsSnapshot action: %w", err)
			}
			rights.ActionDecisions[action] = decision
			if strings.EqualFold(decision, domain.RightsAllowed) {
				rights.Coverage.Actions.Values = append(rights.Coverage.Actions.Values, action)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("iterate EffectiveRightsSnapshot actions: %w", err)
		}
		rows.Close()

		rows, err = tx.Query(ctx, `
			SELECT DISTINCT data_resource_id
			FROM effective_rights_input
			WHERE snapshot_id=$1
			ORDER BY data_resource_id
		`, rights.EffectiveRightsSnapshotID)
		if err != nil {
			return fmt.Errorf("load EffectiveRightsSnapshot scopes: %w", err)
		}
		for rows.Next() {
			var resourceID uuid.UUID
			if err := rows.Scan(&resourceID); err != nil {
				rows.Close()
				return fmt.Errorf("scan EffectiveRightsSnapshot scope: %w", err)
			}
			rights.Coverage.Scopes.Values = append(rights.Coverage.Scopes.Values, domain.ScopeRef{Type: "ALL_RESOURCE", Ref: resourceID.String()})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("iterate EffectiveRightsSnapshot scopes: %w", err)
		}
		rows.Close()

		snapshotInputs, err := rightsinfra.EffectiveRightsInputsTx(ctx, tx, rights.EffectiveRightsSnapshotID)
		if err != nil {
			return err
		}
		targetInputs, err := rightsinfra.RequiredLineageInputsTx(ctx, tx, input.DatasetVersionID)
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM dataset_version_lineage WHERE output_version_id=$1
			)
		`, input.DatasetVersionID).Scan(&input.Derived); err != nil {
			return fmt.Errorf("resolve DatasetVersion derived state for certification: %w", err)
		}
		rights.RequiredInputSetHash = rightsinfra.HashRequiredResourceMembership(snapshotInputs)
		rights.TargetLineageInputSetHash = rightsinfra.HashRequiredResourceMembership(targetInputs)
		rights.FrozenRightsContextHash = hashStrings(
			rights.EffectiveRightsSnapshotID.String(),
			rights.EffectiveRightsSnapshotHash,
			consumerRef,
			purpose,
			rights.RequiredInputSetHash,
			strings.Join(rights.Coverage.Actions.Values, ","),
			scopeRefsKey(rights.Coverage.Scopes.Values),
		)

		if rights.RightsSnapshotID != uuid.Nil {
			var rightsWorkspace uuid.UUID
			var rightsStatus, rightsPurpose, rightsConsumer string
			if err := tx.QueryRow(ctx, `
				SELECT workspace_id, status, purpose, COALESCE(consumer_ref,'')
				FROM rights_snapshot
				WHERE id=$1
				FOR SHARE
			`, rights.RightsSnapshotID).Scan(&rightsWorkspace, &rightsStatus, &rightsPurpose, &rightsConsumer); err != nil {
				return fmt.Errorf("load RightsSnapshot for certification: %w", err)
			}
			var coversEffectiveProvenance bool
			if err := tx.QueryRow(ctx, `
				SELECT NOT EXISTS (
					SELECT 1
					FROM effective_rights_input eri
					WHERE eri.snapshot_id=$1
					  AND eri.declaration_id IS NOT NULL
					  AND NOT EXISTS (
						  SELECT 1
						  FROM rights_snapshot_declaration rsd
						  WHERE rsd.rights_snapshot_id=$2
						    AND rsd.declaration_id=eri.declaration_id
					  )
				) AND NOT EXISTS (
					SELECT 1
					FROM effective_rights_input eri
					WHERE eri.snapshot_id=$1
					  AND eri.binding_id IS NOT NULL
					  AND NOT EXISTS (
						  SELECT 1
						  FROM rights_snapshot_provenance_binding rspb
						  WHERE rspb.rights_snapshot_id=$2
						    AND rspb.binding_id=eri.binding_id
					  )
				)
			`, rights.EffectiveRightsSnapshotID, rights.RightsSnapshotID).Scan(&coversEffectiveProvenance); err != nil {
				return fmt.Errorf("verify RightsSnapshot provenance coverage: %w", err)
			}
			rights.RightsSnapshotFinalized =
				rightsWorkspace == input.WorkspaceID &&
					rightsStatus == "FINALIZED" &&
					strings.EqualFold(strings.TrimSpace(rightsPurpose), strings.TrimSpace(purpose)) &&
					strings.TrimSpace(rightsConsumer) == strings.TrimSpace(consumerRef) &&
					coversEffectiveProvenance
		} else {
			rights.RightsSnapshotFinalized = false
		}
	}

	if profile.ProfileRef == domain.GoldCertificationProfileRef {
		if !strings.EqualFold(strings.TrimSpace(qualityRuleSetRef), "gold/quality/annotation-v1") ||
			!strings.EqualFold(strings.TrimSpace(qualityEvaluatorName), "gold-quality") {
			return fmt.Errorf("Gold certification requires the formal Gold QualityAssessment")
		}
		var gold domain.GoldProductionEvidence
		var bindingStatus, snapshotStatus string
		var snapshotRoot string
		var campaignSchemaHash, campaignTaxonomyHash string
		if err := tx.QueryRow(ctx, `
			SELECT g.id, g.workspace_id, g.output_dataset_version_id,
			       g.annotation_snapshot_id, g.snapshot_root_hash,
			       g.schema_content_sha256, g.taxonomy_content_sha256,
			       g.root_hash, g.status,
			       s.status, s.root_hash,
			       c.schema_content_sha256, c.taxonomy_content_sha256
			FROM gold_production_binding g
			JOIN annotation_snapshot s ON s.id=g.annotation_snapshot_id
			JOIN annotation_campaign c ON c.id=g.annotation_campaign_id
			WHERE g.output_dataset_version_id=$1
			FOR SHARE OF g, s, c
		`, input.DatasetVersionID).Scan(
			&gold.BindingID, &gold.WorkspaceID, &gold.DatasetVersionID,
			&gold.AnnotationSnapshotID, &gold.AnnotationSnapshotRoot,
			&gold.SchemaContentSHA256, &gold.TaxonomyContentSHA256,
			&gold.ProductionBindingRootHash, &bindingStatus,
			&snapshotStatus, &snapshotRoot,
			&campaignSchemaHash, &campaignTaxonomyHash,
		); err != nil {
			return fmt.Errorf("load Gold production proof for certification: %w", err)
		}
		gold.Finalized =
			bindingStatus == "FINALIZED" &&
			snapshotStatus == "FINALIZED" &&
			gold.WorkspaceID == input.WorkspaceID &&
			gold.DatasetVersionID == input.DatasetVersionID &&
			gold.AnnotationSnapshotRoot == snapshotRoot &&
			gold.SchemaContentSHA256 == campaignSchemaHash &&
			gold.TaxonomyContentSHA256 == campaignTaxonomyHash
		input.Gold = &gold
	}

	if input.Traceability != nil && input.Traceability.ID != uuid.Nil {
		var workspaceID uuid.UUID
		var objectType string
		var objectID uuid.UUID
		var hasItems bool
		err := tx.QueryRow(ctx, `
			SELECT es.workspace_id, es.object_type, es.object_id,
			       EXISTS(SELECT 1 FROM evidence_snapshot_item esi WHERE esi.snapshot_id=es.id)
			FROM evidence_snapshot es
			WHERE es.id=$1
			FOR SHARE OF es
		`, input.Traceability.ID).Scan(&workspaceID, &objectType, &objectID, &hasItems)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("load traceability EvidenceSnapshot: %w", err)
		}
		input.Traceability.WorkspaceID = workspaceID
		input.Traceability.DatasetVersionID = objectID
		input.Traceability.Complete = err == nil && workspaceID == input.WorkspaceID && objectType == "DATASET_VERSION" && objectID == input.DatasetVersionID && hasItems
	}

	if input.Evidence != nil && input.Evidence.ID != uuid.Nil {
		var evidenceWorkspace uuid.UUID
		var objectType string
		var objectID uuid.UUID
		err := tx.QueryRow(ctx, `
			SELECT workspace_id, object_type, object_id
			FROM evidence_snapshot
			WHERE id=$1
			FOR SHARE
		`, input.Evidence.ID).Scan(&evidenceWorkspace, &objectType, &objectID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("load certification EvidenceSnapshot: %w", err)
		}
		input.Evidence.WorkspaceID = evidenceWorkspace
		input.Evidence.DatasetVersionID = objectID
		input.Evidence.Complete = err == nil && evidenceWorkspace == input.WorkspaceID && objectType == "DATASET_VERSION" && objectID == input.DatasetVersionID
	}
	return nil
}

func hashStrings(values ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(values, "\n")))
	return hex.EncodeToString(sum[:])
}

func scopeRefsKey(scopes []domain.ScopeRef) string {
	parts := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		parts = append(parts, scope.Type+"|"+scope.Ref)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func dereferenceString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
