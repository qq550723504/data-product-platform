package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
)

type ReviewCommand struct {
	CandidateID uuid.UUID
	ReviewerID  uuid.UUID
	Reason      string
	TraceID     string
}

func (s *MatchService) Confirm(ctx context.Context, cmd ReviewCommand) (domain.MatchJob, error) {
	candidate, err := s.entityRepo.GetCandidate(ctx, cmd.CandidateID)
	if err != nil {
		return domain.MatchJob{}, err
	}
	job, err := s.entityRepo.GetJob(ctx, candidate.JobID)
	if err != nil {
		return domain.MatchJob{}, err
	}
	if err := candidate.Confirm(cmd.ReviewerID, cmd.Reason); err != nil {
		return domain.MatchJob{}, err
	}

	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		record, err := evidence.Append(ctx, tx, evidence.Record{
			WorkspaceID:  job.WorkspaceID,
			EvidenceType: "ENTITY_MATCH_REVIEW",
			Title:        "Manual entity match confirmed",
			SourceType:   "ENTITY_MATCH_CANDIDATE",
			SourceID:     &candidate.ID,
			Metadata: map[string]any{
				"decision":          "CONFIRMED",
				"sourceKey":         candidate.SourceKey,
				"candidateEntityId": candidate.CandidateEntityID,
				"matchMethod":       candidate.MatchMethod,
				"matchRuleId":       candidate.MatchRuleID,
				"confidence":        candidate.Confidence,
				"policyVersion":     job.PolicyVersion,
				"reviewerReason":    candidate.ReviewerReason,
			},
			CreatedBy: &cmd.ReviewerID,
		}, evidence.Relation{ObjectType: "ENTITY_MATCH_CANDIDATE", ObjectID: candidate.ID, RelationType: "SUPPORTS"},
			evidence.Relation{ObjectType: "ENTITY_MATCH_JOB", ObjectID: job.ID, RelationType: "SUPPORTS"})
		if err != nil {
			return err
		}
		candidate.EvidenceID = &record.ID
		if err := s.entityRepo.UpdateCandidateReview(ctx, tx, candidate); err != nil {
			return err
		}
		mapping := mappingFromCandidate(job, candidate, domain.MappingConfirmed, &record.ID)
		if mapping.EntityID == uuid.Nil {
			return domain.ErrCandidateEntityRequired
		}
		if err := s.entityRepo.InsertMapping(ctx, tx, mapping); err != nil {
			return err
		}
		if _, err := s.entityRepo.RefreshJobCountsAndStatus(ctx, tx, job.ID); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &job.WorkspaceID,
			ActorType:   "USER",
			ActorID:     &cmd.ReviewerID,
			Action:      "ENTITY_MATCH_CONFIRMED",
			ObjectType:  "ENTITY_MATCH_CANDIDATE",
			ObjectID:    candidate.ID,
			BeforeState: map[string]any{"status": domain.CandidatePending},
			AfterState: map[string]any{
				"status":     candidate.Status,
				"entityId":   candidate.CandidateEntityID,
				"evidenceId": record.ID,
			},
			Reason:  candidate.ReviewerReason,
			TraceID: cmd.TraceID,
		})
	})
	if err != nil {
		return domain.MatchJob{}, err
	}

	job, err = s.entityRepo.GetJob(ctx, job.ID)
	if err != nil {
		return domain.MatchJob{}, err
	}
	if job.Status == domain.JobRunning && job.OutputDatasetVersionID == nil {
		if err := s.finalize(ctx, job.ID, &cmd.ReviewerID, cmd.TraceID); err != nil {
			return domain.MatchJob{}, fmt.Errorf("finalize reviewed entity match job: %w", err)
		}
	}
	return s.entityRepo.GetJob(ctx, job.ID)
}

func (s *MatchService) Reject(ctx context.Context, cmd ReviewCommand) (domain.MatchJob, error) {
	candidate, err := s.entityRepo.GetCandidate(ctx, cmd.CandidateID)
	if err != nil {
		return domain.MatchJob{}, err
	}
	job, err := s.entityRepo.GetJob(ctx, candidate.JobID)
	if err != nil {
		return domain.MatchJob{}, err
	}
	if err := candidate.Reject(cmd.ReviewerID, cmd.Reason); err != nil {
		return domain.MatchJob{}, err
	}

	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		record, err := evidence.Append(ctx, tx, evidence.Record{
			WorkspaceID:  job.WorkspaceID,
			EvidenceType: "ENTITY_MATCH_REVIEW",
			Title:        "Manual entity match rejected",
			SourceType:   "ENTITY_MATCH_CANDIDATE",
			SourceID:     &candidate.ID,
			Metadata: map[string]any{
				"decision":          "REJECTED",
				"sourceKey":         candidate.SourceKey,
				"candidateEntityId": candidate.CandidateEntityID,
				"matchMethod":       candidate.MatchMethod,
				"matchRuleId":       candidate.MatchRuleID,
				"confidence":        candidate.Confidence,
				"policyVersion":     job.PolicyVersion,
				"reviewerReason":    candidate.ReviewerReason,
			},
			CreatedBy: &cmd.ReviewerID,
		}, evidence.Relation{ObjectType: "ENTITY_MATCH_CANDIDATE", ObjectID: candidate.ID, RelationType: "SUPPORTS"},
			evidence.Relation{ObjectType: "ENTITY_MATCH_JOB", ObjectID: job.ID, RelationType: "SUPPORTS"})
		if err != nil {
			return err
		}
		candidate.EvidenceID = &record.ID
		if err := s.entityRepo.UpdateCandidateReview(ctx, tx, candidate); err != nil {
			return err
		}
		if _, err := s.entityRepo.RefreshJobCountsAndStatus(ctx, tx, job.ID); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &job.WorkspaceID,
			ActorType:   "USER",
			ActorID:     &cmd.ReviewerID,
			Action:      "ENTITY_MATCH_REJECTED",
			ObjectType:  "ENTITY_MATCH_CANDIDATE",
			ObjectID:    candidate.ID,
			BeforeState: map[string]any{"status": domain.CandidatePending},
			AfterState: map[string]any{
				"status":     candidate.Status,
				"evidenceId": record.ID,
			},
			Reason:  candidate.ReviewerReason,
			TraceID: cmd.TraceID,
		})
	})
	if err != nil {
		return domain.MatchJob{}, err
	}

	job, err = s.entityRepo.GetJob(ctx, job.ID)
	if err != nil {
		return domain.MatchJob{}, err
	}
	if job.Status == domain.JobRunning && job.OutputDatasetVersionID == nil {
		if err := s.finalize(ctx, job.ID, &cmd.ReviewerID, cmd.TraceID); err != nil {
			return domain.MatchJob{}, fmt.Errorf("finalize reviewed entity match job: %w", err)
		}
	}
	return s.entityRepo.GetJob(ctx, job.ID)
}
