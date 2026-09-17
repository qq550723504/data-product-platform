package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	datasethttp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/transport/http"
	entityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/application"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	entityhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	resourceapp "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/application"
	resourceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
)

// Only this focused HTTP/DB test uses an instrumented memory store. The separate
// live-browser suite continues to exercise actual MinIO, API and standalone Next.
type boundaryStore struct {
	*memoryStore
	reads  atomic.Int64
	writes atomic.Int64
}

func (s *boundaryStore) Get(ctx context.Context, uri string) (io.ReadCloser, error) {
	s.reads.Add(1)
	return s.memoryStore.Get(ctx, uri)
}

func (s *boundaryStore) Put(ctx context.Context, name string, r io.Reader, size int64, contentType string) (string, error) {
	s.writes.Add(1)
	return s.memoryStore.Put(ctx, name, r, size, contentType)
}

// This checks cross-object ownership, not caller authentication. Configured
// workspace/actor IDs are not a production authorization boundary.
func TestWorkspaceOwnershipHTTPRejectsWithoutSideEffects(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := database.Open(ctx, dsn)
	liveOK(t, err, "open boundary test database")
	defer pool.Close()
	tx := transaction.NewManager(pool)
	resources := resourceinfra.NewPostgresRepository()
	datasets := datasetinfra.NewPostgresRepository(pool)
	entities := entityinfra.NewPostgresRepository(pool)
	store := &boundaryStore{memoryStore: newMemoryStore()}
	create := datasetapp.NewCreateDatasetService(tx, datasets, resources)
	upload := datasetapp.NewUploadVersionService(tx, datasets, store)
	match := entityapp.NewMatchService(repoPath(t, "industry-packs"), tx, entities, datasets, upload, store)
	datasetHandler := datasethttp.NewHandler(create, upload, datasetapp.NewInvalidateVersionService(tx, datasets), datasets)
	entityHandler := entityhttp.NewHandler(match, entities)
	server := httptest.NewServer(httpserver.NewMux(datasetHandler.Register, entityHandler.Register))
	defer server.Close()
	client := server.Client()
	client.Timeout = 10 * time.Second
	owner, foreign, actor := uuid.New(), uuid.New(), uuid.New()
	workspaces := []uuid.UUID{owner, foreign}
	source := mustCreateResource(t, ctx, resourceapp.NewCreateService(tx, resources), owner, "BOUNDARY", "Synthetic boundary source", &actor, "boundary-setup")
	deletedSource := mustCreateResource(t, ctx, resourceapp.NewCreateService(tx, resources), owner, "BOUNDARY-DELETED", "Synthetic unavailable source", &actor, "boundary-setup")
	liveOK(t, func() error {
		_, err := pool.Exec(ctx, `UPDATE data_resource SET deleted_at=now() WHERE id=$1`, deletedSource.ID)
		return err
	}(), "soft-delete synthetic source fixture")
	raw := mustCreateDataset(t, ctx, create, owner, "BOUNDARY-RAW", "RAW", datasetdomain.DatasetTypeRaw, &source.ID, &actor, "boundary-setup")
	standardized := mustCreateDataset(t, ctx, create, owner, "BOUNDARY-STD", "STD", datasetdomain.DatasetTypeStandardized, nil, &actor, "boundary-setup")
	foreignOutput := mustCreateDataset(t, ctx, create, foreign, "BOUNDARY-FOREIGN", "Foreign STD", datasetdomain.DatasetTypeStandardized, nil, &actor, "boundary-setup")
	curated := mustCreateDataset(t, ctx, create, owner, "BOUNDARY-CURATED", "Curated", datasetdomain.DatasetTypeCurated, nil, &actor, "boundary-setup")
	product := mustCreateDataset(t, ctx, create, owner, "BOUNDARY-PRODUCT", "Product", datasetdomain.DatasetTypeProduct, nil, &actor, "boundary-setup")
	filename := "boundary-" + uuid.NewString() + ".csv"
	version := mustUploadCSV(t, ctx, upload, raw.ID, filename, []byte("source_company_id,company_name\nTEST-1,Synthetic boundary company\n"), nil, nil, &actor, "boundary-setup")

	post := func(t *testing.T, route string, body map[string]any) (int, []byte) {
		t.Helper()
		encoded, err := json.Marshal(body)
		liveOK(t, err, "encode HTTP boundary command")
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+route, bytes.NewReader(encoded))
		liveOK(t, err, "create HTTP request")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Actor-ID", actor.String())
		response, err := client.Do(req)
		liveOK(t, err, "send actual handler request")
		defer response.Body.Close()
		content, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		liveOK(t, err, "read HTTP response")
		return response.StatusCode, content
	}
	// Compare persisted facts, not just a success/error status. The filter isolates
	// this test from other packages sharing TEST_POSTGRES_DSN during go test ./....
	snapshot := func(t *testing.T) string {
		t.Helper()
		var value string
		liveOK(t, pool.QueryRow(ctx, `SELECT jsonb_build_object(
			'datasets', (SELECT jsonb_agg(to_jsonb(d) ORDER BY d.id) FROM dataset d WHERE workspace_id=ANY($1::uuid[])),
			'versions', (SELECT jsonb_agg(to_jsonb(v) ORDER BY v.id) FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id WHERE d.workspace_id=ANY($1::uuid[])),
			'types', (SELECT jsonb_agg(to_jsonb(e) ORDER BY e.id) FROM entity_type e WHERE workspace_id=ANY($1::uuid[])),
			'entities', (SELECT jsonb_agg(to_jsonb(e) ORDER BY e.id) FROM entity e WHERE workspace_id=ANY($1::uuid[])),
			'jobs', (SELECT jsonb_agg(to_jsonb(j) ORDER BY j.id) FROM entity_match_job j WHERE workspace_id=ANY($1::uuid[])),
			'audit', (SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM audit_event a WHERE workspace_id=ANY($1::uuid[])),
			'evidence', (SELECT jsonb_agg(to_jsonb(e) ORDER BY e.id) FROM evidence e WHERE workspace_id=ANY($1::uuid[])),
			'outbox', (SELECT jsonb_agg(to_jsonb(o) ORDER BY o.id) FROM outbox_event o WHERE payload->>'workspaceId'=ANY($2::text[]))
		)::text`, workspaces, []string{owner.String(), foreign.String()}).Scan(&value), "snapshot isolated facts")
		return value
	}
	jobBody := func(workspace, output uuid.UUID) map[string]any {
		return map[string]any{"workspaceId": workspace, "inputDatasetVersionId": version.ID, "outputDatasetId": output, "sourceType": "CSV", "sourceRef": filename, "sourceRole": "ANCHOR", "policyRef": companyPolicyRef}
	}
	cases := []struct {
		name, route, code string
		body              map[string]any
	}{
		{"missing resource", "/api/v1/datasets", "SOURCE_RESOURCE_NOT_FOUND", map[string]any{"workspaceId": owner, "code": "BOUNDARY-MISSING", "name": "Rejected", "datasetType": "RAW", "sourceResourceId": uuid.New()}},
		{"soft-deleted resource", "/api/v1/datasets", "SOURCE_RESOURCE_NOT_FOUND", map[string]any{"workspaceId": owner, "code": "BOUNDARY-DELETED-SOURCE", "name": "Rejected", "datasetType": "RAW", "sourceResourceId": deletedSource.ID}},
		{"foreign resource", "/api/v1/datasets", "SOURCE_RESOURCE_WORKSPACE_MISMATCH", map[string]any{"workspaceId": foreign, "code": "BOUNDARY-BAD", "name": "Rejected", "datasetType": "RAW", "sourceResourceId": source.ID}},
		{"foreign input", "/api/v1/entity-match-jobs", "DATASET_WORKSPACE_MISMATCH", jobBody(foreign, foreignOutput.ID)},
		{"foreign output", "/api/v1/entity-match-jobs", "DATASET_WORKSPACE_MISMATCH", jobBody(owner, foreignOutput.ID)},
		{"RAW output", "/api/v1/entity-match-jobs", "OUTPUT_DATASET_TYPE_INVALID", jobBody(owner, raw.ID)},
		{"CURATED output", "/api/v1/entity-match-jobs", "OUTPUT_DATASET_TYPE_INVALID", jobBody(owner, curated.ID)},
		{"PRODUCT output", "/api/v1/entity-match-jobs", "OUTPUT_DATASET_TYPE_INVALID", jobBody(owner, product.ID)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before, reads, writes := snapshot(t), store.reads.Load(), store.writes.Load()
			status, response := post(t, tc.route, tc.body)
			var envelope httpserver.ErrorEnvelope
			liveOK(t, json.Unmarshal(response, &envelope), "decode error envelope")
			if status != http.StatusBadRequest || envelope.Code != tc.code {
				t.Fatalf("status=%d response=%s, want 400/%s", status, response, tc.code)
			}
			if strings.Contains(string(response), owner.String()) || strings.Contains(string(response), foreign.String()) {
				t.Fatalf("error discloses a workspace identity: %s", response)
			}
			if after := snapshot(t); after != before {
				t.Fatal("rejected HTTP command changed persisted facts")
			}
			if store.reads.Load() != reads || store.writes.Load() != writes {
				t.Fatal("rejected command accessed object storage")
			}
		})
	}
	t.Run("same workspace resource accepted", func(t *testing.T) {
		status, response := post(t, "/api/v1/datasets", map[string]any{"workspaceId": owner, "code": "BOUNDARY-GOOD", "name": "Accepted", "datasetType": "RAW", "sourceResourceId": source.ID})
		if status != http.StatusCreated {
			t.Fatalf("positive dataset control: %d %s", status, response)
		}
	})
	t.Run("same workspace STANDARDIZED job accepted", func(t *testing.T) {
		status, response := post(t, "/api/v1/entity-match-jobs", jobBody(owner, standardized.ID))
		if status != http.StatusCreated {
			t.Fatalf("positive job control: %d %s", status, response)
		}
		assertLiveCount(t, ctx, pool, 1, `SELECT count(*) FROM entity_match_job WHERE workspace_id=$1 AND status='SUCCEEDED' AND output_dataset_version_id IS NOT NULL`, owner)
		if store.reads.Load() == 0 {
			t.Fatal("positive control did not read the actual input object")
		}
	})
}
