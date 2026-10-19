package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

type wireWitness struct {
	Execution, CandidateSHA256, Group, Helper string
	Nonce                                     uuid.UUID
	Attempt                                   domain.ProviderAttempt
	Copy                                      domain.Copy
	UpstreamStatus                            int
	At                                        time.Time
}
type wireHandler struct {
	scope                                                []domain.Copy
	access, helper, candidateHash, group, output, budget string
	key                                                  ed25519.PrivateKey
	backend                                              *url.URL
	transport                                            http.RoundTripper
	fresh                                                func() error
	attempt                                              func(context.Context, domain.Copy) (domain.ProviderAttempt, error)
	after                                                func(context.Context, signed) error
}

func (h *wireHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.fresh() != nil || r.Method != "DELETE" || r.ContentLength != 0 || len(r.TransferEncoding) != 0 || r.URL.RawPath != "" || r.URL.User != nil || r.Header.Get("X-Amz-Bypass-Governance-Retention") != "" || r.Header.Get("X-Amz-Copy-Source") != "" || !strings.Contains(r.Header.Get("Authorization"), "Credential="+h.access+"/") {
		w.WriteHeader(403)
		return
	}
	var cp domain.Copy
	for _, target := range h.scope {
		if r.RequestURI == "/"+target.Namespace+"/"+target.Key+"?versionId="+target.ProviderVersion {
			cp = target
		}
	}
	if cp.ID == uuid.Nil {
		w.WriteHeader(403)
		return
	}
	attempt, e := h.attempt(r.Context(), cp)
	if e != nil || h.fresh() != nil {
		w.WriteHeader(403)
		return
	}
	if e = save(filepath.Join(h.budget, "wire-"+cp.ID.String()+".json"), map[string]any{"attempt": attempt, "copy": cp, "execution": execution}); e != nil {
		w.WriteHeader(403)
		return
	}
	upstream := r.Clone(r.Context())
	upstream.URL.Scheme = h.backend.Scheme
	upstream.URL.Host = h.backend.Host
	upstream.RequestURI = ""
	upstream.Body = nil
	// Preserve signed Host. A new, non-reused connection is forwarded once.
	response, e := h.transport.RoundTrip(upstream)
	if e != nil {
		drop(w)
		return
	}
	defer response.Body.Close()
	if response.StatusCode == 204 {
		witness := wireWitness{Execution: execution, CandidateSHA256: h.candidateHash, Group: h.group, Helper: h.helper, Nonce: uuid.New(), Attempt: attempt, Copy: cp, UpstreamStatus: response.StatusCode, At: time.Now().UTC()}
		body, _ := json.Marshal(witness)
		proof := signed{Body: body, Signature: ed25519.Sign(h.key, body)}
		if e = save(filepath.Join(h.output, "wire-"+cp.ID.String()+".json"), proof); e != nil {
			drop(w)
			return
		}
		if cp.Key == "fixture/h2-a.csv" {
			// Real upstream response precedes the hold transaction. No WroteRequest
			// callback or synthetic barrier is accepted as native receipt evidence.
			if h.after == nil || h.after(r.Context(), proof) != nil {
				drop(w)
				return
			}
			drop(w)
			return
		}
		if cp.Key == "fixture/m1-b.csv" {
			drop(w)
			return
		}
	}
	for k, values := range response.Header {
		for _, v := range values {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(response.StatusCode)
	io.Copy(w, io.LimitReader(response.Body, 64<<10))
}
func drop(w http.ResponseWriter) {
	if hijacker, ok := w.(http.Hijacker); ok {
		if conn, _, e := hijacker.Hijack(); e == nil {
			conn.Close()
			return
		}
	}
	panic(http.ErrAbortHandler)
}

type containmentAuthority struct {
	l     loaded
	proof signed
}

func (a containmentAuthority) AuthorizeLifecycle(_ context.Context, _ pgx.Tx, w uuid.UUID, cap string, at time.Time) (principal.RequestPrincipal, error) {
	key, _ := hex.DecodeString(a.l.C.WirePublicKey)
	var p wireWitness
	if !ed25519.Verify(key, a.proof.Body, a.proof.Signature) || json.Unmarshal(a.proof.Body, &p) != nil || p.Execution != execution || p.CandidateSHA256 != a.l.CandidateHash || p.Group != "late-hold" || p.Helper != a.l.G.Helpers["wire"] || p.Copy.Key != "fixture/h2-a.csv" || p.Copy.WorkspaceID != w || p.Nonce == uuid.Nil || p.UpstreamStatus != 204 || p.At.Before(a.l.P.NotBefore) || !p.At.Before(a.l.P.ExpiresAt) || at.Before(p.At) || at.Sub(p.At) > time.Minute || (cap != lc.CapabilityRevokeApproval && cap != lc.CapabilityPlaceHold) {
		return principal.RequestPrincipal{}, errDenied
	}
	return principal.RequestPrincipal{Subject: "remaining-fixture:" + execution, ActorID: a.l.G.Actor, WorkspaceID: w, Capabilities: map[string]struct{}{cap: {}}}, nil
}
func containH2(ctx context.Context, pool *pgxpool.Pool, l loaded, proof signed) error {
	var p wireWitness
	if json.Unmarshal(proof.Body, &p) != nil {
		return errDenied
	}
	target, e := fixedTarget("late-hold", "H2")
	if e != nil || p.Copy != target || p.Attempt.CopyID != target.ID || p.Attempt.WorkspaceID != l.Scope.Workspace {
		return errDenied
	}
	var approval uuid.UUID
	e = pool.QueryRow(ctx, `SELECT d.approval_id FROM lifecycle_provider_attempt a JOIN lifecycle_provider_send_claim c ON c.attempt_id=a.id JOIN lifecycle_disposition_dispatch d ON d.id=a.dispatch_id JOIN lifecycle_disposition_approval p ON p.id=d.approval_id WHERE a.id=$1 AND a.copy_id=$2 AND p.basis_ref=$3 AND p.principal_ref=$4`, p.Attempt.ID, target.ID, "REMAINING_NON_DELETE_REHEARSAL:"+execution, "remaining-fixture:"+execution).Scan(&approval)
	if e != nil {
		return e
	}
	o := lc.NewOccupancyService(transaction.NewManager(pool), &infrastructure.PostgresRepository{}, containmentAuthority{l, proof}, nil, nil)
	service := lc.NewDispositionService(o, nil, nil, nil)
	if _, e = service.RevokeDispositionApproval(ctx, lc.RevokeDispositionApprovalCommand{CommandHeader: lc.CommandHeader{WorkspaceID: l.Scope.Workspace, IdempotencyKey: execution + ":H2-wire-revoke"}, ApprovalID: approval, BasisRef: "native-wire:" + p.Nonce.String()}); e != nil {
		return e
	}
	var holds []lc.FactResult
	for _, cp := range l.Scope.Copies {
		if cp.Key != "fixture/h2-a.csv" && cp.Key != "fixture/h2-b.csv" {
			continue
		}
		hold, e := o.PlaceContentHold(ctx, lc.PlaceContentHoldCommand{CommandHeader: lc.CommandHeader{WorkspaceID: l.Scope.Workspace, IdempotencyKey: execution + ":H2-wire-hold:" + cp.ID.String()}, CopyID: cp.ID, BasisRef: "native-wire:" + p.Nonce.String()})
		if e != nil {
			return e
		}
		holds = append(holds, hold)
	}
	if len(holds) != 2 || !holds[0].PreservationUnconfirmed {
		return errors.New("H2 wire conflict was not preserved")
	}
	return save("/run/output/H2-holds-before-response-loss.json", map[string]any{"witness": proof, "approval_revoked": approval, "holds": holds, "response_released": false, "native_cancel_supported": false})
}
func wire(ctx context.Context) error {
	l, e := load("wire")
	if e != nil {
		return e
	}
	key, e := hex.DecodeString(l.F.WirePrivateKey)
	pub, _ := hex.DecodeString(l.C.WirePublicKey)
	if e != nil || len(key) != ed25519.PrivateKeySize || !strings.EqualFold(hex.EncodeToString(ed25519.PrivateKey(key).Public().(ed25519.PublicKey)), hex.EncodeToString(pub)) || l.F.DeleteSecret != "" {
		return errDenied
	}
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
	var targets []domain.Copy
	for _, stage := range []string{"P2", "T1", "T2", "H2", "M1A", "M1B"} {
		if cp, e := fixedTarget(l.F.Group, stage); e == nil {
			targets = append(targets, cp)
		}
	}
	backend, _ := url.Parse("http://minio:9000")
	handler := &wireHandler{scope: targets, access: l.F.DeleteAccess, helper: l.G.Helpers["wire"], candidateHash: l.CandidateHash, group: l.F.Group, output: "/run/output", budget: "/run/budget", key: key, backend: backend, transport: &http.Transport{Proxy: nil, DisableKeepAlives: true, ResponseHeaderTimeout: 8 * time.Second}, fresh: func() error {
		current, e := load("wire")
		if e != nil || !reflectLoaded(current, l) {
			return errDenied
		}
		return nil
	}}
	handler.attempt = func(ctx context.Context, cp domain.Copy) (domain.ProviderAttempt, error) {
		var ids []uuid.UUID
		rows, e := pool.Query(ctx, `SELECT a.id FROM lifecycle_provider_attempt a JOIN lifecycle_provider_send_claim c ON c.attempt_id=a.id JOIN lifecycle_disposition_dispatch d ON d.id=a.dispatch_id JOIN lifecycle_disposition_approval p ON p.id=d.approval_id WHERE a.workspace_id=$1 AND a.copy_id=$2 AND p.basis_ref=$3 AND p.principal_ref=$4`, cp.WorkspaceID, cp.ID, "REMAINING_NON_DELETE_REHEARSAL:"+execution, "remaining-fixture:"+execution)
		if e != nil {
			return domain.ProviderAttempt{}, e
		}
		for rows.Next() {
			var id uuid.UUID
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return domain.ProviderAttempt{}, e
			}
			ids = append(ids, id)
		}
		e = rows.Err()
		rows.Close()
		if e != nil || len(ids) != 1 {
			return domain.ProviderAttempt{}, errDenied
		}
		var a domain.ProviderAttempt
		e = transaction.NewManager(pool).Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var e error
			a, e = (&infrastructure.PostgresRepository{}).GetAttempt(ctx, tx, cp.WorkspaceID, ids[0], false)
			return e
		})
		return a, e
	}
	handler.after = func(ctx context.Context, p signed) error { return containH2(ctx, pool, l, p) }
	listener, e := net.Listen("tcp", "127.0.0.1:9002")
	if e != nil {
		return e
	}
	server := &http.Server{ReadHeaderTimeout: 3 * time.Second, Handler: handler}
	go func() { <-ctx.Done(); server.Close() }()
	return server.Serve(listener)
}
func reflectLoaded(a, b loaded) bool {
	raw, _ := json.Marshal(a)
	other, _ := json.Marshal(b)
	return string(raw) == string(other)
}

