package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type EntityStatus string

type MappingStatus string

type MatchDecision string

type CandidateStatus string

type JobStatus string

type SourceRole string

type SourceOrigin string

const (
	EntityActive  EntityStatus = "ACTIVE"
	EntityMerged  EntityStatus = "MERGED"
	EntityRetired EntityStatus = "RETIRED"

	MappingAutoMatched MappingStatus = "AUTO_MATCHED"
	MappingConfirmed   MappingStatus = "CONFIRMED"
	MappingRejected    MappingStatus = "REJECTED"
	MappingConflict    MappingStatus = "CONFLICT"

	DecisionAutoMatch  MatchDecision = "AUTO_MATCH"
	DecisionReview     MatchDecision = "REVIEW"
	DecisionUnresolved MatchDecision = "UNRESOLVED"

	CandidateAutoConfirmed CandidateStatus = "AUTO_CONFIRMED"
	CandidatePending       CandidateStatus = "PENDING"
	CandidateConfirmed     CandidateStatus = "CONFIRMED"
	CandidateRejected      CandidateStatus = "REJECTED"
	CandidateUnresolved    CandidateStatus = "UNRESOLVED"

	JobQueued        JobStatus = "QUEUED"
	JobRunning       JobStatus = "RUNNING"
	JobWaitingReview JobStatus = "WAITING_REVIEW"
	JobSucceeded     JobStatus = "SUCCEEDED"
	JobFailed        JobStatus = "FAILED"

	SourceAnchor    SourceRole = "ANCHOR"
	SourceReference SourceRole = "REFERENCE"

	OriginMatchCandidate SourceOrigin = "MATCH_CANDIDATE"
	OriginWorkflowAlias  SourceOrigin = "WORKFLOW_ALIAS"
)

func (o SourceOrigin) Valid() bool {
	switch o {
	case OriginMatchCandidate, OriginWorkflowAlias:
		return true
	default:
		return false
	}
}

var (
	ErrInvalidEntityType       = errors.New("entity type is invalid")
	ErrInvalidEntity           = errors.New("entity is invalid")
	ErrReviewerReasonRequired  = errors.New("reviewer reason is required")
	ErrCandidateNotReviewable  = errors.New("match candidate is not reviewable")
	ErrCandidateEntityRequired = errors.New("candidate entity is required")
	// ErrOutputDatasetType rejects entity resolution output written into a
	// dataset that is not a STANDARDIZED dataset.
	ErrOutputDatasetType = errors.New("entity resolution output dataset must be STANDARDIZED")
	// ErrMappingWorkspaceRequired rejects a mapping that is not bound to a
	// workspace, because mappings are only unique inside one workspace.
	ErrMappingWorkspaceRequired = errors.New("entity mapping workspace is required")
	// ErrMappingDecisionConflict reports that the current decision changed since
	// the caller computed its decision; the caller must re-read before retrying.
	ErrMappingDecisionConflict = errors.New("entity mapping current decision changed concurrently")
	// ErrMappingConfirmedImmutable rejects an automatic match that would replace
	// a mapping a human already confirmed. Human confirmation always wins.
	ErrMappingConfirmedImmutable = errors.New("a human-confirmed entity mapping cannot be replaced by automatic matching")
	// ErrMappingDecisionKeyConflict rejects reusing one idempotency key for a
	// different request. Retries of the same operation are idempotent, but a key
	// reused for a different target, reason, actor or source is a conflict.
	ErrMappingDecisionKeyConflict = errors.New("mapping decision idempotency key is already bound to another request")
	ErrMappingDecisionKeyRequired = errors.New("mapping decision idempotency key is required")
	ErrMappingDecisionOriginRequired = errors.New("mapping decision source origin is required")
	// ErrMappingDecisionExpectationRequired rejects a manual confirmation that
	// would replace an existing current decision while the caller did not state
	// which decision it observed. Observed state must be explicit, not inferred
	// by re-reading the latest value at submit time.
	ErrMappingDecisionExpectationRequired = errors.New("replacing a current entity mapping decision requires the expected current decision")
)

