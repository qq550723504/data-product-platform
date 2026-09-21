package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Decision string
type Disposition string

const (
	DecisionCertified Decision = "CERTIFIED"
	DecisionRejected  Decision = "REJECTED"

	DispositionRevoked    Disposition = "REVOKED"
	DispositionSuperseded Disposition = "SUPERSEDED"

	EvidencePass    = "PASS"
	EvidenceFail    = "FAIL"
	EvidenceUnknown = "UNKNOWN"
	RightsAllowed   = "ALLOWED"
)

type Blocker struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

type QualityAssessmentEvidence struct {
	ID               uuid.UUID
	WorkspaceID      uuid.UUID
	DatasetVersionID uuid.UUID
	GateDecision     string
	Dimensions       map[string]string
	RuleStatuses     map[string]string
}

type RightsCoverage struct {
	Purpose   Applicability
	Actions   Applicability
	Consumers Applicability
	Scopes    ScopeApplicability
}

type RightsEvidence struct {
	WorkspaceID                 uuid.UUID
	DatasetVersionID            uuid.UUID
	RightsSnapshotID            uuid.UUID
	EffectiveRightsSnapshotID   uuid.UUID
	EffectiveRightsSnapshotHash string
	FrozenRightsContextHash     string
	RightsSnapshotFinalized     bool
	EffectiveRightsFinalized    bool
	RequiredInputSetHash        string
	TargetLineageInputSetHash   string
	ActionDecisions             map[string]string
	Coverage                    RightsCoverage
}

type ComplianceEvidence struct {
	ID               uuid.UUID
	WorkspaceID      uuid.UUID
	DatasetVersionID uuid.UUID
	Decision         string
}

type ContractEvidence struct {
	ID               uuid.UUID
	WorkspaceID      uuid.UUID
	DatasetVersionID uuid.UUID
	MatchesProfile   bool
}

type TraceabilityEvidence struct {
	ID               uuid.UUID
	WorkspaceID      uuid.UUID
	DatasetVersionID uuid.UUID
	Complete         bool
}

type EvidenceSnapshot struct {
	ID               uuid.UUID
	WorkspaceID      uuid.UUID
	DatasetVersionID uuid.UUID
	Complete         bool
}

type EvaluationInput struct {
	WorkspaceID      uuid.UUID
	DatasetVersionID uuid.UUID
	Derived          bool
	Quality          QualityAssessmentEvidence
	Rights           *RightsEvidence
	Compliance       *ComplianceEvidence
	Contract         *ContractEvidence
	Traceability     *TraceabilityEvidence
	Evidence         *EvidenceSnapshot
	IssuedAt         time.Time
	ActorID          *uuid.UUID
}

type DatasetCertification struct {
	ID                          uuid.UUID
	WorkspaceID                 uuid.UUID
	DatasetVersionID            uuid.UUID
	QualityAssessmentID         uuid.UUID
	Profile                     ProfileSnapshot
	RightsSnapshotID            *uuid.UUID
	EffectiveRightsSnapshotID   *uuid.UUID
	EffectiveRightsSnapshotHash string
	FrozenRightsContextHash     string
	ComplianceResultID          *uuid.UUID
	ContractVersionID           *uuid.UUID
	TraceabilityEvidenceID      *uuid.UUID
	EvidenceSnapshotID          *uuid.UUID
	Decision                    Decision
	Blockers                    []Blocker
	Reason                      string
	IssuedAt                    time.Time
	ActorID                     *uuid.UUID
}

type CertificationDisposition struct {
	ID                          uuid.UUID
	WorkspaceID                 uuid.UUID
	CertificationID             uuid.UUID
	Disposition                 Disposition
	EffectiveAt                 time.Time
	Reason                      string
	SupersededByCertificationID *uuid.UUID
	EvidenceSnapshotID          *uuid.UUID
	ActorID                     *uuid.UUID
}

type DeliveryContext struct {
	Purpose  string
	Action   string
	Consumer string
	Delivery string
}

type GateResult struct {
	Allowed  bool
	Blockers []Blocker
}

var (
	ErrInvalidEvaluationContext      = errors.New("invalid certification evaluation context")
	ErrInvalidDisposition            = errors.New("invalid certification disposition")
	ErrAmbiguousCurrentCertification = errors.New("multiple current certifications match the requested context")
)

