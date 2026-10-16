package main

// This composition deliberately has no deletion adapter. Its fixed v16 source
// inventory is checked independently of the old four-object reader profile.
import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
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

type remainingConfig struct {
	Base      config
	Group     string
	Batch     uuid.UUID
	NonDelete bool
}

func readRemaining(path string, dst any) error {
	b, err := os.ReadFile(path)
	if err != nil || len(b) > 256<<10 {
		return lc.ErrStorageScope
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(dst) != nil {
		return lc.ErrStorageScope
	}
	var tail any
	if d.Decode(&tail) != io.EOF {
		return lc.ErrStorageScope
	}
	return nil
}
func loadRemaining() (remainingConfig, error) {
	var r remainingConfig
	if err := readRemaining("/run/private/remaining.json", &r); err != nil {
		return r, err
	}
	c := r.Base
	at := time.Now().UTC()
	scope, err := pilot.RemainingScopeFor(r.Group)
	if err != nil || !r.NonDelete || r.Batch == uuid.Nil || !c.Enabled || c.RunID != scope.Run || c.WorkspaceID != scope.Workspace || c.ProviderInstanceID != scope.Provider || c.IncarnationID != scope.Incarnation || c.ActorID == uuid.Nil || c.Epoch == uuid.Nil || c.Endpoint != "127.0.0.1:9000" || c.Token == "" || c.Access == "" || c.Secret == "" || c.PostgresDSN == "" || c.ManifestSHA256 != scope.ManifestSHA256 || !filepath.IsAbs(c.OutputDirectory) || at.Before(c.StartedAt) || !at.Before(c.ExpiresAt) || c.ExpiresAt.Sub(c.StartedAt) > time.Hour {
		return r, lc.ErrStorageScope
	}
	return r, nil
}

type remainingAuthority struct{ original remainingConfig }

func (a remainingAuthority) Resolve(req *http.Request, w uuid.UUID, cap string) (principal.RequestPrincipal, error) {
	r, err := loadRemaining()
	if err != nil || !reflect.DeepEqual(r, a.original) {
		return principal.RequestPrincipal{}, principal.ErrCapabilityDenied
	}
	c := r.Base
	caps := []string{lc.CapabilityAdmitReader, lc.CapabilityTerminateReader, lc.CapabilityInspectOccupancy, lc.CapabilityInspectStorage, lc.CapabilityRestrictUse, lc.CapabilityCopyCustody, lc.CapabilityPreparePlan, lc.CapabilityApproveDisposition, lc.CapabilityDispatch, lc.CapabilityRevokeApproval, lc.CapabilityObserveAttempt, lc.CapabilityReconcileDisposition, lc.CapabilityVerifyAbsence}
	for _, m := range []string{"dataset", "entity", "workflow", "quality", "certification", "delivery"} {
		caps = append(caps, lc.ReferenceOwnerCapability(m))
	}
	resolver, err := principal.NewStaticResolver(true, c.Token, "remaining-fixture:"+r.Batch.String(), c.ActorID.String(), []string{c.WorkspaceID.String()}, caps)
	if err != nil {
		return principal.RequestPrincipal{}, err
	}
	return resolver.Resolve(req, w, cap)
}
func (a remainingAuthority) AuthorizeLifecycle(_ context.Context, _ pgx.Tx, w uuid.UUID, cap string, _ time.Time) (principal.RequestPrincipal, error) {
	req, _ := http.NewRequest("POST", "http://fixture.invalid/internal", nil)
	req.Header.Set("Authorization", "Bearer "+a.original.Base.Token)
	return a.Resolve(req, w, cap)
}
func (a remainingAuthority) AuthorizePlan(ctx context.Context, tx pgx.Tx, w uuid.UUID) (principal.RequestPrincipal, error) {
	return a.AuthorizeLifecycle(ctx, tx, w, lc.CapabilityPreparePlan, time.Now().UTC())
}
func (a remainingAuthority) CheckApproverAuthority(ctx context.Context, tx pgx.Tx, v domain.DispositionApproval, at time.Time) (domain.Check, error) {
	p, err := a.AuthorizeLifecycle(ctx, tx, v.WorkspaceID, lc.CapabilityApproveDisposition, at)
	if err != nil || v.ActorID != p.ActorID || v.PrincipalRef != p.Subject || v.BasisRef != "REMAINING_NON_DELETE_REHEARSAL:"+a.original.Batch.String() {
		return domain.Denied, nil
	}
	return domain.Satisfied, nil
}
func remainingCopies(r remainingConfig) ([]domain.Copy, error) {
	b, err := os.ReadFile("/run/private/manifest.json")
	s := sha256.Sum256(b)
	if err != nil || hex.EncodeToString(s[:]) != r.Base.ManifestSHA256 {
		return nil, lc.ErrStorageScope
	}
	var m manifest
	if json.Unmarshal(b, &m) != nil {
		return nil, lc.ErrStorageScope
	}
	scope, err := pilot.RemainingScopeFor(r.Group)
	if err != nil {
		return nil, err
	}
	var cs []domain.Copy
	for _, v := range m.Objects {
		cs = append(cs, domain.Copy{ID: v.CopyID, WorkspaceID: r.Base.WorkspaceID, DatasetVersionID: v.DatasetVersionID, ProviderInstanceID: r.Base.ProviderInstanceID, IncarnationID: r.Base.IncarnationID, Namespace: v.Namespace, Key: v.Key, ProviderVersion: v.Version, ContentSHA256: v.SHA, ByteSize: v.Bytes})
	}
	if !reflect.DeepEqual(cs, scope.Copies) {
		return nil, lc.ErrStorageScope
	}
	return cs, nil
}
func remainingCopyKey(cs []domain.Copy, key string) (domain.Copy, error) {
	for _, cp := range cs {
		if cp.Key == key {
			return cp, nil
		}
	}
	return domain.Copy{}, lc.ErrStorageScope
}
func remainingServices(r remainingConfig, pool *pgxpool.Pool, cs []domain.Copy) (*lc.OccupancyService, *lc.CopyService, *lc.FixtureReaderService, *lc.FixtureSubjectPins, error) {
	c := r.Base
	g := &gate{}
	o := lc.NewOccupancyService(transaction.NewManager(pool), &infrastructure.PostgresRepository{}, remainingAuthority{r}, g, nil)
	g.s = lc.NewCopyService(o, nil, noCopyProtection{}, nil, c.Epoch)
	native, err := storage.NewLifecycleReader(storage.LifecycleCopyConfig{Endpoint: c.Endpoint, AccessKey: c.Access, SecretKey: c.Secret, WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: namespaces, Journal: lc.PinnedReaderCallJournal{Service: o, Observations: lc.NewPostgresStorageCallJournal(o), SubjectGate: planeCurrentDeliveryGate(pool)}}, cs)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	reader := lc.NewFixtureReaderService(o, native)
	selected := map[uuid.UUID]uuid.UUID{}
	for _, cp := range cs {
		selected[cp.DatasetVersionID] = cp.ID
	}
	return o, g.s, reader, lc.NewFixtureSubjectPins(reader, selected), nil
}
func remainingStep(ctx context.Context, mode string) error {
	r, err := loadRemaining()
	if err != nil {
		return err
	}
	cs, err := remainingCopies(r)
	if err != nil {
		return err
	}
	if mode == "remaining-provision" || mode == "remaining-seal" {
		return remainingProvision(ctx, r, cs, mode == "remaining-seal")
	}
	if mode == "remaining-bind-outputs" {
		return remainingBindOutputs(ctx, r, cs)
	}
	if r.Base.RootAccess != "" || r.Base.RootSecret != "" {
		return errors.New("business runner must be root-free")
	}
	pool, err := pgxpool.New(ctx, r.Base.PostgresDSN)
	if err != nil {
		return err
	}
	defer pool.Close()
	router, err := routing.NewRouter(false)
	if err != nil {
		return err
	}
	outbox.ConfigureAppendObligation(router)
	defer outbox.ConfigureAppendObligation(nil)
	switch mode {
	case "remaining-setup":
		return remainingSetup(ctx, r, pool, cs)
	case "remaining-baseline":
		return remainingBaseline(ctx, r, pool, cs)
	case "remaining-business":
		return remainingBusiness(ctx, r, pool, cs)
	default:
		return errors.New("unknown remaining non-delete step")
	}
}