type EntityType struct {
	ID                    uuid.UUID
	WorkspaceID           uuid.UUID
	Code                  string
	Name                  string
	KeySchema             map[string]any
	AttributeSchema       map[string]any
	MatchingPolicyRef     string
	MatchingPolicyVersion string
	CreatedAt             time.Time
}

type Entity struct {
	ID            uuid.UUID
	WorkspaceID   uuid.UUID
	EntityTypeID  uuid.UUID
	CanonicalKey  string
	CanonicalName string
	Attributes    map[string]any
	Status        EntityStatus
	CreatedAt     time.Time
	CreatedBy     *uuid.UUID
}

type EntityMapping struct {
	ID                 uuid.UUID
	WorkspaceID        uuid.UUID
	EntityID           uuid.UUID
	SourceType         string
	SourceRef          string
	SourceKey          string
	SourceName         string
	MatchMethod        string
	MatchRuleID        string
	MatchPolicyVersion string
	MatchEngineName    string
	MatchEngineVersion string
	MatchModelVersion  string
	Confidence         float64
	Status             MappingStatus
	ReviewedBy         *uuid.UUID
	ReviewedAt         *time.Time
	ReviewerReason     string
	EvidenceID         *uuid.UUID
	CreatedAt          time.Time
	// CurrentDecisionID points at the immutable decision that produced this
	// projection. It is never nil for a persisted mapping.
	CurrentDecisionID *uuid.UUID
}

// MappingDecision is one immutable entry in the decision history of a mapping.
// The current projection lives in EntityMapping; every accepted decision is also
// appended here so prior decisions are never overwritten.
type MappingDecision struct {
	ID                 uuid.UUID
	WorkspaceID        uuid.UUID
	MappingID          uuid.UUID
	EntityID           uuid.UUID
	SourceType         string
	SourceRef          string
	SourceKey          string
	SourceName         string
	MatchMethod        string
	MatchRuleID        string
	MatchPolicyVersion string
	MatchEngineName    string
	MatchEngineVersion string
	MatchModelVersion  string
	Confidence         float64
	Status             MappingStatus
	ReviewedBy         *uuid.UUID
	ReviewedAt         *time.Time
	ReviewerReason     string
	EvidenceID         *uuid.UUID
	DecidedAt          time.Time
	DecidedBy          *uuid.UUID
	// IdempotencyKey deduplicates a retried decision operation and is required
	// for every current decision command.
	IdempotencyKey string
	// SourceOrigin records the explicit current source of the decision.
	SourceOrigin SourceOrigin
	// SourceJobID and SourceCandidateID associate the decision with the entity
	// matching activity that produced it, when one can be proven.
	SourceJobID       *uuid.UUID
	SourceCandidateID *uuid.UUID
}

// MappingDecisionCommand appends one immutable decision and moves the current
// projection pointer in the same transaction.
type MappingDecisionCommand struct {
	Mapping           EntityMapping
	SourceOrigin      SourceOrigin
	SourceJobID       *uuid.UUID
	SourceCandidateID *uuid.UUID
	IdempotencyKey    string
	DecidedBy         *uuid.UUID
	DecidedAt         time.Time
	// ExpectCurrentDecision enables the optimistic concurrency check. When true,
	// the append fails with ErrMappingDecisionConflict unless the mapping's
	// current decision still equals ExpectedCurrentDecisionID (nil = none yet).
	ExpectCurrentDecision     bool
	ExpectedCurrentDecisionID *uuid.UUID
}

type MatchJob struct {
	ID                     uuid.UUID
	WorkspaceID            uuid.UUID
	EntityTypeID           uuid.UUID
	InputDatasetVersionID  uuid.UUID
	OutputDatasetID        uuid.UUID
	SourceType             string
	SourceRef              string
	SourceRole             SourceRole
	PolicyRef              string
	PolicyVersion          string
	PolicyContentSHA256    string
	PolicyContent          []byte
	Status                 JobStatus
	AutoMatchCount         int64
	ReviewCount            int64
	UnresolvedCount        int64
	RejectedCount          int64
	OutputDatasetVersionID *uuid.UUID
	ErrorMessage           string
	CreatedAt              time.Time
	CreatedBy              *uuid.UUID
	StartedAt              *time.Time
	FinishedAt             *time.Time
}

