// One finite non-delete observer. No storage port, sender, queue or IAM secret.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

const purpose = "POST_V22_BUSINESS_NOT_SENT_CLOSURE"
const configPath = "/run/private/closure.json"

type config struct {
	Enabled                                                                    bool
	AuditID, WorkspaceID, ActorID                                              uuid.UUID
	Purpose, PostgresDSN, Core, BinarySHA256, PublicKey, EvidenceSHA256, Token string
	Owners                                                                     map[string]string
	NotBefore, ExpiresAt                                                       time.Time
	MaxProviderDeletes                                                         int
}

func read(path string, value any) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return errors.New("bounded observer input unreadable")
	}
	if len(b) > 32<<10 {
		return errors.New("bounded observer input too large")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e = d.Decode(value); e != nil {
		return errors.New("bounded observer input contract invalid")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("bounded observer input trailing content")
	}
	return nil
}

func validate(c config, at time.Time) error {
	u, e := url.Parse(c.PostgresDSN)
	if e != nil || u.User == nil {
		return lc.ErrStorageScope
	}
	password, present := u.User.Password()
	if u.Scheme != "postgres" || u.Hostname() != "dpp-lifecycle307-808a6960-postgres" || u.Port() != "5432" || u.Path != "/lifecycle307" || u.User.Username() != "fixture" || !present || password == "" || u.RawQuery != "sslmode=disable" || u.Fragment != "" {
		return lc.ErrStorageScope
	}
	pc, e := pgxpool.ParseConfig(c.PostgresDSN)
	if e != nil || pc.ConnConfig.Host != u.Hostname() || pc.ConnConfig.Port != 5432 || pc.ConnConfig.Database != "lifecycle307" || pc.ConnConfig.User != "fixture" || pc.ConnConfig.Password != password || pc.ConnConfig.TLSConfig != nil || len(pc.ConnConfig.Fallbacks) != 0 {
		return lc.ErrStorageScope
	}
	if !c.Enabled || c.AuditID == uuid.Nil || c.AuditID.String() == pilot.PostV22Execution || c.Purpose != purpose || c.Core != pilot.PostV22Core || c.MaxProviderDeletes != 0 ||
		c.WorkspaceID.String() != "a4aa2689-6e8f-4180-a567-dfb875407eac" || c.ActorID.String() != "033a56ad-99c2-43e3-89e0-a749979a8644" ||
		c.NotBefore.IsZero() || !c.ExpiresAt.After(c.NotBefore) || c.ExpiresAt.Sub(c.NotBefore) > 40*time.Minute || at.Before(c.NotBefore) || !at.Before(c.ExpiresAt) || len(c.Owners) == 0 || len(c.Token) < 32 {
		return lc.ErrStorageScope
	}
	for _, s := range []string{c.BinarySHA256, c.PublicKey, c.EvidenceSHA256} {
		if b, e := hex.DecodeString(s); e != nil || len(b) != 32 {
			return lc.ErrStorageScope
		}
	}
	return nil
}

type authority struct{ original config }

func (a authority) AuthorizeLifecycle(_ context.Context, _ pgx.Tx, w uuid.UUID, capability string, at time.Time) (principal.RequestPrincipal, error) {
	var current config
	if read(configPath, &current) != nil {
		return principal.RequestPrincipal{}, lc.ErrStorageScope
	}
	return a.resolve(current, w, capability, at)
}

func (a authority) resolve(current config, w uuid.UUID, capability string, at time.Time) (principal.RequestPrincipal, error) {
	if validate(current, at) != nil || !reflect.DeepEqual(current, a.original) || w != current.WorkspaceID ||
		(capability != lc.CapabilityRevokeApproval && capability != lc.CapabilityObserveAttempt) {
		return principal.RequestPrincipal{}, lc.ErrStorageScope
	}
	resolver, err := principal.NewStaticResolver(true, current.Token, "post-v22-dispatch-observer:"+current.AuditID.String(), current.ActorID.String(),
		[]string{current.WorkspaceID.String()}, []string{lc.CapabilityRevokeApproval, lc.CapabilityObserveAttempt})
	if err != nil {
		return principal.RequestPrincipal{}, err
	}
	req, _ := http.NewRequest(http.MethodPost, "http://fixture.invalid/internal", nil)
	req.Header.Set("Authorization", "Bearer "+a.original.Token)
	return resolver.Resolve(req, w, capability)
}

