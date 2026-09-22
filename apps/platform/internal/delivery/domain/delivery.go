package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusPrepared           Status = "PREPARED"
	StatusIssuancePending    Status = "ISSUANCE_PENDING"
	StatusContainmentPending Status = "CONTAINMENT_PENDING"
	StatusIssued             Status = "ISSUED"
	StatusBlocked            Status = "BLOCKED"
	StatusFailed             Status = "FAILED"
)

type ContainmentStatus string

const (
	ContainmentPending  ContainmentStatus = "PENDING"
	ContainmentResolved ContainmentStatus = "RESOLVED"
)

type GateStage string

const (
	GateInitial          GateStage = "INITIAL"
	GateProviderPrepare  GateStage = "PROVIDER_PREPARE"
	GateReconciliation   GateStage = "RECONCILIATION"
	GateTerminalFinalize GateStage = "TERMINAL_FINALIZE"
	GateReplay           GateStage = "REPLAY"
	GateContainment      GateStage = "CONTAINMENT"
)

type InvocationKind string

const (
	InvocationIssue     InvocationKind = "ISSUE"
	InvocationReconcile InvocationKind = "RECONCILE"
	InvocationRevoke    InvocationKind = "REVOKE"
	InvocationVerify    InvocationKind = "VERIFY"
)

type ObservationKind string

const (
	ObservationCallReturn        ObservationKind = "CALL_RETURN"
	ObservationTimeout           ObservationKind = "TIMEOUT"
	ObservationReconciliation    ObservationKind = "RECONCILIATION"
	ObservationAuthoritativeLook ObservationKind = "AUTHORITATIVE_LOOKUP"
)

type Outcome string

const (
	OutcomeSuccess  Outcome = "SUCCESS"
	OutcomeFailed   Outcome = "FAILED"
	OutcomeUnknown  Outcome = "UNKNOWN"
	OutcomeTimeout  Outcome = "TIMEOUT"
	OutcomeNotFound Outcome = "NOT_FOUND"
)

var (
	ErrInvalidOperation       = errors.New("invalid delivery operation")
	ErrInvalidTransition      = errors.New("invalid delivery operation transition")
	ErrTerminalOperation      = errors.New("delivery operation is terminal")
	ErrCapabilityTooBroad     = errors.New("provider capability is broader than requested context")
	ErrCapabilityUnverifiable = errors.New("provider capability cannot be authoritatively verified")
	ErrCredentialNotIssuable  = errors.New("credential cannot be issued")
)

type Operation struct {
	ID                          uuid.UUID
	WorkspaceID                 uuid.UUID
	DatasetVersionID            uuid.UUID
	CertificationRef           *uuid.UUID
	RetryOfDeliveryOperationID  *uuid.UUID
	IdempotencyKey              string
	ProviderName                string
	ProviderRequestKey          string
	Status                      Status
	CurrentGateDecision         string
	DependencyRevision          int64
	PrincipalRef                string
	EffectiveConsumerRef        string
	DelegationRef               string
	Purpose                     string
	Action                      string
	ScopeRef                    string
	DeliveryChannel             string
	DeliveryMode                string
	RequestedExpiresAt          time.Time
	FreshCapExpiresAt           *time.Time
	CredentialRef               string
	CredentialHash              string
	ProviderCredentialExpiresAt *time.Time
	TerminalReason              string
	CreatedAt                   time.Time
	UpdatedAt                   time.Time
}

func NewOperation(workspaceID, datasetVersionID uuid.UUID, idempotencyKey, providerName, principalRef, consumerRef, delegationRef, purpose, action, scopeRef, channel, mode string, requestedExpiresAt time.Time, certificationRef *uuid.UUID) (Operation, error) {
	if workspaceID == uuid.Nil || datasetVersionID == uuid.Nil || strings.TrimSpace(idempotencyKey) == "" || strings.TrimSpace(providerName) == "" || strings.TrimSpace(principalRef) == "" || strings.TrimSpace(consumerRef) == "" || strings.TrimSpace(purpose) == "" || strings.TrimSpace(action) == "" || strings.TrimSpace(scopeRef) == "" || strings.TrimSpace(channel) == "" || strings.TrimSpace(mode) == "" {
		return Operation{}, ErrInvalidOperation
	}
	if requestedExpiresAt.IsZero() || !requestedExpiresAt.After(time.Now().UTC()) {
		return Operation{}, ErrInvalidOperation
	}
	now := time.Now().UTC()
	return Operation{
		ID:                   uuid.New(),
		WorkspaceID:          workspaceID,
		DatasetVersionID:     datasetVersionID,
		CertificationRef:     certificationRef,
		IdempotencyKey:       strings.TrimSpace(idempotencyKey),
		ProviderName:         strings.TrimSpace(providerName),
		ProviderRequestKey:   "delivery/" + uuid.NewString(),
		Status:               StatusPrepared,
		CurrentGateDecision:  "BLOCKED",
		PrincipalRef:         strings.TrimSpace(principalRef),
		EffectiveConsumerRef: strings.TrimSpace(consumerRef),
		DelegationRef:        strings.TrimSpace(delegationRef),
		Purpose:              strings.TrimSpace(purpose),
		Action:               strings.TrimSpace(action),
		ScopeRef:             strings.TrimSpace(scopeRef),
		DeliveryChannel:      strings.TrimSpace(channel),
		DeliveryMode:         strings.TrimSpace(mode),
		RequestedExpiresAt:   requestedExpiresAt.UTC(),
		CreatedAt:            now,
		UpdatedAt:            now,
	}, nil
}

