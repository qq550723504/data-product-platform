package application

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrAnnotationEngineInvalidRequest  = errors.New("annotation engine invalid request")
	ErrAnnotationEngineUnauthorized    = errors.New("annotation engine unauthorized")
	ErrAnnotationEngineUnavailable     = errors.New("annotation engine unavailable")
	ErrAnnotationEngineRejected        = errors.New("annotation engine rejected request")
	ErrAnnotationEngineInvalidResponse = errors.New("annotation engine invalid response")
)

type EngineLookupState string

const (
	EngineLookupMatched  EngineLookupState = "MATCHED"
	EngineLookupUnknown  EngineLookupState = "UNKNOWN"
	EngineLookupConflict EngineLookupState = "CONFLICT"
)

type EngineCampaignRequest struct {
	WorkspaceID        uuid.UUID
	CampaignID         uuid.UUID
	RequestID          string
	RequestFingerprint string
	Title              string
	SchemaContent      string
	SchemaSHA256       string
}

type EngineCampaignBinding struct {
	Provider          string
	ProviderInstance  string
	ExternalProjectID string
	RequestID         string
	ConfigSHA256      string
}

type EngineCampaignLookup struct {
	State         EngineLookupState
	Binding       *EngineCampaignBinding
	DiagnosticRef string
}

type EngineTask struct {
	TaskID         uuid.UUID
	SourceItemRef  string
	SourceSHA256   string
	TaskText       string
	TaskTextSHA256 string
	CorrelationKey string
}

type EngineSubmitRequest struct {
	WorkspaceID        uuid.UUID
	CampaignID         uuid.UUID
	Binding            EngineCampaignBinding
	RequestID          string
	RequestFingerprint string
	Tasks              []EngineTask
}

type EngineSubmission struct {
	State           EngineLookupState
	RequestID       string
	ExternalTaskIDs map[uuid.UUID]string
	DiagnosticRef   string
}

type EngineLookupRequest struct {
	WorkspaceID        uuid.UUID
	CampaignID         uuid.UUID
	Binding            EngineCampaignBinding
	RequestID          string
	RequestFingerprint string
	Tasks              []EngineTask
}

type EngineResultCursor struct {
	Offset int
}

type EngineResultObservation struct {
	TaskID                 uuid.UUID
	ExternalTaskID         string
	ExternalAnnotationID   string
	ExternalRevision       string
	ExternalAuthorRef      string
	CanonicalPayload       []byte
	CanonicalPayloadSHA256 string
	NormalizerVersion      string
	ProviderSubmitted      bool
	ProviderCancelled      bool
	ProviderDiagnosticRef  string
}

type EngineResultPage struct {
	Results    []EngineResultObservation
	NextCursor *EngineResultCursor
}

type AnnotationEnginePort interface {
	Provider() string
	InstanceRef() string
	EnsureCampaignBinding(context.Context, EngineCampaignRequest) (EngineCampaignBinding, error)
	LookupCampaignBinding(context.Context, EngineCampaignRequest) (EngineCampaignLookup, error)
	VerifyCampaignBinding(context.Context, EngineCampaignBinding) error
	SubmitTasks(context.Context, EngineSubmitRequest) (EngineSubmission, error)
	LookupSubmission(context.Context, EngineLookupRequest) (EngineSubmission, error)
	FetchResults(context.Context, EngineLookupRequest, EngineResultCursor) (EngineResultPage, error)
}

type AnnotationEngineError struct {
	Kind             error
	Operation        string
	Retryable        bool
	OutcomeUncertain bool
	StatusCode       int
	Cause            error
}

func (e *AnnotationEngineError) Error() string {
	if e == nil {
		return ""
	}
	op := strings.TrimSpace(e.Operation)
	if op == "" {
		op = "annotation engine operation"
	}
	if e.Kind != nil {
		return op + ": " + e.Kind.Error()
	}
	return op + ": annotation engine error"
}

func (e *AnnotationEngineError) Unwrap() error {
	if e == nil {
		return nil
	}
	if e.Kind != nil {
		return e.Kind
	}
	return e.Cause
}

func NewAnnotationEngineError(kind error, operation string, retryable bool, statusCode int, cause error) error {
	return NewAnnotationEngineOutcomeError(kind, operation, retryable, false, statusCode, cause)
}

func NewAnnotationEngineOutcomeError(
	kind error,
	operation string,
	retryable bool,
	outcomeUncertain bool,
	statusCode int,
	cause error,
) error {
	return &AnnotationEngineError{
		Kind:             kind,
		Operation:        strings.TrimSpace(operation),
		Retryable:        retryable,
		OutcomeUncertain: outcomeUncertain,
		StatusCode:       statusCode,
		Cause:            cause,
	}
}
