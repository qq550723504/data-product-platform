package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type DatasetType string

type DatasetLifecycle string

type VersionStatus string

const (
	DatasetTypeRaw          DatasetType = "RAW"
	DatasetTypeStandardized DatasetType = "STANDARDIZED"
	DatasetTypeCurated      DatasetType = "CURATED"
	DatasetTypeProduct      DatasetType = "PRODUCT"

	DatasetLifecycleActive     DatasetLifecycle = "ACTIVE"
	DatasetLifecycleDeprecated DatasetLifecycle = "DEPRECATED"
	DatasetLifecycleArchived   DatasetLifecycle = "ARCHIVED"

	VersionCreated    VersionStatus = "CREATED"
	VersionProcessing VersionStatus = "PROCESSING"
	VersionReady      VersionStatus = "READY"
	VersionFailed     VersionStatus = "FAILED"
	VersionInvalid    VersionStatus = "INVALID"
	VersionSuperseded VersionStatus = "SUPERSEDED"
)

var (
	ErrInvalidWorkspace     = errors.New("workspace id is required")
	ErrInvalidDatasetCode   = errors.New("dataset code is required")
	ErrInvalidDatasetName   = errors.New("dataset name is required")
	ErrInvalidDatasetType   = errors.New("invalid dataset type")
	ErrInvalidVersion       = errors.New("version number must be positive")
	ErrInvalidTransition    = errors.New("invalid dataset version state transition")
	ErrStaleVersionRecovery = errors.New("dataset version recovery would replace a newer current version")
	ErrImmutableVersion       = errors.New("dataset version is immutable")
	ErrInvalidReadyMetadata   = errors.New("ready dataset version requires storage URI and checksum")
	ErrIdempotencyKeyRequired = errors.New("dataset upload idempotency key is required")
	ErrIdempotencyConflict    = errors.New("dataset upload idempotency key is already bound to another request")
	// ErrSourceResourceWorkspace rejects a dataset whose source DataResource
	// belongs to another workspace.
	ErrSourceResourceWorkspace = errors.New("source resource belongs to a different workspace")
	// ErrDatasetWorkspace rejects work that mixes datasets from another workspace.
	ErrDatasetWorkspace = errors.New("dataset belongs to a different workspace")
)

type Dataset struct {
	ID               uuid.UUID
	WorkspaceID      uuid.UUID
	ProjectID        *uuid.UUID
	Code             string
	Name             string
	Description      string
	DatasetType      DatasetType
	SourceResourceID *uuid.UUID
	OwnerID          *uuid.UUID
	CurrentVersionID *uuid.UUID
	LifecycleStatus  DatasetLifecycle
	Metadata         map[string]any
	CreatedAt        time.Time
	CreatedBy        *uuid.UUID
}

type DatasetVersion struct {
	ID                          uuid.UUID
	DatasetID                   uuid.UUID
	VersionNo                   int64
	Status                      VersionStatus
	SchemaVersion               string
	StorageType                 string
	StorageURI                  string
	ContentType                 string
	RowCount                    *int64
	ByteSize                    *int64
	ChecksumAlgorithm           string
	ChecksumValue               string
	GeneratedByExecutionID      *uuid.UUID
	GeneratedByEntityMatchJobID *uuid.UUID
	RightsSnapshotID            *uuid.UUID
	SnapshotFrom                *time.Time
	SnapshotTo                  *time.Time
	Metadata                    map[string]any
	CreatedAt                   time.Time
	CreatedBy                   *uuid.UUID
	ReadyAt                     *time.Time
	InvalidatedAt               *time.Time
	InvalidationReason          string
}

