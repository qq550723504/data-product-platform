package platformmanagement

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal/bindings"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestWorkspaceOverviewRealPGGrantExpiryAndRevisionWithoutNativeDirectory(t *testing.T) {
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
	actor, ws, grant := uuid.New(), uuid.New(), uuid.New()
	issuer := "https://workspace-" + uuid.NewString() + ".invalid"
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := db.Exec(ctx, q, args...); e != nil {
			t.Fatal("synthetic workspace binding failed", e)
		}
	}
	exec(`INSERT INTO platform_actor_state(actor_id,enabled)VALUES($1,true)`, actor)
	exec(`INSERT INTO platform_subject_binding(id,issuer,subject,actor_id,valid_from,valid_until)VALUES($1,$2,'subject',$3,now()-interval '1 hour',now()+interval '1 hour')`, uuid.New(), issuer, actor)
	exec(`INSERT INTO platform_workspace_grant(id,actor_id,workspace_id,capabilities,valid_from,valid_until)VALUES($1,$2,$3,$4,now()-interval '1 hour',now()+interval '30 seconds')`, grant, actor, ws, []string{principal.CapabilityManagementRead})
	r := &testResolver{p: principal.RequestPrincipal{ActorID: actor, Subject: actor.String(), WorkspaceID: ws, IdentityIssuer: issuer, IdentitySubject: "subject", IdentityRevision: "1:1:1", IdentityExpiresAt: time.Now().Add(2 * time.Minute)}}
	mux := http.NewServeMux()
	Handler{Principal: r, Gate: PostgresGate{Pool: db, Store: bindings.NewStore(db)}, IdentitySource: "SYNTHETIC_FIXTURE"}.Register(mux)
	read := func(want int) WorkspaceOverview {
		t.Helper()
		req := httptest.NewRequest("GET", "/api/v1/workspaces/"+ws.String()+"/management/workspace", nil)
		req.Header.Set("Authorization", "Bearer synthetic")
		out := httptest.NewRecorder()
		mux.ServeHTTP(out, req)
		var wire struct{ Data WorkspaceOverview }
		if out.Code != want {
			t.Fatalf("status=%d want=%d", out.Code, want)
		}
		if want == 200 {
			if json.Unmarshal(out.Body.Bytes(), &wire) != nil {
				t.Fatal("invalid overview")
			}
		} else {
			var denied map[string]any
			_ = json.Unmarshal(out.Body.Bytes(), &denied)
			if denied["data"] != nil {
				t.Fatal("disclosed revoked workspace")
			}
		}
		return wire.Data
	}
	got := read(200)
	if got.Directory.State != "UNCONFIGURED" || !got.ConfirmationExpiresAt.Before(time.Now().Add(time.Minute)) {
		t.Fatal("directory absence denied current grant or confirmation exceeded grant end")
	}
	exec(`UPDATE platform_workspace_grant SET enabled=false,revision=revision+1 WHERE id=$1`, grant)
	read(403)
	exec(`UPDATE platform_workspace_grant SET enabled=true,revision=revision+1 WHERE id=$1`, grant)
	read(401)
	r.p.IdentityRevision = "1:1:3"
	read(200)
	exec(`UPDATE platform_workspace_grant SET valid_until=now()-interval '1 second',revision=revision+1 WHERE id=$1`, grant)
	r.p.IdentityRevision = "1:1:4"
	read(403)
}
