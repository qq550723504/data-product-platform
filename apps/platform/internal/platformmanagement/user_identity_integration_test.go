package platformmanagement

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal/bindings"
	"os"
	"testing"
)

func TestExplicitUserIdentityLinksRequireCurrentWorkspaceBinding(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("isolated migrated TEST_POSTGRES_DSN required")
	}
	ctx := context.Background()
	db, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	ws, actor, binding, grant := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	issuer := "https://synthetic-" + uuid.NewString() + ".invalid"
	subject := "pairwise-subject"
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := db.Exec(ctx, q, args...); e != nil {
			t.Fatal("isolated identity fixture failed", e)
		}
	}
	exec(`INSERT INTO platform_actor_state(actor_id,enabled)VALUES($1,true)`, actor)
	exec(`INSERT INTO platform_subject_binding(id,issuer,subject,actor_id,valid_from,valid_until)VALUES($1,$2,$3,$4,now()-interval '1 hour',now()+interval '1 hour')`, binding, issuer, subject, actor)
	exec(`INSERT INTO platform_workspace_grant(id,actor_id,workspace_id,capabilities,valid_from,valid_until)VALUES($1,$2,$3,$4,now()-interval '1 hour',now()+interval '1 hour')`, grant, actor, ws, []string{"BUSINESS_ACCEPTANCE_READ"})
	r := BoundUserIdentities{Store: bindings.NewStore(db), Workspace: ws}
	check := func(user, state string) {
		t.Helper()
		got, e := r.ReadUserIdentity(ctx, ws, user)
		if e != nil || got.State != state || !validUserLink(got, ws) {
			t.Fatalf("unexpected link state %s err %v", got.State, e)
		}
		if state == "LINKED" && (got.ActorID == nil || *got.ActorID != actor.String()) {
			t.Fatal("native id substituted for canonical actor")
		}
	}
	check("native-person", "UNCONFIGURED")
	r.References = map[string]UserSubjectReference{"native-person": {Issuer: issuer, Subject: subject, BasisRef: "synthetic-explicit-link"}}
	check("native-person", "LINKED")
	check(actor.String(), "UNLINKED")
	check(subject, "UNLINKED")
	if _, e := r.ReadUserIdentity(ctx, uuid.New(), "native-person"); e == nil {
		t.Fatal("foreign workspace accepted")
	}
	for _, table := range []string{"platform_actor_state", "platform_subject_binding", "platform_workspace_grant"} {
		key, id := "id", grant
		if table == "platform_actor_state" {
			key, id = "actor_id", actor
		}
		if table == "platform_subject_binding" {
			id = binding
		}
		exec("UPDATE "+table+" SET enabled=false,revision=revision+1 WHERE "+key+"=$1", id)
		check("native-person", "UNLINKED")
		exec("UPDATE "+table+" SET enabled=true,revision=revision+1 WHERE "+key+"=$1", id)
		check("native-person", "LINKED")
	}
	exec(`UPDATE platform_workspace_grant SET valid_until=now()-interval '1 second',revision=revision+1 WHERE id=$1`, grant)
	check("native-person", "UNLINKED")
}
