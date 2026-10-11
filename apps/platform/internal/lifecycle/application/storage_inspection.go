package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

// Inspection is a provider observation, never current disposal authority or a
// reader admission. A HEAD result does not verify bytes or prove absence.
type ExactCopyInspection struct {
	Copy       domain.Copy
	ObservedAt time.Time
	CallID     uuid.UUID
}

type CopyProtection struct {
	Copy          domain.Copy
	RetentionMode string // NONE, GOVERNANCE or COMPLIANCE
	RetainUntil   *time.Time
	LegalHold     bool
	ObservedAt    time.Time
	CallIDs       []uuid.UUID
}

type ListedVersion struct {
	Key          string
	Version      string
	ByteSize     int64
	DeleteMarker bool
}

// NamespaceInventory covers one configured namespace at one successful native
// list response. It cannot cover provider-internal media, backups or replicas.
// Only the isolated single-page fixture mode is implemented initially.
type NamespaceInventory struct {
	WorkspaceID, ProviderInstanceID, IncarnationID uuid.UUID
	Namespace                                      string
	Versions                                       []ListedVersion
	ObservedAt                                     time.Time
	CallID                                         uuid.UUID
	ResponseSHA256                                 string
}

type StorageInspector interface {
	InspectExactCopy(context.Context, domain.Copy) (ExactCopyInspection, error)
	InspectRetentionAndHold(context.Context, domain.Copy) (CopyProtection, error)
	InspectNamespace(context.Context, domain.Copy) (NamespaceInventory, error)
}

var (
	ErrStorageScope       = errors.New("lifecycle storage scope mismatch")
	ErrStorageDenied      = errors.New("lifecycle storage denied")
	ErrStorageNotFound    = errors.New("lifecycle storage exact object not found; absence unverified")
	ErrStorageUnknown     = errors.New("lifecycle storage observation unknown")
	ErrStorageContract    = errors.New("lifecycle storage contract violation")
	ErrStorageUnsupported = errors.New("lifecycle storage capability unsupported")
)

const (
	StorageStatExact    = "STAT_EXACT"
	StorageRetention    = "INSPECT_RETENTION"
	StorageLegalHold    = "INSPECT_LEGAL_HOLD"
	StorageListVersions = "LIST_NAMESPACE_VERSIONS"
)

// One start identifies one actual HTTP attempt, rather than an SDK operation
// that may contain several calls. No URL, credentials, headers or payload are
// included. For listing, CopyID is the registered subject motivating the scan;
// Version/Key remain that subject's identity, not a listing prefix.
type StorageCallStart struct {
	ID              uuid.UUID
	Copy            domain.Copy
	Operation       string
	StartedAt       time.Time
	CopyOperationID uuid.UUID             // nonzero only for an already frozen custody intent
	CopyIntent      *domain.CopyOperation // full frozen target, checked before I/O
}

type StorageCallObservation struct {
	CallID            uuid.UUID
	ObservedAt        time.Time
	InvokedAt         *time.Time
	HTTPStatus        int
	RequestRef        string
	ResponseSHA256    string
	Failure           string // empty, TRANSPORT, CONTRACT or JOURNAL
	CopyResultVersion string // native COPY response header, never inferred from key
}

// StorageCallJournal must commit immutable starts before I/O, and append
// observations plus per-call CostEvent/typed allocation when InvokedAt is known.
// A start with no observation remains uncertain. Implementations must not hold
// database locks across I/O or erase a start on failure. There is deliberately
// no no-op/default implementation and no HTTP/worker wiring in this increment.
type StorageCallJournal interface {
	StartStorageCall(context.Context, StorageCallStart) error
	ObserveStorageCall(context.Context, StorageCallObservation) error
}
