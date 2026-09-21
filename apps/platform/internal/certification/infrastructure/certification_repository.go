package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/certification/domain"
)

var ErrCertificationNotFound = errors.New("dataset certification not found")

type CertificationRepository struct {
	pool *pgxpool.Pool
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
			traceability_evidence_id, evidence_snapshot_id, decision, blockers, reason, issued_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
	`, certification.ID, certification.WorkspaceID, certification.DatasetVersionID,
		certification.QualityAssessmentID, certification.Profile.ID, certification.Profile.ProfileRef,
		certification.Profile.Version, certification.Profile.ContentSHA256, string(certification.Profile.Content),
		certification.RightsSnapshotID, certification.EffectiveRightsSnapshotID,
		nullableString(certification.EffectiveRightsSnapshotHash), nullableString(certification.FrozenRightsContextHash),
		certification.ComplianceResultID, certification.ContractVersionID, certification.TraceabilityEvidenceID,
		certification.EvidenceSnapshotID, certification.Decision, blockers, certification.Reason,
		certification.IssuedAt, certification.ActorID)
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
	if err := tx.QueryRow(ctx, `
		SELECT id, workspace_id, dataset_version_id, quality_assessment_id,
		       rights_snapshot_id, effective_rights_snapshot_id, effective_rights_snapshot_hash,
		       frozen_rights_context_hash, compliance_result_id, contract_version_id,
		       traceability_evidence_id, evidence_snapshot_id, decision, blockers, reason, issued_at, created_by
		FROM dataset_certification WHERE id=$1
	`, certificationID).Scan(
		&certification.ID, &certification.WorkspaceID, &certification.DatasetVersionID,
		&certification.QualityAssessmentID, &certification.RightsSnapshotID,
		&certification.EffectiveRightsSnapshotID, &effectiveHash, &contextHash,
		&certification.ComplianceResultID, &certification.ContractVersionID,
		&certification.TraceabilityEvidenceID, &certification.EvidenceSnapshotID,
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
	certification.Profile = profile
	return certification, nil
}

// ListCertificationHistory intentionally returns every matching historical
// fact. Current selection belongs to the domain, where ambiguity can fail
// closed instead of being hidden by an ORDER BY created_at.
func (r *CertificationRepository) ListCertificationHistory(ctx context.Context, workspaceID, datasetVersionID, profileID uuid.UUID, asOf time.Time, profile domain.ProfileSnapshot) (CertificationHistory, error) {
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, workspace_id, dataset_version_id, quality_assessment_id,
		       rights_snapshot_id, effective_rights_snapshot_id, effective_rights_snapshot_hash,
		       frozen_rights_context_hash, compliance_result_id, contract_version_id,
		       traceability_evidence_id, evidence_snapshot_id, decision, blockers, reason, issued_at, created_by
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

	dispositionRows, err := r.pool.Query(ctx, `
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
	if err := row.Scan(
		&certification.ID, &certification.WorkspaceID, &certification.DatasetVersionID,
		&certification.QualityAssessmentID, &certification.RightsSnapshotID,
		&certification.EffectiveRightsSnapshotID, &effectiveHash, &contextHash,
		&certification.ComplianceResultID, &certification.ContractVersionID,
		&certification.TraceabilityEvidenceID, &certification.EvidenceSnapshotID,
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
