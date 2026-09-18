package application

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/matching"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/csvinput"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
)

func readCompanyCSV(reader io.Reader) ([]matching.CompanyRecord, error) {
	csvReader := csvinput.NewReader(reader)
	headers, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("read company CSV header: %w", err)
	}
	index := make(map[string]int, len(headers))
	for i, header := range headers {
		index[strings.TrimSpace(header)] = i
	}
	for _, required := range []string{"source_company_id", "company_name"} {
		if _, ok := index[required]; !ok {
			return nil, fmt.Errorf("company CSV missing required field %s", required)
		}
	}

	result := make([]matching.CompanyRecord, 0)
	for {
		row, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read company CSV row: %w", err)
		}
		raw := make(map[string]string, len(headers))
		for i, header := range headers {
			if i < len(row) {
				raw[strings.TrimSpace(header)] = row[i]
			}
		}
		result = append(result, matching.CompanyRecord{
			SourceCompanyID:         field(raw, "source_company_id"),
			CompanyName:             field(raw, "company_name"),
			UnifiedSocialCreditCode: field(raw, "unified_social_credit_code"),
			LegalRepresentative:     field(raw, "legal_representative"),
			RegisteredAddress:       field(raw, "registered_address"),
			EntryDate:               field(raw, "entry_date"),
			CompanyStatus:           field(raw, "company_status"),
			Raw:                     raw,
		})
	}
	return result, nil
}

func field(values map[string]string, key string) string {
	return strings.TrimSpace(values[key])
}

func newCandidate(jobID uuid.UUID, record matching.CompanyRecord, normalized matching.NormalizedCompany, result matching.Result) domain.MatchCandidate {
	status := domain.CandidateUnresolved
	switch result.Decision {
	case domain.DecisionAutoMatch:
		status = domain.CandidateAutoConfirmed
	case domain.DecisionReview:
		status = domain.CandidatePending
	}

	var entityID *uuid.UUID
	if result.Entity != nil {
		id := result.Entity.ID
		entityID = &id
	}
	return domain.MatchCandidate{
		ID:                 uuid.New(),
		JobID:              jobID,
		SourceKey:          normalized.SourceCompanyID,
		SourceName:         record.CompanyName,
		SourcePayload:      record.Raw,
		NormalizedPayload:  normalizedMap(normalized),
		CandidateEntityID:  entityID,
		Decision:           result.Decision,
		Status:             status,
		MatchMethod:        result.Method,
		MatchRuleID:        result.RuleID,
		MatchEngineName:    result.EngineName,
		MatchEngineVersion: result.EngineVersion,
		MatchModelVersion:  result.ModelVersion,
		Confidence:         result.Confidence,
		CreatedAt:          time.Now().UTC(),
	}
}

func normalizedMap(company matching.NormalizedCompany) map[string]string {
	return map[string]string{
		"source_company_id":             company.SourceCompanyID,
		"normalized_company_name":       company.CompanyName,
		"unified_social_credit_code":    company.UnifiedSocialCreditCode,
		"legal_representative":          company.LegalRepresentative,
		"normalized_registered_address": company.RegisteredAddress,
		"entry_date":                    company.EntryDate,
		"company_status":                company.CompanyStatus,
	}
}

func mappingFromCandidate(job domain.MatchJob, candidate domain.MatchCandidate, status domain.MappingStatus, evidenceID *uuid.UUID) domain.EntityMapping {
	if candidate.CandidateEntityID == nil {
		return domain.EntityMapping{}
	}
	return domain.EntityMapping{
		ID:                 uuid.New(),
		WorkspaceID:        job.WorkspaceID,
		EntityID:           *candidate.CandidateEntityID,
		SourceType:         job.SourceType,
		SourceRef:          job.SourceRef,
		SourceKey:          candidate.SourceKey,
		SourceName:         candidate.SourceName,
		MatchMethod:        candidate.MatchMethod,
		MatchRuleID:        candidate.MatchRuleID,
		MatchPolicyVersion: job.PolicyVersion,
		MatchEngineName:    candidate.MatchEngineName,
		MatchEngineVersion: candidate.MatchEngineVersion,
		MatchModelVersion:  candidate.MatchModelVersion,
		Confidence:         candidate.Confidence,
		Status:             status,
		ReviewedBy:         candidate.ReviewedBy,
		ReviewedAt:         candidate.ReviewedAt,
		ReviewerReason:     candidate.ReviewerReason,
		EvidenceID:         evidenceID,
		CreatedAt:          time.Now().UTC(),
	}
}

