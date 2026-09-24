package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	EngineOperationEnsureCampaign = "ENSURE_CAMPAIGN"
	EngineOperationSubmitTasks    = "SUBMIT_TASKS"

	EngineOperationPending  = "PENDING"
	EngineOperationSending  = "SENDING"
	EngineOperationUnknown  = "UNKNOWN"
	EngineOperationMatched  = "MATCHED"
	EngineOperationRejected = "REJECTED"
	EngineOperationConflict = "CONFLICT"

	EngineAttemptSubmit = "SUBMIT"
	EngineAttemptLookup = "LOOKUP"
	EngineAttemptFetch  = "FETCH"

	EngineAttemptSucceeded     = "SUCCEEDED"
	EngineAttemptUnknown       = "UNKNOWN"
	EngineAttemptRejected      = "REJECTED"
	EngineAttemptConflict      = "CONFLICT"
	EngineAttemptFailedPreSend = "FAILED_PRE_SEND"
)

var (
	ErrInvalidEngineOperation = errors.New("invalid annotation engine operation")
	ErrInvalidEngineAttempt   = errors.New("invalid annotation engine attempt")
	ErrInvalidEngineBinding   = errors.New("invalid annotation engine binding")
)

type EngineOperation struct {
	ID                    uuid.UUID
	WorkspaceID           uuid.UUID
	CampaignID            uuid.UUID
	Provider              string
	ProviderInstanceRef   string
	OperationKind         string
	RequestID             string
	RequestFingerprint    string
	PayloadManifest       []byte
	PayloadManifestHash   []byte
	PayloadManifestSHA256 string
	Status                string
	Revision              int64
	ClaimedBy             string
	ClaimExpiresAt        *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (o EngineOperation) Validate() error {
	if o.ID == uuid.Nil || o.WorkspaceID == uuid.Nil || o.CampaignID == uuid.Nil ||
		strings.TrimSpace(o.Provider) == "" || strings.TrimSpace(o.ProviderInstanceRef) == "" ||
		!validEngineOperationKind(o.OperationKind) || strings.TrimSpace(o.RequestID) == "" ||
		!isSHA256(o.RequestFingerprint) || len(o.PayloadManifest) == 0 || len(o.PayloadManifestHash) == 0 ||
		!isSHA256(o.PayloadManifestSHA256) || !validEngineOperationStatus(o.Status) || o.Revision < 1 {
		return ErrInvalidEngineOperation
	}
	return nil
}

type EngineAttempt struct {
	ID          uuid.UUID
	WorkspaceID uuid.UUID
	OperationID uuid.UUID
	AttemptNo   int
	AttemptKind string
	StartedAt   time.Time
}

func (a EngineAttempt) Validate() error {
	if a.ID == uuid.Nil || a.WorkspaceID == uuid.Nil || a.OperationID == uuid.Nil ||
		a.AttemptNo < 1 || !validEngineAttemptKind(a.AttemptKind) {
		return ErrInvalidEngineAttempt
	}
	return nil
}

type EngineAttemptOutcome struct {
	ID                 uuid.UUID
	AttemptID          uuid.UUID
	Outcome            string
	ProviderStatusCode *int
	DiagnosticRef      string
	OccurredAt         time.Time
}

func (o EngineAttemptOutcome) Validate() error {
	if o.ID == uuid.Nil || o.AttemptID == uuid.Nil || !validEngineAttemptOutcome(o.Outcome) {
		return ErrInvalidEngineAttempt
	}
	if o.ProviderStatusCode != nil && (*o.ProviderStatusCode < 100 || *o.ProviderStatusCode > 599) {
		return ErrInvalidEngineAttempt
	}
	return nil
}

type EngineCampaignBinding struct {
	ID                uuid.UUID
	WorkspaceID       uuid.UUID
	CampaignID        uuid.UUID
	Provider          string
	ProviderInstance  string
	ExternalProjectID string
	RequestID         string
	ConfigSHA256      string
	CreatedAt         time.Time
}

func (b EngineCampaignBinding) Validate() error {
	if b.ID == uuid.Nil || b.WorkspaceID == uuid.Nil || b.CampaignID == uuid.Nil ||
		strings.TrimSpace(b.Provider) == "" || strings.TrimSpace(b.ProviderInstance) == "" ||
		strings.TrimSpace(b.ExternalProjectID) == "" || strings.TrimSpace(b.RequestID) == "" ||
		!isSHA256(b.ConfigSHA256) {
		return ErrInvalidEngineBinding
	}
	return nil
}

type EngineActorBinding struct {
	ID               uuid.UUID
	WorkspaceID      uuid.UUID
	Provider         string
	ProviderInstance string
	ExternalActorRef string
	CoreActorRef     string
	CreatedAt        time.Time
	CreatedBy        *uuid.UUID
}

func (b EngineActorBinding) Validate() error {
	if b.ID == uuid.Nil || b.WorkspaceID == uuid.Nil ||
		strings.TrimSpace(b.Provider) == "" || strings.TrimSpace(b.ProviderInstance) == "" ||
		strings.TrimSpace(b.ExternalActorRef) == "" || strings.TrimSpace(b.CoreActorRef) == "" {
		return ErrInvalidEngineBinding
	}
	return nil
}

type EngineTaskBinding struct {
	ID                uuid.UUID
	WorkspaceID       uuid.UUID
	CampaignBindingID uuid.UUID
	CampaignID        uuid.UUID
	TaskID            uuid.UUID
	ExternalTaskID    string
	CreatedAt         time.Time
}

func (b EngineTaskBinding) Validate() error {
	if b.ID == uuid.Nil || b.WorkspaceID == uuid.Nil || b.CampaignBindingID == uuid.Nil ||
		b.CampaignID == uuid.Nil || b.TaskID == uuid.Nil || strings.TrimSpace(b.ExternalTaskID) == "" {
		return ErrInvalidEngineBinding
	}
	return nil
}

func validEngineOperationKind(value string) bool {
	return value == EngineOperationEnsureCampaign || value == EngineOperationSubmitTasks
}

func validEngineOperationStatus(value string) bool {
	switch value {
	case EngineOperationPending, EngineOperationSending, EngineOperationUnknown,
		EngineOperationMatched, EngineOperationRejected, EngineOperationConflict:
		return true
	default:
		return false
	}
}

func validEngineAttemptKind(value string) bool {
	return value == EngineAttemptSubmit || value == EngineAttemptLookup || value == EngineAttemptFetch
}

func validEngineAttemptOutcome(value string) bool {
	switch value {
	case EngineAttemptSucceeded, EngineAttemptUnknown, EngineAttemptRejected,
		EngineAttemptConflict, EngineAttemptFailedPreSend:
		return true
	default:
		return false
	}
}
