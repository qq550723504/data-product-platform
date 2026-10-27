package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	q "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/readmodel"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This suite requires explicitly prepared synthetic binding fixtures. It never
// mutates them: rejected guard writes and rollback/down probes use rollback TXs.
func TestAcceptanceBindingPostgres(t *testing.T) {
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
	ws := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	dv := uuid.MustParse("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
	standard := uuid.MustParse("82828282-8282-4282-8282-828282828282")
	target := uuid.MustParse("40404040-4040-4040-8040-404040404040")
	id := func(n int) uuid.UUID { return uuid.MustParse(fmt.Sprintf("86000000-0000-4000-8000-%012d", n)) }
	var exists bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM acceptance_party_representation WHERE id=$1)`, id(100)).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Skip("explicit binding preview fixture not prepared")
	}
	repo := NewPostgresRepository(pool)
	query := q.Query{WorkspaceID: ws, DeliveryID: &target, DatasetVersionID: &dv, VersionID: &standard, Limit: 25, ActorID: uuid.MustParse("22222222-2222-4222-8222-222222222222"), Subject: "agent-q2-operator"}
	service := q.Service{Reader: repo}
	_, c, err := service.Read(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if c.Applicability.Status != "MATCHED" || c.ReceivingPartyStatus != "VERIFIED" || !c.Responsibilities[0].IsCurrentCaller || c.WriteEnabled || c.BindingHistory.Page.Total != 29 {
		t.Fatalf("current %+v", c)
	}
	query.BindingOffset = 25
	old := id(2000)
	query.BindingID = &old
	_, paged, err := service.Read(ctx, query)
	if err != nil || paged.Applicability.Status != "MATCHED" || paged.BindingHistory.Selected.ID != old.String() || len(paged.BindingHistory.Items) != 4 {
		t.Fatalf("history selection/page %v %+v", err, paged)
	}
	foreign := id(200)
	query.BindingID = &foreign
	if _, _, err = service.Read(ctx, query); !errors.Is(err, q.ErrNotFound) {
		t.Fatalf("foreign target binding %v", err)
	}
	query.BindingID = nil
	query.BindingOffset = 0
	for n, want := range []string{"MATCHED", "MATCHED", "MISMATCH", "MATCHED", "MATCHED", "MISMATCH", "MISMATCH"} {
		if n == 0 {
			continue
		}
		delivery := id(n)
		query.DeliveryID = &delivery
		_, c, err = service.Read(ctx, query)
		if err != nil || c.Applicability.Status != want {
			t.Fatalf("scenario %d %v %+v", n, err, c)
		}
		expected := map[int]struct {
			index  int
			status string
		}{1: {1, "INACTIVE"}, 2: {2, "INACTIVE"}, 3: {3, "AMBIGUOUS"}, 4: {3, "CONFLICT"}, 6: {2, "ASSIGNED"}}
		if r, ok := expected[n]; ok && c.Responsibilities[r.index].Status != r.status {
			t.Fatalf("scenario %d role %+v", n, c.Responsibilities[r.index])
		}
	}
	query.DeliveryID = &target
	foreignWS := uuid.New()
	query.WorkspaceID = foreignWS
	if _, _, err = service.Read(ctx, query); !errors.Is(err, q.ErrNotFound) {
		t.Fatalf("workspace scope %v", err)
	}
	t.Run("immutable-and-cross-scope-guards", func(t *testing.T) {
		// Each failure has its own transaction: no failed statement can accidentally
		// abort the next check or leave a successful mutation behind.
		for _, table := range []string{"acceptance_party_representation", "acceptance_responsibility_binding", "acceptance_standard_applicability", "acceptance_binding_disposition"} {
			for _, operation := range []string{"UPDATE " + table + " SET recorded_by=recorded_by", "DELETE FROM " + table} {
				tx, e := pool.Begin(ctx)
				if e != nil {
					t.Fatal(e)
				}
				_, e = tx.Exec(ctx, operation)
				tx.Rollback(ctx)
				if e == nil {
					t.Fatalf("historical mutation allowed: %s", operation)
				}
			}
		}
		probes := []struct{ sql, reason string }{
			{`INSERT INTO acceptance_party_representation SELECT gen_random_uuid(),gen_random_uuid(),actor_id,principal_subject,party_ref,basis_evidence_id,valid_from,valid_to,origin,recorded_by,created_at FROM acceptance_party_representation LIMIT 1`, "acceptance evidence crosses workspace"},
			{`INSERT INTO acceptance_responsibility_binding SELECT gen_random_uuid(),workspace_id,delivery_operation_id,dataset_version_id,'RECEIVER','86000000-0000-4000-8000-000000000100',basis_evidence_id,valid_from,valid_to,origin,recorded_by,created_at FROM acceptance_responsibility_binding WHERE id='86000000-0000-4000-8000-000000000110'`, "acceptance representative does not match receiving party"},
			{`INSERT INTO acceptance_standard_applicability SELECT gen_random_uuid(),workspace_id,delivery_operation_id,dataset_version_id,contract_version_id,repeat('f',64),criteria_set_hash,confirmer_binding_id,basis_evidence_id,valid_from,valid_to,origin,recorded_by,created_at FROM acceptance_standard_applicability WHERE id='86000000-0000-4000-8000-000000000120'`, "acceptance standard source crosses scope/hash"},
			{`INSERT INTO acceptance_standard_applicability SELECT gen_random_uuid(),workspace_id,delivery_operation_id,dataset_version_id,contract_version_id,source_sha256,criteria_set_hash,'86000000-0000-4000-8000-000000000110',basis_evidence_id,valid_from,valid_to,origin,recorded_by,created_at FROM acceptance_standard_applicability WHERE id='86000000-0000-4000-8000-000000000120'`, "acceptance confirmer does not match target/role"},
			{`INSERT INTO acceptance_binding_disposition(id,workspace_id,responsibility_id,disposition,effective_at,reason,basis_evidence_id,recorded_by) SELECT gen_random_uuid(),gen_random_uuid(),id,'REVOKED',now(),'foreign workspace',basis_evidence_id,recorded_by FROM acceptance_responsibility_binding LIMIT 1`, "acceptance evidence crosses workspace"},
		}
		for _, probe := range probes {
			tx, e := pool.BeginTx(ctx, pgx.TxOptions{})
			if e != nil {
				t.Fatal(e)
			}
			_, e = tx.Exec(ctx, probe.sql)
			tx.Rollback(ctx)
			var pgerr *pgconn.PgError
			if !errors.As(e, &pgerr) || pgerr.Code != "23514" || !strings.Contains(pgerr.Message, probe.reason) {
				t.Fatalf("wrong rejection: expected %s, got %v", probe.reason, e)
			}
		}
		root, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		down, err := os.ReadFile(filepath.Join(root, "../../../../../migrations/000074_acceptance_bindings.down.sql"))
		if err != nil {
			t.Fatal(err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, string(down))
		tx.Rollback(ctx)
		if err == nil {
			t.Fatal("historical migration down allowed")
		}
	})
}
