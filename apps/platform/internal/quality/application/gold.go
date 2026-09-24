package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	annotationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	goldinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/gold/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
)

const (
	GoldRuleSetRef       = "gold/quality/annotation-v1"
	GoldRuleSetVersion   = "1.0.0"
	GoldEvaluatorName    = "gold-quality"
	GoldEvaluatorVersion = "1"
	goldRuleSetContent   = `{"id":"gold-annotation-quality","version":"1.0.0","evaluator":"gold-quality@1","rules":[{"id":"GOLD-ANNOTATION-COVERAGE","metric":"usableSelectedCount/taskCount","operator":"EQ","threshold":"1/1","severity":"CRITICAL"},{"id":"GOLD-REVIEWED-COVERAGE","metric":"reviewedCount/taskCount","operator":"EQ","threshold":"1/1","severity":"CRITICAL"},{"id":"GOLD-REJECTED-COUNT","metric":"rejectedCount","operator":"EQ","threshold":0,"severity":"CRITICAL"},{"id":"GOLD-SOURCE-UNIQUENESS","metric":"duplicateSourceCount","operator":"EQ","threshold":0,"severity":"CRITICAL"},{"id":"GOLD-OUTPUT-TASK-MAPPING","metric":"mappingInvalidCount","operator":"EQ","threshold":0,"severity":"CRITICAL"},{"id":"GOLD-SCHEMA-VALIDITY","metric":"schemaInvalidCount","operator":"EQ","threshold":0,"severity":"CRITICAL"},{"id":"GOLD-PROVENANCE-COMPLETE","metric":"provenanceInvalidCount","operator":"EQ","threshold":0,"severity":"CRITICAL"},{"id":"GOLD-OUTPUT-COUNT","metric":"outputRowCount==bindingOutputRowCount==usableSelectedCount","operator":"TRUE","threshold":true,"severity":"CRITICAL"},{"id":"GOLD-AGREEMENT","metric":"agreement","operator":"NOT_APPLICABLE_FOR_SINGLE_ANNOTATOR","severity":"INFO"}]}`
)

var (
	ErrGoldQualityNotConfigured = errors.New("Gold quality assessment is not configured")
	ErrGoldProductionProof      = errors.New("Gold production proof is invalid")
)

type GoldPreflightProvider interface {
	GoldQualityPreflight(context.Context, uuid.UUID, uuid.UUID) (annotationapp.GoldQualityPreflight, error)
}

type GoldRunCommand struct {
	WorkspaceID         uuid.UUID
	DatasetVersionID    uuid.UUID
	AssessmentAttemptID uuid.UUID
	ActorID             *uuid.UUID
	TraceID             string
}

func (s *Service) ConfigureGold(
	goldRepo *goldinfra.PostgresRepository,
	preflight GoldPreflightProvider,
) *Service {
	s.goldRepo = goldRepo
	s.goldPreflight = preflight
	return s
}

