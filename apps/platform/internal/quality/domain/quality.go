package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type GateDecision string
type FindingStatus string
type DimensionStatus string

const (
	GatePass            GateDecision = "PASS"
	GatePassWithWarning GateDecision = "PASS_WITH_WARNING"
	GateReview          GateDecision = "REVIEW"
	GateFail            GateDecision = "FAIL"

	FindingPass    FindingStatus = "PASS"
	FindingFail    FindingStatus = "FAIL"
	FindingSkipped FindingStatus = "SKIPPED"

	DimensionPass          DimensionStatus = "PASS"
	DimensionWarn          DimensionStatus = "WARN"
	DimensionReview        DimensionStatus = "REVIEW"
	DimensionFail          DimensionStatus = "FAIL"
	DimensionNotApplicable DimensionStatus = "NOT_APPLICABLE"
)

var QualityDimensions = []string{
	"COMPLETENESS",
	"ACCURACY",
	"CONSISTENCY",
	"UNIQUENESS",
	"TIMELINESS",
	"TRACEABILITY",
}

type DimensionSummary struct {
	Dimension      string          `json:"dimension"`
	Status         DimensionStatus `json:"status"`
	RuleCount      int             `json:"ruleCount"`
	EvaluatedCount int             `json:"evaluatedCount"`
	FailedCount    int             `json:"failedCount"`
}

// FindingPage is the bounded, deterministic view of an assessment's
// immutable findings used by the Quality Report API. The assessment itself
// remains the historical fact; paging only changes how it is read.
type FindingPage struct {
	Items  []Finding
	Limit  int
	Offset int
	Total  int
}

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
	DimensionSummaries   map[string]DimensionSummary
	CreatedAt            time.Time
	CreatedBy            *uuid.UUID
}

type Result = Assessment

const (
	NativeEvaluatorName    = "native-quality"
	NativeEvaluatorVersion = "2"
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
	result.DimensionSummaries = SummarizeDimensions(findings)
	result.Metrics["dimensions"] = result.DimensionSummaries
	result.GateDecision = DecideGate(findings)
	return result
}

func SummarizeDimensions(findings []Finding) map[string]DimensionSummary {
	result := make(map[string]DimensionSummary, len(QualityDimensions))
	for _, dimension := range QualityDimensions {
		result[dimension] = DimensionSummary{Dimension: dimension, Status: DimensionNotApplicable}
	}
	for _, finding := range findings {
		dimension := strings.ToUpper(strings.TrimSpace(finding.Dimension))
		if dimension == "CONFORMITY" {
			dimension = "ACCURACY"
		}
		summary, ok := result[dimension]
		if !ok {
			continue
		}
		summary.RuleCount++
		if finding.Status == FindingSkipped {
			result[dimension] = summary
			continue
		}
		summary.EvaluatedCount++
		if finding.Status == FindingFail {
			summary.FailedCount++
			if strings.EqualFold(finding.Severity, "CRITICAL") {
				summary.Status = DimensionFail
			} else if strings.EqualFold(finding.Severity, "HIGH") && summary.Status != DimensionFail {
				summary.Status = DimensionReview
			} else if summary.Status == DimensionNotApplicable || summary.Status == DimensionPass {
				summary.Status = DimensionWarn
			}
		} else if summary.Status == DimensionNotApplicable {
			summary.Status = DimensionPass
		}
		result[dimension] = summary
	}
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
