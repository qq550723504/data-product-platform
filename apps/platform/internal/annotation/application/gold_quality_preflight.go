package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	annotationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/infrastructure"
	qualitydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
)

type GoldQualityPreflight struct {
	WorkspaceID  uuid.UUID               `json:"workspaceId"`
	CampaignID   uuid.UUID               `json:"campaignId"`
	SnapshotID   uuid.UUID               `json:"snapshotId"`
	SnapshotRoot string                  `json:"snapshotRoot"`
	Blocking     bool                    `json:"blocking"`
	Metrics      map[string]any          `json:"metrics"`
	Findings     []qualitydomain.Finding `json:"findings"`
}

type goldSnapshotManifest struct {
	Tasks []struct {
		ID            string `json:"id"`
		SourceItemRef string `json:"sourceItemRef"`
	} `json:"tasks"`
	Decisions []struct {
		ID               string  `json:"id"`
		TaskID           string  `json:"taskId"`
		Outcome          string  `json:"outcome"`
		ReviewedResultID *string `json:"reviewedResultId,omitempty"`
		SelectedResultID *string `json:"selectedResultId,omitempty"`
	} `json:"decisions"`
	Outputs []struct {
		TaskID           string `json:"taskId"`
		SelectedResultID string `json:"selectedResultId"`
	} `json:"outputs"`
}

func (s *Service) GoldQualityPreflight(
	ctx context.Context,
	workspaceID, campaignID uuid.UUID,
) (GoldQualityPreflight, error) {
	if workspaceID == uuid.Nil || campaignID == uuid.Nil {
		return GoldQualityPreflight{}, annotationdomain.ErrInvalidSnapshot
	}
	snapshot, err := s.repo.GetSnapshotByCampaign(ctx, workspaceID, campaignID)
	if err != nil {
		return GoldQualityPreflight{}, err
	}
	if snapshot.Status != annotationdomain.SnapshotFinalized || snapshot.FinalizedAt == nil {
		return GoldQualityPreflight{}, annotationdomain.ErrInvalidSnapshot
	}
	integrityValid, err := s.repo.GetSnapshotIntegrity(ctx, snapshot.ID)
	if err != nil {
		return GoldQualityPreflight{}, err
	}
	if !integrityValid {
		return GoldQualityPreflight{}, annotationdomain.ErrInvalidSnapshot
	}
	campaign, err := s.repo.GetCampaign(ctx, campaignID)
	if err != nil {
		return GoldQualityPreflight{}, err
	}
	if campaign.WorkspaceID != workspaceID {
		return GoldQualityPreflight{}, annotationdomain.ErrInvalidSnapshot
	}

	var manifest goldSnapshotManifest
	if err := json.Unmarshal(snapshot.Manifest, &manifest); err != nil {
		return GoldQualityPreflight{}, fmt.Errorf("decode finalized annotation snapshot manifest: %w", err)
	}
	selected, err := s.repo.ListSnapshotSelectedResults(ctx, snapshot.ID)
	if err != nil {
		return GoldQualityPreflight{}, err
	}
	return evaluateGoldQualityPreflight(campaign, snapshot.ID, snapshot.RootHash, manifest, selected)
}