func Evaluate(profile ProfileSnapshot, input EvaluationInput) (DatasetCertification, error) {
	if err := profile.Validate(); err != nil {
		return DatasetCertification{}, err
	}
	if input.WorkspaceID == uuid.Nil || input.DatasetVersionID == uuid.Nil {
		return DatasetCertification{}, fmt.Errorf("%w: workspace and DatasetVersion are required", ErrInvalidEvaluationContext)
	}
	if input.Quality.ID == uuid.Nil || input.Quality.WorkspaceID != input.WorkspaceID || input.Quality.DatasetVersionID != input.DatasetVersionID {
		return DatasetCertification{}, fmt.Errorf("%w: QualityAssessment must belong to the target workspace and DatasetVersion", ErrInvalidEvaluationContext)
	}

	certification := DatasetCertification{
		ID:                  uuid.New(),
		WorkspaceID:         input.WorkspaceID,
		DatasetVersionID:    input.DatasetVersionID,
		QualityAssessmentID: input.Quality.ID,
		Profile:             profile,
		Decision:            DecisionRejected,
		IssuedAt:            input.IssuedAt,
		ActorID:             input.ActorID,
	}
	if certification.IssuedAt.IsZero() {
		certification.IssuedAt = time.Now().UTC()
	} else {
		certification.IssuedAt = certification.IssuedAt.UTC()
	}

	blockers := make([]Blocker, 0)
	add := func(code, detail string) { blockers = append(blockers, Blocker{Code: code, Detail: detail}) }
	if profile.QualityGateRequired && input.Quality.GateDecision != EvidencePass {
		add("QUALITY_GATE_NOT_PASSED", "required QualityAssessment gate decision is not PASS")
	}
	for _, dimension := range profile.RequiredQualityDimensions {
		status, ok := input.Quality.Dimensions[dimension]
		if !ok || status != EvidencePass {
			add("QUALITY_DIMENSION_NOT_PASSED", fmt.Sprintf("required quality dimension %s is missing or not PASS", dimension))
		}
	}
	for _, ruleID := range profile.RequiredCriticalRules {
		status, ok := input.Quality.RuleStatuses[ruleID]
		if !ok || status != EvidencePass {
			add("QUALITY_CRITICAL_RULE_NOT_PASSED", fmt.Sprintf("required critical rule %s is missing or not PASS", ruleID))
		}
	}

	if profile.Rights.Required {
		if input.Rights == nil {
			add("RIGHTS_EVIDENCE_MISSING", "profile requires rights evidence")
		} else {
			evaluateRights(profile.Rights, input, add, &certification)
		}
	}
	if profile.ComplianceRequired {
		if input.Compliance == nil {
			add("COMPLIANCE_EVIDENCE_MISSING", "profile requires compliance evidence")
		} else if input.Compliance.ID == uuid.Nil || input.Compliance.WorkspaceID != input.WorkspaceID || input.Compliance.DatasetVersionID != input.DatasetVersionID || input.Compliance.Decision != EvidencePass {
			add("COMPLIANCE_NOT_PASSED", "compliance evidence is missing, cross-workspace, cross-version, or not PASS")
		} else {
			id := input.Compliance.ID
			certification.ComplianceResultID = &id
		}
	}
	if profile.ContractRequired {
		if input.Contract == nil {
			add("CONTRACT_EVIDENCE_MISSING", "profile requires contract evidence")
		} else if input.Contract.ID == uuid.Nil || input.Contract.WorkspaceID != input.WorkspaceID || input.Contract.DatasetVersionID != input.DatasetVersionID || !input.Contract.MatchesProfile {
			add("CONTRACT_NOT_MATCHED", "contract evidence is missing, cross-workspace, cross-version, or does not match the profile")
		} else {
			id := input.Contract.ID
			certification.ContractVersionID = &id
		}
	}
	if profile.TraceabilityRequired {
		if input.Traceability == nil {
			add("TRACEABILITY_EVIDENCE_MISSING", "profile requires traceability evidence")
		} else if input.Traceability.ID == uuid.Nil || input.Traceability.WorkspaceID != input.WorkspaceID || input.Traceability.DatasetVersionID != input.DatasetVersionID || !input.Traceability.Complete {
			add("TRACEABILITY_NOT_COMPLETE", "traceability evidence is missing, cross-workspace, cross-version, or incomplete")
		} else {
			id := input.Traceability.ID
			certification.TraceabilityEvidenceID = &id
		}
	}
	if profile.EvidenceRequired {
		if input.Evidence == nil {
			add("EVIDENCE_SNAPSHOT_MISSING", "profile requires a complete evidence snapshot")
		} else if input.Evidence.ID == uuid.Nil || input.Evidence.WorkspaceID != input.WorkspaceID || input.Evidence.DatasetVersionID != input.DatasetVersionID || !input.Evidence.Complete {
			add("EVIDENCE_SNAPSHOT_INCOMPLETE", "evidence snapshot is missing, cross-workspace, cross-version, or incomplete")
		} else {
			id := input.Evidence.ID
			certification.EvidenceSnapshotID = &id
		}
	}

	certification.Blockers = blockers
	if len(blockers) == 0 {
		certification.Decision = DecisionCertified
		certification.Reason = "all profile requirements satisfied"
	} else {
		codes := make([]string, 0, len(blockers))
		for _, blocker := range blockers {
			codes = append(codes, blocker.Code)
		}
		certification.Reason = "certification rejected: " + strings.Join(codes, ",")
	}
	return certification, nil
}

