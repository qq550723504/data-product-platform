package infrastructure_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
	rightsinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
)

func TestSaveAuthorizationStateRejectsStaleStatus(t *testing.T) {
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

	authorizationID := uuid.New()
	workspaceID := uuid.New()
	now := time.Now().UTC()
	if _, err := pool.Exec(ctx, `
		INSERT INTO data_authorization (
			id, workspace_id, code, grantor_ref, grantee_ref, purpose, status,
			metadata, created_at, updated_at
		) VALUES ($1,$2,$3,'GRANTOR','GRANTEE','TEST_PURPOSE','ACTIVE','{}'::jsonb,$4,$4)
	`, authorizationID, workspaceID, "AUTH-CAS-"+uuid.NewString(), now); err != nil {
		t.Fatalf("insert active authorization: %v", err)
	}

	repo := rightsinfra.NewPostgresRepository(pool)
	suspendWrite, err := repo.GetAuthorization(ctx, authorizationID)
	if err != nil {
		t.Fatalf("read suspension snapshot: %v", err)
	}
	revokeWrite, err := repo.GetAuthorization(ctx, authorizationID)
	if err != nil {
		t.Fatalf("read revoke snapshot: %v", err)
	}
	if suspendWrite.Status != domain.StatusActive || revokeWrite.Status != domain.StatusActive {
		t.Fatalf("initial snapshots = %s/%s, want ACTIVE/ACTIVE", suspendWrite.Status, revokeWrite.Status)
	}
	if err := suspendWrite.Suspend(nil); err != nil {
		t.Fatalf("prepare suspension: %v", err)
	}
	if err := revokeWrite.Revoke(nil); err != nil {
		t.Fatalf("prepare revocation: %v", err)
	}

	txWinner, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin winner transaction: %v", err)
	}
	defer func() { _ = txWinner.Rollback(context.Background()) }()

	if err := repo.SaveAuthorizationState(ctx, txWinner, suspendWrite, domain.StatusActive); err != nil {
		t.Fatalf("save winner suspension: %v", err)
	}

	txLoser, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin loser transaction: %v", err)
	}
	defer func() { _ = txLoser.Rollback(context.Background()) }()

	started := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		close(started)
		result <- repo.SaveAuthorizationState(ctx, txLoser, revokeWrite, domain.StatusActive)
	}()
	<-started

	// Commit the winner while the second transaction is contending on the same
	// workspace fence / authorization row. The loser still carries the stale
	// ACTIVE precondition it observed before the winner committed.
	if err := txWinner.Commit(ctx); err != nil {
		t.Fatalf("commit winner suspension: %v", err)
	}

	if err := <-result; !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("stale revoke error = %v, want ErrInvalidTransition", err)
	}
	if err := txLoser.Rollback(ctx); err != nil {
		t.Fatalf("rollback stale loser: %v", err)
	}

	stored, err := repo.GetAuthorization(ctx, authorizationID)
	if err != nil {
		t.Fatalf("read final authorization: %v", err)
	}
	if stored.Status != domain.StatusSuspended {
		t.Fatalf("final authorization status = %s, want SUSPENDED", stored.Status)
	}
}
