package application_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	entityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	resourceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
)

type memoryStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newMemoryStore() *memoryStore {
	return &memoryStore{objects: map[string][]byte{}}
}

func (s *memoryStore) Put(_ context.Context, objectName string, reader io.Reader, _ int64, _ string) (string, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	uri := "s3://test-bucket/" + objectName
	s.mu.Lock()
	s.objects[uri] = append([]byte(nil), content...)
	s.mu.Unlock()
	return uri, nil
}

func (s *memoryStore) Get(_ context.Context, storageURI string) (io.ReadCloser, error) {
	s.mu.Lock()
	content := append([]byte(nil), s.objects[storageURI]...)
	s.mu.Unlock()
	return io.NopCloser(bytes.NewReader(content)), nil
}

func (s *memoryStore) bytes(storageURI string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.objects[storageURI]...)
}

func TestCompanyEntityResolutionReferenceSlice(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer pool.Close()

	txManager := transaction.NewManager(pool)
	datasetRepo := datasetinfra.NewPostgresRepository(pool)
	store := newMemoryStore()
	createDataset := application.NewCreateDatasetService(txManager, datasetRepo, resourceinfra.NewPostgresRepository())
	uploadDataset := application.NewUploadVersionService(txManager, datasetRepo, store)
	entityRepo := entityinfra.NewPostgresRepository(pool)

	workspaceID := uuid.New()
	rawDataset, err := createDataset.Handle(ctx, application.CreateDatasetCommand{
		WorkspaceID: workspaceID,
		Code:        "ENTERPRISE-RAW-" + uuid.NewString(),
		Name:        "Enterprise RAW",
		DatasetType: datasetdomain.DatasetTypeRaw,
		TraceID:     "entity-integration",
	})
	if err != nil {
		t.Fatalf("create raw dataset: %v", err)
	}
	standardizedDataset, err := createDataset.Handle(ctx, application.CreateDatasetCommand{
		WorkspaceID: workspaceID,
		Code:        "ENTERPRISE-STD-" + uuid.NewString(),
		Name:        "Enterprise standardized",
		DatasetType: datasetdomain.DatasetTypeStandardized,
		TraceID:     "entity-integration",
	})
	if err != nil {
		t.Fatalf("create standardized dataset: %v", err)
	}

	fixture := readFixture(t, "enterprise.csv")
	rawVersion, err := uploadDataset.Handle(ctx, application.UploadVersionCommand{
		DatasetID:   rawDataset.ID,
		Filename:    "enterprise.csv",
		ContentType: "text/csv",
		Content:     fixture,
		TraceID:     "entity-integration",
	})
	if err != nil {
		t.Fatalf("upload raw dataset: %v", err)
	}

	policyRoot := repoPath(t, "industry-packs")
	service := entityapp.NewMatchService(policyRoot, txManager, entityRepo, datasetRepo, uploadDataset, store)
	job, err := service.Start(ctx, entityapp.StartJobCommand{
		WorkspaceID:           workspaceID,
		InputDatasetVersionID: rawVersion.ID,
		OutputDatasetID:       standardizedDataset.ID,
		SourceType:            "CSV",
		SourceRef:             "enterprise.csv",
		SourceRole:            domain.SourceAnchor,
		PolicyRef:             "park/matching/company-match-policy-v1.yaml",
		TraceID:               "entity-integration",
	})
	if err != nil {
		t.Fatalf("start entity match job: %v", err)
	}
	if job.Status != domain.JobWaitingReview {
		t.Fatalf("job status = %s, want WAITING_REVIEW", job.Status)
	}
	if job.ReviewCount != 1 {
		t.Fatalf("review count = %d, want 1", job.ReviewCount)
	}

	candidates, err := entityRepo.ListCandidates(ctx, job.ID)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	var reviewCandidate *domain.MatchCandidate
	for i := range candidates {
		if candidates[i].Status == domain.CandidatePending {
			candidate := candidates[i]
			reviewCandidate = &candidate
			break
		}
	}
	if reviewCandidate == nil {
		t.Fatal("expected one pending review candidate")
	}
	if reviewCandidate.SourceKey != "ENT-005" {
		t.Fatalf("review source = %s, want ENT-005", reviewCandidate.SourceKey)
	}
	if reviewCandidate.MatchRuleID != "COMPANY-NAME-LEGAL-REVIEW" {
		t.Fatalf("review rule = %s, want COMPANY-NAME-LEGAL-REVIEW", reviewCandidate.MatchRuleID)
	}

	reviewerID := uuid.New()
	job, err = service.Confirm(ctx, entityapp.ReviewCommand{
		CandidateID: reviewCandidate.ID,
		ReviewerID:  reviewerID,
		Reason:      "same legal representative and highly similar normalized company name",
		TraceID:     "entity-integration",
	})
	if err != nil {
		t.Fatalf("confirm entity review: %v", err)
	}
	if job.Status != domain.JobSucceeded || job.OutputDatasetVersionID == nil {
		t.Fatalf("job after review = status %s output %v, want SUCCEEDED with output", job.Status, job.OutputDatasetVersionID)
	}

	outputVersion, err := datasetRepo.GetVersion(ctx, *job.OutputDatasetVersionID)
	if err != nil {
		t.Fatalf("get standardized version: %v", err)
	}
	rows := parseCSV(t, store.bytes(outputVersion.StorageURI))
	canonical := map[string]string{}
	for _, row := range rows {
		canonical[row["source_company_id"]] = row["canonical_company_id"]
	}
	if canonical["ENT-001"] == "" || canonical["ENT-001"] != canonical["ENT-005"] {
		t.Fatalf("ENT-001=%q ENT-005=%q, expected same canonical entity", canonical["ENT-001"], canonical["ENT-005"])
	}

	var evidenceCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence WHERE source_type='ENTITY_MATCH_CANDIDATE' AND source_id=$1`, reviewCandidate.ID).Scan(&evidenceCount); err != nil {
		t.Fatalf("count review evidence: %v", err)
	}
	if evidenceCount != 1 {
		t.Fatalf("review evidence count = %d, want 1", evidenceCount)
	}

	// Retrying the same confirmation must not append a second decision,
	// evidence, audit or outbox record. The candidate state guard rejects the
	// replay, and the repository idempotency key makes an in-flight retry return
	// the original decision instead of writing again.
	if _, err := service.Confirm(ctx, entityapp.ReviewCommand{
		CandidateID: reviewCandidate.ID,
		ReviewerID:  reviewerID,
		Reason:      "same legal representative and highly similar normalized company name",
		TraceID:     "entity-integration-replay",
	}); err == nil {
		t.Fatal("replayed confirmation unexpectedly succeeded")
	}

	var decisionCount, auditCount, outboxCount, replayEvidenceCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM entity_mapping_decision
		WHERE workspace_id=$1 AND source_type='CSV' AND source_ref='enterprise.csv' AND source_key=$2
	`, workspaceID, reviewCandidate.SourceKey).Scan(&decisionCount); err != nil {
		t.Fatalf("count mapping decisions: %v", err)
	}
	if decisionCount != 1 {
		t.Fatalf("mapping decision count after replay = %d, want 1", decisionCount)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_event WHERE object_type='ENTITY_MATCH_CANDIDATE' AND object_id=$1`, reviewCandidate.ID).Scan(&auditCount); err != nil {
		t.Fatalf("count audit events: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("audit event count after replay = %d, want 1", auditCount)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM outbox_event o
		JOIN entity_mapping em ON em.id=o.aggregate_id
		WHERE o.aggregate_type='ENTITY_MAPPING'
		  AND em.workspace_id=$1 AND em.source_type='CSV' AND em.source_ref='enterprise.csv' AND em.source_key=$2
	`, workspaceID, reviewCandidate.SourceKey).Scan(&outboxCount); err != nil {
		t.Fatalf("count outbox events: %v", err)
	}
	if outboxCount != 1 {
		t.Fatalf("outbox event count after replay = %d, want 1", outboxCount)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence WHERE source_type='ENTITY_MATCH_CANDIDATE' AND source_id=$1`, reviewCandidate.ID).Scan(&replayEvidenceCount); err != nil {
		t.Fatalf("count review evidence after replay: %v", err)
	}
	if replayEvidenceCount != 1 {
		t.Fatalf("review evidence count after replay = %d, want 1", replayEvidenceCount)
	}
}

func TestAmbiguousMatchRequiresExplicitFrozenSelection(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer pool.Close()

	txManager := transaction.NewManager(pool)
	datasetRepo := datasetinfra.NewPostgresRepository(pool)
	store := newMemoryStore()
	createDataset := application.NewCreateDatasetService(txManager, datasetRepo, resourceinfra.NewPostgresRepository())
	uploadDataset := application.NewUploadVersionService(txManager, datasetRepo, store)
	entityRepo := entityinfra.NewPostgresRepository(pool)

	workspaceID := uuid.New()
	entityType, err := domain.NewEntityType(
		workspaceID, "COMPANY", "Company",
		"park/matching/company-match-policy-v1.yaml", "1.0.0",
	)
	if err != nil {
		t.Fatalf("new entity type: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin entity seed: %v", err)
	}
	storedType, err := entityRepo.EnsureEntityType(ctx, tx, entityType)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("ensure entity type: %v", err)
	}
	common := map[string]any{
		"normalized_company_name":       "同名科技有限公司",
		"normalized_registered_address": "深圳市南山区1号",
	}
	first, err := domain.NewEntity(workspaceID, storedType.ID, "AMB-A", "同名科技 A", common, nil)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("new first entity: %v", err)
	}
	second, err := domain.NewEntity(workspaceID, storedType.ID, "AMB-B", "同名科技 B", common, nil)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("new second entity: %v", err)
	}
	outsider, err := domain.NewEntity(workspaceID, storedType.ID, "OUTSIDE", "其他企业", map[string]any{
		"normalized_company_name":       "其他企业",
		"normalized_registered_address": "其他地址",
	}, nil)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("new outsider entity: %v", err)
	}
	for _, entity := range []domain.Entity{first, second, outsider} {
		if err := entityRepo.InsertEntity(ctx, tx, entity); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("insert entity %s: %v", entity.ID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit entity seed: %v", err)
	}

	rawDataset, err := createDataset.Handle(ctx, application.CreateDatasetCommand{
		WorkspaceID: workspaceID,
		Code:        "AMBIGUOUS-RAW-" + uuid.NewString(),
		Name:        "Ambiguous RAW",
		DatasetType: datasetdomain.DatasetTypeRaw,
		TraceID:     "ambiguous-selection",
	})
	if err != nil {
		t.Fatalf("create raw dataset: %v", err)
	}
	outputDataset, err := createDataset.Handle(ctx, application.CreateDatasetCommand{
		WorkspaceID: workspaceID,
		Code:        "AMBIGUOUS-STD-" + uuid.NewString(),
		Name:        "Ambiguous standardized",
		DatasetType: datasetdomain.DatasetTypeStandardized,
		TraceID:     "ambiguous-selection",
	})
	if err != nil {
		t.Fatalf("create standardized dataset: %v", err)
	}
	rawVersion, err := uploadDataset.Handle(ctx, application.UploadVersionCommand{
		DatasetID:   rawDataset.ID,
		Filename:    "ambiguous.csv",
		ContentType: "text/csv",
		Content: []byte("source_company_id,company_name,registered_address\n" +
			"AMB-001,同名科技有限公司,深圳市南山区1号\n"),
		TraceID: "ambiguous-selection",
	})
	if err != nil {
		t.Fatalf("upload ambiguous dataset: %v", err)
	}

	service := entityapp.NewMatchService(
		repoPath(t, "industry-packs"), txManager, entityRepo, datasetRepo, uploadDataset, store,
	)
	job, err := service.Start(ctx, entityapp.StartJobCommand{
		WorkspaceID:           workspaceID,
		InputDatasetVersionID: rawVersion.ID,
		OutputDatasetID:       outputDataset.ID,
		SourceType:            "CSV",
		SourceRef:             "ambiguous.csv",
		SourceRole:            domain.SourceReference,
		PolicyRef:             "park/matching/company-match-policy-v1.yaml",
		TraceID:               "ambiguous-selection",
	})
	if err != nil {
		t.Fatalf("start ambiguous job: %v", err)
	}
	if job.Status != domain.JobWaitingReview {
		t.Fatalf("job status = %s, want WAITING_REVIEW", job.Status)
	}
	candidates, err := entityRepo.ListCandidates(ctx, job.ID)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1", len(candidates))
	}
	candidate := candidates[0]
	if candidate.Status != domain.CandidatePending || candidate.CandidateEntityID != nil {
		t.Fatalf("ambiguous candidate = status %s entity %v, want PENDING/nil", candidate.Status, candidate.CandidateEntityID)
	}
	if candidate.MatchMethod != "NAME_ADDRESS_AMBIGUOUS" || len(candidate.Alternatives) != 2 {
		t.Fatalf("ambiguous candidate did not freeze alternatives: method=%s alternatives=%+v", candidate.MatchMethod, candidate.Alternatives)
	}
	alternativeIDs := map[uuid.UUID]bool{}
	for _, alternative := range candidate.Alternatives {
		alternativeIDs[alternative.EntityID] = true
	}
	if !alternativeIDs[first.ID] || !alternativeIDs[second.ID] || alternativeIDs[outsider.ID] {
		t.Fatalf("frozen alternatives = %+v, want first+second only", candidate.Alternatives)
	}

	reviewerID := uuid.New()
	if _, err := service.Confirm(ctx, entityapp.ReviewCommand{
		CandidateID: candidate.ID, ReviewerID: reviewerID, Reason: "must choose explicitly",
	}); !errors.Is(err, domain.ErrCandidateEntityRequired) {
		t.Fatalf("confirmation without selection error = %v, want ErrCandidateEntityRequired", err)
	}
	if _, err := service.Confirm(ctx, entityapp.ReviewCommand{
		CandidateID: candidate.ID, ReviewerID: reviewerID, Reason: "not an offered entity",
		SelectedEntityID: &outsider.ID,
	}); !errors.Is(err, domain.ErrCandidateSelectionNotAllowed) {
		t.Fatalf("confirmation outside frozen alternatives error = %v, want ErrCandidateSelectionNotAllowed", err)
	}
	pending, err := entityRepo.GetCandidate(ctx, candidate.ID)
	if err != nil {
		t.Fatalf("reload pending candidate: %v", err)
	}
	if pending.Status != domain.CandidatePending || pending.CandidateEntityID != nil {
		t.Fatalf("rejected selections mutated candidate: %+v", pending)
	}
	var rejectedSideEffects int
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM evidence WHERE source_type='ENTITY_MATCH_CANDIDATE' AND source_id=$1)
		  + (SELECT count(*) FROM audit_event WHERE object_type='ENTITY_MATCH_CANDIDATE' AND object_id=$1)
	`, candidate.ID).Scan(&rejectedSideEffects); err != nil {
		t.Fatalf("count rejected selection side effects: %v", err)
	}
	if rejectedSideEffects != 0 {
		t.Fatalf("rejected selections wrote %d side effects, want 0", rejectedSideEffects)
	}

	job, err = service.Confirm(ctx, entityapp.ReviewCommand{
		CandidateID: candidate.ID,
		ReviewerID:  reviewerID,
		Reason:      "explicitly selected the second frozen alternative",
		SelectedEntityID: &second.ID,
		TraceID:     "ambiguous-selection-confirm",
	})
	if err != nil {
		t.Fatalf("confirm selected alternative: %v", err)
	}
	if job.Status != domain.JobSucceeded || job.OutputDatasetVersionID == nil {
		t.Fatalf("job after explicit selection = %s / %v, want SUCCEEDED with output", job.Status, job.OutputDatasetVersionID)
	}
	confirmed, err := entityRepo.GetCandidate(ctx, candidate.ID)
	if err != nil {
		t.Fatalf("reload confirmed candidate: %v", err)
	}
	if confirmed.Status != domain.CandidateConfirmed || confirmed.CandidateEntityID == nil || *confirmed.CandidateEntityID != second.ID {
		t.Fatalf("confirmed candidate = %+v, want selected entity %s", confirmed, second.ID)
	}
	mapping, err := entityRepo.GetMappingBySource(ctx, workspaceID, "CSV", "ambiguous.csv", "AMB-001")
	if err != nil {
		t.Fatalf("read selected mapping: %v", err)
	}
	if mapping.EntityID != second.ID || mapping.ReviewedBy == nil || *mapping.ReviewedBy != reviewerID {
		t.Fatalf("selected mapping = %+v, want entity %s/reviewer %s", mapping, second.ID, reviewerID)
	}
	var origin string
	if err := pool.QueryRow(ctx, `
		SELECT source_origin
		FROM entity_mapping_decision
		WHERE source_candidate_id=$1
	`, candidate.ID).Scan(&origin); err != nil {
		t.Fatalf("read mapping decision provenance: %v", err)
	}
	if origin != string(domain.OriginManualReview) {
		t.Fatalf("mapping decision origin = %s, want %s", origin, domain.OriginManualReview)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE entity_match_candidate SET candidate_alternatives='[]'::jsonb WHERE id=$1
	`, candidate.ID); err == nil {
		t.Fatal("candidate alternatives mutation unexpectedly succeeded")
	}
	frozen, err := entityRepo.GetCandidate(ctx, candidate.ID)
	if err != nil {
		t.Fatalf("reload frozen alternatives: %v", err)
	}
	if len(frozen.Alternatives) != 2 {
		t.Fatalf("frozen alternatives changed after rejected mutation: %+v", frozen.Alternatives)
	}
}

func TestFailedMatchJobDecisionsRemainHistoryNotAuthority(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer pool.Close()

	txManager := transaction.NewManager(pool)
	datasetRepo := datasetinfra.NewPostgresRepository(pool)
	store := newMemoryStore()
	createDataset := application.NewCreateDatasetService(txManager, datasetRepo, resourceinfra.NewPostgresRepository())
	uploadDataset := application.NewUploadVersionService(txManager, datasetRepo, store)
	entityRepo := entityinfra.NewPostgresRepository(pool)

	workspaceID := uuid.New()
	rawDataset, err := createDataset.Handle(ctx, application.CreateDatasetCommand{
		WorkspaceID: workspaceID,
		Code:        "ENTITY-FAILURE-RAW-" + uuid.NewString(),
		Name:        "Entity failure RAW",
		DatasetType: datasetdomain.DatasetTypeRaw,
		TraceID:     "entity-failure-authority",
	})
	if err != nil {
		t.Fatalf("create raw dataset: %v", err)
	}
	standardizedDataset, err := createDataset.Handle(ctx, application.CreateDatasetCommand{
		WorkspaceID: workspaceID,
		Code:        "ENTITY-FAILURE-STD-" + uuid.NewString(),
		Name:        "Entity failure standardized",
		DatasetType: datasetdomain.DatasetTypeStandardized,
		TraceID:     "entity-failure-authority",
	})
	if err != nil {
		t.Fatalf("create standardized dataset: %v", err)
	}

	sourceRef := "entity-failure-authority.csv"
	sourceKey := "ENT-FAIL-001"
	creditCode := "91310000TESTFAIL001"
	validCSV := []byte("source_company_id,company_name,unified_social_credit_code\n" +
		sourceKey + ",Failure Authority Co," + creditCode + "\n")
	validVersion, err := uploadDataset.Handle(ctx, application.UploadVersionCommand{
		DatasetID:   rawDataset.ID,
		Filename:    "entity-failure-authority-valid.csv",
		ContentType: "text/csv",
		Content:     validCSV,
		TraceID:     "entity-failure-authority",
	})
	if err != nil {
		t.Fatalf("upload valid raw dataset: %v", err)
	}

	service := entityapp.NewMatchService(repoPath(t, "industry-packs"), txManager, entityRepo, datasetRepo, uploadDataset, store)
	firstJob, err := service.Start(ctx, entityapp.StartJobCommand{
		WorkspaceID:           workspaceID,
		InputDatasetVersionID: validVersion.ID,
		OutputDatasetID:       standardizedDataset.ID,
		SourceType:            "CSV",
		SourceRef:             sourceRef,
		SourceRole:            domain.SourceAnchor,
		PolicyRef:             "park/matching/company-match-policy-v1.yaml",
		TraceID:               "entity-failure-authority-first",
	})
	if err != nil {
		t.Fatalf("start first entity match job: %v", err)
	}
	if firstJob.Status != domain.JobSucceeded {
		t.Fatalf("first job status = %s, want SUCCEEDED", firstJob.Status)
	}

	authoritativeBefore, err := entityRepo.GetMappingBySource(ctx, workspaceID, "CSV", sourceRef, sourceKey)
	if err != nil {
		t.Fatalf("read first authoritative mapping: %v", err)
	}
	if authoritativeBefore.CurrentDecisionID == nil {
		t.Fatal("first authoritative mapping has no decision")
	}
	firstDecisionID := *authoritativeBefore.CurrentDecisionID

	failingCSV := []byte("source_company_id,company_name,unified_social_credit_code\n" +
		sourceKey + ",Failure Authority Co," + creditCode + "\n" +
		"ENT-FAIL-002,," + "91310000TESTFAIL002" + "\n")
	failingVersion, err := uploadDataset.Handle(ctx, application.UploadVersionCommand{
		DatasetID:   rawDataset.ID,
		Filename:    "entity-failure-authority-failing.csv",
		ContentType: "text/csv",
		Content:     failingCSV,
		TraceID:     "entity-failure-authority",
	})
	if err != nil {
		t.Fatalf("upload failing raw dataset: %v", err)
	}

	if _, err := service.Start(ctx, entityapp.StartJobCommand{
		WorkspaceID:           workspaceID,
		InputDatasetVersionID: failingVersion.ID,
		OutputDatasetID:       standardizedDataset.ID,
		SourceType:            "CSV",
		SourceRef:             sourceRef,
		SourceRole:            domain.SourceAnchor,
		PolicyRef:             "park/matching/company-match-policy-v1.yaml",
		TraceID:               "entity-failure-authority-second",
	}); err == nil {
		t.Fatal("second entity match job unexpectedly succeeded")
	}

	var failedJobID uuid.UUID
	if err := pool.QueryRow(ctx, `
		SELECT id
		FROM entity_match_job
		WHERE workspace_id=$1 AND source_ref=$2 AND status='FAILED'
		ORDER BY created_at DESC
		LIMIT 1
	`, workspaceID, sourceRef).Scan(&failedJobID); err != nil {
		t.Fatalf("find failed entity match job: %v", err)
	}

	var failedDecisionCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM entity_mapping_decision
		WHERE workspace_id=$1 AND source_job_id=$2 AND source_key=$3
	`, workspaceID, failedJobID, sourceKey).Scan(&failedDecisionCount); err != nil {
		t.Fatalf("count failed-job decisions: %v", err)
	}
	if failedDecisionCount != 1 {
		t.Fatalf("failed-job decision count = %d, want 1 immutable history fact", failedDecisionCount)
	}

	history, err := entityRepo.ListMappingDecisions(ctx, workspaceID, "CSV", sourceRef, sourceKey)
	if err != nil {
		t.Fatalf("list mapping decision history: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("mapping decision history count = %d, want 2", len(history))
	}
	if history[1].SourceJobID == nil || *history[1].SourceJobID != failedJobID {
		t.Fatalf("latest immutable decision source job = %v, want failed job %s", history[1].SourceJobID, failedJobID)
	}

	authoritativeAfter, err := entityRepo.GetMappingBySource(ctx, workspaceID, "CSV", sourceRef, sourceKey)
	if err != nil {
		t.Fatalf("read authoritative mapping after failed job: %v", err)
	}
	if authoritativeAfter.CurrentDecisionID == nil || *authoritativeAfter.CurrentDecisionID != firstDecisionID {
		t.Fatalf("authoritative decision after failed job = %v, want prior successful decision %s", authoritativeAfter.CurrentDecisionID, firstDecisionID)
	}
	if authoritativeAfter.EntityID != authoritativeBefore.EntityID {
		t.Fatalf("authoritative entity after failed job = %s, want %s", authoritativeAfter.EntityID, authoritativeBefore.EntityID)
	}
}