func mixedCheckpoint(ctx context.Context, pool *pgxpool.Pool, l loaded, a domain.ProviderAttempt, disposition *lc.DispositionService, inspector lc.StorageInspector, deps *pilot.RemainingDependencies) error {
	first, _ := fixedTarget("mixed", "M1A")
	var firstAbsent, bUnknown, bAbsent, complete bool
	e := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE copy_id=$1),EXISTS(SELECT 1 FROM lifecycle_provider_observation WHERE attempt_id=$2 AND certainty='CONFIRMED' AND outcome='UNKNOWN'),EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE copy_id=$3),EXISTS(SELECT 1 FROM lifecycle_plan_completion WHERE plan_id=$4)`, first.ID, a.ID, a.CopyID, a.PlanID).Scan(&firstAbsent, &bUnknown, &bAbsent, &complete)
	if e != nil {
		return e
	}
	if !firstAbsent || !bUnknown || bAbsent || complete {
		return errors.New("M1 mixed checkpoint prerequisites differ")
	}
	// C-only native retention/hold GETs do not reveal B's outcome. Deliberately
	// discard the old namespace cache; the C gate is fail closed with inventory
	// UNKNOWN and an independently authoritative provider-retention denial.
	c := l.Scope.Copies[2]
	protection, e := inspector.InspectRetentionAndHold(ctx, c)
	if e != nil {
		return e
	}
	if protection.RetentionMode != "GOVERNANCE" || protection.RetainUntil == nil || !time.Now().UTC().Before(*protection.RetainUntil) {
		return errors.New("M1-C retention is not currently active")
	}
	deps.Protection[c.ID] = protection
	delete(deps.Inventories, c.Namespace)
	var approval uuid.UUID
	if e = pool.QueryRow(ctx, `SELECT approval_id FROM lifecycle_disposition_dispatch WHERE id=$1`, a.DispatchID).Scan(&approval); e != nil {
		return e
	}
	gate, e := disposition.DispatchContentDisposition(ctx, lc.DispatchContentDispositionCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: execution + ":M1-C-before-B-lookup"}, PlanID: a.PlanID, CopyID: c.ID, ApprovalID: approval})
	if e != nil || gate.Admitted || gate.AttemptID != uuid.Nil {
		return errors.New("M1-C retention gate did not block")
	}
	return save("/run/output/M1-mixed-checkpoint.json", map[string]any{"execution": execution, "A_verified_absent": first.ID, "B_confirmed_unknown": a, "B_absence_receipt": false, "C_retained": c, "C_native_protection": protection, "C_gate": gate, "completion": 0, "before_any_B_lookup": true, "at": time.Now().UTC()})
}