func (s *Service) RunGold(ctx context.Context, cmd GoldRunCommand) (domain.Assessment, error) {
	if s == nil || s.goldRepo == nil || s.goldPreflight == nil {
		return domain.Assessment{}, ErrGoldQualityNotConfigured
	}
	attemptID := cmd.AssessmentAttemptID
	if attemptID == uuid.Nil {
		attemptID = uuid.New()
	}
	base := RunCommand{
		WorkspaceID:         cmd.WorkspaceID,
		DatasetVersionID:    cmd.DatasetVersionID,
		RuleSetRef:          GoldRuleSetRef,
		AssessmentAttemptID: attemptID,
		ActorID:             cmd.ActorID,
		TraceID:             cmd.TraceID,
		Now:                 time.Now().UTC(),
	}
	if cmd.AssessmentAttemptID != uuid.Nil {
		attempt, found, err := s.reconcileAttempt(ctx, base, attemptID)
		if err != nil {
			return domain.Assessment{}, err
		}
		if found {
			return attempt, nil
		}
	}

	version, err := s.datasetRepo.GetVersion(ctx, cmd.DatasetVersionID)
	if err != nil {
		return domain.Assessment{}, err
	}
	workspaceID, _, err := s.datasetRepo.GetWorkspaceAndType(ctx, version.DatasetID)
	if err != nil {
		return domain.Assessment{}, err
	}
	if workspaceID != cmd.WorkspaceID {
		return domain.Assessment{}, datasetdomain.ErrDatasetWorkspace
	}
	if version.Status != datasetdomain.VersionReady && version.Status != datasetdomain.VersionSuperseded {
		return domain.Assessment{}, fmt.Errorf("%w: output DatasetVersion is %s", ErrGoldProductionProof, version.Status)
	}

	binding, err := s.goldRepo.GetBindingByOutput(ctx, version.ID)
	if err != nil {
		if errors.Is(err, goldinfra.ErrNotFound) {
			return domain.Assessment{}, fmt.Errorf("%w: FINALIZED Gold production binding is missing", ErrGoldProductionProof)
		}
		return domain.Assessment{}, err
	}
	if err := validateGoldBindingForAssessment(binding, version, cmd.WorkspaceID); err != nil {
		return domain.Assessment{}, err
	}
	var result domain.Assessment
	var replayAssessmentID uuid.UUID
	err = s.tx.WithAdvisoryLock(ctx, assessmentAttemptLockPrefix+attemptID.String(), func(ctx context.Context) error {
		startedAt := time.Now().UTC()
		claimed, _, err := s.claimAttempt(ctx, base, attemptID, startedAt)
		if err != nil {
			return err
		}
		if !claimed {
			state, found, err := s.reconcileAttemptState(ctx, base, attemptID)
			if err != nil {
				return err
			}
			if !found {
				return ErrAssessmentAttemptInProgress
			}
			if state.Outcome == "FAILED" {
				return fmt.Errorf("%w: %s", ErrAssessmentAttemptFailed, state.ErrorMessage)
			}
			if state.AssessmentID == nil {
				return ErrAssessmentAttemptInProgress
			}
			replayAssessmentID = *state.AssessmentID
			return nil
		}

		preflight, err := s.goldPreflight.GoldQualityPreflight(ctx, cmd.WorkspaceID, binding.AnnotationCampaignID)
		if err != nil {
			wrapped := fmt.Errorf("%w: evaluate frozen annotation facts: %v", ErrGoldProductionProof, err)
			if outcomeErr := s.recordAttemptFailureAfterEvaluation(ctx, base, attemptID, wrapped.Error(), time.Now().UTC()); outcomeErr != nil {
				return fmt.Errorf("%v; record attempt outcome: %w", wrapped, outcomeErr)
			}
			return wrapped
		}
		if preflight.SnapshotID != binding.AnnotationSnapshotID ||
			preflight.SnapshotRoot != binding.SnapshotRootHash {
			wrapped := fmt.Errorf("%w: preflight snapshot does not match production binding", ErrGoldProductionProof)
			if outcomeErr := s.recordAttemptFailureAfterEvaluation(ctx, base, attemptID, wrapped.Error(), time.Now().UTC()); outcomeErr != nil {
				return fmt.Errorf("%v; record attempt outcome: %w", wrapped, outcomeErr)
			}
			return wrapped
		}

		findings := cloneGoldFindings(preflight.Findings)
		findings = append(findings, goldOutputCountFinding(binding, version, preflight))
		metrics := cloneGoldMetrics(preflight.Metrics)
		metrics["mode"] = "FORMAL"
		metrics["productionBindingId"] = binding.ID.String()
		metrics["productionBindingRootHash"] = binding.RootHash
		metrics["annotationCampaignId"] = binding.AnnotationCampaignID.String()
		metrics["annotationSnapshotId"] = binding.AnnotationSnapshotID.String()
		metrics["annotationSnapshotRoot"] = binding.SnapshotRootHash
		metrics["inputDatasetVersionId"] = binding.InputDatasetVersionID.String()
		metrics["outputDatasetVersionId"] = version.ID.String()
		metrics["outputChecksumSha256"] = strings.ToLower(version.ChecksumValue)
		metrics["outputRowCount"] = binding.OutputRowCount
		metrics["formalAssessment"] = true

		result = domain.NewAssessment(
			cmd.WorkspaceID,
			version.ID,
			GoldRuleSetRef,
			GoldRuleSetVersion,
			goldRuleSetSHA256(),
			goldRuleSetContent,
			GoldEvaluatorName,
			GoldEvaluatorVersion,
			metrics,
			findings,
			cmd.ActorID,
		)

		err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			if err := s.repo.InsertResult(ctx, tx, result); err != nil {
				return err
			}
			if _, err := deliveryfence.Advance(ctx, tx, cmd.WorkspaceID); err != nil {
				return err
			}
			if _, err := evidence.Append(ctx, tx, evidence.Record{
				WorkspaceID:  cmd.WorkspaceID,
				EvidenceType: "GOLD_QUALITY_ASSESSMENT",
				Title:        "Formal Gold quality assessment",
				SourceType:   "QUALITY_RESULT",
				SourceID:     &result.ID,
				Metadata: map[string]any{
					"datasetVersionId":           version.ID,
					"productionBindingId":        binding.ID,
					"productionBindingRootHash":  binding.RootHash,
					"annotationSnapshotId":       binding.AnnotationSnapshotID,
					"annotationSnapshotRootHash": binding.SnapshotRootHash,
					"ruleSetRef":                 GoldRuleSetRef,
					"ruleSetVersion":             GoldRuleSetVersion,
					"ruleSetContentSha256":       result.RuleSetContentSHA256,
					"gateDecision":               result.GateDecision,
				},
				CreatedBy: cmd.ActorID,
			},
				evidence.Relation{ObjectType: "DATASET_VERSION", ObjectID: version.ID, RelationType: "QUALITY_EVIDENCE"},
				evidence.Relation{ObjectType: "QUALITY_RESULT", ObjectID: result.ID, RelationType: "EVIDENCE_FOR"},
				evidence.Relation{ObjectType: "GOLD_PRODUCTION_BINDING", ObjectID: binding.ID, RelationType: "QUALITY_INPUT"},
			); err != nil {
				return err
			}
			eventType := "QualityPassed"
			if result.GateDecision == domain.GateFail {
				eventType = "QualityFailed"
			} else if result.GateDecision == domain.GateReview {
				eventType = "QualityReviewRequired"
			}
			event, err := outbox.NewEvent("QUALITY_RESULT", result.ID, eventType, map[string]any{
				"qualityResultId":          result.ID,
				"datasetVersionId":         version.ID,
				"productionBindingId":      binding.ID,
				"annotationSnapshotId":     binding.AnnotationSnapshotID,
				"gateDecision":             result.GateDecision,
				"ruleSetVersion":           result.RuleSetVersion,
				"ruleSetContentSha256":     result.RuleSetContentSHA256,
				"evaluatorName":            result.EvaluatorName,
				"evaluatorVersion":         result.EvaluatorVersion,
			})
			if err != nil {
				return err
			}
			if err := outbox.Append(ctx, tx, event); err != nil {
				return err
			}
			if err := audit.Append(ctx, tx, audit.Event{
				WorkspaceID: &cmd.WorkspaceID,
				ActorType:   actorType(cmd.ActorID),
				ActorID:     cmd.ActorID,
				Action:      "GOLD_QUALITY_ASSESSMENT_COMPLETED",
				ObjectType:  "QUALITY_RESULT",
				ObjectID:    result.ID,
				AfterState: map[string]any{
					"datasetVersionId":         version.ID,
					"productionBindingId":       binding.ID,
					"annotationSnapshotId":      binding.AnnotationSnapshotID,
					"gateDecision":              result.GateDecision,
					"ruleSetContentSha256":      result.RuleSetContentSHA256,
				},
				TraceID: cmd.TraceID,
			}); err != nil {
				return err
			}
			_, err = s.repo.AppendAssessmentAttemptOutcome(
				ctx, tx, attemptID, "SUCCEEDED", &result.ID, "", result.CreatedAt,
			)
			return err
		})
		if err != nil {
			if outcomeErr := s.recordAttemptFailureAfterEvaluation(
				ctx, base, attemptID, err.Error(), time.Now().UTC(),
			); outcomeErr != nil {
				return fmt.Errorf("persist Gold quality assessment failed: %v; record attempt outcome: %w", err, outcomeErr)
			}
			return err
		}
		return nil
	})
	if errors.Is(err, transaction.ErrAdvisoryLockBusy) {
		return domain.Assessment{}, ErrAssessmentAttemptInProgress
	}
	if err == nil && replayAssessmentID != uuid.Nil {
		result, err = s.repo.GetAssessment(ctx, replayAssessmentID)
	}
	return result, err
}

