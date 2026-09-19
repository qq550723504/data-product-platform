package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type GateDecision string
type FindingStatus string

const (
	GatePass            GateDecision = "PASS"
	GatePassWithWarning GateDecision = "PASS_WITH_WARNING"
	GateReview          GateDecision = "REVIEW"
	GateFail            GateDecision = "FAIL"

	FindingPass    FindingStatus = "PASS"
	FindingFail    FindingStatus = "FAIL"
	FindingSkipped FindingStatus = "SKIPPED"
)

type Finding struct {
	ID        uuid.UUID
	ResultID  uuid.UUID
	RuleID    string
	Dimension string
	Severity  string
	Status    FindingStatus
	Observed  map[string]any
	Message   string
	CreatedAt time.Time
}

// Assessment is the immutable historical fact produced by a quality
// evaluation. Result remains an alias for the existing quality-result API.
type Assessment struct {
	ID                   uuid.UUID
	WorkspaceID          uuid.UUID
	DatasetVersionID     uuid.UUID
	RuleSetRef           string
	RuleSetVersion       string
	RuleSetContentSHA256 string
	RuleSetContent       string
	EvaluatorName        string
	EvaluatorVersion     string
	GateDecision         GateDecision
	Metrics              map[string]any
	Findings             []Finding
	CreatedAt            time.Time
	CreatedBy            *uuid.UUID
}

type Result = Assessment

const (
	NativeEvaluatorName    = "native-quality"
	NativeEvaluatorVersion = "1"
)

func NewResult(workspaceID, datasetVersionID uuid.UUID, ruleSetRef, version string, metrics map[string]any, findings []Finding, actorID *uuid.UUID) Result {
	return NewAssessment(workspaceID, datasetVersionID, ruleSetRef, version, "", "", NativeEvaluatorName, NativeEvaluatorVersion, metrics, findings, actorID)
}

func NewAssessment(workspaceID, datasetVersionID uuid.UUID, ruleSetRef, version, ruleSetContentSHA256, ruleSetContent, evaluatorName, evaluatorVersion string, metrics map[string]any, findings []Finding, actorID *uuid.UUID) Assessment {
	if metrics == nil {
		metrics = map[string]any{}
	}
	result := Assessment{
		ID:                   uuid.New(),
		WorkspaceID:          workspaceID,
		DatasetVersionID:     datasetVersionID,
		RuleSetRef:           strings.TrimSpace(ruleSetRef),
		RuleSetVersion:       strings.TrimSpace(version),
		RuleSetContentSHA256: strings.TrimSpace(ruleSetContentSHA256),
		RuleSetContent:       ruleSetContent,
		EvaluatorName:        strings.TrimSpace(evaluatorName),
		EvaluatorVersion:     strings.TrimSpace(evaluatorVersion),
		Metrics:              metrics,
		CreatedAt:            time.Now().UTC(),
		CreatedBy:            actorID,
	}
	for i := range findings {
		findings[i].ID = uuid.New()
		findings[i].ResultID = result.ID
		if findings[i].CreatedAt.IsZero() {
			findings[i].CreatedAt = result.CreatedAt
		}
	}
	result.Findings = findings
	result.GateDecision = DecideGate(findings)
	return result
}

func DecideGate(findings []Finding) GateDecision {
	hasWarning := false
	hasHigh := false
	for _, finding := range findings {
		if finding.Status != FindingFail {
			continue
		}
		switch strings.ToUpper(finding.Severity) {
		case "CRITICAL":
			return GateFail
		case "HIGH":
			hasHigh = true
		default:
			hasWarning = true
		}
	}
	if hasHigh {
		return GateReview
	}
	if hasWarning {
		return GatePassWithWarning
	}
	return GatePass
}