func TestMatchJobRejectsDatasetsFromAnotherWorkspaceOrType(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer pool.Close()

	txManager := transaction.NewManager(pool)
	datasetRepo := datasetinfra.NewPostgresRepository(pool)
	store := newMemoryStore()
	createDataset := application.NewCreateDatasetService(txManager, datasetRepo, resourceinfra.NewPostgresRepository())
	uploadDataset := application.NewUploadVersionService(txManager, datasetRepo, store)
	entityRepo := entityinfra.NewPostgresRepository(pool)

	workspaceID := uuid.New()
	otherWorkspaceID := uuid.New()
	rawDataset, err := createDataset.Handle(ctx, application.CreateDatasetCommand{
		WorkspaceID: workspaceID,
		Code:        "ENTERPRISE-RAW-" + uuid.NewString(),
		Name:        "Enterprise RAW",
		DatasetType: datasetdomain.DatasetTypeRaw,
		TraceID:     "entity-tenant-boundary",
	})
	if err != nil {
		t.Fatalf("create raw dataset: %v", err)
	}
	standardizedDataset, err := createDataset.Handle(ctx, application.CreateDatasetCommand{
		WorkspaceID: workspaceID,
		Code:        "ENTERPRISE-STD-" + uuid.NewString(),
		Name:        "Enterprise standardized",
		DatasetType: datasetdomain.DatasetTypeStandardized,
		TraceID:     "entity-tenant-boundary",
	})
	if err != nil {
		t.Fatalf("create standardized dataset: %v", err)
	}
	foreignDataset, err := createDataset.Handle(ctx, application.CreateDatasetCommand{
		WorkspaceID: otherWorkspaceID,
		Code:        "ENTERPRISE-STD-FOREIGN-" + uuid.NewString(),
		Name:        "Foreign standardized",
		DatasetType: datasetdomain.DatasetTypeStandardized,
		TraceID:     "entity-tenant-boundary",
	})
	if err != nil {
		t.Fatalf("create foreign standardized dataset: %v", err)
	}

	rawVersion, err := uploadDataset.Handle(ctx, application.UploadVersionCommand{
		DatasetID:   rawDataset.ID,
		Filename:    "enterprise.csv",
		ContentType: "text/csv",
		Content:     readFixture(t, "enterprise.csv"),
		TraceID:     "entity-tenant-boundary",
	})
	if err != nil {
		t.Fatalf("upload raw dataset: %v", err)
	}

	service := entityapp.NewMatchService(repoPath(t, "industry-packs"), txManager, entityRepo, datasetRepo, uploadDataset, store)
	base := entityapp.StartJobCommand{
		InputDatasetVersionID: rawVersion.ID,
		SourceType:            "CSV",
		SourceRef:             "enterprise.csv",
		SourceRole:            domain.SourceAnchor,
		PolicyRef:             "park/matching/company-match-policy-v1.yaml",
		TraceID:               "entity-tenant-boundary",
	}

	// A job declared in another workspace must not consume this workspace's data.
	foreignInput := base
	foreignInput.WorkspaceID = otherWorkspaceID
	foreignInput.OutputDatasetID = foreignDataset.ID
	if _, err := service.Start(ctx, foreignInput); !errors.Is(err, datasetdomain.ErrDatasetWorkspace) {
		t.Fatalf("cross workspace input error = %v, want ErrDatasetWorkspace", err)
	}

	// The output dataset must belong to the same workspace as the job.
	foreignOutput := base
	foreignOutput.WorkspaceID = workspaceID
	foreignOutput.OutputDatasetID = foreignDataset.ID
	if _, err := service.Start(ctx, foreignOutput); !errors.Is(err, datasetdomain.ErrDatasetWorkspace) {
		t.Fatalf("cross workspace output error = %v, want ErrDatasetWorkspace", err)
	}

	// Entity resolution writes STANDARDIZED data; RAW output is a domain error.
	wrongType := base
	wrongType.WorkspaceID = workspaceID
	wrongType.OutputDatasetID = rawDataset.ID
	if _, err := service.Start(ctx, wrongType); !errors.Is(err, domain.ErrOutputDatasetType) {
		t.Fatalf("non standardized output error = %v, want ErrOutputDatasetType", err)
	}

	var jobCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM entity_match_job WHERE workspace_id IN ($1, $2)`, workspaceID, otherWorkspaceID).Scan(&jobCount); err != nil {
		t.Fatalf("count match jobs: %v", err)
	}
	if jobCount != 0 {
		t.Fatalf("match jobs created for rejected commands = %d, want 0", jobCount)
	}

	// Positive control: the guards must not block a legitimate command.
	valid := base
	valid.WorkspaceID = workspaceID
	valid.OutputDatasetID = standardizedDataset.ID
	if _, err := service.Start(ctx, valid); err != nil {
		t.Fatalf("start valid match job: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM entity_match_job WHERE workspace_id = $1`, workspaceID).Scan(&jobCount); err != nil {
		t.Fatalf("count match jobs after valid command: %v", err)
	}
	if jobCount != 1 {
		t.Fatalf("match jobs after valid command = %d, want 1", jobCount)
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(repoPath(t, "examples", "enterprise-activity", "data", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return content
}

func repoPath(t *testing.T, parts ...string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../../"))
	return filepath.Join(append([]string{root}, parts...)...)
}

func parseCSV(t *testing.T, content []byte) []map[string]string {
	t.Helper()
	reader := csv.NewReader(bytes.NewReader(content))
	headers, err := reader.Read()
	if err != nil {
		t.Fatalf("read output headers: %v", err)
	}
	result := make([]map[string]string, 0)
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read output row: %v", err)
		}
		mapped := make(map[string]string, len(headers))
		for i, header := range headers {
			mapped[header] = row[i]
		}
		result = append(result, mapped)
	}
	return result
}