func validateGoldBindingForAssessment(
	binding goldinfra.ProductionBinding,
	version datasetdomain.DatasetVersion,
	workspaceID uuid.UUID,
) error {
	if binding.Status != "FINALIZED" || binding.FinalizedAt == nil {
		return fmt.Errorf("%w: production binding is not FINALIZED", ErrGoldProductionProof)
	}
	if binding.WorkspaceID != workspaceID ||
		binding.OutputDatasetVersionID != version.ID ||
		version.GeneratedByExecutionID == nil ||
		*version.GeneratedByExecutionID != binding.ExecutionID {
		return fmt.Errorf("%w: producer/output identity mismatch", ErrGoldProductionProof)
	}
	if !strings.EqualFold(binding.OutputChecksumSHA256, version.ChecksumValue) {
		return fmt.Errorf("%w: output checksum mismatch", ErrGoldProductionProof)
	}
	if version.RowCount == nil || *version.RowCount != binding.OutputRowCount {
		return fmt.Errorf("%w: output row count mismatch", ErrGoldProductionProof)
	}
	if strings.TrimSpace(binding.RootHash) == "" || strings.TrimSpace(binding.SnapshotRootHash) == "" {
		return fmt.Errorf("%w: frozen proof hash is missing", ErrGoldProductionProof)
	}
	return nil
}