func NewDataset(id, workspaceID uuid.UUID, code, name string, datasetType DatasetType, sourceResourceID *uuid.UUID, createdBy *uuid.UUID) (Dataset, error) {
	if id == uuid.Nil {
		id = uuid.New()
	}
	if workspaceID == uuid.Nil {
		return Dataset{}, ErrInvalidWorkspace
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return Dataset{}, ErrInvalidDatasetCode
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Dataset{}, ErrInvalidDatasetName
	}
	if !datasetType.Valid() {
		return Dataset{}, ErrInvalidDatasetType
	}

	return Dataset{
		ID:               id,
		WorkspaceID:      workspaceID,
		Code:             code,
		Name:             name,
		DatasetType:      datasetType,
		SourceResourceID: sourceResourceID,
		LifecycleStatus:  DatasetLifecycleActive,
		Metadata:         map[string]any{},
		CreatedAt:        time.Now().UTC(),
		CreatedBy:        createdBy,
	}, nil
}

func NewVersion(id, datasetID uuid.UUID, versionNo int64, createdBy *uuid.UUID) (DatasetVersion, error) {
	if id == uuid.Nil {
		id = uuid.New()
	}
	if datasetID == uuid.Nil || versionNo <= 0 {
		return DatasetVersion{}, ErrInvalidVersion
	}
	return DatasetVersion{
		ID:        id,
		DatasetID: datasetID,
		VersionNo: versionNo,
		Status:    VersionCreated,
		Metadata:  map[string]any{},
		CreatedAt: time.Now().UTC(),
		CreatedBy: createdBy,
	}, nil
}

func (d DatasetType) Valid() bool {
	switch d {
	case DatasetTypeRaw, DatasetTypeStandardized, DatasetTypeCurated, DatasetTypeProduct:
		return true
	default:
		return false
	}
}

func (v *DatasetVersion) StartProcessing() error {
	if v.Status != VersionCreated && v.Status != VersionFailed {
		return ErrInvalidTransition
	}
	v.Status = VersionProcessing
	return nil
}

// MarkReady makes the version's content durable and publishes it.
//
// FAILED is an accepted source state: a failed attempt never stored content, and
// the output idempotency key ties the retry to that same row, so recovery reuses
// the version number instead of consuming a new one. The state change either way
// is CREATED/PROCESSING/FAILED -> READY, which is what the caller records as a
// domain event and an audit event.
func (v *DatasetVersion) MarkReady(storageType, storageURI, contentType, checksumAlgorithm, checksumValue string, rowCount, byteSize int64) error {
	if v.Status != VersionCreated && v.Status != VersionProcessing && v.Status != VersionFailed {
		return ErrInvalidTransition
	}
	if strings.TrimSpace(storageURI) == "" || strings.TrimSpace(checksumValue) == "" {
		return ErrInvalidReadyMetadata
	}
	storageType = strings.TrimSpace(storageType)
	if storageType == "" {
		storageType = "OBJECT_STORAGE"
	}
	rowCountCopy := rowCount
	byteSizeCopy := byteSize
	now := time.Now().UTC()
	v.StorageType = storageType
	v.StorageURI = storageURI
	v.ContentType = contentType
	v.ChecksumAlgorithm = checksumAlgorithm
	v.ChecksumValue = checksumValue
	v.RowCount = &rowCountCopy
	v.ByteSize = &byteSizeCopy
	v.ReadyAt = &now
	v.Status = VersionReady
	return nil
}

func (v *DatasetVersion) MarkFailed() error {
	if v.Status != VersionCreated && v.Status != VersionProcessing {
		return ErrInvalidTransition
	}
	v.Status = VersionFailed
	return nil
}

func (v *DatasetVersion) Invalidate(reason string) error {
	if v.Status != VersionReady {
		if v.Status == VersionInvalid || v.Status == VersionSuperseded {
			return ErrImmutableVersion
		}
		return ErrInvalidTransition
	}
	now := time.Now().UTC()
	v.Status = VersionInvalid
	v.InvalidatedAt = &now
	v.InvalidationReason = strings.TrimSpace(reason)
	return nil
}

func (v *DatasetVersion) Supersede() error {
	if v.Status != VersionReady {
		return ErrInvalidTransition
	}
	v.Status = VersionSuperseded
	return nil
}
