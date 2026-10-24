package pilot

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

func oldBusinessProof(t *testing.T) (ClosedOldBusinessDispatches, PostV22NoSendVerifier, ed25519.PrivateKey, time.Time) {
	t.Helper()
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	owners := map[string]string{strings.Repeat("a", 64): strings.Repeat("b", 64)}
	p := ClosedOldBusinessDispatches{AuditID: uuid.New(), Nonce: uuid.New(), At: at.Add(-time.Second), Execution: PostV22Execution,
		CandidateSHA256: PostV22Candidate, BindingsSHA256: PostV22Bindings, Core: PostV22Core, EvidenceSHA256: strings.Repeat("c", 64),
		Targets: PostV22BusinessDispatches(), Owners: owners, StoppedPIDZero: true, NoRestart: true, ConfigDisabled: true, DeleteIdentityDisabled: true, GatewaySealed: true}
	v := PostV22NoSendVerifier{PublicKey: public, AuditID: p.AuditID, Owners: owners, EvidenceSHA256: p.EvidenceSHA256}
	return p, v, key, at
}

func signOldBusiness(t *testing.T, v PostV22NoSendVerifier, p ClosedOldBusinessDispatches, key ed25519.PrivateKey) PostV22NoSendVerifier {
	t.Helper()
	body, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	v.Proof = SignedClosedReferenceExecutor{Body: body, Signature: ed25519.Sign(key, body)}
	return v
}

func TestPostV22ClosedProofRejectsStaleForeignExpandedAndTamperedEvidence(t *testing.T) {
	p, v, key, at := oldBusinessProof(t)
	ref := "post-v22-closed:" + p.Nonce.String()
	valid := signOldBusiness(t, v, p, key)
	if _, e := valid.Validate(at, ref); e != nil {
		t.Fatal(e)
	}
	// []byte preserves signed bytes through persisted pretty JSON.
	raw, _ := json.MarshalIndent(valid.Proof, "", "  ")
	if e := json.Unmarshal(raw, &valid.Proof); e != nil {
		t.Fatal(e)
	}
	if _, e := valid.Validate(at, ref); e != nil {
		t.Fatal(e)
	}
	changes := map[string]func(*ClosedOldBusinessDispatches){
		"audit":               func(p *ClosedOldBusinessDispatches) { p.AuditID = uuid.New() },
		"nonce":               func(p *ClosedOldBusinessDispatches) { p.Nonce = uuid.New() },
		"old-execution":       func(p *ClosedOldBusinessDispatches) { p.Execution = uuid.NewString() },
		"candidate":           func(p *ClosedOldBusinessDispatches) { p.CandidateSHA256 = strings.Repeat("d", 64) },
		"bindings":            func(p *ClosedOldBusinessDispatches) { p.BindingsSHA256 = strings.Repeat("d", 64) },
		"Core":                func(p *ClosedOldBusinessDispatches) { p.Core = strings.Repeat("d", 64) },
		"target-expansion":    func(p *ClosedOldBusinessDispatches) { p.Targets = append(p.Targets, p.Targets[0]) },
		"target-substitution": func(p *ClosedOldBusinessDispatches) { p.Targets[0].Approval = uuid.New() },
		"owner-loss":          func(p *ClosedOldBusinessDispatches) { p.Owners = map[string]string{} },
		"owner-expansion": func(p *ClosedOldBusinessDispatches) {
			p.Owners = map[string]string{strings.Repeat("d", 64): strings.Repeat("b", 64)}
		},
		"evidence":      func(p *ClosedOldBusinessDispatches) { p.EvidenceSHA256 = strings.Repeat("d", 64) },
		"future":        func(p *ClosedOldBusinessDispatches) { p.At = at.Add(time.Second) },
		"stale":         func(p *ClosedOldBusinessDispatches) { p.At = at.Add(-61 * time.Second) },
		"running-owner": func(p *ClosedOldBusinessDispatches) { p.StoppedPIDZero = false },
		"restart":       func(p *ClosedOldBusinessDispatches) { p.NoRestart = false },
		"permission":    func(p *ClosedOldBusinessDispatches) { p.ConfigDisabled = false },
		"IAM":           func(p *ClosedOldBusinessDispatches) { p.DeleteIdentityDisabled = false },
		"gateway":       func(p *ClosedOldBusinessDispatches) { p.GatewaySealed = false },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			p, v, key, at := oldBusinessProof(t)
			ref := "post-v22-closed:" + p.Nonce.String()
			change(&p)
			if _, e := signOldBusiness(t, v, p, key).Validate(at, ref); !errors.Is(e, lc.ErrStorageContract) {
				t.Fatal("invalid signed evidence admitted", e)
			}
		})
	}
	valid.Proof.Body[0] = '['
	if _, e := valid.Validate(at, ref); e == nil {
		t.Fatal("tamper admitted")
	}
	valid = signOldBusiness(t, v, p, key)
	valid.PublicKey, _, _ = ed25519.GenerateKey(rand.Reader)
	if _, e := valid.Validate(at, ref); e == nil {
		t.Fatal("untrusted signer admitted")
	}
}

func oldBusinessAttempt(d OldBusinessDispatch, at time.Time) domain.ProviderAttempt {
	return domain.ProviderAttempt{ID: d.Attempt, WorkspaceID: uuid.MustParse("a4aa2689-6e8f-4180-a567-dfb875407eac"), PlanID: d.Plan,
		CopyID: d.Copy, DispatchID: d.Dispatch, Action: domain.DeleteExact, ProviderRequestKey: "lifecycle-delete:" + d.Attempt.String(), StartedAt: at.Add(-time.Hour), DependencyRevision: 1}
}

