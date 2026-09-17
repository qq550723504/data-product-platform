package application

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/matching"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

type ObjectReader interface {
	Get(ctx context.Context, storageURI string) (io.ReadCloser, error)
}

type StartJobCommand struct {
	WorkspaceID           uuid.UUID
	InputDatasetVersionID uuid.UUID
	OutputDatasetID       uuid.UUID
	SourceType            string
	SourceRef             string
	SourceRole            domain.SourceRole
	PolicyRef             string
	ActorID               *uuid.UUID
	TraceID               string
}

type MatchService struct {
	policyRoot    string
	tx            *transaction.Manager
	entityRepo    *infrastructure.PostgresRepository
	datasetRepo   *datasetinfra.PostgresRepository
	datasetWriter *datasetapp.UploadVersionService
	store         ObjectReader
	engine        *matching.Engine
}

func NewMatchService(policyRoot string, tx *transaction.Manager, entityRepo *infrastructure.PostgresRepository, datasetRepo *datasetinfra.PostgresRepository, datasetWriter *datasetapp.UploadVersionService, store ObjectReader) *MatchService {
	return &MatchService{
		policyRoot:    policyRoot,
		tx:            tx,
		entityRepo:    entityRepo,
		datasetRepo:   datasetRepo,
		datasetWriter: datasetWriter,
		store:         store,
		engine:        matching.NewEngine(entityRepo),
	}
}

func (s *MatchService) Start(ctx context.Context, cmd StartJobCommand) (domain.MatchJob, error) {
	policyPath, err := matching.ResolvePolicyPath(s.policyRoot, cmd.PolicyRef)
	if err != nil {
		return domain.MatchJob{}, err
	}
	policy, err := matching.LoadPolicy(policyPath)
	if err != nil {
		return domain.MatchJob{}, err
	}
	if policy.Spec.EntityType != "COMPANY" {
		return domain.MatchJob{}, fmt.Errorf("POC matcher supports COMPANY policy, got %s", policy.Spec.EntityType)
	}
	if cmd.WorkspaceID == uuid.Nil || cmd.InputDatasetVersionID == uuid.Nil || cmd.OutputDatasetID == uuid.Nil {
		return domain.MatchJob{}, fmt.Errorf("workspace, input DatasetVersion and output Dataset are required")
	}
	if cmd.SourceRole != domain.SourceAnchor && cmd.SourceRole != domain.SourceReference {
		return domain.MatchJob{}, fmt.Errorf("invalid source role %q", cmd.SourceRole)
	}

	inputVersion, err := s.datasetRepo.GetVersion(ctx, cmd.InputDatasetVersionID)
	if err != nil {
		return domain.MatchJob{}, err
	}
	if inputVersion.Status != datasetdomain.VersionReady {
		return domain.MatchJob{}, fmt.Errorf("input DatasetVersion must be READY")
	}

	entityType, err := domain.NewEntityType(cmd.WorkspaceID, policy.Spec.EntityType, "Company", cmd.PolicyRef, policy.Metadata.Version)
	if err != nil {
		return domain.MatchJob{}, err
	}
	var job domain.MatchJob
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		storedType, err := s.entityRepo.EnsureEntityType(ctx, tx, entityType)
		if err != nil {
			return err
		}
		job = domain.NewMatchJob(cmd.WorkspaceID, storedType.ID, cmd.InputDatasetVersionID, cmd.OutputDatasetID,
			cmd.SourceType, cmd.SourceRef, cmd.SourceRole, cmd.PolicyRef, policy.Metadata.Version, cmd.ActorID)
		if err := s.entityRepo.InsertJob(ctx, tx, job); err != nil {
			return err
		}
		if err := s.entityRepo.MarkJobRunning(ctx, tx, job.ID); err != nil {
			return err
		}
		job.EntityTypeID = storedType.ID
		job.Status = domain.JobRunning
		now := time.Now().UTC()
		job.StartedAt = &now
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &cmd.WorkspaceID,
			ActorType:   actorType(cmd.ActorID),
			ActorID:     cmd.ActorID,
			Action:      "ENTITY_MATCH_JOB_STARTED",
			ObjectType:  "ENTITY_MATCH_JOB",
			ObjectID:    job.ID,
			AfterState: map[string]any{
				"inputDatasetVersionId": cmd.InputDatasetVersionID,
				"outputDatasetId":       cmd.OutputDatasetID,
				"policyRef":             cmd.PolicyRef,
				"policyVersion":         policy.Metadata.Version,
				"sourceRole":            cmd.SourceRole,
			},
			TraceID: cmd.TraceID,
		})
	})
	if err != nil {
		return domain.MatchJob{}, err
	}

	reader, err := s.store.Get(ctx, inputVersion.StorageURI)
	if err != nil {
		return domain.MatchJob{}, s.failJob(ctx, job, err)
	}
	defer reader.Close()

	records, err := readCompanyCSV(reader)
	if err != nil {
		return domain.MatchJob{}, s.failJob(ctx, job, err)
	}
	for _, record := range records {
		if err := s.processRecord(ctx, job, record, policy, cmd.ActorID); err != nil {
			return domain.MatchJob{}, s.failJob(ctx, job, err)
		}
	}

	var status domain.JobStatus
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		refreshed, err := s.entityRepo.RefreshJobCountsAndStatus(ctx, tx, job.ID)
		if err != nil {
			return err
		}
		status = refreshed
		return nil
	})
	if err != nil {
		return domain.MatchJob{}, err
	}

	if status == domain.JobRunning {
		if err := s.finalize(ctx, job.ID, cmd.ActorID, cmd.TraceID); err != nil {
			return domain.MatchJob{}, s.failJob(ctx, job, err)
		}
	}
	return s.entityRepo.GetJob(ctx, job.ID)
}

