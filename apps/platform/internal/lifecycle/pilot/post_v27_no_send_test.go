package pilot

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

func v27Proof(t *testing.T) (ClosedOldBusinessDispatches, PostV27NoSendVerifier, time.Time) {
	p, v, key, at := oldBusinessProof(t)
	p.Execution, p.CandidateSHA256, p.BindingsSHA256, p.Targets = PostV27Execution, PostV27Candidate, PostV27Bindings, PostV27BusinessDispatches()
	return p, PostV27NoSendVerifier(signOldBusiness(t, v, p, key)), at
}

func TestPostV27ProofDoesNotReuseV22Authority(t *testing.T) {
	p, v, at := v27Proof(t)
	if _, err := v.Validate(at, "post-v27-closed:"+p.Nonce.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := PostV22NoSendVerifier(v).Validate(at, "post-v22-closed:"+p.Nonce.String()); err == nil {
		t.Fatal("v27 admitted as v22")
	}
	old, ov, key, oat := oldBusinessProof(t)
	ov = signOldBusiness(t, ov, old, key)
	if _, err := PostV27NoSendVerifier(ov).Validate(oat, "post-v27-closed:"+old.Nonce.String()); err == nil {
		t.Fatal("old proof admitted")
	}
	for _, target := range p.Targets {
		a := oldBusinessAttempt(target, at)
		a.ID = uuid.New()
		if _, err := v.VerifyProviderObservation(context.Background(), nil, a, "post-v27-closed:"+p.Nonce.String(), at); !errors.Is(err, lc.ErrStorageScope) {
			t.Fatal(err)
		}
	}
}

// This is an adapter contract on a new PostgreSQL instance, never a retained
// source. The successful predecessor is exact NOT_SENT/released, not UNKNOWN.
func TestPostV27NoSendPostgresEligibility(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN required")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	variants := []string{"success", "unrevoked", "claim", "UNKNOWN", "released", "absence", "new-attempt", "new-dispatch", "wrong-approval-hash", "wrong-basis", "old-UNKNOWN", "old-claim", "old-unrevoked", "old-unreleased", "old-invoked", "old-wrong-approval", "old-wrong-basis", "old-wrong-hash", "old-wrong-release", "old-missing-attempt"}
	for i, target := range PostV27BusinessDispatches() {
		for _, variant := range variants {
			t.Run(target.Case+"/"+variant, func(t *testing.T) {
				tx, err := conn.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback(ctx)
				_, err = tx.Exec(ctx, `CREATE TEMP TABLE lifecycle_disposition_dispatch(id uuid,workspace_id uuid,plan_id uuid,copy_id uuid,approval_id uuid) ON COMMIT DROP;
CREATE TEMP TABLE lifecycle_disposition_approval(id uuid,plan_hash text,basis_ref text,principal_ref text) ON COMMIT DROP;
CREATE TEMP TABLE lifecycle_approval_revocation(approval_id uuid) ON COMMIT DROP;
CREATE TEMP TABLE lifecycle_provider_send_claim(attempt_id uuid) ON COMMIT DROP;
CREATE TEMP TABLE lifecycle_provider_observation(id uuid,attempt_id uuid,dispatch_id uuid,copy_id uuid,workspace_id uuid,certainty text,outcome text,invoked_at timestamptz) ON COMMIT DROP;
CREATE TEMP TABLE lifecycle_dispatch_release(dispatch_id uuid,observation_id uuid) ON COMMIT DROP;
CREATE TEMP TABLE lifecycle_copy_absence_receipt(copy_id uuid) ON COMMIT DROP;
CREATE TEMP TABLE lifecycle_provider_attempt(id uuid,copy_id uuid,dispatch_id uuid,workspace_id uuid,plan_id uuid) ON COMMIT DROP;`)
				if err != nil {
					t.Fatal(err)
				}
				p, v, at := v27Proof(t)
				a := oldBusinessAttempt(target, at)
				old := PostV22BusinessDispatches()[i]
				for j, d := range []OldBusinessDispatch{old, target} {
					execution := PostV22Execution
					if j == 1 {
						execution = PostV27Execution
					}
					for _, q := range []struct {
						sql  string
						args []any
					}{
						{`INSERT INTO lifecycle_disposition_dispatch VALUES($1,$2,$3,$4,$5)`, []any{d.Dispatch, a.WorkspaceID, d.Plan, d.Copy, d.Approval}},
						{`INSERT INTO lifecycle_disposition_approval VALUES($1,$2,$3,$4)`, []any{d.Approval, d.PlanHash, "REMAINING_NON_DELETE_REHEARSAL:" + execution, "remaining-fixture:" + execution}},
						{`INSERT INTO lifecycle_provider_attempt VALUES($1,$2,$3,$4,$5)`, []any{d.Attempt, d.Copy, d.Dispatch, a.WorkspaceID, d.Plan}},
						{`INSERT INTO lifecycle_approval_revocation VALUES($1)`, []any{d.Approval}},
					} {
						if _, err = tx.Exec(ctx, q.sql, q.args...); err != nil {
							t.Fatal(err)
						}
					}
				}
				obs := uuid.New()
				if _, err = tx.Exec(ctx, `INSERT INTO lifecycle_provider_observation VALUES($1,$2,$3,$4,$5,'NOT_SENT','NOT_SENT',NULL)`, obs, old.Attempt, old.Dispatch, old.Copy, a.WorkspaceID); err != nil {
					t.Fatal(err)
				}
				if _, err = tx.Exec(ctx, `INSERT INTO lifecycle_dispatch_release VALUES($1,$2)`, old.Dispatch, obs); err != nil {
					t.Fatal(err)
				}
				mutations := map[string]struct {
					sql  string
					args []any
				}{
					"unrevoked":           {`DELETE FROM lifecycle_approval_revocation WHERE approval_id=$1`, []any{target.Approval}},
					"claim":               {`INSERT INTO lifecycle_provider_send_claim VALUES($1)`, []any{target.Attempt}},
					"UNKNOWN":             {`INSERT INTO lifecycle_provider_observation(attempt_id,certainty) VALUES($1,'UNKNOWN')`, []any{target.Attempt}},
					"released":            {`INSERT INTO lifecycle_dispatch_release(dispatch_id) VALUES($1)`, []any{target.Dispatch}},
					"absence":             {`INSERT INTO lifecycle_copy_absence_receipt VALUES($1)`, []any{target.Copy}},
					"new-attempt":         {`INSERT INTO lifecycle_provider_attempt(id,copy_id) VALUES($1,$2)`, []any{uuid.New(), target.Copy}},
					"new-dispatch":        {`INSERT INTO lifecycle_disposition_dispatch(id,copy_id) VALUES($1,$2)`, []any{uuid.New(), target.Copy}},
					"wrong-approval-hash": {`UPDATE lifecycle_disposition_approval SET plan_hash='foreign' WHERE id=$1`, []any{target.Approval}},
					"wrong-basis":         {`UPDATE lifecycle_disposition_approval SET basis_ref='foreign' WHERE id=$1`, []any{target.Approval}},
					"old-UNKNOWN":         {`UPDATE lifecycle_provider_observation SET certainty='UNKNOWN'`, nil},
					"old-claim":           {`INSERT INTO lifecycle_provider_send_claim VALUES($1)`, []any{old.Attempt}},
					"old-unrevoked":       {`DELETE FROM lifecycle_approval_revocation WHERE approval_id=$1`, []any{old.Approval}},
					"old-unreleased":      {`DELETE FROM lifecycle_dispatch_release`, nil},
					"old-invoked":         {`UPDATE lifecycle_provider_observation SET invoked_at=now()`, nil},
					"old-wrong-approval":  {`UPDATE lifecycle_disposition_dispatch SET approval_id=$1 WHERE id=$2`, []any{uuid.New(), old.Dispatch}},
					"old-wrong-basis":     {`UPDATE lifecycle_disposition_approval SET basis_ref='foreign' WHERE id=$1`, []any{old.Approval}},
					"old-wrong-hash":      {`UPDATE lifecycle_disposition_approval SET plan_hash='foreign' WHERE id=$1`, []any{old.Approval}},
					"old-wrong-release":   {`UPDATE lifecycle_dispatch_release SET observation_id=$1`, []any{uuid.New()}},
					"old-missing-attempt": {`DELETE FROM lifecycle_provider_attempt WHERE id=$1`, []any{old.Attempt}},
				}
				if q, ok := mutations[variant]; ok {
					if _, err = tx.Exec(ctx, q.sql, q.args...); err != nil {
						t.Fatal(err)
					}
				}
				result, err := v.VerifyProviderObservation(ctx, tx, a, "post-v27-closed:"+p.Nonce.String(), at)
				if variant == "success" {
					if err != nil || result.ValidateFor(a, at) != nil || result.Certainty != domain.InvocationNotSent || result.InvokedAt != nil {
						t.Fatal(result, err)
					}
				} else if !errors.Is(err, lc.ErrStorageDenied) {
					t.Fatal("conflict admitted", variant, err)
				}
			})
		}
	}
}
