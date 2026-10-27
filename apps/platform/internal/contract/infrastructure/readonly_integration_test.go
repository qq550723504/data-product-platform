package infrastructure

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	q "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/readmodel"
	"os"
	"testing"
)

func TestAcceptanceSourcePostgresScope(t *testing.T) {
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
	ws, other, c1, c2, v1, v2 := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO data_contract(id,workspace_id,code,name) VALUES($1,$2,$3,'SYNTHETIC acceptance source integration'),($4,$5,$6,'SYNTHETIC foreign source integration')`, c1, ws, c1.String(), c2, other, c2.String())
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO contract_version(id,contract_id,major_version,minor_version,patch_version,status,document,source_sha256,published_at) VALUES($1,$2,1,0,0,'PUBLISHED','{"spec":{}}',repeat('a',64),now()),($3,$4,1,0,0,'PUBLISHED','{"spec":{}}',repeat('b',64),now())`, v1, c1, v2, c2)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewPostgresRepository(pool)
	result, err := repo.ReadAcceptanceSources(ctx, q.Query{WorkspaceID: ws, Limit: 25})
	if err != nil || result.Total != 1 || len(result.Versions) != 1 || result.Versions[0].Version.ID != v1 {
		t.Fatalf("scoped list %+v %v", result, err)
	}
	_, err = repo.ReadAcceptanceSources(ctx, q.Query{WorkspaceID: ws, VersionID: &v2, Limit: 25})
	if !errors.Is(err, q.ErrNotFound) {
		t.Fatalf("foreign version must be missing: %v", err)
	}
	result, err = repo.ReadAcceptanceSources(ctx, q.Query{WorkspaceID: ws, Limit: 25, Offset: 25})
	if err != nil || result.Total != 1 || len(result.Versions) != 0 {
		t.Fatalf("empty page changed total: %+v %v", result, err)
	}
}
