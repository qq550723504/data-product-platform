package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
)

var ErrNotFound = errors.New("dataset object not found")

// versionColumns is the single SELECT list for hydrating a DatasetVersion. It is
// shared so a new column cannot be added to one query and silently dropped from
// another (scanVersion expects exactly this order).
const versionColumns = `
	id, dataset_id, version_no, status,
	COALESCE(schema_version, ''),
	COALESCE(storage_type, ''),
	COALESCE(storage_uri, ''),
	COALESCE(content_type, ''),
	row_count, byte_size,
	COALESCE(checksum_algorithm, ''),
	COALESCE(checksum_value, ''),
	generated_by_execution_id, generated_by_entity_match_job_id, rights_snapshot_id,
	snapshot_from, snapshot_to, metadata, created_at, created_by, ready_at,
	invalidated_at, COALESCE(invalidation_reason, '')`

type PostgresRepository struct {
	pool *pgxpool.Pool
}

type UploadIdempotencyRecord struct {
	VersionID          uuid.UUID
	RequestFingerprint string
}


func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) InsertDataset(ctx context.Context, tx pgx.Tx, dataset domain.Dataset) error {
	metadata, err := json.Marshal(dataset.Metadata)
	if err != nil {
		return fmt.Errorf("marshal dataset metadata: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO dataset (
			id, workspace_id, project_id, code, name, description, dataset_type,
			source_resource_id, owner_id, lifecycle_status, metadata,
			created_at, created_by, updated_at, updated_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$12,$13)
	`,
		dataset.ID,
		dataset.WorkspaceID,
		dataset.ProjectID,
		dataset.Code,
		dataset.Name,
		dataset.Description,
		dataset.DatasetType,
		dataset.SourceResourceID,
		dataset.OwnerID,
		dataset.LifecycleStatus,
		metadata,
		dataset.CreatedAt,
		dataset.CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("insert dataset: %w", err)
	}
	return nil
}

// AllocateVersion creates the next version row for a Dataset, or returns the row
// already owned by the same producer.
//
// Execution and EntityMatchJob are distinct producer identities and are mutually
// exclusive. Both are written at allocation time so a replay reuses the same
// half-product/output instead of consuming a second version number. The returned
// bool reports whether an existing row was reused instead of a new one being
// allocated.
func (r *PostgresRepository) AllocateVersion(ctx context.Context, tx pgx.Tx, datasetID uuid.UUID, createdBy, generatedByExecutionID, generatedByEntityMatchJobID *uuid.UUID) (domain.DatasetVersion, bool, error) {
	if generatedByExecutionID != nil && generatedByEntityMatchJobID != nil {
		return domain.DatasetVersion{}, false, fmt.Errorf("dataset version cannot have both execution and entity-match producers")
	}

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dataset WHERE id = $1 AND deleted_at IS NULL)`, datasetID).Scan(&exists); err != nil {
		return domain.DatasetVersion{}, false, fmt.Errorf("check dataset: %w", err)
	}
	if !exists {
		return domain.DatasetVersion{}, false, ErrNotFound
	}

	// Serialize version allocation using the Dataset row rather than a global lock.
	if _, err := tx.Exec(ctx, `SELECT id FROM dataset WHERE id = $1 FOR UPDATE`, datasetID); err != nil {
		return domain.DatasetVersion{}, false, fmt.Errorf("lock dataset: %w", err)
	}

	if generatedByExecutionID != nil {
		existing, found, err := findVersionByExecutionOutputTx(ctx, tx, datasetID, *generatedByExecutionID)
		if err != nil {
			return domain.DatasetVersion{}, false, err
		}
		if found {
			return existing, true, nil
		}
	}

	if generatedByEntityMatchJobID != nil {
		existing, found, err := findVersionByEntityMatchJobOutputTx(ctx, tx, datasetID, *generatedByEntityMatchJobID)
		if err != nil {
			return domain.DatasetVersion{}, false, err
		}
		if found {
			return existing, true, nil
		}
	}

	var versionNo int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(version_no), 0) + 1 FROM dataset_version WHERE dataset_id = $1`, datasetID).Scan(&versionNo); err != nil {
		return domain.DatasetVersion{}, false, fmt.Errorf("allocate dataset version: %w", err)
	}
	version, err := domain.NewVersion(uuid.New(), datasetID, versionNo, createdBy)
	if err != nil {
		return domain.DatasetVersion{}, false, err
	}
	// The idempotency key must be written at allocation time, not at SetReady:
	// otherwise a concurrent replay could allocate a second half-product row that
	// the partial unique index never sees.
	version.GeneratedByExecutionID = generatedByExecutionID
	version.GeneratedByEntityMatchJobID = generatedByEntityMatchJobID
	if err := r.insertVersion(ctx, tx, version); err != nil {
		// Defense in depth: the Dataset lock already serializes allocation, but a row
		// inserted through another path must still not become a second output.
		if generatedByExecutionID != nil && isExecutionOutputConflict(err) {
			existing, found, findErr := findVersionByExecutionOutputTx(ctx, tx, datasetID, *generatedByExecutionID)
			if findErr != nil {
				return domain.DatasetVersion{}, false, findErr
			}
			if found {
				return existing, true, nil
			}
		}
		if generatedByEntityMatchJobID != nil && isEntityMatchOutputConflict(err) {
			existing, found, findErr := findVersionByEntityMatchJobOutputTx(ctx, tx, datasetID, *generatedByEntityMatchJobID)
			if findErr != nil {
				return domain.DatasetVersion{}, false, findErr
			}
			if found {
				return existing, true, nil
			}
		}
		return domain.DatasetVersion{}, false, err
	}
	return version, false, nil
}

// findVersionByExecutionOutputTx reads the output row already owned by an
// Execution, if any, including half-products (CREATED/PROCESSING/FAILED).
//
// A published READY row wins over a half-product so that an installation which
// accumulated several rows for one pair before C2-a replays idempotently against
// the output that is actually published. Among rows of the same state the oldest
// wins, so a half-product is always repaired in place instead of a second row
// being picked arbitrarily.
func findVersionByExecutionOutputTx(ctx context.Context, tx pgx.Tx, datasetID, executionID uuid.UUID) (domain.DatasetVersion, bool, error) {
	version, err := scanVersion(tx.QueryRow(ctx, `
		SELECT `+versionColumns+`
		FROM dataset_version
		WHERE dataset_id = $1 AND generated_by_execution_id = $2
		ORDER BY
			(status = 'READY') DESC,
			(status IN ('CREATED', 'PROCESSING')) DESC,
			(status = 'FAILED') DESC,
			version_no ASC
		LIMIT 1
	`, datasetID, executionID))
	if errors.Is(err, ErrNotFound) {
		return domain.DatasetVersion{}, false, nil
	}
	if err != nil {
		return domain.DatasetVersion{}, false, err
	}
	return version, true, nil
}

// findVersionByEntityMatchJobOutputTx reads the output row already owned by an
// EntityMatchJob, including retryable half-products and published history.
func findVersionByEntityMatchJobOutputTx(ctx context.Context, tx pgx.Tx, datasetID, jobID uuid.UUID) (domain.DatasetVersion, bool, error) {
	version, err := scanVersion(tx.QueryRow(ctx, `
		SELECT `+versionColumns+`
		FROM dataset_version
		WHERE dataset_id = $1 AND generated_by_entity_match_job_id = $2
		ORDER BY
			(status = 'READY') DESC,
			(status = 'SUPERSEDED') DESC,
			(status IN ('CREATED', 'PROCESSING')) DESC,
			(status = 'FAILED') DESC,
			version_no ASC
		LIMIT 1
	`, datasetID, jobID))
	if errors.Is(err, ErrNotFound) {
		return domain.DatasetVersion{}, false, nil
	}
	if err != nil {
		return domain.DatasetVersion{}, false, err
	}
	return version, true, nil
}

// isExecutionOutputConflict reports a unique violation on the C2-a output index.
func isExecutionOutputConflict(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505" && pgErr.ConstraintName == "uq_dataset_version_execution_output"
}

func isEntityMatchOutputConflict(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505" && pgErr.ConstraintName == "uq_dataset_version_entity_match_output"
}

func (r *PostgresRepository) insertVersion(ctx context.Context, tx pgx.Tx, version domain.DatasetVersion) error {
	metadata, err := json.Marshal(version.Metadata)
	if err != nil {
		return fmt.Errorf("marshal dataset version metadata: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, metadata, created_at, created_by,
			generated_by_execution_id, generated_by_entity_match_job_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, version.ID, version.DatasetID, version.VersionNo, version.Status, metadata, version.CreatedAt, version.CreatedBy,
		version.GeneratedByExecutionID, version.GeneratedByEntityMatchJobID)
	if err != nil {
		return fmt.Errorf("insert dataset version: %w", err)
	}
	return nil
}

// LockVersion locks a version row and returns its committed state. The output
// writer uses it to prove, at commit time, that it is still the attempt allowed
// to publish: a concurrent delivery of the same Execution may already have done
// so, in which case the loser adopts the published row instead of rewriting it.
func (r *PostgresRepository) LockVersion(ctx context.Context, tx pgx.Tx, versionID uuid.UUID) (domain.DatasetVersion, error) {
	return scanVersion(tx.QueryRow(ctx, `
		SELECT `+versionColumns+`
		FROM dataset_version WHERE id = $1 FOR UPDATE
	`, versionID))
}

// SetReady publishes a version under the row lock, requiring exactly one row.
//
// FAILED is an accepted source state: several deliveries of one Execution share
// the same output row, so a losing attempt whose object write failed may have
// marked it FAILED after this attempt allocated it. This attempt holds content
// that is durably stored, so it republishes the row in one atomic transition
// (the same CREATED/PROCESSING/FAILED -> READY change the domain models). The
// transition is deliberately strict: a zero-row update means the committed state
// is no longer publishable (for example another caller already invalidated it),
// and reporting success there would announce a READY fact the database does not
// hold.
func (r *PostgresRepository) SetReady(ctx context.Context, tx pgx.Tx, version domain.DatasetVersion) (*domain.DatasetVersion, error) {
	if version.Status != domain.VersionReady {
		return nil, fmt.Errorf("set ready requires READY domain state")
	}
	workspaceID, err := r.workspaceForDataset(ctx, tx, version.DatasetID)
	if err != nil {
		return nil, err
	}
	if _, err := deliveryfence.Advance(ctx, tx, workspaceID); err != nil {
		return nil, err
	}

	var previousVersionID *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT current_version_id FROM dataset WHERE id = $1 FOR UPDATE`, version.DatasetID).Scan(&previousVersionID); err != nil {
		return nil, fmt.Errorf("lock dataset current version: %w", err)
	}
	var previousVersion *domain.DatasetVersion
	if previousVersionID != nil && *previousVersionID != version.ID {
		lockedPrevious, err := scanVersion(tx.QueryRow(ctx, `
			SELECT `+versionColumns+`
			FROM dataset_version
			WHERE id = $1
			FOR UPDATE
		`, *previousVersionID))
		if err != nil {
			return nil, fmt.Errorf("read current dataset version: %w", err)
		}
		if lockedPrevious.VersionNo > version.VersionNo {
			return nil, fmt.Errorf("dataset version %s is older than current version %s: %w", version.ID, *previousVersionID, domain.ErrStaleVersionRecovery)
		}
		if lockedPrevious.Status == domain.VersionReady {
			copy := lockedPrevious
			previousVersion = &copy
		}
	}
	metadata, err := json.Marshal(version.Metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal dataset version metadata: %w", err)
	}

	commandTag, err := tx.Exec(ctx, `
		UPDATE dataset_version
		SET status = $2,
		    storage_type = $3,
		    storage_uri = $4,
		    content_type = $5,
		    row_count = $6,
		    byte_size = $7,
		    checksum_algorithm = $8,
		    checksum_value = $9,
		    generated_by_execution_id = $10,
		    generated_by_entity_match_job_id = $11,
		    metadata = $12,
		    ready_at = $13
		WHERE id = $1 AND status IN ('CREATED','PROCESSING','FAILED')
	`,
		version.ID,
		version.Status,
		version.StorageType,
		version.StorageURI,
		version.ContentType,
		version.RowCount,
		version.ByteSize,
		version.ChecksumAlgorithm,
		version.ChecksumValue,
		version.GeneratedByExecutionID,
		version.GeneratedByEntityMatchJobID,
		metadata,
		version.ReadyAt,
	)
	if err != nil {
		return nil, fmt.Errorf("mark dataset version ready: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return nil, fmt.Errorf("mark dataset version ready: %w", domain.ErrInvalidTransition)
	}

	if previousVersion != nil {
		tag, err := tx.Exec(ctx, `UPDATE dataset_version SET status = 'SUPERSEDED' WHERE id = $1 AND status = 'READY'`, previousVersion.ID)
		if err != nil {
			return nil, fmt.Errorf("supersede previous dataset version: %w", err)
		}
		if tag.RowsAffected() == 1 {
			previousVersion.Status = domain.VersionSuperseded
		} else {
			previousVersion = nil
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE dataset SET current_version_id = $2, updated_at = now() WHERE id = $1`, version.DatasetID, version.ID); err != nil {
		return nil, fmt.Errorf("set dataset current version: %w", err)
	}
	return previousVersion, nil
}

// SetFailed records a failed attempt on a half-product. It is deliberately
// tolerant of zero rows: several deliveries share one output row, so an attempt
// whose object write failed must not fail a row another delivery already
// published. Only CREATED/PROCESSING rows can be failed.
func (r *PostgresRepository) SetFailed(ctx context.Context, tx pgx.Tx, versionID uuid.UUID) error {
	workspaceID, err := r.workspaceForVersion(ctx, tx, versionID)
	if err != nil {
		return err
	}
	if _, err := deliveryfence.Advance(ctx, tx, workspaceID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE dataset_version SET status = 'FAILED' WHERE id = $1 AND status IN ('CREATED','PROCESSING')`, versionID)
	if err != nil {
		return fmt.Errorf("mark dataset version failed: %w", err)
	}
	return nil
}

// Fail applies the explicit DatasetVersion failure command. Unlike SetFailed,
// which is best-effort for an object-store attempt that may race a publisher,
// this method requires the requested state transition to win exactly once.
func (r *PostgresRepository) Fail(ctx context.Context, tx pgx.Tx, version domain.DatasetVersion) error {
	if version.Status != domain.VersionFailed {
		return fmt.Errorf("fail requires FAILED domain state")
	}
	commandTag, err := tx.Exec(ctx, `
		UPDATE dataset_version
		SET status = 'FAILED'
		WHERE id = $1 AND status IN ('CREATED','PROCESSING')
	`, version.ID)
	if err != nil {
		return fmt.Errorf("fail dataset version: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return domain.ErrInvalidTransition
	}
	return nil
}

func (r *PostgresRepository) Invalidate(ctx context.Context, tx pgx.Tx, version domain.DatasetVersion) error {
	if version.Status != domain.VersionInvalid {
		return fmt.Errorf("invalidate requires INVALID domain state")
	}
	commandTag, err := tx.Exec(ctx, `
		UPDATE dataset_version
		SET status = 'INVALID', invalidated_at = $2, invalidation_reason = $3
		WHERE id = $1 AND status = 'READY'
	`, version.ID, version.InvalidatedAt, version.InvalidationReason)
	if err != nil {
		return fmt.Errorf("invalidate dataset version: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return domain.ErrInvalidTransition
	}
	if _, err := tx.Exec(ctx, `UPDATE dataset SET current_version_id = NULL, updated_at = now() WHERE current_version_id = $1`, version.ID); err != nil {
		return fmt.Errorf("clear invalid current dataset version: %w", err)
	}
	return nil
}

// GetWorkspaceAndType returns the workspace and type of a dataset. Callers use
// it to keep multi-step business actions inside one tenant: a foreign key on
// dataset(id) proves the dataset exists but not who owns it.
func (r *PostgresRepository) GetWorkspaceAndType(ctx context.Context, datasetID uuid.UUID) (uuid.UUID, domain.DatasetType, error) {
	var workspaceID uuid.UUID
	var datasetType domain.DatasetType
	err := r.pool.QueryRow(ctx, `
		SELECT workspace_id, dataset_type FROM dataset
		WHERE id=$1 AND deleted_at IS NULL
	`, datasetID).Scan(&workspaceID, &datasetType)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", ErrNotFound
	}
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("read dataset workspace and type: %w", err)
	}
	return workspaceID, datasetType, nil
}

func (r *PostgresRepository) LockUploadIdempotencyTx(ctx context.Context, tx pgx.Tx, workspaceID uuid.UUID, key string) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2))`, workspaceID.String(), "DATASET.UPLOAD_VERSION:"+key); err != nil {
		return fmt.Errorf("lock dataset upload idempotency key: %w", err)
	}
	return nil
}

func (r *PostgresRepository) FindUploadIdempotencyTx(ctx context.Context, tx pgx.Tx, workspaceID uuid.UUID, key string) (UploadIdempotencyRecord, bool, error) {
	var record UploadIdempotencyRecord
	err := tx.QueryRow(ctx, `
		SELECT object_id, COALESCE(request_fingerprint, '')
		FROM command_idempotency
		WHERE workspace_id=$1 AND command_type='DATASET.UPLOAD_VERSION' AND idempotency_key=$2
		FOR UPDATE
	`, workspaceID, key).Scan(&record.VersionID, &record.RequestFingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return UploadIdempotencyRecord{}, false, nil
	}
	if err != nil {
		return UploadIdempotencyRecord{}, false, fmt.Errorf("find dataset upload idempotency record: %w", err)
	}
	return record, true, nil
}

func (r *PostgresRepository) InsertUploadIdempotencyTx(ctx context.Context, tx pgx.Tx, workspaceID uuid.UUID, key string, versionID uuid.UUID, fingerprint string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO command_idempotency (
			workspace_id, command_type, idempotency_key, object_id, result_ref, request_fingerprint
		) VALUES ($1, 'DATASET.UPLOAD_VERSION', $2, $3, $3, $4)
	`, workspaceID, key, versionID, fingerprint)
	if err != nil {
		return fmt.Errorf("insert dataset upload idempotency record: %w", err)
	}
	return nil
}

