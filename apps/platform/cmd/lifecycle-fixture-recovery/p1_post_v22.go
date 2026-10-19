package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"os"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	lifecyclequeue "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/transport/queue"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	platformqueue "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/queue"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

const p1Copy = "5a4c9789-3e67-4e54-ab58-128d111cbd65"
const p1Attempt = "009f4317-8855-4f99-80ec-cafc225f654e"

type p1AuditConfig struct {
	AuditID                                                       uuid.UUID
	Purpose, PostgresDSN, RedisAddr, RecoveryConfig, BinarySHA256 string
	Core, Native                                                  string
	NotBefore, ExpiresAt                                          time.Time
	MaxProviderDeletes                                            int
}

func p1Digest(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func validateP1Audit(c p1AuditConfig, r pilot.RecoveryConfig, at time.Time) error {
	s, _ := pilot.RemainingScopeFor("business-races")
	u, err := url.Parse(c.PostgresDSN)
	if err != nil || u.User == nil {
		return lc.ErrStorageScope
	}
	password, hasPassword := u.User.Password()
	if u.Scheme != "postgres" || u.Hostname() != "dpp-lifecycle307-808a6960-postgres" || u.Port() != "5432" || u.Path != "/lifecycle307" || u.User.Username() != "fixture" || !hasPassword || password == "" || u.RawQuery != "sslmode=disable" || u.Fragment != "" {
		return lc.ErrStorageScope
	}
	parsed, err := pgxpool.ParseConfig(c.PostgresDSN)
	if err != nil {
		return lc.ErrStorageScope
	}
	pc := parsed.ConnConfig
	if pc.Host != u.Hostname() || pc.Port != 5432 || pc.Database != "lifecycle307" || pc.User != "fixture" || pc.Password != password || pc.TLSConfig != nil || len(pc.Fallbacks) != 0 {
		return lc.ErrStorageScope
	}
	if c.AuditID == uuid.Nil || c.AuditID.String() == pilot.RemainingExecution || c.Purpose != "POST_V22_P1_READONLY_RECONCILIATION" || c.MaxProviderDeletes != 0 ||
		c.Core != "9bc6e9ec894f9fecb1566940d9b4680e0a9ddee52f31f9aec1f45afdb35c7ab0" || c.Native != "f16efe4cfa33694dc57b6cc2afe0a372379abce29846ba523ddd1dce455f43ce" ||
		c.PostgresDSN == "" || c.RedisAddr != "dpp307-p1-readonly-"+c.AuditID.String()[:8]+"-redis:6379" || c.RecoveryConfig != "/run/private/recovery.json" || len(c.BinarySHA256) != 64 ||
		c.NotBefore.IsZero() || !c.ExpiresAt.After(c.NotBefore) || c.ExpiresAt.Sub(c.NotBefore) > 40*time.Minute || at.Before(c.NotBefore) || !at.Before(c.ExpiresAt) ||
		!r.Enabled || !r.SyntheticOnly || r.RunID != c.AuditID || r.WorkspaceID != s.Workspace || r.ProviderInstanceID != s.Provider || r.IncarnationID != s.Incarnation ||
		r.ActorID.String() != "033a56ad-99c2-43e3-89e0-a749979a8644" || r.Endpoint != "127.0.0.1:9000" || r.InspectAccess != "inspect808a69606c36" || r.InspectSecret == "" || r.Token == "" || r.ManifestSHA256 != s.ManifestSHA256 || !r.ValidUntil.Equal(c.ExpiresAt) {
		return lc.ErrStorageScope
	}
	return nil
}

// A selected handler alone is insufficient: the dispatcher must filter the
// exact newly requested event before claiming any other historical obligation.
type p1SelectedEnqueuer struct {
	workspace, work uuid.UUID
	queue           *lifecyclequeue.Client
}

func (s p1SelectedEnqueuer) EnqueueRecovery(ctx context.Context, w, id uuid.UUID) error {
	if w != s.workspace || id != s.work {
		return lc.ErrStorageScope
	}
	return s.queue.EnqueueRecovery(ctx, w, id)
}

type p1NativeReceipt struct {
	Present []struct {
		Head       lc.ExactCopyInspection
		Protection lc.CopyProtection
	}
	Inventories []lc.NamespaceInventory
}

func p1NativePrecheck(ctx context.Context, inspector lc.StorageInspector, copies []lc.CopyProtection) (p1NativeReceipt, error) {
	var receipt p1NativeReceipt
	if len(copies) != 8 {
		return receipt, lc.ErrStorageScope
	}
	expected := map[string]map[string]domain.Copy{}
	subjects := map[string]domain.Copy{}
	foundP1 := false
	for _, v := range copies {
		c := v.Copy
		if c.Validate() != nil {
			return receipt, lc.ErrStorageScope
		}
		if expected[c.Namespace] == nil {
			expected[c.Namespace] = map[string]domain.Copy{}
		}
		subjects[c.Namespace] = c
		if c.ID.String() == p1Copy {
			foundP1 = true
			continue
		}
		expected[c.Namespace][c.Key+"\x00"+c.ProviderVersion] = c
		head, err := inspector.InspectExactCopy(ctx, c)
		if err != nil {
			return receipt, err
		}
		got, err := inspector.InspectRetentionAndHold(ctx, c)
		if err != nil {
			return receipt, err
		}
		if got.LegalHold != v.LegalHold || got.RetentionMode != v.RetentionMode || !reflect.DeepEqual(got.RetainUntil, v.RetainUntil) {
			return receipt, lc.ErrStorageContract
		}
		receipt.Present = append(receipt.Present, struct {
			Head       lc.ExactCopyInspection
			Protection lc.CopyProtection
		}{head, got})
	}
	if !foundP1 {
		return receipt, lc.ErrStorageScope
	}
	for ns, c := range subjects {
		inv, err := inspector.InspectNamespace(ctx, c)
		if err != nil {
			return receipt, err
		}
		if len(inv.Versions) != len(expected[ns]) {
			return receipt, lc.ErrStorageContract
		}
		for _, v := range inv.Versions {
			cp, ok := expected[ns][v.Key+"\x00"+v.Version]
			if !ok || v.DeleteMarker || v.ByteSize != cp.ByteSize {
				return receipt, lc.ErrStorageContract
			}
			delete(expected[ns], v.Key+"\x00"+v.Version)
		}
		if len(expected[ns]) != 0 {
			return receipt, lc.ErrStorageContract
		}
		receipt.Inventories = append(receipt.Inventories, inv)
	}
	return receipt, nil
}

func runP1PostV22(ctx context.Context, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) > 16<<10 {
		return lc.ErrStorageScope
	}
	var c p1AuditConfig
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil || d.Decode(new(any)) != io.EOF {
		return lc.ErrStorageScope
	}
	if c.RecoveryConfig != "/run/private/recovery.json" {
		return lc.ErrStorageScope
	}
	var r pilot.RecoveryConfig
	raw, err = os.ReadFile(c.RecoveryConfig)
	if err != nil {
		return err
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil || d.Decode(new(any)) != io.EOF {
		return lc.ErrStorageScope
	}
	if err = validateP1Audit(c, r, time.Now().UTC()); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	raw, err = os.ReadFile(self)
	if err != nil || p1Digest(raw) != c.BinarySHA256 {
		return lc.ErrStorageScope
	}
	// Frozen metadata is only the expected inventory for this new read-only
	// audit. It grants neither use nor deletion and is never edited after v22.
	raw, err = os.ReadFile("/run/private/bindings.json")
	if err != nil || p1Digest(raw) != "8e54b47e4a1f83c7ce66555ed6ce2f3ec66c962a394a8fa1feb84eeaccd82fc5" {
		return lc.ErrStorageScope
	}
	var bindings []struct {
		Group  string `json:"group"`
		Copies []struct {
			Copy        domain.Copy
			LegalHold   bool
			Retention   string
			RetainUntil *time.Time
		} `json:"copy_protection"`
	}
	if json.Unmarshal(raw, &bindings) != nil {
		return lc.ErrStorageScope
	}
	var protections []lc.CopyProtection
	for _, b := range bindings {
		if b.Group == "business-races" {
			for _, v := range b.Copies {
				protections = append(protections, lc.CopyProtection{Copy: v.Copy, LegalHold: v.LegalHold, RetentionMode: v.Retention, RetainUntil: v.RetainUntil})
			}
		}
	}
	pool, err := pgxpool.New(ctx, c.PostgresDSN)
	if err != nil {
		return err
	}
	defer pool.Close()
	var original bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_provider_attempt a JOIN lifecycle_provider_observation o ON o.attempt_id=a.id
 WHERE a.id=$1 AND a.workspace_id=$2 AND a.copy_id=$3 AND o.certainty='CONFIRMED' AND o.outcome='ACCEPTED')`, uuid.MustParse(p1Attempt), r.WorkspaceID, uuid.MustParse(p1Copy)).Scan(&original); err != nil {
		return err
	}
	if !original {
		return lc.ErrStorageScope
	}
	var prior bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE workspace_id=$1 AND copy_id=$2)`, r.WorkspaceID, uuid.MustParse(p1Copy)).Scan(&prior); err != nil {
		return err
	}
	if prior {
		return errors.New("P1 already has an immutable absence receipt; do not repeat this audit")
	}
	exact, inspector, _, err := pilot.BuildRecoveryInspectorAt(c.RecoveryConfig, pool)
	if err != nil {
		return err
	}
	nativeReceipt, err := p1NativePrecheck(ctx, inspector, protections)
	if err != nil {
		return err
	}
	if err = write("/run/output/native-precheck.json", map[string]any{"P1_exact_absent_in_full_namespace": true, "present_versions": 7, "fresh_native": nativeReceipt, "bytes_hash_reverification": "NOT_PERFORMED_HEAD_AND_PROTECTION_ONLY", "native_delete": 0}); err != nil {
		return err
	}
	router, err := routing.NewRouter(false)
	if err != nil {
		return err
	}
	outbox.ConfigureAppendObligation(router)
	defer outbox.ConfigureAppendObligation(nil)
	client := platformqueue.NewClient(platformqueue.Config{Addr: c.RedisAddr})
	defer client.Close()
	queue := lifecyclequeue.NewClient(client)
	service := lc.NewRecoveryService(exact, inspector, transaction.NewManager(pool), queue, []uuid.UUID{r.WorkspaceID})
	request, err := service.RequestRecovery(ctx, lc.RequestRecoveryCommand{CommandHeader: lc.CommandHeader{WorkspaceID: r.WorkspaceID, IdempotencyKey: "post-v22-p1:" + c.AuditID.String()}, AttemptID: uuid.MustParse(p1Attempt), BasisRef: "fresh-readonly-p1:" + c.AuditID.String()})
	if err != nil {
		return err
	}
	var event uuid.UUID
	if err = pool.QueryRow(ctx, `SELECT id FROM outbox_event WHERE aggregate_id=$1 AND event_type=$2`, request.ID, lc.ReconciliationRequestedEvent).Scan(&event); err != nil {
		return err
	}
	unsupported := func(context.Context, outbox.PublishedEvent) error { return lc.ErrStorageUnsupported }
	dispatcher, err := outbox.NewDispatcher(pool, outbox.Config{ConsumerName: "post-v22-p1:" + c.AuditID.String(), OnlyEventID: event, Logger: slog.New(slog.DiscardHandler)}, router,
		outbox.HandlerRegistration{Name: routing.HandlerExecutionQueue, Handle: unsupported}, outbox.HandlerRegistration{Name: routing.HandlerExternalPublicationQueue, Handle: unsupported},
		outbox.HandlerRegistration{Name: routing.HandlerLifecycleRecoveryQueue, Handle: lifecyclequeue.OutboxHandler(p1SelectedEnqueuer{r.WorkspaceID, request.ID, queue})})
	if err != nil {
		return err
	}
	if err = dispatcher.DispatchOnce(ctx); err != nil {
		return err
	}
	var state string
	if err = pool.QueryRow(ctx, `SELECT status FROM outbox_event WHERE id=$1`, event).Scan(&state); err != nil {
		return err
	}
	if state != "PUBLISHED" {
		return errors.New("selected P1 outbox obligation not published")
	}
	done := make(chan error, 1)
	mux := asynq.NewServeMux()
	mux.HandleFunc(lifecyclequeue.TaskRecover, func(x context.Context, t *asynq.Task) error {
		id, ok := asynq.GetTaskID(x)
		if !ok || id != "lifecycle/recovery/"+request.ID.String() {
			return lc.ErrStorageScope
		}
		e := lifecyclequeue.Handle(p1SelectedRecovery{r.WorkspaceID, request.ID, service})(x, t)
		select {
		case done <- e:
		default:
		}
		return e
	})
	server := platformqueue.NewServer(platformqueue.Config{Addr: c.RedisAddr})
	if err = server.Start(mux); err != nil {
		return err
	}
	defer server.Shutdown()
	select {
	case err = <-done:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	var receipt, completion uuid.UUID
	if err = pool.QueryRow(ctx, `SELECT a.id,c.id FROM lifecycle_copy_absence_receipt a JOIN lifecycle_plan_completion c ON c.plan_id=a.plan_id WHERE a.workspace_id=$1 AND a.copy_id=$2`, r.WorkspaceID, uuid.MustParse(p1Copy)).Scan(&receipt, &completion); err != nil {
		return err
	}
	return write("/run/output/P1-reconciled.json", map[string]any{"audit_id": c.AuditID, "original_attempt": p1Attempt, "copy": p1Copy, "work": request.ID, "event": event, "outbox": "PUBLISHED", "actual_asynq_worker": true, "absence_receipt": receipt, "plan_completion": completion, "original_outcome": "CONFIRMED ACCEPTED_UNCHANGED", "native_delete": 0})
}

type p1SelectedRecovery struct {
	workspace, work uuid.UUID
	service         *lc.RecoveryService
}

func (s p1SelectedRecovery) RunRecovery(ctx context.Context, w, id uuid.UUID) error {
	if w != s.workspace || id != s.work {
		return lc.ErrStorageScope
	}
	return s.service.RunRecovery(ctx, w, id)
}
