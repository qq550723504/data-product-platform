// Package pilot wires only a finite, explicitly enabled synthetic environment.
// It is not a general storage executor or production authority implementation.
package pilot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

type RecoveryConfig struct {
	Enabled                                                        bool
	SyntheticOnly                                                  bool
	RunID, WorkspaceID, ProviderInstanceID, IncarnationID, ActorID uuid.UUID
	Endpoint, InspectAccess, InspectSecret, Token, ManifestSHA256  string
	ValidUntil                                                     time.Time
}

func load(path string) (RecoveryConfig, error) {
	var c RecoveryConfig
	b, err := os.ReadFile(path)
	if err != nil || len(b) > 16<<10 {
		return c, errors.New("recovery configuration unreadable")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil {
		return c, errors.New("recovery configuration invalid")
	}
	var tail any
	if d.Decode(&tail) != io.EOF {
		return c, errors.New("recovery configuration trailing data")
	}
	if !c.SyntheticOnly || c.RunID == uuid.Nil || c.WorkspaceID == uuid.Nil || c.ProviderInstanceID == uuid.Nil || c.IncarnationID == uuid.Nil || c.ActorID == uuid.Nil || c.Endpoint != "127.0.0.1:9000" || c.InspectAccess == "" || c.InspectSecret == "" || c.Token == "" || len(c.ManifestSHA256) != 64 || c.ValidUntil.IsZero() {
		return c, errors.New("recovery configuration scope incomplete")
	}
	return c, nil
}

type authority struct {
	path, token string
	original    RecoveryConfig
}

func (a authority) AuthorizeLifecycle(_ context.Context, _ pgx.Tx, w uuid.UUID, cap string, at time.Time) (principal.RequestPrincipal, error) {
	c, err := load(a.path)
	if err != nil {
		return principal.RequestPrincipal{}, principal.ErrNotConfigured
	}
	expected := a.original
	enabled := c.Enabled
	c.Enabled = expected.Enabled
	if c != expected || !enabled || !at.Before(c.ValidUntil) {
		return principal.RequestPrincipal{}, principal.ErrCapabilityDenied
	}
	r, err := principal.NewStaticResolver(true, c.Token, "lifecycle-recovery:"+c.RunID.String(), c.ActorID.String(), []string{c.WorkspaceID.String()}, []string{lc.CapabilityReconcileDisposition, lc.CapabilityInspectStorage, lc.CapabilityVerifyAbsence})
	if err != nil {
		return principal.RequestPrincipal{}, err
	}
	req, _ := http.NewRequest(http.MethodPost, "http://fixture.invalid/internal", nil)
	req.Header.Set("Authorization", "Bearer "+a.token)
	return r.Resolve(req, w, cap)
}

// No configuration means a fail-closed task handler; it does not acknowledge a
// recovery request or grant native I/O. Main still registers the queue obligation.
func BuildRecovery(pool *pgxpool.Pool, locker lc.RecoveryLocker, q lc.RecoveryEnqueuer) (*lc.RecoveryService, error) {
	if copyPath := os.Getenv("LIFECYCLE_COPY_RECOVERY_CONFIG"); copyPath != "" {
		return BuildCopyRecoveryAt(copyPath, pool, locker, q)
	}
	path := os.Getenv("LIFECYCLE_RECOVERY_CONFIG")
	if path == "" {
		return nil, nil
	}
	return BuildRecoveryAt(path, pool, locker, q)
}
func BuildRecoveryAt(path string, pool *pgxpool.Pool, locker lc.RecoveryLocker, q lc.RecoveryEnqueuer) (*lc.RecoveryService, error) {
	e, inspector, c, err := BuildRecoveryInspectorAt(path, pool)
	if err != nil {
		return nil, err
	}
	return lc.NewRecoveryService(e, inspector, locker, q, []uuid.UUID{c.WorkspaceID}), nil
}

// The controlled acceptance composition can wrap this read-only Port for a
// process-crash barrier, while using the identical authority and native journal.
func BuildRecoveryInspectorAt(path string, pool *pgxpool.Pool) (*lc.ExactExecution, lc.StorageInspector, RecoveryConfig, error) {
	c, err := load(path)
	if err != nil {
		return nil, nil, c, err
	}
	repo := &infrastructure.PostgresRepository{}
	o := lc.NewOccupancyService(transaction.NewManager(pool), repo, authority{path, c.Token, c}, nil, nil)
	inspector, err := storage.NewLifecycleInspector(storage.LifecycleInspectionConfig{Endpoint: c.Endpoint, AccessKey: c.InspectAccess, SecretKey: c.InspectSecret, WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: []string{"dpp-lifecycle307-online", "dpp-lifecycle307-quarantine"}, Journal: lc.NewReconciliationStorageCallJournal(o)})
	if err != nil {
		return nil, nil, c, err
	}
	execution := lc.NewExactExecution(lc.NewDispositionService(o, nil, nil, nil))
	return execution, inspector, c, nil
}