func TestPostV22ClosedProofRejectsForeignAttemptBeforeDatabase(t *testing.T) {
	p, v, key, at := oldBusinessProof(t)
	v = signOldBusiness(t, v, p, key)
	a := oldBusinessAttempt(p.Targets[0], at)
	for _, change := range []func(*domain.ProviderAttempt){
		func(a *domain.ProviderAttempt) { a.ID = uuid.New() },
		func(a *domain.ProviderAttempt) { a.WorkspaceID = uuid.New() },
		func(a *domain.ProviderAttempt) { a.CopyID = uuid.New() },
		func(a *domain.ProviderAttempt) { a.DispatchID = uuid.New() },
		func(a *domain.ProviderAttempt) { a.PlanID = uuid.New() },
	} {
		bad := a
		change(&bad)
		// nil tx makes an accidental query observable as a test failure.
		if _, e := v.VerifyProviderObservation(context.Background(), nil, bad, "post-v22-closed:"+p.Nonce.String(), at); !errors.Is(e, lc.ErrStorageScope) {
			t.Fatal("foreign attempt reached database", e)
		}
	}
}

// Adapter SQL contract on actual PostgreSQL, using isolated TEMP tables. This
// does not claim full migration/fence/Command acceptance; that has separate tests.
func TestPostV22NoSendPostgresEligibility(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN required for real PostgreSQL adapter contract")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	for _, variant := range []string{"success", "unrevoked", "claim", "UNKNOWN", "released", "absence", "new-attempt", "new-dispatch", "wrong-approval-hash"} {
		t.Run(variant, func(t *testing.T) {
			tx, e := conn.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback(ctx)
			_, e = tx.Exec(ctx, `CREATE TEMP TABLE lifecycle_disposition_dispatch(id uuid,workspace_id uuid,plan_id uuid,copy_id uuid,approval_id uuid) ON COMMIT DROP;
	 CREATE TEMP TABLE lifecycle_disposition_approval(id uuid,plan_hash text,basis_ref text,principal_ref text) ON COMMIT DROP;
	 CREATE TEMP TABLE lifecycle_approval_revocation(approval_id uuid) ON COMMIT DROP;
	 CREATE TEMP TABLE lifecycle_provider_send_claim(attempt_id uuid) ON COMMIT DROP;
	 CREATE TEMP TABLE lifecycle_provider_observation(attempt_id uuid) ON COMMIT DROP;
	 CREATE TEMP TABLE lifecycle_dispatch_release(dispatch_id uuid) ON COMMIT DROP;
	 CREATE TEMP TABLE lifecycle_copy_absence_receipt(copy_id uuid) ON COMMIT DROP;
	 CREATE TEMP TABLE lifecycle_provider_attempt(id uuid,copy_id uuid) ON COMMIT DROP;`)
			if e != nil {
				t.Fatal(e)
			}
			p, v, key, at := oldBusinessProof(t)
			v = signOldBusiness(t, v, p, key)
			d := p.Targets[0]
			a := oldBusinessAttempt(d, at)
			for _, q := range []struct {
				sql  string
				args []any
			}{
				{`INSERT INTO lifecycle_disposition_dispatch VALUES($1,$2,$3,$4,$5)`, []any{d.Dispatch, a.WorkspaceID, d.Plan, d.Copy, d.Approval}},
				{`INSERT INTO lifecycle_disposition_approval VALUES($1,$2,$3,$4)`, []any{d.Approval, d.PlanHash, "REMAINING_NON_DELETE_REHEARSAL:" + PostV22Execution, "remaining-fixture:" + PostV22Execution}},
				{`INSERT INTO lifecycle_provider_attempt VALUES($1,$2)`, []any{d.Attempt, d.Copy}},
			} {
				if _, e = tx.Exec(ctx, q.sql, q.args...); e != nil {
					t.Fatal(e)
				}
			}
			if variant != "unrevoked" {
				if _, e = tx.Exec(ctx, `INSERT INTO lifecycle_approval_revocation VALUES($1)`, d.Approval); e != nil {
					t.Fatal(e)
				}
			}
			switch variant {
			case "claim":
				_, e = tx.Exec(ctx, `INSERT INTO lifecycle_provider_send_claim VALUES($1)`, d.Attempt)
			case "UNKNOWN":
				_, e = tx.Exec(ctx, `INSERT INTO lifecycle_provider_observation VALUES($1)`, d.Attempt)
			case "released":
				_, e = tx.Exec(ctx, `INSERT INTO lifecycle_dispatch_release VALUES($1)`, d.Dispatch)
			case "absence":
				_, e = tx.Exec(ctx, `INSERT INTO lifecycle_copy_absence_receipt VALUES($1)`, d.Copy)
			case "new-attempt":
				_, e = tx.Exec(ctx, `INSERT INTO lifecycle_provider_attempt VALUES($1,$2)`, uuid.New(), d.Copy)
			case "new-dispatch":
				_, e = tx.Exec(ctx, `INSERT INTO lifecycle_disposition_dispatch(id,copy_id) VALUES($1,$2)`, uuid.New(), d.Copy)
			case "wrong-approval-hash":
				_, e = tx.Exec(ctx, `UPDATE lifecycle_disposition_approval SET plan_hash='foreign'`)
			}
			if e != nil {
				t.Fatal(e)
			}
			observation, e := v.VerifyProviderObservation(ctx, tx, a, "post-v22-closed:"+p.Nonce.String(), at)
			if variant == "success" {
				if e != nil || observation.ValidateFor(a, at) != nil || observation.Certainty != domain.InvocationNotSent || observation.InvokedAt != nil {
					t.Fatal("exact revoked no-send failed", observation, e)
				}
			} else if !errors.Is(e, lc.ErrStorageDenied) {
				t.Fatal("conflict was not rejected", variant, e)
			}
		})
	}
}
