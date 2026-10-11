package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	madmin "github.com/minio/madmin-go/v3"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Only fresh schema64 runs. Original DELETE scopes are never enabled here.
func recoveryFixtureStep(ctx context.Context, mode string) error {
	c, p, e := load()
	if e != nil {
		return e
	}
	cs, e := copies(c)
	if e != nil {
		return e
	}
	pool, e := pgxpool.New(ctx, c.PostgresDSN)
	if e != nil {
		return errors.New("database unavailable")
	}
	defer pool.Close()
	var schema int
	if e = pool.QueryRow(ctx, `SELECT max(version) FROM schema_migration`).Scan(&schema); e != nil || schema != 64 {
		return errors.New("fresh schema64 required; do not upgrade retained environments")
	}
	router, e := routing.NewRouter(false)
	if e != nil {
		return e
	}
	outbox.ConfigureAppendObligation(router)
	defer outbox.ConfigureAppendObligation(nil)
	o := lc.NewOccupancyService(transaction.NewManager(pool), &infrastructure.PostgresRepository{}, authority{}, nil, nil)
	proofs := &cache{proofs: map[uuid.UUID]lc.CopyProtection{}}
	var ns uuid.UUID
	if e = pool.QueryRow(ctx, `SELECT id FROM lifecycle_fixture_namespace WHERE workspace_id=$1 AND namespace=$2`, c.WorkspaceID, namespaces[1]).Scan(&ns); e != nil {
		return e
	}
	slots := []lc.CopyTargetSlot{}
	for i, key := range keys {
		action := domain.RestoreCopy
		if i == 0 {
			action = domain.ArchiveCopy
		}
		slots = append(slots, lc.CopyTargetSlot{Slot: key, Key: key, Namespace: namespaces[1], NamespaceID: ns, Action: action})
	}
	ledger := &infrastructure.FileRestoreLedger{Path: "/run/witness/current.jsonl", WorkspaceID: c.WorkspaceID, DatasetVersionID: cs[0].DatasetVersionID, Epoch: p.Epoch}
	service := lc.NewCopyService(o, slots, proofs, ledger, p.Epoch)
	if mode == "copy-witness" {
		cut, e := service.ExportRestoreCheckpoint(ctx, c.WorkspaceID, cs[0].DatasetVersionID)
		if e != nil {
			return e
		}
		if e = ledger.Append(cut); e != nil {
			return e
		}
		return write(filepath.Join(c.OutputDirectory, "copy-witness-"+fmt.Sprint(cut.Sequence)+".json"), cut)
	}
	if mode == "copy-restrict" {
		r, e := o.RestrictContentUse(ctx, lc.RestrictContentUseCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "copy-recovery-current-restriction"}, DatasetVersionID: cs[0].DatasetVersionID, BasisRef: "controlled-current-restriction-after-restore"})
		if e != nil {
			return e
		}
		return write(filepath.Join(c.OutputDirectory, "copy-current-restriction.json"), r)
	}
	journal := lc.CopyCallJournal{Service: service, Metadata: lc.NewPostgresStorageCallJournal(o)}
	originals, e := storage.NewLifecycleInspector(storage.LifecycleInspectionConfig{Endpoint: c.Endpoint, AccessKey: c.InspectAccess, SecretKey: c.InspectSecret, WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: namespaces, Journal: journal})
	if e != nil {
		return e
	}
	if mode == "copy-final-readback" {
		objects := []map[string]any{}
		for _, cp := range cs {
			stat, e := originals.InspectExactCopy(ctx, cp)
			if e != nil {
				return e
			}
			pr, e := originals.InspectRetentionAndHold(ctx, cp)
			if e != nil {
				return e
			}
			objects = append(objects, map[string]any{"copy": cp, "stat_call_id": stat.CallID, "protection": pr})
		}
		inventories := []lc.NamespaceInventory{}
		for _, cp := range []domain.Copy{cs[0], cs[2]} {
			inv, e := originals.InspectNamespace(ctx, cp)
			if e != nil {
				return e
			}
			inventories = append(inventories, inv)
		}
		return write(filepath.Join(c.OutputDirectory, "copy-originals-final.json"), map[string]any{"objects": objects, "inventories": inventories, "native_delete": 0, "cleanup": 0})
	}
	action, key, source := domain.ArchiveCopy, keys[0], cs[0]
	if mode == "response-loss-restore" {
		action, key = domain.RestoreCopy, keys[1]
		if requested := os.Getenv("LIFECYCLE_ARCHIVE_TARGET"); requested != "" {
			found := false
			for _, allowed := range keys[1:] {
				if requested == allowed {
					key = requested
					found = true
				}
			}
			if !found {
				return lc.ErrStorageScope
			}
		}
		var archiveID uuid.UUID
		if e = pool.QueryRow(ctx, `SELECT op.id FROM lifecycle_copy_operation op JOIN lifecycle_copy_decision d ON d.operation_id=op.id WHERE op.workspace_id=$1 AND op.action='ARCHIVE' AND d.decision='ARCHIVED'`, c.WorkspaceID).Scan(&archiveID); e != nil {
			return e
		}
		archived, e := service.GetOperation(ctx, c.WorkspaceID, archiveID)
		if e != nil {
			return e
		}
		var version string
		if e = pool.QueryRow(ctx, `SELECT provider_version FROM lifecycle_fixture_copy WHERE id=$1`, archived.TargetCopyID).Scan(&version); e != nil {
			return e
		}
		source = archived.Materialized(version)
	}
	suffix := action
	if action == domain.RestoreCopy {
		suffix += "-" + filepath.Base(key)
	}
	marker := filepath.Join(c.OutputDirectory, "copy-producer-"+suffix+".json")
	if _, e = os.Stat(marker); !os.IsNotExist(e) {
		return errors.New("one-shot original producer already recorded; no resend")
	}
	var exists bool
	if e = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_copy_operation WHERE workspace_id=$1 AND target_namespace_id=$2 AND target_key=$3)`, c.WorkspaceID, ns, key).Scan(&exists); e != nil || exists {
		return errors.New("original target intent already exists; recover it, never resend")
	}
	if action == domain.ArchiveCopy {
		if _, e = service.BindFixtureCopyPolicy(ctx, lc.BindCopyPolicyCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "copy-recovery-policy"}, DatasetVersionID: source.DatasetVersionID, UseUntil: p.ExpiresAt, RestoreUntil: p.ExpiresAt}); e != nil {
			return e
		}
	}
	inspect, e := storage.NewLifecycleInspector(storage.LifecycleInspectionConfig{Endpoint: c.Endpoint, AccessKey: p.Access, SecretKey: p.Secret, WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: namespaces, Journal: journal})
	if e != nil {
		return e
	}
	pr, e := inspect.InspectRetentionAndHold(ctx, source)
	if e != nil {
		return e
	}
	proofs.proofs[source.ID] = pr
	op, e := service.PrepareCopy(ctx, lc.PrepareCopyCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "copy-recovery-produce-" + action + ":" + key}, SourceCopyID: source.ID, TargetSlot: key, BasisRef: "controlled-native-client-response-loss", ExpiresAt: p.ExpiresAt})
	if e != nil {
		return e
	}
	copier, e := storage.NewLifecycleCopyResponseLossFixture(storage.LifecycleCopyConfig{Endpoint: c.Endpoint, AccessKey: p.Access, SecretKey: p.Secret, WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: namespaces, Journal: journal})
	if e != nil {
		return e
	}
	_, call, copyErr := copier.CreateCopy(ctx, op)
	if !errors.Is(copyErr, lc.ErrStorageUnknown) || call == uuid.Nil {
		return errors.New("response loss fault did not yield durable UNKNOWN")
	}
	var work uuid.UUID
	if e = pool.QueryRow(ctx, `SELECT id FROM lifecycle_reconciliation_work WHERE copy_call_id=$1`, call).Scan(&work); e != nil {
		return e
	}
	if e = write(marker, map[string]any{"operation": op, "copy_call_id": call, "work_id": work, "native_copy_requests": 1, "native_delete_requests": 0, "fault": "NATIVE_COPY_CLIENT_RESPONSE_DISCARDED_AND_PROCESS_EXIT_BEFORE_BIND", "original_certainty": "UNKNOWN", "cleanup": 0}); e != nil {
		return e
	}
	os.Exit(93)
	return nil
}
func provisionReadOnlyRecovery(ctx context.Context, disable bool) error {
	c, p, e := load()
	if e != nil {
		return e
	}
	if p.RecoveryAccess == "" || p.RecoverySecret == "" {
		return errors.New("separate read-only recovery principal required")
	}
	admin, e := madmin.New(c.Endpoint, c.RootAccess, c.RootSecret, false)
	if e != nil {
		return e
	}
	admin.SetCustomTransport(adminTransport{&http.Transport{Proxy: nil, DisableKeepAlives: true, ForceAttemptHTTP2: false, ResponseHeaderTimeout: 10 * time.Second}})
	one := func(fn func(context.Context) error) error {
		return fn(context.WithValue(ctx, adminKey{}, &adminCall{}))
	}
	if disable {
		return one(func(ctx context.Context) error {
			return admin.SetUserStatus(ctx, p.RecoveryAccess, madmin.AccountDisabled)
		})
	}
	resources := []string{}
	for _, ns := range namespaces {
		for _, k := range []string{"fixture/a.csv", "fixture/b.csv"} {
			resources = append(resources, "arn:aws:s3:::"+ns+"/"+k)
		}
	}
	for _, key := range keys {
		resources = append(resources, "arn:aws:s3:::"+namespaces[1]+"/"+key)
	}
	policy, _ := json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": []any{
		map[string]any{"Effect": "Allow", "Action": []string{"s3:ListBucketVersions"}, "Resource": []string{"arn:aws:s3:::" + namespaces[0], "arn:aws:s3:::" + namespaces[1]}},
		map[string]any{"Effect": "Allow", "Action": []string{"s3:GetObjectVersion", "s3:GetObjectRetention"}, "Resource": resources, "Condition": map[string]any{"Null": map[string]string{"s3:versionid": "false"}}},
		map[string]any{"Effect": "Allow", "Action": []string{"s3:GetObjectLegalHold"}, "Resource": resources},
	}})
	name := "lifecycle307-copy-recovery-" + c.RunID.String()
	if e = one(func(ctx context.Context) error { return admin.AddCannedPolicy(ctx, name, policy) }); e != nil {
		return errors.New("read-only policy provisioning unknown; no retry")
	}
	if e = one(func(ctx context.Context) error { return admin.AddUser(ctx, p.RecoveryAccess, p.RecoverySecret) }); e != nil {
		return errors.New("read-only identity provisioning unknown; no retry")
	}
	if e = one(func(ctx context.Context) error {
		_, e := admin.AttachPolicy(ctx, madmin.PolicyAssociationReq{Policies: []string{name}, User: p.RecoveryAccess})
		return e
	}); e != nil {
		return errors.New("read-only policy attachment unknown; no retry")
	}
	return write(filepath.Join(c.OutputDirectory, "copy-recovery-iam-policy.json"), json.RawMessage(policy))
}