func evaluateGoldQualityPreflight(
	campaign annotationdomain.Campaign,
	snapshotID uuid.UUID,
	rootHash string,
	manifest goldSnapshotManifest,
	selected []annotationinfra.SnapshotSelectedResult,
) (GoldQualityPreflight, error) {
	n := len(manifest.Tasks)
	d := len(manifest.Decisions)
	rejected := 0
	corrected := 0
	accepted := 0

	taskIDs := make(map[string]struct{}, n)
	sourceSeen := make(map[string]struct{}, n)
	duplicateSources := make([]any, 0)
	for _, task := range manifest.Tasks {
		taskIDs[task.ID] = struct{}{}
		ref := strings.TrimSpace(task.SourceItemRef)
		if ref == "" {
			duplicateSources = append(duplicateSources, map[string]any{"taskId": task.ID, "sourceItemRef": ref})
			continue
		}
		if _, exists := sourceSeen[ref]; exists {
			duplicateSources = append(duplicateSources, map[string]any{"taskId": task.ID, "sourceItemRef": ref})
		}
		sourceSeen[ref] = struct{}{}
	}

	decisionByTask := make(map[string]struct {
		Outcome          string
		SelectedResultID string
	}, d)
	for _, decision := range manifest.Decisions {
		selectedID := ""
		if decision.SelectedResultID != nil {
			selectedID = strings.TrimSpace(*decision.SelectedResultID)
		}
		decisionByTask[decision.TaskID] = struct {
			Outcome          string
			SelectedResultID string
		}{Outcome: decision.Outcome, SelectedResultID: selectedID}
		switch decision.Outcome {
		case annotationdomain.ReviewAccept:
			accepted++
		case annotationdomain.ReviewCorrect:
			corrected++
		case annotationdomain.ReviewReject:
			rejected++
		}
	}

	resultByID := make(map[string]annotationinfra.SnapshotSelectedResult, len(selected))
	correctedFromCount := 0
	for _, result := range selected {
		resultByID[result.ResultID.String()] = result
		if result.CorrectedFromResultID != nil {
			correctedFromCount++
		}
	}

	mappingProblems := make([]any, 0)
	schemaProblems := make([]any, 0)
	usableSelectedTasks := make(map[string]struct{}, len(manifest.Outputs))
	outputSeen := make(map[string]struct{}, len(manifest.Outputs))
	for _, output := range manifest.Outputs {
		if _, duplicated := outputSeen[output.TaskID]; duplicated {
			mappingProblems = append(mappingProblems, map[string]any{
				"taskId": output.TaskID, "reason": "task appears more than once in outputs",
			})
			continue
		}
		outputSeen[output.TaskID] = struct{}{}
		if _, ok := taskIDs[output.TaskID]; !ok {
			mappingProblems = append(mappingProblems, map[string]any{"taskId": output.TaskID, "reason": "output task not frozen"})
			continue
		}
		result, ok := resultByID[output.SelectedResultID]
		if !ok {
			mappingProblems = append(mappingProblems, map[string]any{"taskId": output.TaskID, "selectedResultId": output.SelectedResultID, "reason": "selected result unavailable"})
			continue
		}
		if result.TaskID.String() != output.TaskID {
			mappingProblems = append(mappingProblems, map[string]any{"taskId": output.TaskID, "selectedResultId": output.SelectedResultID, "reason": "selected result belongs to different task"})
			continue
		}
		decision, ok := decisionByTask[output.TaskID]
		if !ok || (decision.Outcome != annotationdomain.ReviewAccept && decision.Outcome != annotationdomain.ReviewCorrect) ||
			decision.SelectedResultID != output.SelectedResultID {
			mappingProblems = append(mappingProblems, map[string]any{"taskId": output.TaskID, "selectedResultId": output.SelectedResultID, "reason": "output does not match terminal ACCEPT/CORRECT decision"})
			continue
		}
		if err := validateAnnotationPayload(campaign.Schema, result.CanonicalPayload); err != nil {
			schemaProblems = append(schemaProblems, map[string]any{"taskId": result.TaskID, "resultId": result.ResultID})
			continue
		}
		usableSelectedTasks[output.TaskID] = struct{}{}
	}
	if len(selected) != len(manifest.Outputs) {
		mappingProblems = append(mappingProblems, map[string]any{"selectedResultCount": len(selected), "outputCount": len(manifest.Outputs), "reason": "selected result/output count mismatch"})
	}

	a := len(usableSelectedTasks)
	findings := make([]qualitydomain.Finding, 0, 8)
	metrics := map[string]any{
		"taskCount":              n,
		"selectedCount":          len(manifest.Outputs),
		"usableSelectedCount":    a,
		"reviewedCount":          d,
		"acceptedCount":          accepted,
		"rejectedCount":          rejected,
		"correctedCount":         corrected,
		"correctedSelectedCount": correctedFromCount,
		"schemaInvalidCount":     len(schemaProblems),
		"annotationCoverage":     ratioObservation(a, n),
		"reviewedCoverage":       ratioObservation(d, n),
		"reviewPassRate":         ratioObservation(accepted+corrected, d),
		"agreement":              "NOT_APPLICABLE",
	}

	findings = append(findings,
		ratioFinding("GOLD-ANNOTATION-COVERAGE", "COMPLETENESS", "CRITICAL",
			"Every frozen task must have a schema-valid ACCEPT/CORRECT selected result.", a, n, n > 0 && a == n),
		ratioFinding("GOLD-REVIEWED-COVERAGE", "COMPLETENESS", "CRITICAL",
			"Every frozen task must have a terminal review decision.", d, n, n > 0 && d == n),
		countZeroFinding("GOLD-REJECTED-COUNT", "ACCURACY", "CRITICAL",
			"Gold Pilot requires zero REJECT decisions.", rejected),
		sampleFinding(
			"GOLD-SOURCE-UNIQUENESS", "UNIQUENESS", "CRITICAL",
			"Frozen tasks must have non-empty unique source item references.",
			len(duplicateSources) == 0, len(duplicateSources), duplicateSources,
		),
		sampleFinding(
			"GOLD-OUTPUT-TASK-MAPPING", "CONSISTENCY", "CRITICAL",
			"Each output must map one frozen task to the exact result selected by its ACCEPT/CORRECT decision.",
			len(mappingProblems) == 0, len(mappingProblems), mappingProblems,
		),
		sampleFinding(
			"GOLD-SCHEMA-VALIDITY", "ACCURACY", "CRITICAL",
			"Every selected result must validate against the frozen campaign schema.",
			len(schemaProblems) == 0 && len(selected) == len(manifest.Outputs), len(schemaProblems), schemaProblems,
		),
	)

	provenanceProblems := make([]any, 0)
	if strings.TrimSpace(rootHash) == "" {
		provenanceProblems = append(provenanceProblems, map[string]any{"reason": "snapshot root hash missing"})
	}
	if strings.TrimSpace(campaign.Schema.ContentSHA256) == "" {
		provenanceProblems = append(provenanceProblems, map[string]any{"reason": "frozen schema hash missing"})
	}
	for _, decision := range manifest.Decisions {
		if strings.TrimSpace(decision.ID) == "" || strings.TrimSpace(decision.TaskID) == "" {
			provenanceProblems = append(provenanceProblems, map[string]any{"reason": "decision identity incomplete"})
		}
		if (decision.Outcome == annotationdomain.ReviewAccept || decision.Outcome == annotationdomain.ReviewCorrect) &&
			(decision.SelectedResultID == nil || strings.TrimSpace(*decision.SelectedResultID) == "") {
			provenanceProblems = append(provenanceProblems, map[string]any{"taskId": decision.TaskID, "reason": "selected result provenance missing"})
		}
	}
	findings = append(findings, sampleFinding(
		"GOLD-PROVENANCE-COMPLETE", "TRACEABILITY", "CRITICAL",
		"Snapshot, schema, decisions and selected result provenance must be complete.",
		len(provenanceProblems) == 0, len(provenanceProblems), provenanceProblems,
	))

	findings = append(findings, qualitydomain.Finding{
		RuleID:    "GOLD-AGREEMENT",
		Dimension: "CONSISTENCY",
		Severity:  "INFO",
		Status:    qualitydomain.FindingSkipped,
		Message:   "NOT_APPLICABLE: the Pilot has one primary annotator per task and no multi-annotation consensus fixture.",
		Observed: map[string]any{
			"agreement":                     "NOT_APPLICABLE",
			"multiAnnotationFactsAvailable": false,
		},
	})

	blocking := false
	for _, finding := range findings {
		if finding.Status == qualitydomain.FindingFail && strings.EqualFold(finding.Severity, "CRITICAL") {
			blocking = true
			break
		}
	}
	metrics["blocking"] = blocking

	return GoldQualityPreflight{
		WorkspaceID:  campaign.WorkspaceID,
		CampaignID:   campaign.ID,
		SnapshotID:   snapshotID,
		SnapshotRoot: rootHash,
		Blocking:     blocking,
		Metrics:      metrics,
		Findings:     findings,
	}, nil
}

