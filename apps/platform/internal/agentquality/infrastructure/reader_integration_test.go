package infrastructure_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/agentquality"
	agentinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/agentquality/infrastructure"
	agenthttp "github.com/qq550723504/data-product-platform/apps/platform/internal/agentquality/transport/http"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/readmodel"
)

func TestPostgresAuthorizedContextIsReadOnlyAndWhitelisted(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ws, foreign, ds, other, v, old := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, d := range []struct{ id, ws uuid.UUID }{{ds, ws}, {other, foreign}} {
		_, err = pool.Exec(ctx, `INSERT INTO dataset(id,workspace_id,code,name,dataset_type) VALUES($1,$2,$3,'Q1 source','RAW')`, d.id, d.ws, d.id.String())
		if err != nil {
			t.Fatal(err)
		}
	}
	for i, id := range []uuid.UUID{old, v} {
		_, err = pool.Exec(ctx, `INSERT INTO dataset_version(id,dataset_id,version_no,status,storage_uri,checksum_value,row_count,metadata) VALUES($1,$2,$3,'READY','private://storage-secret','fixture-checksum',10,'{"secret":"metadata-secret"}')`, id, ds, i+1)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = pool.Exec(ctx, `INSERT INTO dataset_version(id,dataset_id,version_no,status) VALUES($1,$2,1,'CREATED')`, uuid.New(), other)
	if err != nil {
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	readonly, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer readonly.Close()
	s := &agentquality.Service{Reader: agentinfra.Reader{Catalog: readmodel.NewRepository(readonly), DatasetRepo: datasetinfra.NewPostgresRepository(readonly)}}
	for _, q := range []agentquality.Query{
		{WorkspaceID: ws, Limit: 1}, {WorkspaceID: ws, DatasetID: ds, Limit: 1, Offset: 1}, {WorkspaceID: ws, VersionID: old, Limit: 25},
	} {
		c, e := s.Read(ctx, q)
		if e != nil {
			t.Fatal(e)
		}
		if c.Datasets != nil && (c.Datasets.Page.Total != 1 || len(c.Datasets.Items) != 1 || c.Datasets.Items[0].ID != ds) {
			t.Fatalf("catalog isolation: %+v", c)
		}
		if c.Versions != nil && (c.Versions.Page.Total != 2 || len(c.Versions.Items) != 1 || c.Versions.Items[0].ID != old) {
			t.Fatalf("pagination: %+v", c)
		}
		if q.VersionID != uuid.Nil && (c.Version == nil || c.Version.ID != old || c.Version.VersionNo != 1) {
			t.Fatalf("historical selection changed: %+v", c)
		}
		b, _ := json.Marshal(c)
		for _, secret := range []string{"storage-secret", "metadata-secret", "storageUri", "metadata"} {
			if strings.Contains(string(b), secret) {
				t.Fatalf("unsafe context: %s", b)
			}
		}
	}
	if _, e := s.Read(ctx, agentquality.Query{WorkspaceID: foreign, VersionID: v, Limit: 25}); !errors.Is(e, agentquality.ErrNotFound) {
		t.Fatalf("cross workspace: %v", e)
	}
	if _, e := s.Read(ctx, agentquality.Query{WorkspaceID: ws, DatasetID: other, VersionID: v, Limit: 25}); !errors.Is(e, agentquality.ErrNotFound) {
		t.Fatalf("mismatched source: %v", e)
	}
	// Exercise the actual HTTP boundary with these same read-only repositories.
	resolver, err := principal.NewStaticResolver(true, "q1-test-only", "q1-integration", uuid.NewString(), []string{ws.String()}, []string{principal.CapabilityHumanDecision})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	(&agenthttp.Handler{Service: s, Principal: resolver}).Register(mux)
	for _, tc := range []struct {
		workspace, dataset, version uuid.UUID
		status                      int
	}{
		{ws, ds, old, 200}, {ws, other, old, 404}, {ws, other, uuid.Nil, 404}, {foreign, ds, old, 403},
	} {
		path := "/api/v1/workspaces/" + tc.workspace.String() + "/agent-quality-context?datasetId=" + tc.dataset.String()
		if tc.version != uuid.Nil {
			path += "&versionId=" + tc.version.String()
		}
		request := httptest.NewRequest("GET", path, nil)
		request.Header.Set("Authorization", "Bearer q1-test-only")
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != tc.status {
			t.Fatalf("HTTP %d want %d: %s", response.Code, tc.status, response.Body)
		}
		if tc.status == 200 {
			var result agentquality.Context
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Version == nil || result.Version.ID != old {
				t.Fatal("HTTP replaced exact historical version")
			}
		}
	}
	// Every query above ran on PostgreSQL connections which reject all writes.
	var audits, outbox int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM audit_event WHERE workspace_id=$1),(SELECT count(*) FROM outbox_event WHERE aggregate_id IN ($2,$3,$4))`, ws, ds, v, old).Scan(&audits, &outbox); err != nil {
		t.Fatal(err)
	}
	if audits != 0 || outbox != 0 {
		t.Fatalf("GET created facts: audits=%d outbox=%d", audits, outbox)
	}
}
