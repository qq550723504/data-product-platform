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
)

var (
	ErrInvalidEntityType       = errors.New("entity type is invalid")
	ErrInvalidEntity           = errors.New("entity is invalid")
	ErrReviewerReasonRequired  = errors.New("reviewer reason is required")
	ErrCandidateNotReviewable  = errors.New("match candidate is not reviewable")
	ErrCandidateEntityRequired = errors.New("candidate entity is required")
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
