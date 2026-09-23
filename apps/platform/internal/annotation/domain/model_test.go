package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func frozenSpec() FrozenSpec {
	return FrozenSpec{
		Ref:             "ref",
		Version:         "1.0.0",
		ContentSHA256:   strings.Repeat("a", 64),
		ContentSnapshot: "versioned-content",
	}
}

func TestNewCampaignFreezesRequiredSpecs(t *testing.T) {
	c, err := NewCampaign(CampaignSpec{
		WorkspaceID:              uuid.New(),
		InputDatasetVersionID:    uuid.New(),
		InputCertificationID:     uuid.New(),
		AnnotationContributionID: uuid.New(),
		Purpose:                  "gold-training",
		Action:                   "PROCESS",
		ConsumerRef:              "gold-pilot",
		Schema:                   frozenSpec(),
		Taxonomy:                 frozenSpec(),
		Rubric:                   frozenSpec(),
		Renderer:                 frozenSpec(),
		ReviewPolicy:             frozenSpec(),
	}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("NewCampaign: %v", err)
	}
	if c.Status != CampaignDraft || c.Revision != 1 {
		t.Fatalf("campaign state/revision = %s/%d, want DRAFT/1", c.Status, c.Revision)
	}
}

func TestReviewDecisionRequiresExplicitAuthoritativeResult(t *testing.T) {
	reviewed := uuid.New()
	selected := uuid.New()
	base := ReviewDecision{
		ID:                   uuid.New(),
		WorkspaceID:          uuid.New(),
		CampaignID:           uuid.New(),
		TaskID:               uuid.New(),
		ReviewAttemptID:      uuid.New(),
		ReviewedResultID:     &reviewed,
		SelectedResultID:     &reviewed,
		ReviewerRef:          "reviewer",
		Outcome:              ReviewAccept,
		Reason:               "verified",
		ExpectedTaskRevision: 2,
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid ACCEPT: %v", err)
	}

	correct := base
	correct.ID = uuid.New()
	correct.Outcome = ReviewCorrect
	correct.SelectedResultID = &selected
	if err := correct.Validate(); err != nil {
		t.Fatalf("valid CORRECT: %v", err)
	}

	reject := base
	reject.ID = uuid.New()
	reject.Outcome = ReviewReject
	reject.SelectedResultID = nil
	if err := reject.Validate(); err != nil {
		t.Fatalf("valid REJECT: %v", err)
	}

	bad := base
	bad.ID = uuid.New()
	bad.Outcome = ReviewAccept
	bad.SelectedResultID = &selected
	if err := bad.Validate(); err == nil {
		t.Fatal("ACCEPT with a different selected result should fail")
	}
}

func TestReviewAttemptAllowsNonSuccessPhysicalActivity(t *testing.T) {
	a := ReviewAttempt{
		ID:                   uuid.New(),
		WorkspaceID:          uuid.New(),
		CampaignID:           uuid.New(),
		TaskID:               uuid.New(),
		ReviewerRef:          "reviewer",
		ExpectedTaskRevision: 3,
		Action:               ReviewAccept,
		Reason:               "checked",
		IdempotencyKey:       "attempt-1",
		RequestFingerprint:   strings.Repeat("b", 64),
		Outcome:              ReviewAttemptStaleConflict,
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("stale review attempt must remain a valid auditable activity: %v", err)
	}
}

func TestSnapshotRequiresFullTaskDecisionDenominator(t *testing.T) {
	s := Snapshot{
		ID:                    uuid.New(),
		WorkspaceID:           uuid.New(),
		CampaignID:            uuid.New(),
		Status:                SnapshotBuilding,
		Manifest:              []byte(`{"tasks":[]}`),
		ManifestHashPayload:   []byte(`{"tasks":[]}`),
		RootHash:              strings.Repeat("c", 64),
		ExpectedTaskCount:     2,
		ExpectedResultCount:   2,
		ExpectedDecisionCount: 1,
		ExpectedOutputCount:   1,
	}
	if err := s.Validate(); err == nil {
		t.Fatal("snapshot with fewer decisions than tasks should fail")
	}
}
