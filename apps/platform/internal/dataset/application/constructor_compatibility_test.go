package application_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	resourceapp "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/application"
	resourcedomain "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/domain"
	resourceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
)

func TestDatasetConstructorCompatibilityStillChecksOwnership(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx := transaction.NewManager(pool)
	repo := infrastructure.NewPostgresRepository(pool)
	resources := resourceinfra.NewPostgresRepository()
	owner, foreign := uuid.New(), uuid.New()
	source, err := resourceapp.NewCreateService(tx, resources).Handle(ctx, resourceapp.CreateDataResourceCommand{
		WorkspaceID: owner, Code: "CTOR-" + uuid.NewString(), Name: "Synthetic source", ResourceType: resourcedomain.ResourceTypeFileCollection,
	})
	if err != nil {
		t.Fatal(err)
	}
	constructors := map[string]*application.CreateDatasetService{
		"legacy two arguments": application.NewCreateDatasetService(tx, repo),
		"explicit repository":  application.NewCreateDatasetService(tx, repo, resources),
		"nil repository":       application.NewCreateDatasetService(tx, repo, nil),
	}
	for name, service := range constructors {
		t.Run(name, func(t *testing.T) {
			command := application.CreateDatasetCommand{
				WorkspaceID: foreign, Code: "CTOR-" + uuid.NewString(), Name: "Rejected", DatasetType: domain.DatasetTypeRaw, SourceResourceID: &source.ID,
			}
			if _, err := service.Handle(ctx, command); !errors.Is(err, domain.ErrSourceResourceWorkspace) {
				t.Fatalf("constructor bypassed workspace check: %v", err)
			}
			var count int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM dataset WHERE workspace_id=$1 AND code=$2", foreign, command.Code).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatal("rejected creation wrote a dataset")
			}
			command.WorkspaceID = owner
			if _, err := service.Handle(ctx, command); err != nil {
				t.Fatalf("constructor blocks legitimate source: %v", err)
			}
		})
	}
}

func TestDatasetConstructorRejectsAmbiguousDependencies(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("multiple resource repositories were silently accepted")
		}
	}()
	application.NewCreateDatasetService(nil, nil, nil, nil)
}