func (r *PostgresRepository) WorkspaceForDatasetTx(ctx context.Context, tx pgx.Tx, datasetID uuid.UUID) (uuid.UUID, error) {
	var workspaceID uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT workspace_id FROM dataset WHERE id=$1 AND deleted_at IS NULL
	`, datasetID).Scan(&workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("read dataset workspace: %w", err)
	}
	return workspaceID, nil
}

func (r *PostgresRepository) GetVersion(ctx context.Context, versionID uuid.UUID) (domain.DatasetVersion, error) {
	return scanVersion(r.pool.QueryRow(ctx, `
		SELECT `+versionColumns+`
		FROM dataset_version WHERE id = $1
	`, versionID))
}

func (r *PostgresRepository) GetVersionTx(ctx context.Context, tx pgx.Tx, versionID uuid.UUID) (domain.DatasetVersion, error) {
	return scanVersion(tx.QueryRow(ctx, `
		SELECT `+versionColumns+`
		FROM dataset_version WHERE id = $1
	`, versionID))
}

func scanVersion(row pgx.Row) (domain.DatasetVersion, error) {
	var v domain.DatasetVersion
	var metadataBytes []byte
	err := row.Scan(
		&v.ID,
		&v.DatasetID,
		&v.VersionNo,
		&v.Status,
		&v.SchemaVersion,
		&v.StorageType,
		&v.StorageURI,
		&v.ContentType,
		&v.RowCount,
		&v.ByteSize,
		&v.ChecksumAlgorithm,
		&v.ChecksumValue,
		&v.GeneratedByExecutionID,
		&v.GeneratedByEntityMatchJobID,
		&v.RightsSnapshotID,
		&v.SnapshotFrom,
		&v.SnapshotTo,
		&metadataBytes,
		&v.CreatedAt,
		&v.CreatedBy,
		&v.ReadyAt,
		&v.InvalidatedAt,
		&v.InvalidationReason,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DatasetVersion{}, ErrNotFound
	}
	if err != nil {
		return domain.DatasetVersion{}, fmt.Errorf("scan dataset version: %w", err)
	}
	if len(metadataBytes) > 0 {
		decoder := json.NewDecoder(bytes.NewReader(metadataBytes))
		decoder.UseNumber()
		if err := decoder.Decode(&v.Metadata); err != nil {
			return domain.DatasetVersion{}, fmt.Errorf("decode dataset version metadata: %w", err)
		}
	}
	return v, nil
}
