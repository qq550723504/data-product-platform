package application

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
)

func TestActivateCampaignFailsClosedWithoutGuard(t *testing.T) {
	service := NewService(nil, nil, nil)
	_, err := service.ActivateCampaign(t.Context(), ActivateCampaignCommand{
		WorkspaceID: uuid.New(),
		CampaignID:  uuid.New(),
	})
	if !errors.Is(err, ErrActivationGuardRequired) {
		t.Fatalf("ActivateCampaign error = %v, want activation guard required", err)
	}
}

func TestSnapshotManifestKeepsRejectedTaskInDecisionDenominator(t *testing.T) {
	campaign := annotationdomain.Campaign{
		ID:                       uuid.New(),
		WorkspaceID:              uuid.New(),
		InputDatasetVersionID:    uuid.New(),
		InputCertificationID:     uuid.New(),
		AnnotationContributionID: uuid.New(),
		TaskManifestHash:         strings.Repeat("a", 64),
		InputChecksumSHA256:      strings.Repeat("9", 64),
		Schema:                   annotationdomain.FrozenSpec{Ref: "schema", Version: "1", ContentSHA256: strings.Repeat("b", 64), ContentSnapshot: "schema"},
		Taxonomy:                 annotationdomain.FrozenSpec{Ref: "taxonomy", Version: "1", ContentSHA256: strings.Repeat("c", 64), ContentSnapshot: "taxonomy"},
		Rubric:                   annotationdomain.FrozenSpec{Ref: "rubric", Version: "1", ContentSHA256: strings.Repeat("d", 64), ContentSnapshot: "rubric"},
		Renderer:                 annotationdomain.FrozenSpec{Ref: "renderer", Version: "1", ContentSHA256: strings.Repeat("e", 64), ContentSnapshot: "renderer"},
		ReviewPolicy:             annotationdomain.FrozenSpec{Ref: "review", Version: "1", ContentSHA256: strings.Repeat("f", 64), ContentSnapshot: "review"},
	}
	taskID := uuid.New()
	task := annotationdomain.Task{
		ID: taskID, WorkspaceID: campaign.WorkspaceID, CampaignID: campaign.ID,
		SourceItemRef: "row:1", SourceContentSHA256: strings.Repeat("1", 64),
		TaskTextSHA256: strings.Repeat("2", 64), PrimaryAnnotatorRef: "annotator",
		Status: annotationdomain.TaskReviewed, Revision: 2,
	}
	decision := annotationdomain.ReviewDecision{
		ID: uuid.New(), WorkspaceID: campaign.WorkspaceID, CampaignID: campaign.ID, TaskID: taskID,
		ReviewAttemptID: uuid.New(), ReviewerRef: "reviewer", Outcome: annotationdomain.ReviewReject,
		Reason: "invalid annotation", ExpectedTaskRevision: 1,
	}

	builtAt := time.Date(2026, 9, 23, 10, 0, 0, 123456000, time.UTC)
	encoded, outputCount, err := snapshotManifest(
		campaign, []annotationdomain.Task{task}, nil, []annotationdomain.ReviewDecision{decision}, builtAt, nil,
	)
	if err != nil {
		t.Fatalf("snapshotManifest: %v", err)
	}
	if outputCount != 0 {
		t.Fatalf("output count = %d, want 0 for REJECT", outputCount)
	}
	var manifest struct {
		Tasks     []any `json:"tasks"`
		Decisions []any `json:"decisions"`
		Outputs   []any `json:"outputs"`
	}
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(manifest.Tasks) != 1 || len(manifest.Decisions) != 1 || len(manifest.Outputs) != 0 {
		t.Fatalf("manifest denominator tasks/decisions/outputs = %d/%d/%d, want 1/1/0",
			len(manifest.Tasks), len(manifest.Decisions), len(manifest.Outputs))
	}
}

func TestTaskManifestHashIsOrderIndependent(t *testing.T) {
	campaignID := uuid.New()
	workspaceID := uuid.New()
	first := annotationdomain.Task{
		ID:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		WorkspaceID: workspaceID, CampaignID: campaignID, SourceItemRef: "row:1",
		SourceContentSHA256: strings.Repeat("1", 64), TaskTextSHA256: strings.Repeat("2", 64),
	}
	second := annotationdomain.Task{
		ID:          uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		WorkspaceID: workspaceID, CampaignID: campaignID, SourceItemRef: "row:2",
		SourceContentSHA256: strings.Repeat("3", 64), TaskTextSHA256: strings.Repeat("4", 64),
	}
	left, err := taskManifestHash([]annotationdomain.Task{first, second})
	if err != nil {
		t.Fatalf("hash left: %v", err)
	}
	right, err := taskManifestHash([]annotationdomain.Task{second, first})
	if err != nil {
		t.Fatalf("hash right: %v", err)
	}
	if left != right {
		t.Fatalf("task manifest hash depends on input order: %s != %s", left, right)
	}
}

