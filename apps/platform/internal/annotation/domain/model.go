package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	CampaignDraft     = "DRAFT"
	CampaignActive    = "ACTIVE"
	CampaignSealed    = "SEALED"
	CampaignCancelled = "CANCELLED"

	TaskPending    = "PENDING"
	TaskReviewable = "REVIEWABLE"
	TaskReviewed   = "REVIEWED"

	ReviewAccept  = "ACCEPT"
	ReviewReject  = "REJECT"
	ReviewCorrect = "CORRECT"

	ReviewAttemptSucceeded     = "SUCCEEDED"
	ReviewAttemptStaleConflict = "STALE_CONFLICT"
	ReviewAttemptRejected      = "REJECTED"

	SnapshotBuilding  = "BUILDING"
	SnapshotFinalized = "FINALIZED"
)

var (
	ErrInvalidCampaign       = errors.New("invalid annotation campaign")
	ErrInvalidTask           = errors.New("invalid annotation task")
	ErrInvalidResult         = errors.New("invalid annotation result")
	ErrInvalidReviewAttempt  = errors.New("invalid annotation review attempt")
	ErrInvalidReviewDecision = errors.New("invalid annotation review decision")
	ErrInvalidSnapshot       = errors.New("invalid annotation snapshot")
)

type FrozenSpec struct {
	Ref             string
	Version         string
	ContentSHA256   string
	ContentSnapshot string
}

func (s FrozenSpec) Validate(name string) error {
	if strings.TrimSpace(s.Ref) == "" || strings.TrimSpace(s.Version) == "" ||
		!isSHA256(s.ContentSHA256) || s.ContentSnapshot == "" {
		return fmt.Errorf("%w: %s spec is incomplete", ErrInvalidCampaign, name)
	}
	return nil
}

type Campaign struct {
	ID                       uuid.UUID
	WorkspaceID              uuid.UUID
	InputDatasetVersionID    uuid.UUID
	InputCertificationID     uuid.UUID
	AnnotationContributionID uuid.UUID
	Purpose                  string
	Action                   string
	ConsumerRef              string
	ScopeType                string
	ScopeRef                 string
	Schema                   FrozenSpec
	Taxonomy                 FrozenSpec
	Rubric                   FrozenSpec
	Renderer                 FrozenSpec
	ReviewPolicy             FrozenSpec
	Status                   string
	Revision                 int64
	ExpectedTaskCount        int
	TaskManifestHash         string
	CreatedAt                time.Time
	CreatedBy                *uuid.UUID
	ActivatedAt              *time.Time
	SealedAt                 *time.Time
	CancelledAt              *time.Time
}

type CampaignSpec struct {
	ID                       uuid.UUID
	WorkspaceID              uuid.UUID
	InputDatasetVersionID    uuid.UUID
	InputCertificationID     uuid.UUID
	AnnotationContributionID uuid.UUID
	Purpose                  string
	Action                   string
	ConsumerRef              string
	ScopeType                string
	ScopeRef                 string
	Schema                   FrozenSpec
	Taxonomy                 FrozenSpec
	Rubric                   FrozenSpec
	Renderer                 FrozenSpec
	ReviewPolicy             FrozenSpec
	ActorID                  *uuid.UUID
}

