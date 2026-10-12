package pilot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// No root/admin/COPY credentials or ability to enable a provider identity.
type CopyRecoveryConfig struct {
	RecoveryConfig
	Epoch, DatasetVersionID uuid.UUID
	LedgerPath              string
	ValidFrom               time.Time
}

func loadCopyRecovery(path string) (CopyRecoveryConfig, error) {
	var c CopyRecoveryConfig
	b, e := os.ReadFile(path)
	if e != nil || len(b) > 16<<10 {
		return c, errors.New("copy recovery configuration unreadable")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil {
		return c, errors.New("copy recovery configuration invalid")
	}
	var tail any
	if d.Decode(&tail) != io.EOF {
		return c, errors.New("copy recovery trailing data")
	}
	if !c.SyntheticOnly || c.RunID == uuid.Nil || c.WorkspaceID == uuid.Nil || c.ProviderInstanceID == uuid.Nil || c.IncarnationID == uuid.Nil || c.ActorID == uuid.Nil || c.Epoch == uuid.Nil || c.DatasetVersionID == uuid.Nil || !filepath.IsAbs(c.LedgerPath) || c.Endpoint != "127.0.0.1:9000" || c.InspectAccess == "" || c.InspectSecret == "" || c.Token == "" || len(c.ManifestSHA256) != 64 || c.ValidFrom.IsZero() || !c.ValidUntil.After(c.ValidFrom) || c.ValidUntil.Sub(c.ValidFrom) > time.Hour {
		return c, errors.New("copy recovery scope incomplete")
	}
	return c, nil
}

type copyRecoveryAuthority struct {
	path     string
	original CopyRecoveryConfig
}

func (a copyRecoveryAuthority) AuthorizeLifecycle(_ context.Context, _ pgx.Tx, w uuid.UUID, cap string, at time.Time) (principal.RequestPrincipal, error) {
	c, e := loadCopyRecovery(a.path)
	if e != nil {
		return principal.RequestPrincipal{}, principal.ErrNotConfigured
	}
	enabled := c.Enabled
	c.Enabled = a.original.Enabled
	if c != a.original || !enabled || at.Before(c.ValidFrom) || !at.Before(c.ValidUntil) {
		return principal.RequestPrincipal{}, principal.ErrCapabilityDenied
	}
	r, e := principal.NewStaticResolver(true, c.Token, "lifecycle-copy-recovery:"+c.RunID.String(), c.ActorID.String(), []string{c.WorkspaceID.String()}, []string{lc.CapabilityReconcileCopy, lc.CapabilityInspectStorage, lc.CapabilityRestoreAdmission})
	if e != nil {
		return principal.RequestPrincipal{}, e
	}
	req, _ := http.NewRequest(http.MethodPost, "http://fixture.invalid/internal", nil)
	req.Header.Set("Authorization", "Bearer "+a.original.Token)
	return r.Resolve(req, w, cap)
}

type noCachedProtection struct{}

func (noCachedProtection) CurrentCopyProtection(context.Context, pgx.Tx, domain.Copy, time.Time) (lc.CopyProtection, error) {
	return lc.CopyProtection{}, lc.ErrStorageUnknown
}
func BuildCopyRecoveryAt(path string, pool *pgxpool.Pool, locker lc.RecoveryLocker, q lc.RecoveryEnqueuer) (*lc.RecoveryService, error) {
	return BuildCopyRecoveryWithPortAt(path, pool, locker, q, nil)
}

// Acceptance may wrap the same read-only Port with a finite process-exit barrier.
func BuildCopyRecoveryWithPortAt(path string, pool *pgxpool.Pool, locker lc.RecoveryLocker, q lc.RecoveryEnqueuer, wrap func(lc.CopyRecoveryPort) lc.CopyRecoveryPort) (*lc.RecoveryService, error) {
	c, e := loadCopyRecovery(path)
	if e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var ns uuid.UUID
	e = pool.QueryRow(ctx, `SELECT id FROM lifecycle_fixture_namespace WHERE workspace_id=$1 AND provider_instance_id=$2 AND incarnation_id=$3 AND namespace='dpp-lifecycle307-quarantine'`, c.WorkspaceID, c.ProviderInstanceID, c.IncarnationID).Scan(&ns)
	if e != nil {
		return nil, lc.ErrStorageScope
	}
	var belongs bool
	if e = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_fixture_copy WHERE workspace_id=$1 AND dataset_version_id=$2)`, c.WorkspaceID, c.DatasetVersionID).Scan(&belongs); e != nil || !belongs {
		return nil, lc.ErrStorageScope
	}
	o := lc.NewOccupancyService(transaction.NewManager(pool), &infrastructure.PostgresRepository{}, copyRecoveryAuthority{path, c}, nil, nil)
	ledger := &infrastructure.FileRestoreLedger{Path: c.LedgerPath, WorkspaceID: c.WorkspaceID, DatasetVersionID: c.DatasetVersionID, Epoch: c.Epoch}
	slots := []lc.CopyTargetSlot{{Slot: "archive", Action: domain.ArchiveCopy, Namespace: "dpp-lifecycle307-quarantine", NamespaceID: ns, Key: "archive/a.csv"}}
	for _, name := range []string{"online", "old-cut", "missing", "gap"} {
		slots = append(slots, lc.CopyTargetSlot{Slot: name, Action: domain.RestoreCopy, Namespace: "dpp-lifecycle307-quarantine", NamespaceID: ns, Key: "restore/" + name + "-a.csv"})
	}
	copies := lc.NewCopyService(o, slots, noCachedProtection{}, ledger, c.Epoch)
	journal := lc.CopyRecoveryCallJournal{Service: copies, Metadata: lc.NewCopyReconciliationMetadataJournal(o)}
	native, e := storage.NewLifecycleCopyRecovery(storage.LifecycleCopyConfig{Endpoint: c.Endpoint, AccessKey: c.InspectAccess, SecretKey: c.InspectSecret, WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: []string{"dpp-lifecycle307-online", "dpp-lifecycle307-quarantine"}, Journal: journal})
	if e != nil {
		return nil, e
	}
	inspector, e := storage.NewLifecycleInspector(storage.LifecycleInspectionConfig{Endpoint: c.Endpoint, AccessKey: c.InspectAccess, SecretKey: c.InspectSecret, WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: []string{"dpp-lifecycle307-online", "dpp-lifecycle307-quarantine"}, Journal: journal})
	if e != nil {
		return nil, e
	}
	var port lc.CopyRecoveryPort = native
	if wrap != nil {
		port = wrap(port)
	}
	return lc.NewRecoveryService(lc.NewExactExecution(lc.NewDispositionService(o, nil, nil, nil)), inspector, locker, q, []uuid.UUID{c.WorkspaceID}).WithCopyRecovery(lc.NewCopyRecoveryService(copies, port)), nil
}
