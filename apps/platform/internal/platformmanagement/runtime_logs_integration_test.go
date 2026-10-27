package platformmanagement

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal/bindings"
	"os"
	"testing"
	"time"
)

func TestRuntimeLogPostgresDualAuthorityAndInFlightRevocation(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("isolated migrated PostgreSQL required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	h, resolver, _, source, path := logTestSetup(t)
	resolver.p.IdentityIssuer = "https://runtime-" + uuid.NewString() + ".invalid"
	actor, ws, grant := resolver.p.ActorID, resolver.p.WorkspaceID, uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := db.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`INSERT INTO platform_actor_state(actor_id)VALUES($1)`, actor)
	exec(`INSERT INTO platform_subject_binding(id,issuer,subject,actor_id,valid_from,valid_until)VALUES($1,$2,$3,$4,now()-interval '1 hour',now()+interval '1 hour')`, uuid.New(), resolver.p.IdentityIssuer, resolver.p.IdentitySubject, actor)
	exec(`INSERT INTO platform_workspace_grant(id,actor_id,workspace_id,capabilities,valid_from,valid_until)VALUES($1,$2,$3,$4,now()-interval '1 hour',now()+interval '1 hour')`, grant, actor, ws, []string{principal.CapabilityManagementRead})
	h.Gate = PostgresGate{Pool: db, Store: bindings.NewStore(db)}
	logRequest(t, h, path, 403)
	if source.calls != 0 {
		t.Fatal("source read before independent log permission")
	}
	exec(`UPDATE platform_workspace_grant SET capabilities=$2,revision=revision+1 WHERE id=$1`, grant, []string{principal.CapabilityManagementRead, principal.CapabilityRuntimeLogRead})
	resolver.p.IdentityRevision = "1:1:2"
	logRequest(t, h, path, 200)
	source.after = func() {
		exec(`UPDATE platform_workspace_grant SET capabilities=$2,revision=revision+1 WHERE id=$1`, grant, []string{principal.CapabilityManagementRead})
	}
	wire := logRequest(t, h, path, 401)
	if wire.Data.Items != nil {
		t.Fatal("in-flight revoked read escaped")
	}
	source.after = nil
	resolver.p.IdentityRevision = "1:1:3"
	logRequest(t, h, path, 403)
	exec(`UPDATE platform_workspace_grant SET capabilities=$2,revision=revision+1 WHERE id=$1`, grant, []string{principal.CapabilityRuntimeLogRead})
	resolver.p.IdentityRevision = "1:1:4"
	logRequest(t, h, path, 403)
	exec(`UPDATE platform_workspace_grant SET capabilities=$2,valid_until=now()-interval '1 second',revision=revision+1 WHERE id=$1`, grant, []string{principal.CapabilityManagementRead, principal.CapabilityRuntimeLogRead})
	resolver.p.IdentityRevision = "1:1:5"
	logRequest(t, h, path, 403)
	for i, sourceErr := range []error{nil, LogUnavailable} {
		exec(`UPDATE platform_workspace_grant SET valid_until=now()+interval '1 hour',revision=revision+1 WHERE id=$1`, grant)
		resolver.p.IdentityRevision = fmt.Sprintf("1:1:%d", 6+i*2)
		source.err = sourceErr
		source.after = func() {
			exec(`UPDATE platform_workspace_grant SET valid_until=now()-interval '1 second',revision=revision+1 WHERE id=$1`, grant)
		}
		wire := logRequest(t, h, path, 403)
		if wire.Data.Items != nil {
			t.Fatal("grant expiring during source I/O exposed observations")
		}
		source.after = nil
	}
}
