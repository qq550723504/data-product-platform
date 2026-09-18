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
