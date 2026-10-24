package bindings

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"os"
	"testing"
	"time"
)

func authorityFixture(t *testing.T) (*pgxpool.Pool, principal.RequestPrincipal) {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	ctx := context.Background()
	db, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(db.Close)
	actor, workspace := uuid.New(), uuid.New()
	sub := uuid.New().String()
	issuer := "https://fixture.invalid/" + uuid.NewString()
	if _, e = db.Exec(ctx, `INSERT INTO platform_actor_state(actor_id)VALUES($1)`, actor); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(ctx, `INSERT INTO platform_subject_binding(id,issuer,subject,actor_id,valid_from,valid_until)VALUES($1,$2,$3,$4,now()-interval '1 hour',now()+interval '1 hour')`, uuid.New(), issuer, sub, actor); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(ctx, `INSERT INTO platform_workspace_grant(id,actor_id,workspace_id,capabilities,valid_from,valid_until)VALUES($1,$2,$3,$4,now()-interval '1 hour',now()+interval '1 hour')`, uuid.New(), actor, workspace, []string{principal.CapabilityAcceptanceRead, principal.CapabilityAcceptanceDraft}); e != nil {
		t.Fatal(e)
	}
	return db, principal.RequestPrincipal{ActorID: actor, Subject: actor.String(), WorkspaceID: workspace, IdentityIssuer: issuer, IdentitySubject: sub, IdentityRevision: "1:1:1", IdentityExpiresAt: time.Now().Add(2 * time.Minute)}
}
func waitForAdvisoryWait(t *testing.T, db *pgxpool.Pool, pid int32) {
	t.Helper()
	end := time.Now().Add(3 * time.Second)
	for time.Now().Before(end) {
		var waiting bool
		if e := db.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE pid=$1 AND locktype='advisory' AND NOT granted)`, pid).Scan(&waiting); e != nil {
			t.Fatal(e)
		}
		if waiting {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no real advisory lock wait observed")
}
func TestAuthorityRevocationBeforeAdmissionUsesFreshTransactionRead(t *testing.T) {
	db, p := authorityFixture(t)
	ctx := context.Background()
	store := NewStore(db)
	writer, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer writer.Rollback(ctx)
	if _, e = writer.Exec(ctx, `UPDATE platform_actor_state SET enabled=false,revision=revision+1 WHERE actor_id=$1`, p.ActorID); e != nil {
		t.Fatal(e)
	}
	reader, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer reader.Rollback(ctx)
	var pid int32
	if e = reader.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { _, e := store.ValidateInTx(ctx, reader, p, principal.CapabilityAcceptanceRead); done <- e }()
	waitForAdvisoryWait(t, db, pid)
	if e = writer.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case e = <-done:
		if !errors.Is(e, principal.ErrWorkspaceDenied) {
			t.Fatalf("revoked actor admitted: %v", e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("authority did not resume")
	}
}
func TestAuthorityAdmittedTransactionFencesConcurrentRevocation(t *testing.T) {
	db, p := authorityFixture(t)
	ctx := context.Background()
	reader, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer reader.Rollback(ctx)
	a, e := NewStore(db).ValidateInTx(ctx, reader, p, principal.CapabilityAcceptanceDraft)
	if e != nil || a.Principal.ActorID != p.ActorID || a.SubjectBindingID == uuid.Nil || a.WorkspaceGrantID == uuid.Nil {
		t.Fatalf("current authority unavailable: %+v %v", a, e)
	}
	writer, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer writer.Rollback(ctx)
	var pid int32
	_ = writer.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid)
	done := make(chan error, 1)
	go func() {
		_, e := writer.Exec(ctx, `UPDATE platform_actor_state SET enabled=false,revision=revision+1 WHERE actor_id=$1`, p.ActorID)
		if e == nil {
			e = writer.Commit(ctx)
		}
		done <- e
	}()
	waitForAdvisoryWait(t, db, pid)
	// A test-only temp marker demonstrates atomic work; no business table changes.
	if _, e = reader.Exec(ctx, `CREATE TEMP TABLE authority_probe(n int) ON COMMIT DROP; INSERT INTO authority_probe VALUES(1)`); e != nil {
		t.Fatal(e)
	}
	if e = reader.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case e = <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("revocation did not resume")
	}
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = NewStore(db).ValidateInTx(ctx, tx, p, principal.CapabilityAcceptanceRead); !errors.Is(e, principal.ErrWorkspaceDenied) {
		t.Fatalf("post-revocation admission: %v", e)
	}
}
func TestAuthorityExpiryAfterFenceWaitAndRepeatableReadFailClosed(t *testing.T) {
	db, p := authorityFixture(t)
	ctx := context.Background()
	hold, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer hold.Rollback(ctx)
	_, e = hold.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, mutationFence)
	if e != nil {
		t.Fatal(e)
	}
	reader, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer reader.Rollback(ctx)
	var pid int32
	_ = reader.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid)
	p.IdentityExpiresAt = time.Now().Add(100 * time.Millisecond)
	done := make(chan error, 1)
	go func() {
		_, e := NewStore(db).ValidateInTx(ctx, reader, p, principal.CapabilityAcceptanceRead)
		done <- e
	}()
	waitForAdvisoryWait(t, db, pid)
	deadline := time.Now().Add(3 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("database clock did not pass authority expiry")
		}
		var expired bool
		if e = db.QueryRow(ctx, `SELECT clock_timestamp()>$1`, p.IdentityExpiresAt).Scan(&expired); e != nil {
			t.Fatal(e)
		}
		if expired {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = hold.Rollback(ctx)
	select {
	case e = <-done:
		if !errors.Is(e, principal.ErrWorkspaceDenied) {
			t.Fatalf("expired authority admitted: %v", e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("expiry wait stuck")
	}
	rr, e := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if e != nil {
		t.Fatal(e)
	}
	defer rr.Rollback(ctx)
	if _, e = NewStore(db).ValidateInTx(ctx, rr, p, principal.CapabilityAcceptanceRead); !errors.Is(e, principal.ErrAuthorityUnavailable) {
		t.Fatalf("stale isolation accepted: %v", e)
	}
}
