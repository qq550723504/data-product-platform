package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func testProfile(t *testing.T) ProfileSnapshot {
	t.Helper()
	profile := CertificationProfile{
		ProfileRef: "pack/enterprise-activity", Code: "enterprise-activity", Name: "Enterprise Activity", Version: "1",
		Purpose:                   Applicability{Mode: ApplicabilityExplicit, Values: []string{"COMMERCIAL"}},
		Actions:                   Applicability{Mode: ApplicabilityExplicit, Values: []string{"SHARE"}},
		Consumers:                 Applicability{Mode: ApplicabilityExplicit, Values: []string{"consumer-a"}},
		Delivery:                  Applicability{Mode: ApplicabilityExplicit, Values: []string{"DIRECT_DATA"}},
		QualityGateRequired:       true,
		RequiredQualityDimensions: []string{"COMPLETENESS"},
		RequiredCriticalRules:     []string{"Q-1"},
		Rights: RightsRequirement{
			Required:  true,
			Purpose:   Applicability{Mode: ApplicabilityExplicit, Values: []string{"COMMERCIAL"}},
			Actions:   Applicability{Mode: ApplicabilityExplicit, Values: []string{"SHARE"}},
			Consumers: Applicability{Mode: ApplicabilityExplicit, Values: []string{"consumer-a"}},
			Scopes:    ScopeApplicability{Mode: ApplicabilityExplicit, Values: []ScopeRef{{Type: "ALL_RESOURCE", Ref: "dataset-scope"}}},
		},
		ComplianceRequired: true, ContractRequired: true, ContractCode: "enterprise-contract", TraceabilityRequired: true, EvidenceRequired: true,
	}
	snapshot, err := profile.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func testInput(workspaceID, datasetVersionID uuid.UUID) EvaluationInput {
	return EvaluationInput{
		WorkspaceID: workspaceID, DatasetVersionID: datasetVersionID, DatasetVersionStatus: "READY", Derived: true,
		IssuedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Quality:  QualityAssessmentEvidence{ID: uuid.New(), WorkspaceID: workspaceID, DatasetVersionID: datasetVersionID, GateDecision: EvidencePass, Dimensions: map[string]string{"COMPLETENESS": EvidencePass}, RuleStatuses: map[string]string{"Q-1": EvidencePass}},
		Rights: &RightsEvidence{
			WorkspaceID: workspaceID, DatasetVersionID: datasetVersionID, RightsSnapshotID: uuid.New(), EffectiveRightsSnapshotID: uuid.New(), EffectiveRightsSnapshotHash: "effective-hash", FrozenRightsContextHash: "context-hash", RightsSnapshotFinalized: true, EffectiveRightsFinalized: true, RequiredInputSetHash: "lineage-hash", TargetLineageInputSetHash: "lineage-hash", ActionDecisions: map[string]string{"SHARE": RightsAllowed}, Coverage: RightsCoverage{
				Purpose: Applicability{Mode: ApplicabilityExplicit, Values: []string{"COMMERCIAL"}}, Actions: Applicability{Mode: ApplicabilityExplicit, Values: []string{"SHARE"}}, Consumers: Applicability{Mode: ApplicabilityExplicit, Values: []string{"consumer-a"}}, Scopes: ScopeApplicability{Mode: ApplicabilityExplicit, Values: []ScopeRef{{Type: "ALL_RESOURCE", Ref: "dataset-scope"}}},
			},
		},
		Compliance:   &ComplianceEvidence{ID: uuid.New(), WorkspaceID: workspaceID, DatasetVersionID: datasetVersionID, Decision: EvidencePass},
		Contract:     &ContractEvidence{ID: uuid.New(), WorkspaceID: workspaceID, DatasetVersionID: datasetVersionID, MatchesProfile: true},
		Traceability: &TraceabilityEvidence{ID: uuid.New(), WorkspaceID: workspaceID, DatasetVersionID: datasetVersionID, Complete: true},
		Evidence:     &EvidenceSnapshot{ID: uuid.New(), WorkspaceID: workspaceID, DatasetVersionID: datasetVersionID, Complete: true},
	}
}

func TestEvaluateRejectsMissingRightsAndComplianceFailure(t *testing.T) {
	workspaceID, datasetVersionID := uuid.New(), uuid.New()
	profile := testProfile(t)
	input := testInput(workspaceID, datasetVersionID)
	input.Rights = nil
	input.Compliance.Decision = EvidenceFail
	certification, err := Evaluate(profile, input)
	if err != nil {
		t.Fatalf("evaluate certification: %v", err)
	}
	if certification.Decision != DecisionRejected {
		t.Fatalf("decision = %s, want REJECTED", certification.Decision)
	}
	assertBlocker(t, certification.Blockers, "RIGHTS_EVIDENCE_MISSING")
	assertBlocker(t, certification.Blockers, "COMPLIANCE_NOT_PASSED")
}

func TestRejectedCertificationDoesNotFreezeUnfinalizedRightsSnapshot(t *testing.T) {
	workspaceID, datasetVersionID := uuid.New(), uuid.New()
	input := testInput(workspaceID, datasetVersionID)
	input.Rights.RightsSnapshotFinalized = false
	input.Rights.ActionDecisions["SHARE"] = "DENIED"

	certification, err := Evaluate(testProfile(t), input)
	if err != nil {
		t.Fatalf("evaluate certification: %v", err)
	}
	if certification.Decision != DecisionRejected {
		t.Fatalf("decision = %s, want REJECTED", certification.Decision)
	}
	if certification.RightsSnapshotID != nil {
		t.Fatal("unfinalized RightsSnapshot was frozen into the certification")
	}
}

func TestEvaluateCertifiesOnlyWhenAllFrozenEvidenceMatches(t *testing.T) {
	workspaceID, datasetVersionID := uuid.New(), uuid.New()
	certification, err := Evaluate(testProfile(t), testInput(workspaceID, datasetVersionID))
	if err != nil {
		t.Fatalf("evaluate certification: %v", err)
	}
	if certification.Decision != DecisionCertified || len(certification.Blockers) != 0 {
		t.Fatalf("certification = %#v, want CERTIFIED without blockers", certification)
	}
	if certification.EffectiveRightsSnapshotID == nil || certification.RightsSnapshotID == nil || certification.EvidenceSnapshotID == nil {
		t.Fatal("certification did not freeze required evidence identities")
	}
}

func TestEvaluateRejectsUnusableDatasetVersion(t *testing.T) {
	workspaceID, datasetVersionID := uuid.New(), uuid.New()
	input := testInput(workspaceID, datasetVersionID)
	input.DatasetVersionStatus = "INVALID"

	certification, err := Evaluate(testProfile(t), input)
	if err != nil {
		t.Fatalf("evaluate certification: %v", err)
	}
	if certification.Decision != DecisionRejected {
		t.Fatalf("decision = %s, want REJECTED", certification.Decision)
	}
	assertBlocker(t, certification.Blockers, "DATASET_VERSION_NOT_READY")
}

func TestEvaluateChecksRequiredQualityItemsEvenWhenQualityGateIsDisabled(t *testing.T) {
	workspaceID, datasetVersionID := uuid.New(), uuid.New()
	profile := testProfile(t)
	profileDefinition := profile.CertificationProfile
	profileDefinition.QualityGateRequired = false
	var err error
	profile, err = profileDefinition.Snapshot()
	if err != nil {
		t.Fatalf("snapshot profile: %v", err)
	}
	input := testInput(workspaceID, datasetVersionID)
	input.Quality.Dimensions = nil
	input.Quality.RuleStatuses = nil

	certification, err := Evaluate(profile, input)
	if err != nil {
		t.Fatalf("evaluate certification: %v", err)
	}
	if certification.Decision != DecisionRejected {
		t.Fatalf("decision = %s, want REJECTED", certification.Decision)
	}
	assertBlocker(t, certification.Blockers, "QUALITY_DIMENSION_NOT_PASSED")
	assertBlocker(t, certification.Blockers, "QUALITY_CRITICAL_RULE_NOT_PASSED")
}

func TestEvaluateRejectsCrossVersionQualityAndEffectiveRightsMismatch(t *testing.T) {
	workspaceID, datasetVersionID := uuid.New(), uuid.New()
	input := testInput(workspaceID, datasetVersionID)
	input.Quality.DatasetVersionID = uuid.New()
	certification, err := Evaluate(testProfile(t), input)
	if err == nil {
		t.Fatal("cross-version QualityAssessment was accepted as a rejected certification")
	}
	input = testInput(workspaceID, datasetVersionID)
	input.Rights.RequiredInputSetHash = "missing-input"
	certification, err = Evaluate(testProfile(t), input)
	if err != nil {
		t.Fatalf("evaluate mismatched effective rights: %v", err)
	}
	if certification.Decision != DecisionRejected {
		t.Fatalf("decision = %s, want REJECTED", certification.Decision)
	}
	assertBlocker(t, certification.Blockers, "EFFECTIVE_RIGHTS_INPUT_SET_MISMATCH")
}

func TestCurrentCertificationRequiresDispositionRulesAndExplicitProfileContext(t *testing.T) {
	workspaceID, datasetVersionID := uuid.New(), uuid.New()
	certification, err := Evaluate(testProfile(t), testInput(workspaceID, datasetVersionID))
	if err != nil {
		t.Fatal(err)
	}
	if !certification.CurrentAt(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), nil) {
		t.Fatal("certification should be current without disposition")
	}
	gate := certification.CheckCurrent(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), nil, DeliveryContext{Purpose: "COMMERCIAL", Action: "RAW_EXPORT", Consumer: "consumer-a", Delivery: "DIRECT_DATA"})
	if gate.Allowed {
		t.Fatal("profile incorrectly covered an action outside its frozen applicability")
	}
	assertBlocker(t, gate.Blockers, "CERTIFICATION_ACTION_NOT_COVERED")
	replacement := uuid.New()
	disposition, err := NewDisposition(workspaceID, certification.ID, DispositionSuperseded, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), "replacement evaluation", &replacement, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if certification.CurrentAt(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), []CertificationDisposition{disposition}) {
		t.Fatal("superseded certification remained current")
	}
	if _, err := NewDisposition(workspaceID, certification.ID, DispositionRevoked, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), "revoked", &replacement, nil, nil); err == nil {
		t.Fatal("REVOKED disposition accepted a replacement certification")
	}
}