func (s *MatchService) processRecord(ctx context.Context, job domain.MatchJob, record matching.CompanyRecord, policy matching.Policy, actorID *uuid.UUID) error {
	normalized := matching.NormalizeCompany(record, policy)
	if normalized.SourceCompanyID == "" || normalized.CompanyName == "" {
		return fmt.Errorf("source company id and company name are required")
	}

	result, err := s.engine.Match(ctx, job.EntityTypeID, normalized, policy)
	if err != nil {
		return err
	}
	candidate := newCandidate(job.ID, record, normalized, result)

	return s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		entity := result.Entity
		if entity == nil && result.Decision == domain.DecisionUnresolved && job.SourceRole == domain.SourceAnchor {
			seeded, err := domain.NewEntity(job.WorkspaceID, job.EntityTypeID, normalized.UnifiedSocialCreditCode, record.CompanyName, map[string]any{
				"normalized_company_name":       normalized.CompanyName,
				"normalized_registered_address": normalized.RegisteredAddress,
				"legal_representative":          normalized.LegalRepresentative,
				"registered_address":            record.RegisteredAddress,
				"entry_date":                    normalized.EntryDate,
				"company_status":                normalized.CompanyStatus,
			}, actorID)
			if err != nil {
				return err
			}
			if err := s.entityRepo.InsertEntity(ctx, tx, seeded); err != nil {
				return err
			}
			entity = &seeded
			candidate.CandidateEntityID = &seeded.ID
			candidate.Decision = domain.DecisionAutoMatch
			candidate.Status = domain.CandidateAutoConfirmed
			candidate.MatchMethod = "ANCHOR_SEED"
			candidate.MatchRuleID = "ANCHOR_SEED"
			candidate.MatchEngineName = "RULES"
			candidate.MatchEngineVersion = "1"
			candidate.MatchModelVersion = ""
			candidate.Confidence = 1
		}

		if entity != nil {
			candidate.CandidateEntityID = &entity.ID
		}

		if strings.TrimSpace(candidate.MatchEngineName) != "" && !strings.EqualFold(candidate.MatchEngineName, "RULES") {
			record, err := evidence.Append(ctx, tx, evidence.Record{
				WorkspaceID:  job.WorkspaceID,
				EvidenceType: "ENTITY_MATCH_CANDIDATE_GENERATED",
				Title:        "Probabilistic entity match candidate generated",
				SourceType:   "ENTITY_MATCH_CANDIDATE",
				SourceID:     &candidate.ID,
				Metadata: map[string]any{
					"jobId":              job.ID,
					"sourceKey":          candidate.SourceKey,
					"candidateEntityId":  candidate.CandidateEntityID,
					"decision":           candidate.Decision,
					"confidence":         candidate.Confidence,
					"matchMethod":        candidate.MatchMethod,
					"matchRuleId":        candidate.MatchRuleID,
					"engineName":         candidate.MatchEngineName,
					"engineVersion":      candidate.MatchEngineVersion,
					"modelVersion":       candidate.MatchModelVersion,
					"matchPolicyRef":     policy.Metadata.Name,
					"matchPolicyVersion": policy.Metadata.Version,
				},
				CreatedBy: actorID,
			},
				evidence.Relation{ObjectType: "ENTITY_MATCH_JOB", ObjectID: job.ID, RelationType: "SUPPORTS"},
				evidence.Relation{ObjectType: "ENTITY_MATCH_CANDIDATE", ObjectID: candidate.ID, RelationType: "SUPPORTS"},
			)
			if err != nil {
				return err
			}
			candidate.EvidenceID = &record.ID
		}

		if candidate.Status == domain.CandidateAutoConfirmed && candidate.CandidateEntityID != nil {
			mapping := mappingFromCandidate(job, candidate, domain.MappingAutoMatched, candidate.EvidenceID)
			if err := s.entityRepo.InsertMapping(ctx, tx, mapping); err != nil {
				return err
			}
		}
		return s.entityRepo.InsertCandidate(ctx, tx, candidate)
	})
}

func (s *MatchService) failJob(ctx context.Context, job domain.MatchJob, cause error) error {
	_ = s.tx.Do(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE entity_match_job SET status='FAILED', error_message=$2, finished_at=now() WHERE id=$1`, job.ID, cause.Error())
		return err
	})
	return cause
}

func emitJobCompleted(ctx context.Context, tx pgx.Tx, job domain.MatchJob, outputVersionID uuid.UUID) error {
	event, err := outbox.NewEvent("ENTITY_MATCH_JOB", job.ID, "EntityMatchCompleted", map[string]any{
		"jobId":                  job.ID,
		"outputDatasetVersionId": outputVersionID,
		"policyVersion":          job.PolicyVersion,
	})
	if err != nil {
		return err
	}
	return outbox.Append(ctx, tx, event)
}