func (s *MatchService) finalize(ctx context.Context, jobID uuid.UUID, actorID *uuid.UUID, traceID string) error {
	job, err := s.entityRepo.GetJob(ctx, jobID)
	if err != nil {
		return err
	}
	candidates, err := s.entityRepo.ListCandidates(ctx, jobID)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		if candidate.Status == domain.CandidatePending {
			return nil
		}
	}

	content, err := standardizedCSV(candidates)
	if err != nil {
		return err
	}
	version, err := s.datasetWriter.Handle(ctx, datasetapp.UploadVersionCommand{
		DatasetID:   job.OutputDatasetID,
		Filename:    fmt.Sprintf("standardized-company-%s.csv", job.ID.String()),
		ContentType: "text/csv; charset=utf-8",
		Content:     content,
		ActorID:     actorID,
		TraceID:     traceID,
	})
	if err != nil {
		return err
	}

	return s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.datasetRepo.AddLineage(ctx, tx, version.ID, job.InputDatasetVersionID, "ENTITY_RESOLUTION", nil); err != nil {
			return err
		}
		if err := s.entityRepo.CompleteJob(ctx, tx, job.ID, version.ID); err != nil {
			return err
		}
		if err := emitJobCompleted(ctx, tx, job, version.ID); err != nil {
			return err
		}
		record, err := evidence.Append(ctx, tx, evidence.Record{
			WorkspaceID:  job.WorkspaceID,
			EvidenceType: "ENTITY_MATCH_EXECUTION",
			Title:        "Entity matching output produced",
			SourceType:   "ENTITY_MATCH_JOB",
			SourceID:     &job.ID,
			Metadata: map[string]any{
				"policyRef":              job.PolicyRef,
				"policyVersion":          job.PolicyVersion,
				"inputDatasetVersionId":  job.InputDatasetVersionID,
				"outputDatasetVersionId": version.ID,
				"sourceRole":             job.SourceRole,
			},
			CreatedBy: actorID,
		}, evidence.Relation{ObjectType: "ENTITY_MATCH_JOB", ObjectID: job.ID, RelationType: "SUPPORTS"},
			evidence.Relation{ObjectType: "DATASET_VERSION", ObjectID: version.ID, RelationType: "SUPPORTS"})
		if err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &job.WorkspaceID,
			ActorType:   actorType(actorID),
			ActorID:     actorID,
			Action:      "ENTITY_MATCH_JOB_COMPLETED",
			ObjectType:  "ENTITY_MATCH_JOB",
			ObjectID:    job.ID,
			AfterState: map[string]any{
				"status":                 domain.JobSucceeded,
				"outputDatasetVersionId": version.ID,
				"evidenceId":             record.ID,
			},
			TraceID: traceID,
		})
	})
}

func standardizedCSV(candidates []domain.MatchCandidate) ([]byte, error) {
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].SourceKey < candidates[j].SourceKey })
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	headers := []string{
		"canonical_company_id", "source_company_id", "company_name", "normalized_company_name",
		"unified_social_credit_code", "legal_representative", "registered_address",
		"normalized_registered_address", "entry_date", "company_status",
		"match_status", "match_method", "match_rule_id", "match_confidence",
	}
	if err := writer.Write(headers); err != nil {
		return nil, err
	}
	for _, candidate := range candidates {
		canonicalID := ""
		if candidate.CandidateEntityID != nil && (candidate.Status == domain.CandidateAutoConfirmed || candidate.Status == domain.CandidateConfirmed) {
			canonicalID = candidate.CandidateEntityID.String()
		}
		row := []string{
			canonicalID,
			candidate.SourceKey,
			candidate.SourceName,
			candidate.NormalizedPayload["normalized_company_name"],
			candidate.NormalizedPayload["unified_social_credit_code"],
			candidate.NormalizedPayload["legal_representative"],
			candidate.SourcePayload["registered_address"],
			candidate.NormalizedPayload["normalized_registered_address"],
			candidate.NormalizedPayload["entry_date"],
			candidate.NormalizedPayload["company_status"],
			string(candidate.Status),
			candidate.MatchMethod,
			candidate.MatchRuleID,
			fmt.Sprintf("%.5f", candidate.Confidence),
		}
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func emitMappingDecision(ctx context.Context, tx pgx.Tx, decision domain.MappingDecision) error {
	event, err := outbox.NewEvent("ENTITY_MAPPING", decision.MappingID, "EntityMappingDecisionRecorded", map[string]any{
		"decisionId":        decision.ID,
		"mappingId":         decision.MappingID,
		"entityId":          decision.EntityID,
		"sourceType":        decision.SourceType,
		"sourceRef":         decision.SourceRef,
		"sourceKey":         decision.SourceKey,
		"status":            decision.Status,
		"sourceOrigin":      decision.SourceOrigin,
		"sourceJobId":       decision.SourceJobID,
		"sourceCandidateId": decision.SourceCandidateID,
	})
	if err != nil {
		return err
	}
	return outbox.Append(ctx, tx, event)
}

func actorType(actorID *uuid.UUID) string {
	if actorID == nil {
		return "SYSTEM"
	}
	return "USER"
}
