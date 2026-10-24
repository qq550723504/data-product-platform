package infrastructure

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	q "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/readmodel"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal/bindings"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal/oidc"
	"os"
	"testing"
	"time"
)

func TestResponsibilityTransactionFencesDispositionAndRejectsForeignScope(t *testing.T) {
	dsn := os.Getenv("ACCEPTANCE_IDENTITY_TEST_DSN")
	if dsn == "" {
		t.Skip("dedicated prepared 1A fixture not configured")
	}
	ctx := context.Background()
	db, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	ws := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	version := uuid.MustParse("74000000-0000-4000-8000-000000001001")
	original := uuid.MustParse("74000000-0000-4000-8000-000000001002")
	target := uuid.New()
	var p principal.RequestPrincipal
	p.WorkspaceID = ws
	p.IdentityExpiresAt = time.Now().Add(time.Minute)
	e = db.QueryRow(ctx, `SELECT a.actor_id,b.issuer,b.subject FROM platform_actor_state a JOIN platform_subject_binding b ON b.actor_id=a.actor_id WHERE a.actor_id='73000000-0000-4000-8000-000000000001'`).Scan(&p.ActorID, &p.IdentityIssuer, &p.IdentitySubject)
	if e != nil {
		t.Fatal(e)
	}
	p.Subject = p.ActorID.String()
	// Resolve the current revision; browser revocation tests retain their
	// append-only revision progression and must not be reset for this fixture.
	current, e := bindings.NewStore(db).Lookup(ctx, oidc.Identity{Issuer: p.IdentityIssuer, Subject: p.IdentitySubject}, ws)
	if e != nil || !current.Active {
		t.Fatalf("fixture authority unavailable: %v", e)
	}
	p.IdentityRevision = current.AuthorityRevision
	setup, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer setup.Rollback(ctx)
	_, e = setup.Exec(ctx, `INSERT INTO delivery_operation(id,workspace_id,dataset_version_id,idempotency_key,provider_name,provider_request_key,status,current_gate_decision,principal_ref,effective_consumer_ref,purpose,action,scope_ref,delivery_channel,delivery_mode,requested_expires_at,terminal_reason) SELECT $1,workspace_id,dataset_version_id,$2,provider_name,$2,status,current_gate_decision,principal_ref,effective_consumer_ref,purpose,action,scope_ref,delivery_channel,delivery_mode,now()+interval '1 hour','SYNTHETIC_CONCURRENCY_FIXTURE' FROM delivery_operation WHERE id=$3`, target, target.String(), original)
	if e != nil {
		t.Fatal(e)
	}
	basis, e := evidence.Append(ctx, setup, evidence.Record{WorkspaceID: ws, EvidenceType: "SYNTHETIC_BUSINESS_BINDING", Title: "1A transaction-role fence fixture", SourceType: "SYNTHETIC_ACCEPTANCE", SourceID: &target, CreatedBy: &p.ActorID})
	if e != nil {
		t.Fatal(e)
	}
	roleID := uuid.New()
	_, e = setup.Exec(ctx, `INSERT INTO acceptance_responsibility_binding(id,workspace_id,delivery_operation_id,dataset_version_id,role,representation_id,basis_evidence_id,valid_from,valid_to,origin,recorded_by)SELECT CASE WHEN role='PROVIDER' THEN $4::uuid ELSE gen_random_uuid() END,workspace_id,$1,dataset_version_id,role,representation_id,$3,valid_from,valid_to,origin,recorded_by FROM acceptance_responsibility_binding WHERE delivery_operation_id=$2`, target, original, basis.ID, roleID)
	if e != nil {
		t.Fatal(e)
	}
	if e = setup.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	reader := AcceptanceIdentityReader{Repository: NewPostgresRepository(db), Authority: bindings.NewStore(db)}
	query := q.Query{WorkspaceID: ws, DeliveryID: &target, DatasetVersionID: &version, Limit: 25}
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	a, role, e := reader.CheckResponsibilityInTx(ctx, tx, p, query, "CreateBusinessAcceptance")
	if e != nil || role == nil || role.Binding.ID != roleID.String() || a.Principal.ActorID != p.ActorID {
		t.Fatalf("current role unavailable: %+v %v", role, e)
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
		_, e := writer.Exec(ctx, `INSERT INTO acceptance_binding_disposition(id,workspace_id,responsibility_id,disposition,effective_at,reason,basis_evidence_id,recorded_by)VALUES($1,$2,$3,'REVOKED',clock_timestamp(),'SYNTHETIC role fence test',$4,$5)`, uuid.New(), ws, roleID, basis.ID, p.ActorID)
		if e == nil {
			e = writer.Commit(ctx)
		}
		done <- e
	}()
	waited := false
	end := time.Now().Add(3 * time.Second)
	for time.Now().Before(end) {
		if e = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE pid=$1 AND locktype='advisory' AND NOT granted)`, pid).Scan(&waited); e != nil {
			t.Fatal(e)
		}
		if waited {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waited {
		t.Fatal("no actual disposition fence wait")
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case e = <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("disposition did not resume")
	}
	next, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer next.Rollback(ctx)
	if _, _, e = reader.CheckResponsibilityInTx(ctx, next, p, query, "CreateBusinessAcceptance"); !errors.Is(e, principal.ErrCapabilityDenied) {
		t.Fatalf("disposed role admitted: %v", e)
	}
	query.WorkspaceID = uuid.New()
	if _, _, e = reader.CheckResponsibilityInTx(ctx, next, p, query, "CreateBusinessAcceptance"); !errors.Is(e, principal.ErrWorkspaceDenied) {
		t.Fatalf("foreign target admitted: %v", e)
	}
}
