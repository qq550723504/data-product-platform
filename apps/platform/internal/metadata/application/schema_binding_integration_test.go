package application

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
	infra "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
)

func TestSourceSchemaRejectsReplacedPrimaryBinding(t *testing.T) {
	if os.Getenv("TEST_POSTGRES_DSN") == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, os.Getenv("TEST_POSTGRES_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	resource, workspace := uuid.New(), uuid.New()
	_, err = tx.Exec(ctx, `INSERT INTO data_resource(id,workspace_id,code,name,resource_type) VALUES($1,$2,$3,'schema binding test','TABLE_LIKE')`, resource, workspace, resource.String())
	if err != nil {
		t.Fatal(err)
	}
	provider := d.Provider("TEST_METADATA")
	b, err := infra.NewPostgresRepository(pool).UpsertBinding(ctx, tx, d.ResourceBinding{ResourceID: resource, Provider: provider, EntityType: "TABLE", ExternalID: uuid.NewString(), ExternalFQN: "test.energy", IsPrimary: true})
	if err != nil {
		t.Fatal(err)
	}
	s := &SchemaService{Service: &Service{provider: provider}}
	req := d.SchemaRequest{BindingID: b.ID, ExternalID: b.ExternalID, Plan: d.SchemaPlan{ResourceID: resource, ExternalFQN: b.ExternalFQN}}
	if err := s.currentBinding(ctx, tx, req); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE resource_binding SET is_primary=false WHERE id=$1`, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.currentBinding(ctx, tx, req); !errors.Is(err, ErrSchemaDenied) {
		t.Fatalf("replaced primary: %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE resource_binding SET is_primary=true,external_id=$2 WHERE id=$1`, b.ID, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if err := s.currentBinding(ctx, tx, req); !errors.Is(err, ErrSchemaDenied) {
		t.Fatalf("changed provider identity: %v", err)
	}
}