type MatchCandidate struct {
	ID                 uuid.UUID
	JobID              uuid.UUID
	SourceKey          string
	SourceName         string
	SourcePayload      map[string]string
	NormalizedPayload  map[string]string
	CandidateEntityID  *uuid.UUID
	Decision           MatchDecision
	Status             CandidateStatus
	MatchMethod        string
	MatchRuleID        string
	MatchEngineName    string
	MatchEngineVersion string
	MatchModelVersion  string
	Confidence         float64
	ReviewedBy         *uuid.UUID
	ReviewedAt         *time.Time
	ReviewerReason     string
	EvidenceID         *uuid.UUID
	CreatedAt          time.Time
}

func NewEntityType(workspaceID uuid.UUID, code, name, policyRef, policyVersion string) (EntityType, error) {
	if workspaceID == uuid.Nil || strings.TrimSpace(code) == "" || strings.TrimSpace(name) == "" {
		return EntityType{}, ErrInvalidEntityType
	}
	return EntityType{
		ID:                    uuid.New(),
		WorkspaceID:           workspaceID,
		Code:                  strings.TrimSpace(code),
		Name:                  strings.TrimSpace(name),
		KeySchema:             map[string]any{},
		AttributeSchema:       map[string]any{},
		MatchingPolicyRef:     strings.TrimSpace(policyRef),
		MatchingPolicyVersion: strings.TrimSpace(policyVersion),
		CreatedAt:             time.Now().UTC(),
	}, nil
}

func NewEntity(workspaceID, entityTypeID uuid.UUID, canonicalKey, canonicalName string, attributes map[string]any, actorID *uuid.UUID) (Entity, error) {
	if workspaceID == uuid.Nil || entityTypeID == uuid.Nil || strings.TrimSpace(canonicalName) == "" {
		return Entity{}, ErrInvalidEntity
	}
	if attributes == nil {
		attributes = map[string]any{}
	}
	return Entity{
		ID:            uuid.New(),
		WorkspaceID:   workspaceID,
		EntityTypeID:  entityTypeID,
		CanonicalKey:  strings.TrimSpace(canonicalKey),
		CanonicalName: strings.TrimSpace(canonicalName),
		Attributes:    attributes,
		Status:        EntityActive,
		CreatedAt:     time.Now().UTC(),
		CreatedBy:     actorID,
	}, nil
}

func NewMatchJob(workspaceID, entityTypeID, inputVersionID, outputDatasetID uuid.UUID, sourceType, sourceRef string, sourceRole SourceRole, policyRef, policyVersion string, actorID *uuid.UUID) MatchJob {
	return MatchJob{
		ID:                    uuid.New(),
		WorkspaceID:           workspaceID,
		EntityTypeID:          entityTypeID,
		InputDatasetVersionID: inputVersionID,
		OutputDatasetID:       outputDatasetID,
		SourceType:            strings.TrimSpace(sourceType),
		SourceRef:             strings.TrimSpace(sourceRef),
		SourceRole:            sourceRole,
		PolicyRef:             strings.TrimSpace(policyRef),
		PolicyVersion:         strings.TrimSpace(policyVersion),
		Status:                JobQueued,
		CreatedAt:             time.Now().UTC(),
		CreatedBy:             actorID,
	}
}

// EnsureMappingDecisionAllowed enforces that automatic matching never replaces a
// mapping a human confirmed. Human confirmation always wins; a later human
// confirmation may still supersede an earlier one.
func EnsureMappingDecisionAllowed(currentStatus *MappingStatus, next MappingStatus) error {
	if currentStatus != nil && *currentStatus == MappingConfirmed && next == MappingAutoMatched {
		return ErrMappingConfirmedImmutable
	}
	return nil
}

