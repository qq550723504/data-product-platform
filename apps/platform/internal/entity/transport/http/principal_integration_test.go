package entityhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	entityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/application"
	entitydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	platformprincipal "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	resourceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
)

type reviewHTTPMemoryStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newReviewHTTPMemoryStore() *reviewHTTPMemoryStore {
	return &reviewHTTPMemoryStore{objects: map[string][]byte{}}
}

func (s *reviewHTTPMemoryStore) Put(_ context.Context, objectName string, reader io.Reader, _ int64, _ string) (string, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	uri := "s3://review-http-test/" + objectName
	s.mu.Lock()
	s.objects[uri] = append([]byte(nil), content...)
	s.mu.Unlock()
	return uri, nil
}

func (s *reviewHTTPMemoryStore) Get(_ context.Context, storageURI string) (io.ReadCloser, error) {
	s.mu.Lock()
	content := append([]byte(nil), s.objects[storageURI]...)
	s.mu.Unlock()
	return io.NopCloser(bytes.NewReader(content)), nil
}

func TestEntityReviewHTTPRequiresTrustedPrincipalAndPersistsResolvedActor(t *testing.T) {
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
	store := newReviewHTTPMemoryStore()
	createDataset := datasetapp.NewCreateDatasetService(txManager, datasetRepo, resourceinfra.NewPostgresRepository())
	uploadDataset := datasetapp.NewUploadVersionService(txManager, datasetRepo, store)
	entityRepo := entityinfra.NewPostgresRepository(pool)

	workspaceID := uuid.New()
	rawDataset, err := createDataset.Handle(ctx, datasetapp.CreateDatasetCommand{
		WorkspaceID: workspaceID,
		Code:        "REVIEW-HTTP-RAW-" + uuid.NewString(),
		Name:        "Review HTTP raw",
		DatasetType: datasetdomain.DatasetTypeRaw,
		TraceID:     "review-http-principal",
	})
	if err != nil {
		t.Fatalf("create raw dataset: %v", err)
	}
	outputDataset, err := createDataset.Handle(ctx, datasetapp.CreateDatasetCommand{
		WorkspaceID: workspaceID,
		Code:        "REVIEW-HTTP-STD-" + uuid.NewString(),
		Name:        "Review HTTP standardized",
		DatasetType: datasetdomain.DatasetTypeStandardized,
		TraceID:     "review-http-principal",
	})
	if err != nil {
		t.Fatalf("create standardized dataset: %v", err)
	}
	rawVersion, err := uploadDataset.Handle(ctx, datasetapp.UploadVersionCommand{
		DatasetID:   rawDataset.ID,
		Filename:    "enterprise.csv",
		ContentType: "text/csv",
		Content:     readReviewHTTPFixture(t, "enterprise.csv"),
		TraceID:     "review-http-principal",
	})
	if err != nil {
		t.Fatalf("upload raw dataset: %v", err)
	}

	service := entityapp.NewMatchService(
		reviewHTTPRepoPath(t, "industry-packs"),
		txManager,
		entityRepo,
		datasetRepo,
		uploadDataset,
		store,
	)
	job, err := service.Start(ctx, entityapp.StartJobCommand{
		WorkspaceID:           workspaceID,
		InputDatasetVersionID: rawVersion.ID,
		OutputDatasetID:       outputDataset.ID,
		SourceType:            "CSV",
		SourceRef:             "enterprise.csv",
		SourceRole:            entitydomain.SourceAnchor,
		PolicyRef:             "park/matching/company-match-policy-v1.yaml",
		TraceID:               "review-http-principal",
	})
	if err != nil {
		t.Fatalf("start entity match job: %v", err)
	}
	if job.Status != entitydomain.JobWaitingReview {
		t.Fatalf("job status = %s, want WAITING_REVIEW", job.Status)
	}
	candidates, err := entityRepo.ListCandidates(ctx, job.ID)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	var candidateID uuid.UUID
	for _, candidate := range candidates {
		if candidate.Status == entitydomain.CandidatePending {
			candidateID = candidate.ID
			break
		}
	}
	if candidateID == uuid.Nil {
		t.Fatal("expected one pending review candidate")
	}

	trustedActor := uuid.New()
	forgedActor := uuid.New()
	trustedResolver, err := platformprincipal.NewStaticResolver(
		true,
		"review-secret",
		"reviewer:http-integration",
		trustedActor.String(),
		[]string{workspaceID.String()},
		[]string{platformprincipal.CapabilityHumanDecision},
	)
	if err != nil {
		t.Fatalf("create trusted resolver: %v", err)
	}
	trustedMux := http.NewServeMux()
	NewHandler(service, entityRepo, trustedResolver).Register(trustedMux)

	baselineAudit, baselineEvidence, baselineOutbox := reviewHTTPSideEffectCounts(t, ctx, pool)

	t.Run("unauthenticated fails before business side effects", func(t *testing.T) {
		recorder := postReviewHTTP(t, trustedMux, candidateID, "", forgedActor)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401: %s", recorder.Code, recorder.Body.String())
		}
		assertReviewHTTPErrorCode(t, recorder, "AUTHENTICATION_REQUIRED")
		assertReviewHTTPPending(t, ctx, entityRepo, candidateID)
		assertReviewHTTPSideEffectCounts(t, ctx, pool, baselineAudit, baselineEvidence, baselineOutbox)
	})

	foreignResolver, err := platformprincipal.NewStaticResolver(
		true,
		"review-secret",
		"reviewer:foreign-workspace",
		uuid.NewString(),
		[]string{uuid.NewString()},
		[]string{platformprincipal.CapabilityHumanDecision},
	)
	if err != nil {
		t.Fatalf("create foreign resolver: %v", err)
	}
	foreignMux := http.NewServeMux()
	NewHandler(service, entityRepo, foreignResolver).Register(foreignMux)

	t.Run("cross workspace fails closed with no business side effects", func(t *testing.T) {
		recorder := postReviewHTTP(t, foreignMux, candidateID, "review-secret", forgedActor)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403: %s", recorder.Code, recorder.Body.String())
		}
		assertReviewHTTPErrorCode(t, recorder, "WORKSPACE_ACCESS_DENIED")
		assertReviewHTTPPending(t, ctx, entityRepo, candidateID)
		assertReviewHTTPSideEffectCounts(t, ctx, pool, baselineAudit, baselineEvidence, baselineOutbox)
	})

	t.Run("forged actor header cannot change authoritative reviewer", func(t *testing.T) {
		recorder := postReviewHTTP(t, trustedMux, candidateID, "review-secret", forgedActor)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
		}
		candidate, err := entityRepo.GetCandidate(ctx, candidateID)
		if err != nil {
			t.Fatalf("reload candidate: %v", err)
		}
		if candidate.ReviewedBy == nil || *candidate.ReviewedBy != trustedActor {
			t.Fatalf("reviewed by = %v, want trusted actor %s", candidate.ReviewedBy, trustedActor)
		}
		if *candidate.ReviewedBy == forgedActor {
			t.Fatal("forged X-Actor-ID became authoritative")
		}

		var trustedAudit, forgedAudit, trustedEvidence, forgedEvidence int
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM audit_event
			WHERE object_type='ENTITY_MATCH_CANDIDATE' AND object_id=$1
			  AND action='ENTITY_MATCH_CONFIRMED' AND actor_id=$2
		`, candidateID, trustedActor).Scan(&trustedAudit); err != nil {
			t.Fatalf("count trusted audit: %v", err)
		}
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM audit_event
			WHERE object_type='ENTITY_MATCH_CANDIDATE' AND object_id=$1
			  AND action='ENTITY_MATCH_CONFIRMED' AND actor_id=$2
		`, candidateID, forgedActor).Scan(&forgedAudit); err != nil {
			t.Fatalf("count forged audit: %v", err)
		}
		if trustedAudit != 1 || forgedAudit != 0 {
			t.Fatalf("audit actor counts trusted=%d forged=%d, want 1/0", trustedAudit, forgedAudit)
		}
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM evidence
			WHERE source_type='ENTITY_MATCH_CANDIDATE' AND source_id=$1 AND created_by=$2
		`, candidateID, trustedActor).Scan(&trustedEvidence); err != nil {
			t.Fatalf("count trusted evidence: %v", err)
		}
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM evidence
			WHERE source_type='ENTITY_MATCH_CANDIDATE' AND source_id=$1 AND created_by=$2
		`, candidateID, forgedActor).Scan(&forgedEvidence); err != nil {
			t.Fatalf("count forged evidence: %v", err)
		}
		if trustedEvidence != 1 || forgedEvidence != 0 {
			t.Fatalf("evidence actor counts trusted=%d forged=%d, want 1/0", trustedEvidence, forgedEvidence)
		}
	})
}

