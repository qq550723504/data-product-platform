package platformmanagement

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAuditEqualTimestampPagesStayCompleteAndWorkspaceScoped(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("isolated migrated TEST_POSTGRES_DSN required")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	workspace, foreign, actor := uuid.New(), uuid.New(), uuid.New()
	at := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	want := map[uuid.UUID]bool{}
	for _, ws := range []uuid.UUID{workspace, foreign} {
		for i := 0; i < 58; i++ {
			id := uuid.New()
			_, err := db.Exec(ctx, `INSERT INTO audit_event(id,workspace_id,actor_type,actor_id,action,object_type,object_id,occurred_at) VALUES($1,$2,'USER',$3,'SYNTHETIC_REVIEW','Dataset',$4,$5)`, id, ws, actor, uuid.New(), at)
			if err != nil {
				t.Fatal(err)
			}
			if ws == workspace {
				want[id] = true
			}
		}
	}
	reader, cursor := PostgresAudit{Pool: db}, ""
	seen := map[uuid.UUID]bool{}
	for n, size := range []int{25, 25, 8} {
		page, err := reader.List(ctx, workspace, actor, cursor)
		if err != nil || len(page.Items) != size {
			t.Fatalf("page %d length=%d err=%v", n, len(page.Items), err)
		}
		for _, event := range page.Items {
			if !want[event.ID] || seen[event.ID] || !event.OccurredAt.Equal(at) {
				t.Fatal("foreign, duplicate or changed history")
			}
			seen[event.ID] = true
		}
		if n < 2 {
			if page.Next == "" {
				t.Fatal("missing next page")
			}
			for _, scope := range [][2]uuid.UUID{{foreign, actor}, {workspace, uuid.New()}} {
				if _, err := reader.List(ctx, scope[0], scope[1], page.Next); err == nil {
					t.Fatal("cross-scope cursor accepted")
				}
			}
		} else if page.Next != "" {
			t.Fatal("extra page after exact membership")
		}
		cursor = page.Next
	}
	if len(seen) != len(want) {
		t.Fatal("audit history lost rows at equal timestamp")
	}
}