func NewCampaign(spec CampaignSpec, now time.Time) (Campaign, error) {
	if spec.WorkspaceID == uuid.Nil || spec.InputDatasetVersionID == uuid.Nil ||
		spec.InputCertificationID == uuid.Nil || spec.AnnotationContributionID == uuid.Nil ||
		strings.TrimSpace(spec.Purpose) == "" || strings.TrimSpace(spec.Action) == "" {
		return Campaign{}, ErrInvalidCampaign
	}
	for name, frozen := range map[string]FrozenSpec{
		"schema": spec.Schema, "taxonomy": spec.Taxonomy, "rubric": spec.Rubric,
		"renderer": spec.Renderer, "review policy": spec.ReviewPolicy,
	} {
		if err := frozen.Validate(name); err != nil {
			return Campaign{}, err
		}
	}
	if spec.ID == uuid.Nil {
		spec.ID = uuid.New()
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return Campaign{
		ID:                       spec.ID,
		WorkspaceID:              spec.WorkspaceID,
		InputDatasetVersionID:    spec.InputDatasetVersionID,
		InputCertificationID:     spec.InputCertificationID,
		AnnotationContributionID: spec.AnnotationContributionID,
		Purpose:                  strings.TrimSpace(spec.Purpose),
		Action:                   strings.TrimSpace(spec.Action),
		ConsumerRef:              strings.TrimSpace(spec.ConsumerRef),
		ScopeType:                strings.TrimSpace(spec.ScopeType),
		ScopeRef:                 strings.TrimSpace(spec.ScopeRef),
		Schema:                   spec.Schema,
		Taxonomy:                 spec.Taxonomy,
		Rubric:                   spec.Rubric,
		Renderer:                 spec.Renderer,
		ReviewPolicy:             spec.ReviewPolicy,
		Status:                   CampaignDraft,
		Revision:                 1,
		CreatedAt:                now.UTC(),
		CreatedBy:                spec.ActorID,
	}, nil
}

type Task struct {
	ID                  uuid.UUID
	WorkspaceID         uuid.UUID
	CampaignID          uuid.UUID
	SourceItemRef       string
	SourceContentSHA256 string
	TaskTextSHA256      string
	PrimaryAnnotatorRef string
	Status              string
	Revision            int64
	CurrentDecisionID   *uuid.UUID
	CreatedAt           time.Time
}

func NewTask(workspaceID, campaignID uuid.UUID, sourceItemRef, sourceHash, taskTextHash, annotatorRef string, now time.Time) (Task, error) {
	if workspaceID == uuid.Nil || campaignID == uuid.Nil || strings.TrimSpace(sourceItemRef) == "" ||
		!isSHA256(sourceHash) || !isSHA256(taskTextHash) {
		return Task{}, ErrInvalidTask
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return Task{
		ID:                  uuid.New(),
		WorkspaceID:         workspaceID,
		CampaignID:          campaignID,
		SourceItemRef:       strings.TrimSpace(sourceItemRef),
		SourceContentSHA256: sourceHash,
		TaskTextSHA256:      taskTextHash,
		PrimaryAnnotatorRef: strings.TrimSpace(annotatorRef),
		Status:              TaskPending,
		Revision:            1,
		CreatedAt:           now.UTC(),
	}, nil
}

type Result struct {
	ID                     uuid.UUID
	WorkspaceID            uuid.UUID
	CampaignID             uuid.UUID
	TaskID                 uuid.UUID
	AuthorRef              string
	ProviderBindingRef     string
	ExternalTaskID         string
	ExternalAnnotationID   string
	ExternalRevision       string
	ObservationKey         string
	CanonicalPayload       []byte
	CanonicalPayloadSHA256 string
	NormalizerVersion      string
	CorrectedFromResultID  *uuid.UUID
	CreatedAt              time.Time
	CreatedBy              *uuid.UUID
}

func (r Result) Validate() error {
	if r.ID == uuid.Nil || r.WorkspaceID == uuid.Nil || r.CampaignID == uuid.Nil || r.TaskID == uuid.Nil ||
		strings.TrimSpace(r.AuthorRef) == "" || strings.TrimSpace(r.ObservationKey) == "" || len(r.CanonicalPayload) == 0 ||
		!isSHA256(r.CanonicalPayloadSHA256) || strings.TrimSpace(r.NormalizerVersion) == "" {
		return ErrInvalidResult
	}
	return nil
}

type ReviewAttempt struct {
	ID                   uuid.UUID
	WorkspaceID          uuid.UUID
	CampaignID           uuid.UUID
	TaskID               uuid.UUID
	ReviewerRef          string
	ExpectedTaskRevision int64
	Action               string
	Reason               string
	IdempotencyKey       string
	RequestFingerprint   string
	CreatedAt            time.Time
}

func (a ReviewAttempt) Validate() error {
	if a.ID == uuid.Nil || a.WorkspaceID == uuid.Nil || a.CampaignID == uuid.Nil || a.TaskID == uuid.Nil ||
		strings.TrimSpace(a.ReviewerRef) == "" || a.ExpectedTaskRevision < 1 ||
		!validReviewAction(a.Action) || strings.TrimSpace(a.Reason) == "" ||
		strings.TrimSpace(a.IdempotencyKey) == "" || !isSHA256(a.RequestFingerprint) {
		return ErrInvalidReviewAttempt
	}
	return nil
}

type ReviewAttemptOutcome struct {
	ID         uuid.UUID
	AttemptID  uuid.UUID
	Outcome    string
	ErrorCode  string
	OccurredAt time.Time
}

func (o ReviewAttemptOutcome) Validate() error {
	if o.ID == uuid.Nil || o.AttemptID == uuid.Nil || !validReviewAttemptOutcome(o.Outcome) {
		return ErrInvalidReviewAttempt
	}
	return nil
}

type ReviewDecision struct {
	ID                   uuid.UUID
	WorkspaceID          uuid.UUID
	CampaignID           uuid.UUID
	TaskID               uuid.UUID
	ReviewAttemptID      uuid.UUID
	ReviewedResultID     *uuid.UUID
	SelectedResultID     *uuid.UUID
	ReviewerRef          string
	Outcome              string
	Reason               string
	ExpectedTaskRevision int64
	CreatedAt            time.Time
}

func (d ReviewDecision) Validate() error {
	if d.ID == uuid.Nil || d.WorkspaceID == uuid.Nil || d.CampaignID == uuid.Nil ||
		d.TaskID == uuid.Nil || d.ReviewAttemptID == uuid.Nil || strings.TrimSpace(d.ReviewerRef) == "" ||
		!validReviewAction(d.Outcome) || strings.TrimSpace(d.Reason) == "" || d.ExpectedTaskRevision < 1 {
		return ErrInvalidReviewDecision
	}
	switch d.Outcome {
	case ReviewAccept:
		if d.ReviewedResultID == nil || d.SelectedResultID == nil || *d.ReviewedResultID != *d.SelectedResultID {
			return ErrInvalidReviewDecision
		}
	case ReviewCorrect:
		if d.ReviewedResultID == nil || d.SelectedResultID == nil || *d.ReviewedResultID == *d.SelectedResultID {
			return ErrInvalidReviewDecision
		}
	case ReviewReject:
		if d.SelectedResultID != nil {
			return ErrInvalidReviewDecision
		}
	}
	return nil
}

type Snapshot struct {
	ID                    uuid.UUID
	WorkspaceID           uuid.UUID
	CampaignID            uuid.UUID
	Status                string
	Manifest              []byte
	ManifestHashPayload   []byte
	RootHash              string
	ExpectedTaskCount     int
	ExpectedResultCount   int
	ExpectedDecisionCount int
	ExpectedOutputCount   int
	CreatedAt             time.Time
	CreatedBy             *uuid.UUID
	FinalizedAt           *time.Time
}

func (s Snapshot) Validate() error {
	if s.ID == uuid.Nil || s.WorkspaceID == uuid.Nil || s.CampaignID == uuid.Nil ||
		(s.Status != SnapshotBuilding && s.Status != SnapshotFinalized) ||
		len(s.Manifest) == 0 || len(s.ManifestHashPayload) == 0 || !isSHA256(s.RootHash) ||
		s.ExpectedTaskCount < 1 || s.ExpectedResultCount < 0 || s.ExpectedDecisionCount < 1 ||
		s.ExpectedOutputCount < 0 || s.ExpectedDecisionCount != s.ExpectedTaskCount {
		return ErrInvalidSnapshot
	}
	return nil
}

func validReviewAction(v string) bool {
	return v == ReviewAccept || v == ReviewReject || v == ReviewCorrect
}

func validReviewAttemptOutcome(v string) bool {
	switch v {
	case ReviewAttemptSucceeded, ReviewAttemptStaleConflict, ReviewAttemptRejected:
		return true
	default:
		return false
	}
}

func isSHA256(v string) bool {
	if len(v) != 64 {
		return false
	}
	for _, r := range v {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
