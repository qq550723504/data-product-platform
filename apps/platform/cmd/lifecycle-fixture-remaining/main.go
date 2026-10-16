// Finite candidate07 operator. The preparation/check path has no database or
// provider I/O. Sending requires a separate exact confirmation and fresh signed
// physical topology; there is no enable, credential provisioning or retry mode.
package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
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
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

const execution = "effa0385-9308-42eb-bd66-d0c379b6a989"
const profile = "candidate07/v17"

var errDenied = errors.New("remaining candidate authority denied; no database/provider I/O permitted")

type groupBinding struct {
	CaseHelpers, CaseConfigSHA256                                                                     map[string]string
	PolicyHeads                                                                                       map[string]uuid.UUID
	CheckpointCore, CheckpointNative                                                                  string
	ConfigSHA256                                                                                      map[string]string
	Group                                                                                             string
	Actor                                                                                             uuid.UUID
	Core, Native, Gateway, Network                                                                    string
	CoreDumpSHA256, GlobalsSHA256, NativeArchiveSHA256, IAMSHA256, GatewaySHA256, SealedGatewaySHA256 string
	Copies                                                                                            []domain.Copy
	Helpers                                                                                           map[string]string
}
type candidate struct {
	OwnerBinarySHA256                                                                                               string
	WirePublicKey                                                                                                   string
	NotBefore, NotAfter                                                                                             time.Time
	Profile, Execution, Source, SourceFreezeSHA256, ReaderBinarySHA256, SenderBinarySHA256, TargetDigest, PublicKey string
	Groups                                                                                                          []groupBinding
}
type config struct {
	RedisAddr                                                                                           string
	Group, PostgresDSN, InspectAccess, InspectSecret, DeleteAccess, DeleteSecret, Token, WirePrivateKey string
}
type permission struct {
	NotBefore                                                             time.Time
	Enabled                                                               bool
	Profile, Execution, CandidateSHA256, ConfirmationSHA256, BinarySHA256 string
	ConfirmedAt, ExpiresAt                                                time.Time
	MaxPerTarget, MaxTotal                                                int
}
type topology struct {
	ConfigSHA256                                                                                   string
	Execution, CandidateSHA256, Group, Stage, Helper, Core, Native, Gateway, Network, BinarySHA256 string
	At                                                                                             time.Time
	NoPorts, NoRoot, NoSocket, ReadOnly, NoRestart, Internal                                       bool
}
type signed struct{ Body, Signature []byte }