func evaluateRights(requirement RightsRequirement, input EvaluationInput, add func(string, string), certification *DatasetCertification) {
	rights := input.Rights
	if rights.WorkspaceID != input.WorkspaceID || rights.DatasetVersionID != input.DatasetVersionID {
		add("RIGHTS_TARGET_MISMATCH", "rights evidence does not belong to the target workspace and DatasetVersion")
		return
	}
	if rights.RightsSnapshotID == uuid.Nil || !rights.RightsSnapshotFinalized {
		add("RIGHTS_SNAPSHOT_NOT_FINALIZED", "rights evidence must reference a finalized immutable RightsSnapshot")
	}
	if rights.EffectiveRightsSnapshotID == uuid.Nil || !rights.EffectiveRightsFinalized || strings.TrimSpace(rights.EffectiveRightsSnapshotHash) == "" {
		add("EFFECTIVE_RIGHTS_NOT_FINALIZED", "rights-required certification must reference a finalized immutable EffectiveRightsSnapshot and hash")
	} else {
		id := rights.EffectiveRightsSnapshotID
		certification.EffectiveRightsSnapshotID = &id
		certification.EffectiveRightsSnapshotHash = rights.EffectiveRightsSnapshotHash
	}
	if strings.TrimSpace(rights.FrozenRightsContextHash) == "" {
		add("RIGHTS_CONTEXT_HASH_MISSING", "frozen rights context hash is required")
	} else {
		certification.FrozenRightsContextHash = rights.FrozenRightsContextHash
	}
	if input.Derived {
		if strings.TrimSpace(rights.RequiredInputSetHash) == "" || strings.TrimSpace(rights.TargetLineageInputSetHash) == "" || rights.RequiredInputSetHash != rights.TargetLineageInputSetHash {
			add("EFFECTIVE_RIGHTS_INPUT_SET_MISMATCH", "effective rights required-input membership does not match target DatasetVersion lineage")
		}
	}
	for _, action := range requirement.Actions.Values {
		if strings.ToUpper(strings.TrimSpace(rights.ActionDecisions[action])) != RightsAllowed {
			add("RIGHTS_ACTION_NOT_ALLOWED", fmt.Sprintf("required rights action %s is missing or not ALLOWED", action))
		}
	}
	if !rights.Coverage.Purpose.CoversApplicability(requirement.Purpose, true) {
		add("RIGHTS_PURPOSE_COVERAGE_MISSING", "frozen rights purpose coverage does not cover the profile")
	}
	if !rights.Coverage.Actions.CoversApplicability(requirement.Actions, true) {
		add("RIGHTS_ACTION_COVERAGE_MISSING", "frozen rights action coverage does not cover the profile")
	}
	if !rights.Coverage.Consumers.CoversApplicability(requirement.Consumers, false) {
		add("RIGHTS_CONSUMER_COVERAGE_MISSING", "frozen rights consumer coverage does not cover the profile")
	}
	if !rights.Coverage.Scopes.Covers(requirement.Scopes) {
		add("RIGHTS_SCOPE_COVERAGE_MISSING", "frozen rights normalized scope coverage does not cover the profile")
	}
	if rights.RightsSnapshotID != uuid.Nil && rights.RightsSnapshotFinalized {
		id := rights.RightsSnapshotID
		certification.RightsSnapshotID = &id
	}
}

