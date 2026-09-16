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

type Result struct {
	ID               uuid.UUID
	WorkspaceID      uuid.UUID
	DatasetVersionID uuid.UUID
	RuleSetRef       string
	RuleSetVersion   string
	GateDecision     GateDecision
	Metrics          map[string]any
	Findings         []Finding
	CreatedAt        time.Time
	CreatedBy        *uuid.UUID
}

func NewResult(workspaceID, datasetVersionID uuid.UUID, ruleSetRef, version string, metrics map[string]any, findings []Finding, actorID *uuid.UUID) Result {
	if metrics == nil {
		metrics = map[string]any{}
	}
	result := Result{
		ID:               uuid.New(),
		WorkspaceID:      workspaceID,
		DatasetVersionID: datasetVersionID,
		RuleSetRef:       strings.TrimSpace(ruleSetRef),
		RuleSetVersion:   strings.TrimSpace(version),
		Metrics:          metrics,
		CreatedAt:        time.Now().UTC(),
		CreatedBy:        actorID,
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