func writeResult(value any) error {
	f, e := os.OpenFile("/run/output/result.json", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if e = json.NewEncoder(f).Encode(value); e != nil {
		return e
	}
	return f.Sync()
}

func run(ctx context.Context, c config, results *[]map[string]any) error {
	if e := validate(c, time.Now().UTC()); e != nil {
		return e
	}
	ctx, cancel := context.WithDeadline(ctx, c.ExpiresAt)
	defer cancel()
	self, e := os.Executable()
	if e != nil {
		return e
	}
	binary, e := os.ReadFile(self)
	if e != nil {
		return e
	}
	sum := sha256.Sum256(binary)
	if hex.EncodeToString(sum[:]) != c.BinarySHA256 {
		return lc.ErrStorageContract
	}
	var proof pilot.SignedClosedReferenceExecutor
	if e = read("/run/private/proof.json", &proof); e != nil {
		return e
	}
	public, _ := hex.DecodeString(c.PublicKey)
	verifier := pilot.PostV22NoSendVerifier{PublicKey: public, Proof: proof, AuditID: c.AuditID, Owners: c.Owners, EvidenceSHA256: c.EvidenceSHA256}
	var body pilot.ClosedOldBusinessDispatches
	if e = json.Unmarshal(proof.Body, &body); e != nil {
		return lc.ErrStorageContract
	}
	ref := "post-v22-closed:" + body.Nonce.String()
	if _, e = verifier.Validate(time.Now().UTC(), ref); e != nil {
		return e
	}
	pool, e := pgxpool.New(ctx, c.PostgresDSN)
	if e != nil {
		return e
	}
	defer pool.Close()
	o := lc.NewOccupancyService(transaction.NewManager(pool), &infrastructure.PostgresRepository{}, authority{c}, nil, nil)
	service := lc.NewDispositionService(o, nil, nil, verifier)
	for _, d := range pilot.PostV22BusinessDispatches() {
		var revoked bool
		if e = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_approval_revocation WHERE approval_id=$1)`, d.Approval).Scan(&revoked); e != nil {
			return e
		}
		entry := map[string]any{"case": d.Case, "attempt": d.Attempt, "old_approval_already_revoked": revoked}
		*results = append(*results, entry)
		if !revoked {
			fact, err := service.RevokeDispositionApproval(ctx, lc.RevokeDispositionApprovalCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: c.AuditID.String() + ":" + d.Case + ":revoke"}, ApprovalID: d.Approval, BasisRef: "post-v22-no-send:" + c.AuditID.String()})
			if err != nil {
				return err
			}
			entry["new_approval_revocation"] = fact
		}
		fact, err := service.RecordProviderObservation(ctx, lc.RecordProviderObservationCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: c.AuditID.String() + ":" + d.Case + ":NOT_SENT"}, AttemptID: d.Attempt, ObservationRef: ref})
		if err != nil {
			return err
		}
		entry["NOT_SENT"] = fact
	}
	return nil
}

func main() {
	var c config
	results := []map[string]any{}
	e := read(configPath, &c)
	if e == nil {
		e = pilot.WithRecoveryRouting(func(_ *outbox.Router) error { return run(context.Background(), c, &results) })
	}
	result := map[string]any{"audit_id": c.AuditID, "cases": results, "native_delete": 0, "pins_released": 0, "budgets_refunded": 0, "completed": e == nil}
	if e != nil {
		result["original_error"] = e.Error()
	}
	if writeErr := writeResult(result); writeErr != nil {
		fmt.Fprintln(os.Stderr, "result persistence failed:", writeErr)
		os.Exit(1)
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