func (c DatasetCertification) CurrentAt(asOf time.Time, dispositions []CertificationDisposition) bool {
	if c.Decision != DecisionCertified {
		return false
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	if c.IssuedAt.IsZero() || c.IssuedAt.After(asOf) {
		return false
	}
	for _, disposition := range dispositions {
		if disposition.CertificationID == c.ID && !disposition.EffectiveAt.After(asOf) && (disposition.Disposition == DispositionRevoked || disposition.Disposition == DispositionSuperseded) {
			return false
		}
	}
	return true
}

func (c DatasetCertification) Covers(context DeliveryContext) bool {
	return c.Profile.Purpose.Covers(context.Purpose, true) &&
		c.Profile.Actions.Covers(context.Action, true) &&
		c.Profile.Consumers.Covers(context.Consumer, false) &&
		c.Profile.Delivery.Covers(context.Delivery, true)
}

// CheckCurrent is the certification-only part of CurrentDeliveryGate. It does
// not re-evaluate current rights, dataset usability, or caller authority; the
// delivery command must run those gates separately under its shared fence.
func (c DatasetCertification) CheckCurrent(asOf time.Time, dispositions []CertificationDisposition, context DeliveryContext) GateResult {
	result := GateResult{Allowed: false, Blockers: make([]Blocker, 0)}
	add := func(code, detail string) {
		result.Blockers = append(result.Blockers, Blocker{Code: code, Detail: detail})
	}
	if !c.CurrentAt(asOf, dispositions) {
		add("CERTIFICATION_NOT_CURRENT", "certification is not CERTIFIED, is not issued as of the requested time, or has an effective disposition")
	}
	if !c.Profile.Purpose.Covers(context.Purpose, true) {
		add("CERTIFICATION_PURPOSE_NOT_COVERED", "requested purpose is not covered by the frozen CertificationProfile")
	}
	if !c.Profile.Actions.Covers(context.Action, true) {
		add("CERTIFICATION_ACTION_NOT_COVERED", "requested action is not covered by the frozen CertificationProfile")
	}
	if !c.Profile.Consumers.Covers(context.Consumer, false) {
		add("CERTIFICATION_CONSUMER_NOT_COVERED", "requested consumer is not covered by the frozen CertificationProfile")
	}
	if !c.Profile.Delivery.Covers(context.Delivery, true) {
		add("CERTIFICATION_DELIVERY_NOT_COVERED", "requested delivery channel or mode is not covered by the frozen CertificationProfile")
	}
	result.Allowed = len(result.Blockers) == 0
	return result
}

func NewDisposition(workspaceID, certificationID uuid.UUID, disposition Disposition, effectiveAt time.Time, reason string, supersededBy, evidenceID, actorID *uuid.UUID) (CertificationDisposition, error) {
	if workspaceID == uuid.Nil || certificationID == uuid.Nil || strings.TrimSpace(reason) == "" || effectiveAt.IsZero() {
		return CertificationDisposition{}, fmt.Errorf("%w: workspace, certification, reason, and effectiveAt are required", ErrInvalidDisposition)
	}
	if disposition != DispositionRevoked && disposition != DispositionSuperseded {
		return CertificationDisposition{}, fmt.Errorf("%w: unsupported disposition %q", ErrInvalidDisposition, disposition)
	}
	if disposition == DispositionSuperseded && (supersededBy == nil || *supersededBy == uuid.Nil || *supersededBy == certificationID) {
		return CertificationDisposition{}, fmt.Errorf("%w: SUPERSEDED requires a different replacement certification", ErrInvalidDisposition)
	}
	if disposition == DispositionRevoked && supersededBy != nil {
		return CertificationDisposition{}, fmt.Errorf("%w: REVOKED cannot carry a replacement certification", ErrInvalidDisposition)
	}
	return CertificationDisposition{
		ID:                          uuid.New(),
		WorkspaceID:                 workspaceID,
		CertificationID:             certificationID,
		Disposition:                 disposition,
		EffectiveAt:                 effectiveAt.UTC(),
		Reason:                      strings.TrimSpace(reason),
		SupersededByCertificationID: supersededBy,
		EvidenceSnapshotID:          evidenceID,
		ActorID:                     actorID,
	}, nil
}

func SelectCurrent(certifications []DatasetCertification, dispositions []CertificationDisposition, workspaceID, datasetVersionID uuid.UUID, profileRef string, asOf time.Time) (DatasetCertification, error) {
	var selected DatasetCertification
	count := 0
	for _, certification := range certifications {
		if certification.WorkspaceID != workspaceID || certification.DatasetVersionID != datasetVersionID || certification.Profile.ProfileRef != profileRef || !certification.CurrentAt(asOf, dispositions) {
			continue
		}
		selected = certification
		count++
	}
	if count > 1 {
		return DatasetCertification{}, ErrAmbiguousCurrentCertification
	}
	if count == 0 {
		return DatasetCertification{}, nil
	}
	return selected, nil
}