func ratioFinding(id, dimension, severity, expectation string, numerator, denominator int, pass bool) qualitydomain.Finding {
	observed := ratioObservation(numerator, denominator)
	observed["expectation"] = expectation
	observed["threshold"] = "1/1"
	finding := qualitydomain.Finding{
		RuleID: id, Dimension: dimension, Severity: severity,
		Status: qualitydomain.FindingPass, Observed: observed,
	}
	if !pass {
		finding.Status = qualitydomain.FindingFail
		finding.Message = fmt.Sprintf("%s observed %d/%d", expectation, numerator, denominator)
	}
	return finding
}

func countZeroFinding(id, dimension, severity, expectation string, count int) qualitydomain.Finding {
	finding := qualitydomain.Finding{
		RuleID: id, Dimension: dimension, Severity: severity,
		Status: qualitydomain.FindingPass,
		Observed: map[string]any{
			"count": count, "threshold": 0, "expectation": expectation, "affectedCount": count,
		},
	}
	if count != 0 {
		finding.Status = qualitydomain.FindingFail
		finding.Message = fmt.Sprintf("%s observed %d", expectation, count)
	}
	return finding
}

func sampleFinding(id, dimension, severity, expectation string, pass bool, affected int, samples []any) qualitydomain.Finding {
	observed := map[string]any{
		"affectedCount": affected, "threshold": 0, "expectation": expectation,
	}
	if len(samples) > 0 {
		if len(samples) > 5 {
			samples = samples[:5]
		}
		observed["sample"] = samples
	}
	finding := qualitydomain.Finding{
		RuleID: id, Dimension: dimension, Severity: severity,
		Status: qualitydomain.FindingPass, Observed: observed,
	}
	if !pass {
		finding.Status = qualitydomain.FindingFail
		finding.Message = fmt.Sprintf("%s affected=%d", expectation, affected)
	}
	return finding
}

func ratioObservation(numerator, denominator int) map[string]any {
	rate := "UNDEFINED"
	if denominator > 0 {
		rate = fmt.Sprintf("%d/%d", numerator, denominator)
	}
	return map[string]any{
		"numerator":   numerator,
		"denominator": denominator,
		"rate":        rate,
	}
}
