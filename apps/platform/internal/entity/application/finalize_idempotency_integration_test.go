package application

import (
	"bytes"
	"context"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

type finalizeMemoryStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newFinalizeMemoryStore() *finalizeMemoryStore {
	return &finalizeMemoryStore{objects: map[string][]byte{}}
}

func (s *finalizeMemoryStore) Put(_ context.Context, objectName string, reader io.Reader, _ int64, _ string) (string, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	uri := "s3://entity-finalize-test/" + objectName
	s.mu.Lock()
	s.objects[uri] = append([]byte(nil), content...)
	s.mu.Unlock()
	return uri, nil
}

func (s *finalizeMemoryStore) Get(_ context.Context, storageURI string) (io.ReadCloser, error) {
	s.mu.Lock()
	content := append([]byte(nil), s.objects[storageURI]...)
	s.mu.Unlock()
	return io.NopCloser(bytes.NewReader(content)), nil
}

type finalizeFixture struct {
	service       *MatchService
	entityRepo    *entityinfra.PostgresRepository
	datasetRepo   *datasetinfra.PostgresRepository
	datasetWriter *datasetapp.UploadVersionService
	job           domain.MatchJob
}

func TestMatchFinalizeReusesReadyOutputAfterCrash(t *testing.T) {
	ctx := context.Background()
	pool := openFinalizeTestDB(t, ctx)
	defer pool.Close()

	fixture := createFinalizeFixture(t, ctx, pool)
	candidates, err := fixture.entityRepo.ListCandidates(ctx, fixture.job.ID)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	content, err := standardizedCSV(candidates)
	if err != nil {
		t.Fatalf("build standardized output: %v", err)
	}

	// Simulate the crash window: object/output publication committed, but the
	// entity_match_job terminal transaction never ran.
	prepublished, err := fixture.datasetWriter.Handle(ctx, datasetapp.UploadVersionCommand{
		DatasetID:                   fixture.job.OutputDatasetID,
		Filename:                    "prepublished.csv",
		ContentType:                 "text/csv; charset=utf-8",
		Content:                     content,
		GeneratedByEntityMatchJobID: &fixture.job.ID,
		TraceID:                     "entity-finalize-crash-window",
	})
	if err != nil {
		t.Fatalf("prepublish entity-match output: %v", err)
	}
	before, err := fixture.entityRepo.GetJob(ctx, fixture.job.ID)
	if err != nil {
		t.Fatalf("read job before recovery: %v", err)
	}
	if before.Status != domain.JobRunning || before.OutputDatasetVersionID != nil {
		t.Fatalf("job before recovery = status %s output %v, want RUNNING/nil", before.Status, before.OutputDatasetVersionID)
	}

	if err := fixture.service.finalize(ctx, fixture.job.ID, nil, "entity-finalize-recovery"); err != nil {
		t.Fatalf("recover entity-match finalize: %v", err)
	}
	assertFinalizeFacts(t, ctx, pool, fixture.job.ID, prepublished.ID)
}

func TestMatchFinalizeConcurrentCallsProduceOneOutput(t *testing.T) {
	ctx := context.Background()
	pool := openFinalizeTestDB(t, ctx)
	defer pool.Close()

	fixture := createFinalizeFixture(t, ctx, pool)

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- fixture.service.finalize(ctx, fixture.job.ID, nil, "entity-finalize-concurrent")
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent finalizer: %v", err)
		}
	}

	job, err := fixture.entityRepo.GetJob(ctx, fixture.job.ID)
	if err != nil {
		t.Fatalf("read completed job: %v", err)
	}
	if job.Status != domain.JobSucceeded || job.OutputDatasetVersionID == nil {
		t.Fatalf("job = status %s output %v, want SUCCEEDED with output", job.Status, job.OutputDatasetVersionID)
	}
	assertFinalizeFacts(t, ctx, pool, fixture.job.ID, *job.OutputDatasetVersionID)
}

func openFinalizeTestDB(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	router, err := routing.NewRouter(false)
	if err != nil {
		pool.Close()
		t.Fatalf("build outbox routing table: %v", err)
	}
	outbox.ConfigureAppendObligation(router)
	return pool
}

func createFinalizeFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) finalizeFixture {
	t.Helper()
	workspaceID := uuid.New()
	entityTypeID := uuid.New()
	entityID := uuid.New()
	inputDatasetID := uuid.New()
	inputVersionID := uuid.New()
	outputDatasetID := uuid.New()
	jobID := uuid.New()
	candidateID := uuid.New()
	mappingID := uuid.New()
	decisionID := uuid.New()
	now := time.Now().UTC()

	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type)
		VALUES
			($1,$3,$4,$5,'RAW'),
			($2,$3,$6,$7,'STANDARDIZED')
	`, inputDatasetID, outputDatasetID, workspaceID,
		"FINALIZE-RAW-"+uuid.NewString(), "Finalize RAW",
		"FINALIZE-STD-"+uuid.NewString(), "Finalize standardized"); err != nil {
		t.Fatalf("insert datasets: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, row_count, byte_size, checksum_algorithm, checksum_value,
			metadata, created_at, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT_STORAGE',$3,'text/csv',1,1,'SHA256','input-checksum','{}'::jsonb,$4,$4)
	`, inputVersionID, inputDatasetID, "s3://entity-finalize-test/input.csv", now); err != nil {
		t.Fatalf("insert input DatasetVersion: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE dataset SET current_version_id=$2 WHERE id=$1`, inputDatasetID, inputVersionID); err != nil {
		t.Fatalf("set current input version: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO entity_type (id, workspace_id, code, name)
		VALUES ($1,$2,$3,$4)
	`, entityTypeID, workspaceID, "COMPANY-"+uuid.NewString(), "Company"); err != nil {
		t.Fatalf("insert entity type: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO entity (id, workspace_id, entity_type_id, canonical_key, canonical_name, attributes)
		VALUES ($1,$2,$3,$4,$5,'{}'::jsonb)
	`, entityID, workspaceID, entityTypeID, "COMPANY-001", "Acme Corp"); err != nil {
		t.Fatalf("insert entity: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO entity_match_job (
			id, workspace_id, entity_type_id, input_dataset_version_id, output_dataset_id,
			source_type, source_ref, source_role, policy_ref, policy_version,
			status, created_at, started_at
		) VALUES ($1,$2,$3,$4,$5,'CSV','enterprise.csv','ANCHOR','test-policy','1','RUNNING',$6,$6)
	`, jobID, workspaceID, entityTypeID, inputVersionID, outputDatasetID, now); err != nil {
		t.Fatalf("insert match job: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO entity_match_candidate (
			id, job_id, source_key, source_name, source_payload, normalized_payload,
			candidate_entity_id, decision, status, match_method, match_rule_id,
			match_engine_name, match_engine_version, match_model_version,
			confidence, created_at
		) VALUES (
			$1,$2,'ENT-001','Acme Corp',
			'{"registered_address":"1 Test St"}'::jsonb,
			'{"normalized_company_name":"ACME CORP","unified_social_credit_code":"USCC-001","legal_representative":"Alice","normalized_registered_address":"1 TEST ST","entry_date":"2020-01-01","company_status":"ACTIVE"}'::jsonb,
			$3,'AUTO_MATCH','AUTO_CONFIRMED','EXACT','TEST-RULE','RULES','1','',1,$4
		)
	`, candidateID, jobID, entityID, now); err != nil {
		t.Fatalf("insert match candidate: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin mapping fixture transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `
		INSERT INTO entity_mapping (
			id, entity_id, source_type, source_ref, source_key, source_name,
			match_method, match_rule_id, match_policy_version, confidence, status,
			created_at, match_engine_name, match_engine_version, match_model_version,
			workspace_id, current_decision_id
		) VALUES ($1,$2,'CSV','enterprise.csv','ENT-001','Acme Corp','EXACT','TEST-RULE','1',1,'AUTO_MATCHED',$3,'RULES','1','',$4,$5)
	`, mappingID, entityID, now, workspaceID, decisionID); err != nil {
		t.Fatalf("insert mapping projection: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO entity_mapping_decision (
			id, workspace_id, mapping_id, entity_id, source_type, source_ref, source_key,
			source_name, match_method, match_rule_id, match_policy_version,
			match_engine_name, match_engine_version, match_model_version, confidence,
			status, decided_at, idempotency_key, source_origin, source_job_id, source_candidate_id
		) VALUES (
			$1,$2,$3,$4,'CSV','enterprise.csv','ENT-001','Acme Corp','EXACT','TEST-RULE','1',
			'RULES','1','',1,'AUTO_MATCHED',$5,$6,'MATCH_CANDIDATE',$7,$8
		)
	`, decisionID, workspaceID, mappingID, entityID, now,
		"fixture:"+candidateID.String(), jobID, candidateID); err != nil {
		t.Fatalf("insert mapping decision: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit mapping fixture: %v", err)
	}

	txManager := transaction.NewManager(pool)
	datasetRepo := datasetinfra.NewPostgresRepository(pool)
	entityRepo := entityinfra.NewPostgresRepository(pool)
	store := newFinalizeMemoryStore()
	writer := datasetapp.NewUploadVersionService(txManager, datasetRepo, store)
	service := NewMatchService("", txManager, entityRepo, datasetRepo, writer, store)

	job, err := entityRepo.GetJob(ctx, jobID)
	if err != nil {
		t.Fatalf("load match job fixture: %v", err)
	}
	return finalizeFixture{
		service:       service,
		entityRepo:    entityRepo,
		datasetRepo:   datasetRepo,
		datasetWriter: writer,
		job:           job,
	}
}

func assertFinalizeFacts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, jobID, outputVersionID uuid.UUID) {
	t.Helper()
	var outputCount, proofCount, eventCount, evidenceCount, auditCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM dataset_version
		WHERE generated_by_entity_match_job_id=$1
	`, jobID).Scan(&outputCount); err != nil {
		t.Fatalf("count entity-match outputs: %v", err)
	}
	if outputCount != 1 {
		t.Fatalf("entity-match output versions = %d, want 1", outputCount)
	}
	var storedProducerID *uuid.UUID
	if err := pool.QueryRow(ctx, `
		SELECT generated_by_entity_match_job_id FROM dataset_version WHERE id=$1
	`, outputVersionID).Scan(&storedProducerID); err != nil {
		t.Fatalf("read output producer: %v", err)
	}
	if storedProducerID == nil || *storedProducerID != jobID {
		t.Fatalf("output producer = %v, want %s", storedProducerID, jobID)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM entity_resolution_output_decision
		WHERE output_dataset_version_id=$1 AND source_job_id=$2
	`, outputVersionID, jobID).Scan(&proofCount); err != nil {
		t.Fatalf("count frozen resolution proof: %v", err)
	}
	if proofCount != 1 {
		t.Fatalf("resolution proof rows = %d, want 1", proofCount)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM outbox_event
		WHERE aggregate_type='ENTITY_MATCH_JOB' AND aggregate_id=$1 AND event_type='EntityMatchCompleted'
	`, jobID).Scan(&eventCount); err != nil {
		t.Fatalf("count completion events: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("EntityMatchCompleted events = %d, want 1", eventCount)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM evidence
		WHERE source_type='ENTITY_MATCH_JOB' AND source_id=$1 AND evidence_type='ENTITY_MATCH_EXECUTION'
	`, jobID).Scan(&evidenceCount); err != nil {
		t.Fatalf("count completion evidence: %v", err)
	}
	if evidenceCount != 1 {
		t.Fatalf("completion evidence = %d, want 1", evidenceCount)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_event
		WHERE object_type='ENTITY_MATCH_JOB' AND object_id=$1 AND action='ENTITY_MATCH_JOB_COMPLETED'
	`, jobID).Scan(&auditCount); err != nil {
		t.Fatalf("count completion audit: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("completion audit events = %d, want 1", auditCount)
	}

	var status domain.JobStatus
	var storedOutputID *uuid.UUID
	if err := pool.QueryRow(ctx, `
		SELECT status, output_dataset_version_id FROM entity_match_job WHERE id=$1
	`, jobID).Scan(&status, &storedOutputID); err != nil {
		t.Fatalf("read completed job facts: %v", err)
	}
	if status != domain.JobSucceeded || storedOutputID == nil || *storedOutputID != outputVersionID {
		t.Fatalf("completed job = status %s output %v, want SUCCEEDED/%s", status, storedOutputID, outputVersionID)
	}
}