// SameMappingDecision reports whether two optional current-decision pointers
// refer to the same decision, including both being absent.
func SameMappingDecision(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// EnsureMappingDecisionExpectation makes the concurrency contract explicit for
// manual confirmations:
//
//   - a first confirmation (no current decision) is safe even without a token;
//   - replacing an existing decision requires expectCurrent and an expected
//     pointer equal to the observed decision;
//   - a confirmation that does not request the check is rejected when a current
//     decision already exists, instead of silently overwriting it.
//
// Automatic decisions are governed by EnsureMappingDecisionAllowed, not here.
func EnsureMappingDecisionExpectation(currentDecisionID *uuid.UUID, expectCurrent bool, expected *uuid.UUID, next MappingStatus) error {
	if expectCurrent {
		if !SameMappingDecision(currentDecisionID, expected) {
			return ErrMappingDecisionConflict
		}
		return nil
	}
	if next == MappingConfirmed && currentDecisionID != nil {
		return ErrMappingDecisionExpectationRequired
	}
	return nil
}

// MappingDecisionMatchesRequest reports whether a retried command has the same
// request semantics as the decision already recorded under its idempotency key.
// Random identifiers (decision id, mapping id, evidence id) and timestamps are
// ignored so genuine retries stay idempotent, while a different target entity,
// decision status, reviewer reason, actor, provenance or source association is
// treated as an idempotency-key conflict.
func MappingDecisionMatchesRequest(existing MappingDecision, cmd MappingDecisionCommand) bool {
	mapping := cmd.Mapping
	origin := cmd.SourceOrigin
	return existing.EntityID == mapping.EntityID &&
		existing.SourceType == mapping.SourceType &&
		existing.SourceRef == mapping.SourceRef &&
		existing.SourceKey == mapping.SourceKey &&
		existing.SourceName == mapping.SourceName &&
		existing.MatchMethod == mapping.MatchMethod &&
		existing.MatchRuleID == mapping.MatchRuleID &&
		existing.MatchPolicyVersion == mapping.MatchPolicyVersion &&
		existing.MatchEngineName == mapping.MatchEngineName &&
		existing.MatchEngineVersion == mapping.MatchEngineVersion &&
		existing.MatchModelVersion == mapping.MatchModelVersion &&
		existing.Confidence == mapping.Confidence &&
		existing.Status == mapping.Status &&
		existing.ReviewerReason == mapping.ReviewerReason &&
		SameMappingDecision(existing.ReviewedBy, mapping.ReviewedBy) &&
		existing.SourceOrigin == origin &&
		SameMappingDecision(existing.SourceJobID, cmd.SourceJobID) &&
		SameMappingDecision(existing.SourceCandidateID, cmd.SourceCandidateID)
}

// NormalizeMappingDecisionKey trims the required idempotency key and rejects
// missing or oversized keys.
func NormalizeMappingDecisionKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrMappingDecisionKeyRequired
	}
	if len(value) > 255 {
		return "", ErrMappingDecisionKeyConflict
	}
	return value, nil
}

func (c *MatchCandidate) Confirm(reviewerID uuid.UUID, reason string) error {
	if c.Status != CandidatePending || c.CandidateEntityID == nil {
		return ErrCandidateNotReviewable
	}
	reason = strings.TrimSpace(reason)
	if reviewerID == uuid.Nil || reason == "" {
		return ErrReviewerReasonRequired
	}
	now := time.Now().UTC()
	c.Status = CandidateConfirmed
	c.ReviewedBy = &reviewerID
	c.ReviewedAt = &now
	c.ReviewerReason = reason
	return nil
}

func (c *MatchCandidate) Reject(reviewerID uuid.UUID, reason string) error {
	if c.Status != CandidatePending {
		return ErrCandidateNotReviewable
	}
	reason = strings.TrimSpace(reason)
	if reviewerID == uuid.Nil || reason == "" {
		return ErrReviewerReasonRequired
	}
	now := time.Now().UTC()
	c.Status = CandidateRejected
	c.ReviewedBy = &reviewerID
	c.ReviewedAt = &now
	c.ReviewerReason = reason
	return nil
}
