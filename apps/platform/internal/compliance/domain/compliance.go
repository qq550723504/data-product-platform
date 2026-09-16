package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type GateDecision string
type FindingStatus string

const (
	GatePass   GateDecision = "PASS"
	GateReview GateDecision = "REVIEW"
	GateFail   GateDecision = "FAIL"

	FindingPass   FindingStatus = "PASS"
	FindingReview FindingStatus = "REVIEW"
	FindingFail   FindingStatus = "FAIL"
)

type Finding struct {
	ID        uuid.UUID
	ResultID  uuid.UUID
	FieldName string
	Category  string
	Action    string
	Status    FindingStatus
	Message   string
	CreatedAt time.Time
}

type Result struct {
	ID               uuid.UUID
	WorkspaceID      uuid.UUID
	DatasetVersionID uuid.UUID
	PolicyRef        string
	PolicyVersion    string
	GateDecision     GateDecision
	Summary          map[string]any
	Findings         []Finding
	CreatedAt        time.Time
	CreatedBy        *uuid.UUID
}

func NewResult(workspaceID, datasetVersionID uuid.UUID, policyRef, policyVersion string, summary map[string]any, findings []Finding, actorID *uuid.UUID) Result {
	if summary == nil {
		summary = map[string]any{}
	}
	result := Result{
		ID:               uuid.New(),
		WorkspaceID:      workspaceID,
		DatasetVersionID: datasetVersionID,
		PolicyRef:        strings.TrimSpace(policyRef),
		PolicyVersion:    strings.TrimSpace(policyVersion),
		Summary:          summary,
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
	hasReview := false
	for _, finding := range findings {
		if finding.Status == FindingFail {
			return GateFail
		}
		if finding.Status == FindingReview {
			hasReview = true
		}
	}
	if hasReview {
		return GateReview
	}
	return GatePass
}