func TestCurrentCertificationRejectsMissingContextEvenForAnyProfile(t *testing.T) {
	certification := DatasetCertification{
		ID:       uuid.New(),
		Decision: DecisionCertified,
		IssuedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Profile: ProfileSnapshot{CertificationProfile: CertificationProfile{
			Purpose:   Applicability{Mode: ApplicabilityAny},
			Actions:   Applicability{Mode: ApplicabilityAny},
			Consumers: Applicability{Mode: ApplicabilityAny},
			Delivery:  Applicability{Mode: ApplicabilityAny},
		}},
	}
	gate := certification.CheckCurrent(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), nil, DeliveryContext{})
	if gate.Allowed {
		t.Fatal("missing delivery context was accepted by an ANY profile")
	}
	if len(gate.Blockers) != 4 {
		t.Fatalf("blockers = %#v, want one blocker for each missing context dimension", gate.Blockers)
	}
}

func TestSelectCurrentCertificationDoesNotGuessLatest(t *testing.T) {
	workspaceID, datasetVersionID := uuid.New(), uuid.New()
	profile := testProfile(t)
	first, err := Evaluate(profile, testInput(workspaceID, datasetVersionID))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Evaluate(profile, testInput(workspaceID, datasetVersionID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SelectCurrent([]DatasetCertification{first, second}, nil, workspaceID, datasetVersionID, profile.ProfileRef, time.Now().UTC()); !errors.Is(err, ErrAmbiguousCurrentCertification) {
		t.Fatalf("select current error = %v, want ambiguity", err)
	}
}

func assertBlocker(t *testing.T, blockers []Blocker, code string) {
	t.Helper()
	for _, blocker := range blockers {
		if blocker.Code == code {
			return
		}
	}
	t.Fatalf("missing blocker %s in %#v", code, blockers)
}