func digest(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func read(path string, v any) error {
	b, e := os.ReadFile(path)
	if e != nil || len(b) > 256<<10 {
		return errDenied
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return errDenied
	}
	var tail any
	if d.Decode(&tail) != io.EOF {
		return errDenied
	}
	return nil
}
func save(path string, v any) error {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if e = json.NewEncoder(f).Encode(v); e != nil {
		return e
	}
	return f.Sync()
}
func fixedTarget(group, stage string) (domain.Copy, error) {
	s, e := pilot.RemainingScopeFor(group)
	if e != nil {
		return domain.Copy{}, e
	}
	keys := map[string]string{"P1": "fixture/p1.csv", "P2": "fixture/p2.csv", "T1": "fixture/t1.csv", "T2": "fixture/t2.csv", "H2": "fixture/h2-a.csv", "M1A": "fixture/m1-a.csv", "M1B": "fixture/m1-b.csv"}
	for _, cp := range s.Copies {
		if cp.Key == keys[stage] {
			return cp, nil
		}
	}
	return domain.Copy{}, errDenied
}
func validateCandidate(c candidate) error {
	if c.NotBefore.Format(time.RFC3339) != pilot.RemainingNotBefore || c.NotAfter.Format(time.RFC3339) != pilot.RemainingNotAfter {
		return errDenied
	}
	if c.NotBefore.IsZero() || !c.NotAfter.After(c.NotBefore) || c.NotAfter.Sub(c.NotBefore) > 40*time.Minute {
		return errDenied
	}
	if c.Profile != profile || c.Execution != execution || len(c.Source) != 40 || len(c.SourceFreezeSHA256) != 64 || len(c.ReaderBinarySHA256) != 64 || len(c.SenderBinarySHA256) != 64 || len(c.TargetDigest) != 64 || len(c.Groups) != 3 {
		return errDenied
	}
	if raw, e := hex.DecodeString(c.OwnerBinarySHA256); e != nil || len(raw) != 32 {
		return errDenied
	}
	key, e := hex.DecodeString(c.PublicKey)
	if e != nil || len(key) != ed25519.PublicKeySize {
		return errDenied
	}
	wireKey, e := hex.DecodeString(c.WirePublicKey)
	if e != nil || len(wireKey) != ed25519.PublicKeySize || bytes.Equal(wireKey, key) {
		return errDenied
	}
	seen := map[string]bool{}
	for _, g := range c.Groups {
		s, e := pilot.RemainingScopeFor(g.Group)
		if e != nil || seen[g.Group] || g.Actor == uuid.Nil || len(g.Core) != 64 || len(g.Native) != 64 || len(g.Gateway) != 64 || len(g.Network) != 64 {
			return errDenied
		}
		seen[g.Group] = true
		for _, id := range []string{g.CheckpointCore, g.CheckpointNative} {
			if raw, e := hex.DecodeString(id); e != nil || len(raw) != 32 {
				return errDenied
			}
		}
		caseStages := map[string][]string{"business-races": {"renew", "business"}, "late-hold": {"late", "close-H1"}, "mixed": {"mixed", "mixed-b", "mixed-c"}}[g.Group]
		if len(g.CaseHelpers) != len(caseStages) || len(g.CaseConfigSHA256) != len(caseStages) {
			return errDenied
		}
		for _, stage := range caseStages {
			for _, value := range []string{g.CaseHelpers[stage], g.CaseConfigSHA256[stage]} {
				if raw, e := hex.DecodeString(value); e != nil || len(raw) != 32 {
					return errDenied
				}
			}
		}
		for _, h := range []string{g.CoreDumpSHA256, g.GlobalsSHA256, g.NativeArchiveSHA256, g.IAMSHA256, g.GatewaySHA256, g.SealedGatewaySHA256} {
			if raw, e := hex.DecodeString(h); e != nil || len(raw) != 32 {
				return errDenied
			}
		}
		found := map[uuid.UUID]bool{}
		keys := map[string]bool{}
		extra := 0
		for _, cp := range g.Copies {
			key := cp.Namespace + "/" + cp.Key
			if cp.Validate() != nil || found[cp.ID] || keys[key] || cp.WorkspaceID != s.Workspace || cp.ProviderInstanceID != s.Provider || cp.IncarnationID != s.Incarnation {
				return errDenied
			}
			found[cp.ID] = true
			keys[key] = true
			original := false
			for _, originalCopy := range s.Copies {
				if cp == originalCopy {
					original = true
				}
			}
			if !original {
				if g.Group != "business-races" || cp.Namespace != "dpp-lifecycle307-online" || (cp.Key != "fixture/remaining-standardized.csv" && cp.Key != "fixture/remaining-processed.csv") {
					return errDenied
				}
				extra++
			}
		}
		if len(g.Copies) != len(s.Copies)+extra || (g.Group == "business-races" && extra != 2) || (g.Group != "business-races" && extra != 0) {
			return errDenied
		}
		if g.Group == "business-races" {
			if len(g.PolicyHeads) != 6 {
				return errDenied
			}
			for _, cp := range g.Copies {
				if cp.Key != "fixture/control-hold.csv" && cp.Key != "fixture/control-retention.csv" && g.PolicyHeads[cp.DatasetVersionID.String()] == uuid.Nil {
					return errDenied
				}
			}
		} else if len(g.PolicyHeads) != 0 {
			return errDenied
		}
		for stage, id := range g.Helpers {
			if raw, e := hex.DecodeString(id); e != nil || len(raw) != 32 {
				return errDenied
			}
			if raw, e := hex.DecodeString(g.ConfigSHA256[stage]); e != nil || len(raw) != 32 {
				return errDenied
			}
		}
		expectedStages := []string{"wire"}
		for _, stage := range []string{"P1", "P2", "T1", "T2", "H2", "M1A", "M1B"} {
			if _, e := fixedTarget(g.Group, stage); e == nil {
				expectedStages = append(expectedStages, stage, "recover-"+stage)
			}
		}
		if len(g.Helpers) != len(expectedStages) || len(g.ConfigSHA256) != len(expectedStages) {
			return errDenied
		}
		for _, stage := range expectedStages {
			if g.Helpers[stage] == "" || g.ConfigSHA256[stage] == "" {
				return errDenied
			}
		}
		for _, cp := range s.Copies {
			if !found[cp.ID] {
				return errDenied
			}
		}
	}
	var targets []domain.Copy
	for _, group := range []string{"business-races", "late-hold", "mixed"} {
		for _, stage := range []string{"P1", "P2", "T1", "T2", "H2", "M1A", "M1B"} {
			if cp, e := fixedTarget(group, stage); e == nil {
				targets = append(targets, cp)
			}
		}
	}
	raw, _ := json.Marshal(targets)
	if digest(raw) != c.TargetDigest {
		return errDenied
	}
	return nil
}

type loaded struct {
	C             candidate
	G             groupBinding
	F             config
	P             permission
	CandidateHash string
	Scope         pilot.RemainingScope
}

func load(stage string) (loaded, error) {
	var l loaded
	// This is intentionally the first read. A sealed candidate cannot even open
	// a connection pool, import provider credentials or consume a send budget.
	if read("/run/private/permission.json", &l.P) != nil || !l.P.Enabled {
		return l, errDenied
	}
	raw, e := os.ReadFile("/run/private/candidate.json")
	if e != nil {
		return l, errDenied
	}
	l.CandidateHash = digest(raw)
	if read("/run/private/candidate.json", &l.C) != nil || validateCandidate(l.C) != nil || read("/run/private/operator.json", &l.F) != nil {
		return l, errDenied
	}
	p := l.P
	at := time.Now().UTC()
	if validateWindow(l.C, p, at) != nil {
		return l, errDenied
	}
	if p.Profile != profile || p.Execution != execution || p.CandidateSHA256 != l.CandidateHash || p.BinarySHA256 != l.C.SenderBinarySHA256 || p.MaxPerTarget != 1 || p.MaxTotal != 7 || p.ConfirmedAt.IsZero() || at.Before(p.ConfirmedAt) || !p.NotBefore.Equal(l.C.NotBefore) || !p.ExpiresAt.Equal(l.C.NotAfter) || at.Before(p.NotBefore) || !at.Before(p.ExpiresAt) {
		return l, errDenied
	}
	confirmation, e := os.ReadFile("/run/private/human-confirmation.txt")
	expected, e2 := os.ReadFile("/run/private/confirmation-template.txt")
	if e != nil || e2 != nil || !bytes.Equal(confirmation, expected) || digest(confirmation) != p.ConfirmationSHA256 || !bytes.Contains(confirmation, []byte(execution)) || !bytes.Contains(confirmation, []byte(l.CandidateHash)) {
		return l, errDenied
	}
	binary, e := os.Executable()
	if e != nil {
		return l, errDenied
	}
	b, e := os.ReadFile(binary)
	if e != nil || digest(b) != p.BinarySHA256 {
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
	operator, e := os.ReadFile("/run/private/operator.json")
	if e != nil || digest(operator) != l.G.ConfigSHA256[stage] {
		return l, errDenied
	}
	var proof signed
	if read("/run/private/topology-"+stage+".json", &proof) != nil {
		return l, errDenied
	}
	key, _ := hex.DecodeString(l.C.PublicKey)
	var t topology
	if !ed25519.Verify(key, proof.Body, proof.Signature) || json.Unmarshal(proof.Body, &t) != nil || t.Execution != execution || t.CandidateSHA256 != l.CandidateHash || t.Group != l.F.Group || t.Stage != stage || t.Helper != l.G.Helpers[stage] || len(t.Helper) != 64 || t.Core != l.G.Core || t.Native != l.G.Native || t.Gateway != l.G.Gateway || t.Network != l.G.Network || t.BinarySHA256 != p.BinarySHA256 || t.ConfigSHA256 != l.G.ConfigSHA256[stage] || !t.NoPorts || !t.NoRoot || !t.NoSocket || !t.ReadOnly || !t.NoRestart || !t.Internal || at.Before(t.At) || at.Sub(t.At) > time.Minute {
		return l, errDenied
	}
	return l, nil
}

func strictConfig(l loaded) error {
	dsn, e := url.Parse(l.F.PostgresDSN)
	if e != nil || dsn.Scheme != "postgres" || dsn.Host != "dpp-lifecycle307-"+l.Scope.Run.String()[:8]+"-postgres:5432" || dsn.Path != "/lifecycle307" || dsn.User == nil || dsn.User.Username() != "fixture" || dsn.RawQuery != "sslmode=disable" || l.F.Token == "" || l.F.InspectAccess != "inspect"+strings.ReplaceAll(l.Scope.Run.String(), "-", "")[:12] || l.F.InspectSecret == "" || l.F.DeleteAccess != "delete"+l.Scope.Run.String()[:8] {
		return errDenied
	}
	if password, ok := dsn.User.Password(); !ok || password == "" {
		return errDenied
	}
	return nil
}
func validateWindow(c candidate, p permission, at time.Time) error {
	if c.NotBefore.IsZero() || c.NotAfter.Sub(c.NotBefore) <= 0 || c.NotAfter.Sub(c.NotBefore) > 40*time.Minute || !p.NotBefore.Equal(c.NotBefore) || !p.ExpiresAt.Equal(c.NotAfter) || p.ConfirmedAt.IsZero() || at.Before(p.ConfirmedAt) || at.Before(c.NotBefore) || !at.Before(c.NotAfter) {
		return errDenied
	}
	return nil
}

type authority struct {
	stage    string
	original loaded
}

func (a authority) AuthorizeLifecycle(_ context.Context, _ pgx.Tx, w uuid.UUID, cap string, _ time.Time) (principal.RequestPrincipal, error) {
	l, e := load(a.stage)
	if e != nil || !reflect.DeepEqual(l, a.original) {
		return principal.RequestPrincipal{}, errDenied
	}
	caps := []string{lc.CapabilityDispatch, lc.CapabilityInspectStorage, lc.CapabilityVerifyAbsence, lc.CapabilityReconcileDisposition}
	resolver, e := principal.NewStaticResolver(true, l.F.Token, "remaining-fixture:"+execution, l.G.Actor.String(), []string{l.Scope.Workspace.String()}, caps)
	if e != nil {
		return principal.RequestPrincipal{}, e
	}
	req, _ := http.NewRequest("POST", "http://fixture.invalid/internal", nil)
	req.Header.Set("Authorization", "Bearer "+l.F.Token)
	return resolver.Resolve(req, w, cap)
}
func (a authority) CheckApproverAuthority(ctx context.Context, tx pgx.Tx, v domain.DispositionApproval, at time.Time) (domain.Check, error) {
	p, e := a.AuthorizeLifecycle(ctx, tx, v.WorkspaceID, lc.CapabilityDispatch, at)
	if e != nil || v.ActorID != p.ActorID || v.PrincipalRef != p.Subject || v.BasisRef != "REMAINING_NON_DELETE_REHEARSAL:"+execution {
		return domain.Denied, nil
	}
	return domain.Satisfied, nil
}

type journal struct {
	exact     *lc.ExactExecution
	authority authority
}

func (j journal) ClaimExactSend(ctx context.Context, a domain.ProviderAttempt, c domain.Copy) error {
	if _, e := load(j.authority.stage); e != nil {
		return e
	}
	// Burn a durable per-target slot before the DB claim. A failed/unknown claim
	// loses this slot conservatively; a fresh helper cannot retry another attempt.
	if e := save(filepath.Join("/run/budget", c.ID.String()+".json"), map[string]any{"execution": execution, "attempt": a, "copy": c, "at": time.Now().UTC()}); e != nil {
		return e
	}
	return j.exact.ClaimExactSend(ctx, a, c)
}
func (j journal) ObserveExactSend(ctx context.Context, p domain.ProviderObservation) error {
	return j.exact.ObserveExactSend(ctx, p)
}

func send(ctx context.Context, stage string) error {
	l, e := load(stage)
	if e != nil {
		return e
	}
	target, e := fixedTarget(l.F.Group, stage)
	if e != nil {
		return e
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
	manager := transaction.NewManager(pool)
	repo := &infrastructure.PostgresRepository{}
	auth := authority{stage, l}
	o := lc.NewOccupancyService(manager, repo, auth, nil, nil)
	deps := &pilot.RemainingDependencies{Execution: uuid.MustParse(execution), All: l.G.Copies, ValidUntil: l.P.ExpiresAt, Protection: map[uuid.UUID]lc.CopyProtection{}, Inventories: map[string]lc.NamespaceInventory{}}
	disposition := lc.NewDispositionService(o, auth, deps, nil)
	exact := lc.NewExactExecution(disposition)
	inspector, e := storage.NewLifecycleInspector(storage.LifecycleInspectionConfig{Endpoint: "127.0.0.1:9000", AccessKey: l.F.InspectAccess, SecretKey: l.F.InspectSecret, WorkspaceID: l.Scope.Workspace, ProviderInstanceID: l.Scope.Provider, IncarnationID: l.Scope.Incarnation, Namespaces: []string{"dpp-lifecycle307-online", "dpp-lifecycle307-quarantine"}, Journal: lc.NewPostgresStorageCallJournal(o)})
	if e != nil {
		return e
	}
	var ids []uuid.UUID
	rows, e := pool.Query(ctx, `SELECT a.id FROM lifecycle_provider_attempt a JOIN lifecycle_disposition_dispatch d ON d.id=a.dispatch_id JOIN lifecycle_disposition_approval p ON p.id=d.approval_id WHERE a.workspace_id=$1 AND a.copy_id=$2 AND p.basis_ref=$3 AND p.principal_ref=$4`, l.Scope.Workspace, target.ID, "REMAINING_NON_DELETE_REHEARSAL:"+execution, "remaining-fixture:"+execution)
	if e != nil {
		return e
	}
	for rows.Next() {
		var id uuid.UUID
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil || len(ids) != 1 {
		return errors.New("exactly one fresh case dispatch required")
	}
	var attempt domain.ProviderAttempt
	if e = manager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		attempt, err = repo.GetAttempt(ctx, tx, l.Scope.Workspace, ids[0], false)
		return err
	}); e != nil {
		return e
	}
	caseName := stage
	if stage == "M1A" || stage == "M1B" {
		caseName = "M1"
	}
	matched := false
	for _, p := range l.Scope.Plans {
		if p.Case == caseName && p.ID == attempt.PlanID {
			matched = true
		}
	}
	if !matched {
		return errors.New("attempt is not in the frozen case plan")
	}
	if stage == "M1B" {
		first, _ := fixedTarget(l.F.Group, "M1A")
		var absent bool
		if e = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE workspace_id=$1 AND copy_id=$2)`, l.Scope.Workspace, first.ID).Scan(&absent); e != nil || !absent {
			return errors.New("M1A absence must commit before M1B")
		}
	}
	protection, e := inspector.InspectRetentionAndHold(ctx, target)
	if e != nil {
		return e
	}
	inventory, e := inspector.InspectNamespace(ctx, target)
	if e != nil {
		return e
	}
	deps.Protection[target.ID] = protection
	deps.Inventories[target.Namespace] = inventory
	deleter, e := storage.NewLifecycleDeleter(storage.LifecycleDeleteConfig{Endpoint: "127.0.0.1:9002", AccessKey: l.F.DeleteAccess, SecretKey: l.F.DeleteSecret, Targets: []domain.Copy{target}, Journal: journal{exact, auth}})
	if e != nil {
		return e
	}
	sendErr := deleter.DeleteExactCopy(ctx, attempt, target)
	var certainty, outcome string
	if e = pool.QueryRow(ctx, `SELECT certainty,outcome FROM lifecycle_provider_observation WHERE attempt_id=$1 ORDER BY observed_at DESC,id DESC LIMIT 1`, attempt.ID).Scan(&certainty, &outcome); e != nil {
		return e
	}
	if e = save("/run/output/"+stage+"-returned.json", map[string]any{"execution": execution, "target": target, "attempt": attempt, "certainty": certainty, "outcome": outcome, "send_error": sendErr != nil, "automatic_resend": false, "automatic_recovery": false}); e != nil {
		return e
	}
	if certainty != domain.InvocationConfirmed {
		return lc.ErrStorageUnknown
	}
	if stage == "H2" || stage == "M1B" {
		if outcome != domain.ProviderUnknown {
			return lc.ErrStorageContract
		}
		if stage == "M1B" {
			return mixedCheckpoint(ctx, pool, l, attempt, disposition, inspector, deps)
		}
		return nil
	}
	return sendErr
}
func main() {
	stage := flag.String("stage", "", "fixed case or wire")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	var e error
	if *stage == "check-candidate" {
		var c candidate
		e = read("/run/private/candidate.json", &c)
		if e == nil {
			e = validateCandidate(c)
		}
		if e == nil {
			path, err := os.Executable()
			if err != nil {
				e = err
			} else if raw, err := os.ReadFile(path); err != nil || digest(raw) != c.SenderBinarySHA256 {
				e = errDenied
			}
		}
		if e == nil {
			fmt.Println("PASS_FROZEN_CANDIDATE_OFFLINE; database/provider I/O=0")
		}
	} else if *stage == "check-disabled" {
		_, e = load("P1")
		if e != nil {
			fmt.Println("SEALED: authority denied before database/provider I/O; DELETE=0")
			return
		}
		e = errors.New("candidate unexpectedly armed")
	} else if strings.HasPrefix(*stage, "recover-") && knownCase(strings.TrimPrefix(*stage, "recover-")) {
		e = recoverExact(ctx, *stage)
	} else if *stage == "wire" {
		e = wire(ctx)
	} else if knownCase(*stage) {
		e = send(ctx, *stage)
	} else {
		e = errDenied
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}

func knownCase(stage string) bool {
	switch stage {
	case "P1", "P2", "T1", "T2", "H2", "M1A", "M1B":
		return true
	}
	return false
}