func TestCampaignFingerprintChangesWithSemanticPayload(t *testing.T) {
	base := annotationdomain.CampaignSpec{
		WorkspaceID:              uuid.New(),
		InputDatasetVersionID:    uuid.New(),
		InputCertificationID:     uuid.New(),
		AnnotationContributionID: uuid.New(),
		Purpose:                  "gold-training",
		Action:                   "PROCESS",
		Schema: annotationdomain.FrozenSpec{
			Ref: "schema", Version: "1", ContentSHA256: strings.Repeat("a", 64), ContentSnapshot: "schema",
		},
		Taxonomy: annotationdomain.FrozenSpec{
			Ref: "taxonomy", Version: "1", ContentSHA256: strings.Repeat("b", 64), ContentSnapshot: "taxonomy",
		},
		Rubric: annotationdomain.FrozenSpec{
			Ref: "rubric", Version: "1", ContentSHA256: strings.Repeat("c", 64), ContentSnapshot: "rubric",
		},
		Renderer: annotationdomain.FrozenSpec{
			Ref: "renderer", Version: "1", ContentSHA256: strings.Repeat("d", 64), ContentSnapshot: "renderer",
		},
		ReviewPolicy: annotationdomain.FrozenSpec{
			Ref: "review", Version: "1", ContentSHA256: strings.Repeat("e", 64), ContentSnapshot: "review",
		},
	}
	left, err := campaignFingerprint(base)
	if err != nil {
		t.Fatalf("campaign fingerprint: %v", err)
	}
	changed := base
	changed.Purpose = "different-purpose"
	right, err := campaignFingerprint(changed)
	if err != nil {
		t.Fatalf("changed campaign fingerprint: %v", err)
	}
	if left == right {
		t.Fatal("campaign fingerprint must change when command semantics change")
	}
}

func TestReviewFingerprintChangesWithConflictingPayload(t *testing.T) {
	resultID := uuid.New()
	base := ReviewAnnotationCommand{
		WorkspaceID:          uuid.New(),
		CampaignID:           uuid.New(),
		TaskID:               uuid.New(),
		ExpectedTaskRevision: 2,
		ReviewerRef:          "reviewer",
		Action:               annotationdomain.ReviewAccept,
		Reason:               "verified",
		IdempotencyKey:       "review-key",
		ReviewedResultID:     &resultID,
	}
	left, err := reviewFingerprint(base)
	if err != nil {
		t.Fatalf("review fingerprint: %v", err)
	}
	changed := base
	changed.Reason = "different-reason"
	right, err := reviewFingerprint(changed)
	if err != nil {
		t.Fatalf("changed review fingerprint: %v", err)
	}
	if left == right {
		t.Fatal("review fingerprint must change when same-key command semantics change")
	}
}

func TestValidateAnnotationPayloadAgainstFrozenSchema(t *testing.T) {
	schema := pilotSchemaSpec("A", "B")
	if err := validateAnnotationPayload(schema, []byte("{\"label\":\"A\"}")); err != nil {
		t.Fatalf("valid payload: %v", err)
	}
	if err := validateAnnotationPayload(schema, []byte("{\"label\":\"C\"}")); err == nil {
		t.Fatal("unknown label should be rejected")
	}
	if err := validateAnnotationPayload(schema, []byte("{\"label\":\"A\",\"extra\":true}")); err == nil {
		t.Fatal("payload with fields outside frozen schema should be rejected")
	}
}

func TestProviderReplayIgnoresObservationAlias(t *testing.T) {
	existing := annotationdomain.Result{
		ID:                     uuid.New(),
		WorkspaceID:            uuid.New(),
		CampaignID:             uuid.New(),
		TaskID:                 uuid.New(),
		AuthorRef:              "annotator",
		ProviderBindingRef:     "provider-binding",
		ExternalTaskID:         "external-task",
		ExternalAnnotationID:   "external-annotation",
		ExternalRevision:       "7",
		ObservationKey:         "old-alias",
		CanonicalPayloadSHA256: strings.Repeat("a", 64),
		NormalizerVersion:      "v1",
	}
	cmd := RecordResultCommand{
		WorkspaceID:            existing.WorkspaceID,
		CampaignID:             existing.CampaignID,
		TaskID:                 existing.TaskID,
		AuthorRef:              existing.AuthorRef,
		ProviderBindingRef:     existing.ProviderBindingRef,
		ExternalTaskID:         existing.ExternalTaskID,
		ExternalAnnotationID:   existing.ExternalAnnotationID,
		ExternalRevision:       existing.ExternalRevision,
		ObservationKey:         "new-alias",
		CanonicalPayloadSHA256: existing.CanonicalPayloadSHA256,
		NormalizerVersion:      existing.NormalizerVersion,
	}

	if !providerResultReplayMatches(existing, cmd) {
		t.Fatal("provider identity replay should ignore observation alias")
	}
	if resultReplayMatches(existing, cmd) {
		t.Fatal("alias-key replay must remain strict when observation alias differs")
	}
}

func TestValidateReplayAliasRejectsAliasBoundToDifferentResult(t *testing.T) {
	existing := annotationdomain.Result{
		ID:             uuid.New(),
		ObservationKey: "old-alias",
	}
	other := annotationdomain.Result{
		ID:             uuid.New(),
		ObservationKey: "new-alias",
	}

	if err := validateReplayAlias(existing, "old-alias", nil); err != nil {
		t.Fatalf("original alias should remain valid: %v", err)
	}
	if err := validateReplayAlias(existing, "new-alias", nil); err != nil {
		t.Fatalf("unbound alias should be accepted for provider replay: %v", err)
	}
	if err := validateReplayAlias(existing, "new-alias", &other); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting alias error = %v, want idempotency conflict", err)
	}
}