func (o *Operation) Transition(to Status) error {
	if o == nil {
		return ErrInvalidOperation
	}
	if o.IsTerminal() {
		return ErrTerminalOperation
	}
	allowed := map[Status]map[Status]bool{
		StatusPrepared: {
			StatusIssuancePending: true,
			StatusIssued:          true,
			StatusBlocked:         true,
			StatusFailed:          true,
		},
		StatusIssuancePending: {
			StatusIssued:             true,
			StatusBlocked:            true,
			StatusFailed:             true,
			StatusContainmentPending: true,
		},
		StatusContainmentPending: {
			StatusBlocked: true,
			StatusFailed:  true,
		},
	}
	if !allowed[o.Status][to] {
		return ErrInvalidTransition
	}
	o.Status = to
	o.UpdatedAt = time.Now().UTC()
	return nil
}

func (o Operation) IsTerminal() bool {
	return o.Status == StatusIssued || o.Status == StatusBlocked || o.Status == StatusFailed
}

type GateRequest struct {
	OperationID          uuid.UUID
	WorkspaceID          uuid.UUID
	DatasetVersionID     uuid.UUID
	CertificationRef     *uuid.UUID
	PrincipalRef         string
	EffectiveConsumerRef string
	DelegationRef        string
	Purpose              string
	Action               string
	ScopeRef             string
	DeliveryChannel      string
	DeliveryMode         string
	RequestedExpiresAt   time.Time
	Stage                GateStage
}

type GateEvaluation struct {
	ID                   uuid.UUID
	EvaluationKey        string
	Stage                GateStage
	Allowed              bool
	Blockers             []string
	DependencyRevision   int64
	PrincipalRef         string
	EffectiveConsumerRef string
	DelegationRef        string
	FreshCapExpiresAt    *time.Time
}

func (e GateEvaluation) Decision() string {
	if e.Allowed {
		return "ALLOWED"
	}
	return "BLOCKED"
}

type Capability struct {
	Credential                  string
	CapabilityRef               string
	CapabilityHash              string
	ProviderCredentialExpiresAt time.Time
	DatasetVersionID            uuid.UUID
	ConsumerRef                 string
	Action                      string
	ScopeRef                    string
	DeliveryChannel             string
	DeliveryMode                string
	AuthoritativelyVerified     bool
}

func (c Capability) ValidateAgainst(o Operation, evaluation GateEvaluation) error {
	if !c.AuthoritativelyVerified || strings.TrimSpace(c.CapabilityRef) == "" || strings.TrimSpace(c.Credential) == "" || c.ProviderCredentialExpiresAt.IsZero() {
		return ErrCapabilityUnverifiable
	}
	if c.DatasetVersionID != o.DatasetVersionID || strings.TrimSpace(c.ConsumerRef) != o.EffectiveConsumerRef || strings.TrimSpace(c.Action) != o.Action || strings.TrimSpace(c.ScopeRef) != o.ScopeRef || strings.TrimSpace(c.DeliveryChannel) != o.DeliveryChannel || strings.TrimSpace(c.DeliveryMode) != o.DeliveryMode {
		return ErrCapabilityTooBroad
	}
	if evaluation.FreshCapExpiresAt == nil || c.ProviderCredentialExpiresAt.After(evaluation.FreshCapExpiresAt.UTC()) || !c.ProviderCredentialExpiresAt.After(time.Now().UTC()) {
		return ErrCredentialNotIssuable
	}
	return nil
}
