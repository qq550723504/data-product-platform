package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
)

func readOnly(stage string) (loaded, error) {
	var l loaded
	raw, e := os.ReadFile("/run/private/candidate.json")
	if e != nil {
		return l, errDenied
	}
	l.CandidateHash = digest(raw)
	if read("/run/private/candidate.json", &l.C) != nil || validateCandidate(l.C) != nil || read("/run/private/operator.json", &l.F) != nil || l.F.DeleteSecret != "" || l.F.WirePrivateKey != "" {
		return l, errDenied
	}
	l.Scope, e = pilot.RemainingScopeFor(l.F.Group)
	if e != nil {
		return l, errDenied
	}
	for _, g := range l.C.Groups {
		if g.Group == l.F.Group {
			l.G = g
		}
	}
	if strictConfig(l) != nil {
		return l, errDenied
	}
	binary, e := os.Executable()
	if e != nil {
		return l, errDenied
	}
	b, e := os.ReadFile(binary)
	if e != nil || digest(b) != l.C.SenderBinarySHA256 {
		return l, errDenied
	}
	var proof signed
	if read("/run/private/topology-"+stage+".json", &proof) != nil {
		return l, errDenied
	}
	pub, _ := hex.DecodeString(l.C.PublicKey)
	var t topology
	at := time.Now().UTC()
	if !ed25519.Verify(pub, proof.Body, proof.Signature) || json.Unmarshal(proof.Body, &t) != nil || t.Execution != execution || t.CandidateSHA256 != l.CandidateHash || t.Group != l.F.Group || t.Stage != stage || t.Helper != l.G.Helpers[stage] || len(t.Helper) != 64 || t.Core != l.G.Core || t.Native != l.G.Native || t.Gateway != l.G.Gateway || t.Network != l.G.Network || t.BinarySHA256 != l.C.SenderBinarySHA256 || !t.NoPorts || !t.NoRoot || !t.NoSocket || !t.ReadOnly || !t.NoRestart || !t.Internal || at.Before(t.At) || at.Sub(t.At) > time.Minute {
		return l, errDenied
	}
	return l, nil
}
func recoverExact(ctx context.Context, stage string) error {
	l, e := readOnly(stage)
	if e != nil {
		return e
	}
	target, e := fixedTarget(l.F.Group, strings.TrimPrefix(stage, "recover-"))
	if e != nil {
		return e
	}
	var cfg pilot.RecoveryConfig
	if read("/run/private/recovery.json", &cfg) != nil || cfg.WorkspaceID != l.Scope.Workspace || cfg.RunID != l.Scope.Run || cfg.ProviderInstanceID != l.Scope.Provider || cfg.IncarnationID != l.Scope.Incarnation || cfg.ActorID != l.G.Actor || cfg.ManifestSHA256 != l.Scope.ManifestSHA256 || cfg.Endpoint != "127.0.0.1:9000" || cfg.InspectAccess != l.F.InspectAccess || cfg.InspectSecret != l.F.InspectSecret || cfg.Token != l.F.Token || !cfg.Enabled || !cfg.SyntheticOnly || !time.Now().UTC().Before(cfg.ValidUntil) {
		return errDenied
	}
	// Recovery config has a separate read-only inspector identity, no DELETE
	// secret. It does not reuse the expired human send window as read authority.
	pool, e := pgxpool.New(ctx, l.F.PostgresDSN)
	if e != nil {
		return e
	}
	defer pool.Close()
	router, e := routing.NewRouter(false)
	if e != nil {
		return e
	}
	outbox.ConfigureAppendObligation(router)
	defer outbox.ConfigureAppendObligation(nil)
	var work, attempt uuid.UUID
	e = pool.QueryRow(ctx, `SELECT w.id,a.id FROM lifecycle_reconciliation_work w JOIN lifecycle_provider_attempt a ON a.id=w.attempt_id JOIN lifecycle_disposition_dispatch d ON d.id=a.dispatch_id JOIN lifecycle_disposition_approval p ON p.id=d.approval_id WHERE a.workspace_id=$1 AND a.copy_id=$2 AND p.basis_ref=$3 AND p.principal_ref=$4`, target.WorkspaceID, target.ID, "REMAINING_NON_DELETE_REHEARSAL:"+execution, "remaining-fixture:"+execution).Scan(&work, &attempt)
	if e != nil {
		return e
	}
	if stage == "recover-M1B" {
		var checkpoint map[string]json.RawMessage
		if read("/run/private/M1-mixed-checkpoint.json", &checkpoint) != nil {
			return errDenied
		}
		var recorded domain.ProviderAttempt
		var ex string
		var ordered bool
		var at time.Time
		if json.Unmarshal(checkpoint["B_confirmed_unknown"], &recorded) != nil || recorded.ID != attempt || json.Unmarshal(checkpoint["execution"], &ex) != nil || ex != execution || json.Unmarshal(checkpoint["before_any_B_lookup"], &ordered) != nil || !ordered || json.Unmarshal(checkpoint["at"], &at) != nil || at.After(time.Now().UTC()) {
			return errDenied
		}
		var earlyLookup bool
		e = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_storage_call c JOIN lifecycle_fixture_copy cp ON cp.id=c.copy_id JOIN lifecycle_provider_observation o ON o.attempt_id=$1 WHERE c.started_at>o.observed_at AND c.started_at<$2 AND o.outcome='UNKNOWN' AND (cp.id=$3 OR (c.operation='LIST_NAMESPACE_VERSIONS' AND cp.namespace_id=(SELECT namespace_id FROM lifecycle_fixture_copy WHERE id=$3))))`, attempt, at, target.ID).Scan(&earlyLookup)
		if e != nil {
			return e
		}
		if earlyLookup {
			return errors.New("M1B lookup preceded mixed checkpoint")
		}
		var valid bool
		e = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_provider_observation o WHERE o.attempt_id=$1 AND o.certainty='CONFIRMED' AND o.outcome='UNKNOWN' AND o.observed_at<=$2) AND EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE copy_id=$3 AND observed_at<=$2) AND NOT EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE copy_id=$4 AND observed_at<$2) AND NOT EXISTS(SELECT 1 FROM lifecycle_plan_completion WHERE plan_id=$5 AND confirmed_at<$2)`, attempt, at, l.Scope.Copies[0].ID, target.ID, recorded.PlanID).Scan(&valid)
		if e != nil {
			return e
		}
		if !valid {
			return errors.New("M1 checkpoint precedes its committed mixed outcomes")
		}
	}
	exact, inspector, _, e := pilot.BuildRecoveryInspectorAt("/run/private/recovery.json", pool)
	if e != nil {
		return e
	}
	if e = recoverThroughQueue(ctx, pool, l, stage, work, exact, inspector); e != nil {
		return e
	}
	return save("/run/output/"+stage+".json", map[string]any{"execution": execution, "target": target, "work": work, "attempt": attempt, "read_only": true, "resend": 0})
}