func postReviewHTTP(t *testing.T, handler http.Handler, candidateID uuid.UUID, token string, forgedActor uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	body := bytes.NewBufferString(`{"reason":"verified through trusted principal boundary"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/entity-match-reviews/"+candidateID.String()+"/confirm", body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Actor-ID", forgedActor.String())
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func assertReviewHTTPErrorCode(t *testing.T, recorder *httptest.ResponseRecorder, expected string) {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	code, _ := body["code"].(string)
	if nested, ok := body["error"].(map[string]any); ok {
		if value, ok := nested["code"].(string); ok {
			code = value
		}
	}
	if code != expected {
		t.Fatalf("error code = %q, want %q; body=%s", code, expected, recorder.Body.String())
	}
}

func assertReviewHTTPPending(t *testing.T, ctx context.Context, repo *entityinfra.PostgresRepository, candidateID uuid.UUID) {
	t.Helper()
	candidate, err := repo.GetCandidate(ctx, candidateID)
	if err != nil {
		t.Fatalf("reload candidate: %v", err)
	}
	if candidate.Status != entitydomain.CandidatePending || candidate.ReviewedBy != nil {
		t.Fatalf("candidate mutated after rejected request: status=%s reviewedBy=%v", candidate.Status, candidate.ReviewedBy)
	}
}

func reviewHTTPSideEffectCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (int, int, int) {
	t.Helper()
	var auditCount, evidenceCount, outboxCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_event").Scan(&auditCount); err != nil {
		t.Fatalf("count audit events: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM evidence").Scan(&evidenceCount); err != nil {
		t.Fatalf("count evidence: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM outbox_event").Scan(&outboxCount); err != nil {
		t.Fatalf("count outbox events: %v", err)
	}
	return auditCount, evidenceCount, outboxCount
}

func assertReviewHTTPSideEffectCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, auditWant, evidenceWant, outboxWant int) {
	t.Helper()
	auditGot, evidenceGot, outboxGot := reviewHTTPSideEffectCounts(t, ctx, pool)
	if auditGot != auditWant || evidenceGot != evidenceWant || outboxGot != outboxWant {
		t.Fatalf(
			"side effects changed after rejected request: audit=%d/%d evidence=%d/%d outbox=%d/%d",
			auditGot, auditWant, evidenceGot, evidenceWant, outboxGot, outboxWant,
		)
	}
}

func readReviewHTTPFixture(t *testing.T, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(reviewHTTPRepoPath(t, "examples", "enterprise-activity", "data", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return content
}

func reviewHTTPRepoPath(t *testing.T, parts ...string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../../../"))
	return filepath.Join(append([]string{root}, parts...)...)
}