func goldOutputCountFinding(
	binding goldinfra.ProductionBinding,
	version datasetdomain.DatasetVersion,
	preflight annotationapp.GoldQualityPreflight,
) domain.Finding {
	usable, _ := integerMetric(preflight.Metrics["usableSelectedCount"])
	actual := int64(-1)
	if version.RowCount != nil {
		actual = *version.RowCount
	}
	pass := actual >= 0 && actual == binding.OutputRowCount && actual == usable
	finding := domain.Finding{
		RuleID:    "GOLD-OUTPUT-COUNT",
		Dimension: "CONSISTENCY",
		Severity:  "CRITICAL",
		Status:    domain.FindingPass,
		Observed: map[string]any{
			"outputRowCount":        actual,
			"bindingOutputRowCount": binding.OutputRowCount,
			"usableSelectedCount":   usable,
			"expectation":           "output rows must equal schema-valid ACCEPT/CORRECT selected tasks",
		},
	}
	if !pass {
		finding.Status = domain.FindingFail
		finding.Message = fmt.Sprintf(
			"Gold output row count mismatch: output=%d binding=%d usable=%d",
			actual, binding.OutputRowCount, usable,
		)
	}
	return finding
}

func cloneGoldFindings(input []domain.Finding) []domain.Finding {
	result := make([]domain.Finding, len(input))
	for i, finding := range input {
		result[i] = finding
		result[i].ID = uuid.Nil
		result[i].ResultID = uuid.Nil
		result[i].CreatedAt = time.Time{}
		if finding.Observed != nil {
			result[i].Observed = make(map[string]any, len(finding.Observed))
			for key, value := range finding.Observed {
				result[i].Observed[key] = value
			}
		}
	}
	return result
}

func cloneGoldMetrics(input map[string]any) map[string]any {
	result := make(map[string]any, len(input)+12)
	for key, value := range input {
		result[key] = value
	}
	return result
}

func integerMetric(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), typed == float64(int64(typed))
	default:
		return 0, false
	}
}

func goldRuleSetSHA256() string {
	sum := sha256.Sum256([]byte(goldRuleSetContent))
	return hex.EncodeToString(sum[:])
}
