package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGoldCertificationRejectsBlockingQualityFailure(t *testing.T) {
	workspaceID := uuid.New()
	versionID := uuid.New()
	qualityID := uuid.New()
	rightsID := uuid.New()
	evidenceID := uuid.New()
	bindingID := uuid.New()
	snapshotID := uuid.New()

	profileSpec, err := NewGoldCertificationProfile(
		"GOLD-PILOT",
		"USE",
		"GOLD-PILOT-CONSUMER",
		[]ScopeRef{{Type: "ALL_RESOURCE", Ref: "resource-1"}},
	)
	if err != nil {
		t.Fatalf("Gold profile: %v", err)
	}
	profile, err := profileSpec.SnapshotForWorkspace(workspaceID, nil)
	if err != nil {
		t.Fatalf("Gold profile snapshot: %v", err)
	}

	dimensions := map[string]string{}
	for _, dimension := range profile.RequiredQualityDimensions {
		dimensions[dimension] = EvidencePass
	}
	rules := map[string]string{}
	for _, ruleID := range profile.RequiredCriticalRules {
		rules[ruleID] = EvidencePass
	}

	certification, err := Evaluate(profile, EvaluationInput{
		WorkspaceID:          workspaceID,
		DatasetVersionID:     versionID,
		DatasetVersionStatus: "READY",
		Derived:              true,
		Quality: QualityAssessmentEvidence{
			ID:               qualityID,
			WorkspaceID:      workspaceID,
			DatasetVersionID: versionID,
			GateDecision:     EvidenceFail,
			Dimensions:       dimensions,
			RuleStatuses:     rules,
		},
		Rights: &RightsEvidence{
			WorkspaceID:                 workspaceID,
			DatasetVersionID:            versionID,
			EffectiveRightsSnapshotID:   rightsID,
			EffectiveRightsSnapshotHash: "rights-root",
			FrozenRightsContextHash:     "rights-context",
			EffectiveRightsFinalized:    true,
			RequiredInputSetHash:        "required-set",
			TargetLineageInputSetHash:   "required-set",
			ActionDecisions:             map[string]string{"USE": RightsAllowed},
			Coverage: RightsCoverage{
				Purpose:   Applicability{Mode: ApplicabilityExplicit, Values: []string{"GOLD-PILOT"}},
				Actions:   Applicability{Mode: ApplicabilityExplicit, Values: []string{"USE"}},
				Consumers: Applicability{Mode: ApplicabilityExplicit, Values: []string{"GOLD-PILOT-CONSUMER"}},
				Scopes:    ScopeApplicability{Mode: ApplicabilityExplicit, Values: []ScopeRef{{Type: "ALL_RESOURCE", Ref: "resource-1"}}},
			},
		},
		Evidence: &EvidenceSnapshot{
			ID:               evidenceID,
			WorkspaceID:      workspaceID,
			DatasetVersionID: versionID,
			Complete:         true,
		},
		Gold: &GoldProductionEvidence{
			BindingID:                 bindingID,
			WorkspaceID:               workspaceID,
			DatasetVersionID:          versionID,
			AnnotationSnapshotID:      snapshotID,
			AnnotationSnapshotRoot:    "snapshot-root",
			SchemaContentSHA256:       "schema-hash",
			TaxonomyContentSHA256:     "taxonomy-hash",
			ProductionBindingRootHash: "binding-root",
			Finalized:                 true,
		},
		IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("evaluate Gold certification: %v", err)
	}
	if certification.Decision != DecisionRejected {
		t.Fatalf("decision=%s want=%s", certification.Decision, DecisionRejected)
	}
	var qualityBlocker bool
	for _, blocker := range certification.Blockers {
		if blocker.Code == "QUALITY_GATE_NOT_PASSED" {
			qualityBlocker = true
		}
	}
	if !qualityBlocker {
		t.Fatalf("missing quality blocker: %+v", certification.Blockers)
	}
	if certification.GoldProductionBindingID == nil || *certification.GoldProductionBindingID != bindingID ||
		certification.AnnotationSnapshotID == nil || *certification.AnnotationSnapshotID != snapshotID {
		t.Fatalf("rejected certification did not freeze Gold proof: %+v", certification)
	}
}
