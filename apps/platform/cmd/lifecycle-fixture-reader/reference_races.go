// Non-delete business races. This composition has no DELETE adapter, credentials,
// send claim or worker that can send DELETE. Database dispatch is admission only.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	certificationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/application"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	deliveryapp "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/application"
	entityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/application"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	parkindicator "github.com/qq550723504/data-product-platform/apps/platform/internal/industrypack/park/indicator"
	pinnedworkflow "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/adapters/workflow"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/contentreference"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	qualityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/application"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
	native "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/native"
	workflowqueue "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/transport/queue"
	"io"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"sync"
	"time"
)

type referenceAuthority struct{ authority }

func (a referenceAuthority) AuthorizePlan(ctx context.Context, tx pgx.Tx, w uuid.UUID) (principal.RequestPrincipal, error) {
	return a.AuthorizeLifecycle(ctx, tx, w, lc.CapabilityPreparePlan, time.Now().UTC())
}
func (a referenceAuthority) CheckApproverAuthority(ctx context.Context, tx pgx.Tx, v domain.DispositionApproval, at time.Time) (domain.Check, error) {
	p, _, e := a.current(ctx, tx, v.WorkspaceID)
	if e != nil || p.ActorID != v.ActorID || p.Subject != v.PrincipalRef || v.BasisRef != "CONTROLLED_NON_DELETE_ADMISSION_EXERCISE" {
		return domain.Denied, nil
	}
	return domain.Satisfied, nil
}
func (a referenceAuthority) current(ctx context.Context, tx pgx.Tx, w uuid.UUID) (principal.RequestPrincipal, time.Time, error) {
	at := time.Now().UTC()
	p, e := a.AuthorizeLifecycle(ctx, tx, w, lc.CapabilityApproveDisposition, at)
	return p, at, e
}

type referenceDependencies struct {
	c           config
	all         []domain.Copy
	protection  map[uuid.UUID]lc.CopyProtection
	inventories map[string]lc.NamespaceInventory
}

func (d *referenceDependencies) EvaluateDisposalDependencies(ctx context.Context, tx pgx.Tx, r lc.DisposalDependencyRequest) (lc.DisposalDependencies, error) {
	v := lc.DisposalDependencies{WorkspaceID: r.Copy.WorkspaceID, PlanID: r.Plan.ID, CopyID: r.Copy.ID, PlanHash: r.Plan.ContentHash, ActorID: r.Caller.ActorID, CheckedAt: r.AsOf, DependencyRevision: r.DependencyRevision, InventoryComplete: domain.Unknown, ReferencesReleased: domain.Unknown, ProviderRetentionAllows: domain.Unknown, LedgerCurrent: domain.Unknown, WindowMode: domain.UseDeadline}
	var policy, event uuid.UUID
	var start, retain, due time.Time
	e := tx.QueryRow(ctx, `SELECT id,start_event_id,starts_at,retain_until,dispose_due_at FROM lifecycle_fixture_disposal_basis WHERE workspace_id=$1 AND run_id=$2 AND approved_plan_id=$3 AND approved_hash=$4`, d.c.WorkspaceID, d.c.RunID, r.Plan.ID, r.Plan.ContentHash).Scan(&policy, &event, &start, &retain, &due)
	if e != nil {
		return v, e
	}
	v.Retention = []domain.RetentionBoundary{{PolicyID: policy, StartEventID: event, StartsAt: start, RetainUntil: retain}}
	v.DisposalDueAt = due
	inv, ok := d.inventories[r.Copy.Namespace]
	p, pok := d.protection[r.Copy.ID]
	until := d.c.ExpiresAt
	if ok && pok {
		expected := map[string]domain.Copy{}
		for _, c := range d.all {
			if c.Namespace == r.Copy.Namespace {
				expected[c.Key+"\x00"+c.ProviderVersion] = c
			}
		}
		complete := len(inv.Versions) == len(expected)
		for _, x := range inv.Versions {
			c, found := expected[x.Key+"\x00"+x.Version]
			if !found || x.DeleteMarker || x.ByteSize != c.ByteSize {
				complete = false
			}
			delete(expected, x.Key+"\x00"+x.Version)
		}
		if complete && len(expected) == 0 && inv.WorkspaceID == r.Copy.WorkspaceID && inv.ProviderInstanceID == r.Copy.ProviderInstanceID && inv.IncarnationID == r.Copy.IncarnationID {
			v.InventoryComplete = domain.Satisfied
		}
		if p.Copy == r.Copy && !p.LegalHold && (p.RetentionMode == "NONE" || (p.RetainUntil != nil && !r.AsOf.Before(*p.RetainUntil))) {
			v.ProviderRetentionAllows = domain.Satisfied
		} else {
			v.ProviderRetentionAllows = domain.Denied
		}
		for _, at := range []time.Time{p.ObservedAt, inv.ObservedAt} {
			if at.After(r.AsOf) {
				return v, lc.ErrStorageContract
			}
			if cap := at.Add(time.Minute); cap.Before(until) {
				until = cap
			}
		}
		ids := append([]uuid.UUID{inv.CallID}, p.CallIDs...)
		var n int
		e = tx.QueryRow(ctx, `SELECT count(DISTINCT c.id) FROM lifecycle_storage_call c JOIN lifecycle_storage_observation o ON o.call_id=c.id WHERE c.workspace_id=$1 AND c.id=ANY($2) AND o.invoked_at IS NOT NULL AND o.failure='' AND o.http_status IN(200,400)`, d.c.WorkspaceID, ids).Scan(&n)
		if e != nil {
			return v, e
		}
		if n == 3 {
			v.LedgerCurrent = domain.Satisfied
		}
	}
	v.ValidUntil = &until
	v.ReferencesReleased, _, e = lc.InspectFixtureReferences(ctx, tx, r.Copy)
	return v, e
}

type referenceInputBarrier struct {
	pins             *lc.FixtureSubjectPins
	target           uuid.UUID
	entered, release chan struct{}
	once             sync.Once
}

func (b *referenceInputBarrier) ReadVersion(ctx context.Context, v uuid.UUID) (io.ReadCloser, error) {
	r, e := b.pins.ReadVersion(ctx, v)
	if e != nil {
		return nil, e
	}
	if v == b.target {
		b.once.Do(func() { close(b.entered) })
		select {
		case <-b.release:
		case <-ctx.Done():
			r.Close()
			return nil, ctx.Err()
		}
	}
	return r, nil
}

type referenceStreamBarrier struct {
	inner            *planeHTTPWriter
	entered, release chan struct{}
}

func (b *referenceStreamBarrier) Write(p []byte) (int, error) { return b.WriteContext(b.inner.ctx, p) }
func (b *referenceStreamBarrier) WriteContext(ctx context.Context, p []byte) (int, error) {
	n, e := b.inner.WriteContext(ctx, p[:128])
	if e != nil {
		return n, e
	}
	close(b.entered)
	select {
	case <-b.release:
	case <-ctx.Done():
		return n, ctx.Err()
	}
	rest, e := b.inner.WriteContext(ctx, p[128:])
	return n + rest, e
}

type referenceDemand struct {
	ref  contentreference.Reference
	fact uuid.UUID
	copy domain.Copy
	loss string
}

func referenceDemands(ctx context.Context, c config, pool *pgxpool.Pool, svc *lc.FixtureReferenceService, cp domain.Copy) ([]referenceDemand, error) {
	kinds := []string{"LINEAGE", "ENTITY_MATCH", "EXECUTION_INPUT", "EXECUTION_MAPPING", "QUALITY_RESULT", "QUALITY_ATTEMPT", "CERTIFICATION", "DELIVERY"}
	var demands []referenceDemand
	for _, kind := range kinds {
		spec, _ := contentreference.Lookup(kind)
		keys := "id,''"
		if kind == "EXECUTION_INPUT" {
			keys = "execution_id,input_name"
		}
		rows, e := pool.Query(ctx, "SELECT "+keys+" FROM "+pgx.Identifier{spec.Table}.Sanitize()+" WHERE "+pgx.Identifier{spec.Column}.Sanitize()+"=$1", cp.DatasetVersionID)
		if e != nil {
			return nil, e
		}
		var refs []contentreference.Reference
		for rows.Next() {
			ref := contentreference.Reference{Kind: kind}
			if e = rows.Scan(&ref.SubjectID, &ref.InputName); e != nil {
				rows.Close()
				return nil, e
			}
			refs = append(refs, ref)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
		for _, ref := range refs {
			key := cp.ID.String() + ":" + ref.Kind + ":" + ref.SubjectID.String() + ":" + ref.InputName
			fact, e := svc.DeclareFixtureReference(ctx, lc.DeclareFixtureReferenceCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "require-" + key}, CopyID: cp.ID, Reference: ref, BasisRef: "CONTROLLED_SYNTHETIC_OWNER_BYTE_REQUIREMENT"})
			if e != nil {
				return nil, e
			}
			demands = append(demands, referenceDemand{ref, fact.ID, cp, spec.Loss})
		}
	}
	return demands, nil
}
func releaseReference(ctx context.Context, c config, svc *lc.FixtureReferenceService, d referenceDemand) (lc.FactResult, error) {
	return svc.ReleaseFixtureReference(ctx, lc.ReleaseFixtureReferenceCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "release-" + d.fact.String()}, RequirementID: d.fact, Reference: d.ref, CopyID: d.copy.ID, BasisRef: "CONTROLLED_SYNTHETIC_OWNER_VERIFIED_STOP_AND_CAPABILITY_LOSS", AcknowledgedLoss: d.loss, OwnerVerifiedQuiescent: true})
}
func planeReferenceRaces(ctx context.Context, c config, pool *pgxpool.Pool, original []domain.Copy) error {
	if !c.ReferenceDemand {
		return lc.ErrStorageScope
	}
	// Consume this non-delete run before any new business/provider activity.
	// A partial failure is retained; it never silently resumes the same phase.
	binaryPath, e := os.Executable()
	if e != nil {
		return e
	}
	binary, e := os.ReadFile(binaryPath)
	if e != nil {
		return e
	}
	sum := sha256.Sum256(binary)
	settings := map[string]string{}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" || setting.Key == "vcs.modified" || setting.Key == "GOOS" || setting.Key == "GOARCH" {
				settings[setting.Key] = setting.Value
			}
		}
	}
	if e = write(c, "reference-races-start.json", map[string]any{"run_id": c.RunID, "binary_sha256": hex.EncodeToString(sum[:]), "build_settings": settings, "provider_delete_authorized": false, "actual_delete": "NOT_RUN", "started_at": time.Now().UTC()}); e != nil {
		return e
	}
	state, e := loadPlaneState(c)
	if e != nil {
		return e
	}
	cs, e := planeCopies(ctx, c, pool)
	if e != nil {
		return e
	}
	o, policy, reader, pins, e := planeServices(ctx, c, pool, cs)
	if e != nil {
		return e
	}
	// Setup binds the alpha/standardized producer; this distinct business slice
	// also declares beta's current bounded use policy before pin admission.
	if _, e = policy.BindFixtureCopyPolicy(ctx, lc.BindCopyPolicyCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "reference-beta-policy"}, DatasetVersionID: original[1].DatasetVersionID, UseUntil: c.ExpiresAt, RestoreUntil: c.ExpiresAt}); e != nil {
		return e
	}
	tx := transaction.NewManager(pool)
	wf := workflowinfra.NewPostgresRepository(pool)
	exec := workflowapp.NewExecutionService(tx, wf)
	writer, e := planeWriter(ctx, c, pool, o, pins, original, state)
	if e != nil {
		return e
	}
	engine := native.NewEngine("/run/repo/industry-packs", tx, datasetinfra.NewPostgresRepository(pool), entityinfra.NewPostgresRepository(pool), wf, writer, planeCache{}, parkindicator.NewCalculator()).WithInputReader(pins)
	handler := workflowqueue.NewHandler(exec, wf, &pinnedworkflow.PinnedEngine{Pins: pins, Inner: engine}).WithNativeExecutionLocker(tx)
	create := func(key string) (workflowdomain.Execution, error) {
		return exec.Create(ctx, workflowapp.CreateExecutionCommand{WorkspaceID: c.WorkspaceID, WorkflowVersionID: state.WorkflowVersionID, OutputDatasetID: state.OutputDatasetID, TargetPeriod: pilot.PlanePeriod, Inputs: []workflowdomain.InputBinding{{Name: "enterprise_raw", DatasetVersionID: original[0].DatasetVersionID}, {Name: "enterprise_resolution", DatasetVersionID: state.StandardizedVersionID}, {Name: "lease_raw", DatasetVersionID: original[1].DatasetVersionID}, {Name: "energy_raw", DatasetVersionID: original[1].DatasetVersionID}}, ActorID: &c.ActorID, IdempotencyKey: key})
	}
	run := func(h *workflowqueue.Handler, ex workflowdomain.Execution) error {
		b, _ := json.Marshal(map[string]any{"executionId": ex.ID})
		return h.Handle(ctx, asynq.NewTask(workflowqueue.TaskExecute, b))
	}
	baseline, e := create("reference-native-baseline")
	if e != nil {
		return e
	}
	if e = run(handler, baseline); e != nil {
		return e
	}
	completed, e := wf.GetExecution(ctx, baseline.ID)
	if e != nil || completed.Status != workflowdomain.ExecutionSucceeded {
		return errors.New("native baseline failed")
	}
	cs, e = planeCopies(ctx, c, pool)
	if e != nil {
		return e
	}
	// Refresh the registered inventory after actual STANDARDIZED/CURATED output.
	alpha, beta := original[0], original[3]
	targets := []domain.Copy{alpha, beta}
	deps := &referenceDependencies{c: c, all: cs, protection: map[uuid.UUID]lc.CopyProtection{}, inventories: map[string]lc.NamespaceInventory{}}
	inspector, e := storage.NewLifecycleInspector(storage.LifecycleInspectionConfig{Endpoint: c.Endpoint, AccessKey: c.InspectAccess, SecretKey: c.InspectSecret, WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: namespaces, Journal: lc.NewPostgresStorageCallJournal(o)})
	if e != nil {
		return e
	}
	refresh := func() error {
		for _, cp := range targets {
			p, e := inspector.InspectRetentionAndHold(ctx, cp)
			if e != nil {
				return e
			}
			inv, e := inspector.InspectNamespace(ctx, cp)
			if e != nil {
				return e
			}
			deps.protection[cp.ID] = p
			deps.inventories[cp.Namespace] = inv
		}
		return nil
	}
	if e = refresh(); e != nil {
		return e
	}
	ids := []uuid.UUID{}
	for _, cp := range cs {
		ids = append(ids, cp.ID)
	}
	plans := lc.NewPlanService(tx, &infrastructure.PostgresRepository{}, referenceAuthority{})
	inventory, e := plans.PrepareDeletePlan(ctx, lc.PrepareDeletePlanCommand{WorkspaceID: c.WorkspaceID, CopyIDs: ids, IdempotencyKey: "reference-inventory"})
	if e != nil {
		return e
	}
	plan, e := plans.PrepareDeletePlan(ctx, lc.PrepareDeletePlanCommand{WorkspaceID: c.WorkspaceID, CopyIDs: []uuid.UUID{alpha.ID, beta.ID}, IdempotencyKey: "reference-scope"})
	if e != nil {
		return e
	}
	dispose := lc.NewDispositionService(o, referenceAuthority{}, deps, nil)
	if _, e = lc.NewExactExecution(dispose).RegisterFixtureDisposalBasis(ctx, lc.FixtureDisposalBasisCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "reference-basis"}, RunID: c.RunID, InventoryPlanID: inventory.ID, ApprovedPlanID: plan.ID, InventoryHash: inventory.ContentHash, ApprovedHash: plan.ContentHash, BasisRef: "CONTROLLED_NON_DELETE_ADMISSION_EXERCISE"}); e != nil {
		return e
	}
	// Approval participates in current use admission, so admit the actual owners
	// first for the reader-first order. It is never authority to send DELETE.
	var approval lc.FactResult
	dispatch := func(cp domain.Copy, key string) (lc.DispatchResult, error) {
		return dispose.DispatchContentDisposition(ctx, lc.DispatchContentDispositionCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: key}, PlanID: plan.ID, ApprovalID: approval.ID, CopyID: cp.ID})
	}
	active, e := create("reference-active-processing")
	if e != nil {
		return e
	}
	queued, e := create("reference-queued-processing")
	if e != nil {
		return e
	}
	recoverable, e := create("reference-stale-recovery")
	if e != nil {
		return e
	}
	// Start before restriction; no lease reset or SQL status patch is used.
	recoverable, e = exec.StartWithReferenceCheck(ctx, recoverable.ID, "native:"+recoverable.ID.String(), "reference-recovery")
	if e != nil {
		return e
	}
	barrier := &referenceInputBarrier{pins: pins, target: alpha.DatasetVersionID, entered: make(chan struct{}), release: make(chan struct{})}
	blockedEngine := native.NewEngine("/run/repo/industry-packs", tx, datasetinfra.NewPostgresRepository(pool), entityinfra.NewPostgresRepository(pool), wf, writer, planeCache{}, parkindicator.NewCalculator()).WithInputReader(barrier)
	blockedHandler := workflowqueue.NewHandler(exec, wf, &pinnedworkflow.PinnedEngine{Pins: pins, Inner: blockedEngine}).WithNativeExecutionLocker(tx)
	processDone := make(chan error, 1)
	go func() { processDone <- run(blockedHandler, active) }()
	select {
	case <-barrier.entered:
	case e := <-processDone:
		return fmt.Errorf("native owner stopped before input barrier: %v", e)
	case <-ctx.Done():
		return ctx.Err()
	}
	current := planeCurrentDeliveryGate(pool)
	direct := deliveryapp.NewDirectDataService(tx, current.repo, current.gate, datasetinfra.NewPostgresRepository(pool)).WithTransferPins(pins)
	caller, e := authority{}.AuthorizeLifecycle(ctx, nil, c.WorkspaceID, lc.CapabilityAdmitReader, time.Now().UTC())
	if e != nil {
		return e
	}
	owned := lc.WithFixtureReaderCaller(ctx, caller)
	directCmd := deliveryapp.DirectDataCommand{WorkspaceID: c.WorkspaceID, DatasetVersionID: beta.DatasetVersionID, ProfileID: state.ProfileID, PrincipalRef: caller.Subject, EffectiveConsumerRef: pilot.PlaneConsumer, Purpose: pilot.PlanePurpose, Action: "READ", ScopeType: "ALL_RESOURCE", IdempotencyKey: "reference-issued-transfer"}
	issued, e := direct.Deliver(owned, directCmd)
	if e != nil || !issued.PayloadReady {
		return errors.New("real current delivery gate failed")
	}
	streamEntered, streamRelease := make(chan struct{}), make(chan struct{})
	streamDone := make(chan error, 1)
	var transfer lc.FixtureReadResult
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return e
	}
	srv := &http.Server{ReadHeaderTimeout: 3 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+c.Token {
			http.Error(w, "denied", 403)
			streamDone <- lc.ErrStorageDenied
			return
		}
		output := &referenceStreamBarrier{inner: &planeHTTPWriter{ResponseWriter: w, ctx: owned}, entered: streamEntered, release: streamRelease}
		result, e := reader.ReadPinnedTo(owned, c.WorkspaceID, issued.TransferPinID, output)
		transfer = result
		streamDone <- e
	})}
	defer srv.Close()
	go srv.Serve(l)
	payloadDone := make(chan error, 1)
	go func() {
		req, _ := http.NewRequestWithContext(ctx, "POST", "http://"+l.Addr().String()+"/fixture/deliver", nil)
		req.Header.Set("Authorization", "Bearer "+c.Token)
		response, e := http.DefaultClient.Do(req)
		if e == nil {
			defer response.Body.Close()
			b, readErr := io.ReadAll(response.Body)
			e = readErr
			if int64(len(b)) != beta.ByteSize {
				e = errors.New("slow TCP payload mismatch")
			}
		}
		payloadDone <- e
	}()
	select {
	case <-streamEntered:
	case <-ctx.Done():
		return ctx.Err()
	}
	approval, e = dispose.ApproveContentDisposition(ctx, lc.ApproveContentDispositionCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "reference-approval"}, PlanID: plan.ID, BasisRef: "CONTROLLED_NON_DELETE_ADMISSION_EXERCISE", ExpiresAt: c.ExpiresAt})
	if e != nil {
		return e
	}
	refs := lc.NewFixtureReferenceService(o, datasetapp.FixtureReferenceOwner{}, entityapp.FixtureReferenceOwner{}, workflowapp.FixtureReferenceOwner{}, qualityapp.FixtureReferenceOwner{}, certificationapp.FixtureReferenceOwner{}, deliveryapp.FixtureReferenceOwner{})
	var demands []referenceDemand
	for _, cp := range targets {
		list, e := referenceDemands(ctx, c, pool, refs, cp)
		if e != nil {
			return e
		}
		demands = append(demands, list...)
		if _, e = o.RestrictContentUse(ctx, lc.RestrictContentUseCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "reference-restrict-" + cp.ID.String()}, DatasetVersionID: cp.DatasetVersionID, BasisRef: "CONTROLLED_NON_DELETE_ADMISSION_EXERCISE"}); e != nil {
			return e
		}
	}
	for _, d := range demands {
		if _, e = releaseReference(ctx, c, refs, d); e == nil {
			return errors.New("live business pin released by assertion")
		}
	}
	for _, cp := range targets {
		result, e := dispatch(cp, "reference-active-"+cp.ID.String())
		if e != nil || result.Admitted {
			return errors.New("active business dispatch admitted")
		}
	}
	close(barrier.release)
	if e = <-processDone; e != nil {
		return e
	}
	failed, e := wf.GetExecution(ctx, active.ID)
	if e != nil || failed.Status != workflowdomain.ExecutionFailed || failed.OutputDatasetVersionID != nil {
		return errors.New("restricted in-flight processing accepted output")
	}
	// Cancellation is unnecessary: native scope and the synchronous HTTP writer
	// finish explicitly. A cancelled/crashed owner would retain its pin.
	close(streamRelease)
	if e = <-streamDone; e != nil {
		return e
	}
	if e = <-payloadDone; e != nil {
		return e
	}
	if !transfer.StopConfirmed || transfer.BytesWritten != beta.ByteSize {
		return errors.New("transfer stop unconfirmed")
	}
	for _, d := range demands {
		if _, e = releaseReference(ctx, c, refs, d); e != nil {
			return e
		}
	}
	if e = refresh(); e != nil {
		return e
	}
	var dispatches []lc.DispatchResult
	for _, cp := range targets {
		result, e := dispatch(cp, "reference-drained-"+cp.ID.String())
		if e != nil || !result.Admitted {
			return fmt.Errorf("drained native business dispatch blocked: %+v %v", result, e)
		}
		dispatches = append(dispatches, result)
	}
	before, e := count(ctx, pool, `SELECT count(*) FROM lifecycle_storage_call WHERE operation IN('READ_PINNED','PROCESSING_PUT')`)
	if e != nil {
		return e
	}
	if e = run(handler, queued); e != nil {
		return e
	}
	if e = run(handler, queued); e != nil {
		return e
	} // genuine queue replay
	if _, e = exec.Retry(ctx, workflowapp.RetryExecutionCommand{ExecutionID: queued.ID, IdempotencyKey: "reference-explicit-retry", ActorID: &c.ActorID}); e == nil {
		return errors.New("retry created late source references")
	}
	// Wait until the real recovery lease expires, without changing persisted time.
	if recoverable.StartedAt == nil {
		return errors.New("native recovery start time missing")
	}
	timer := time.NewTimer(time.Until(recoverable.StartedAt.Add(10*time.Minute + time.Second)))
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		return ctx.Err()
	}
	reconciler := workflowapp.NewNativeReconciler(tx, exec, wf, datasetinfra.NewPostgresRepository(pool), &pinnedworkflow.PinnedEngine{Pins: pins, Inner: engine})
	if e = reconciler.RunOnce(ctx); e != nil {
		return e
	}
	if _, e = direct.Deliver(owned, directCmd); !errors.Is(e, deliveryapp.ErrDirectDataReplayRequiresNewAttempt) {
		return errors.New("same-key direct replay reopened payload")
	}
	directCmd.IdempotencyKey = "reference-reservation-first-delivery"
	if next, e := direct.Deliver(owned, directCmd); e == nil && next.PayloadReady {
		return errors.New("reservation-first direct returned payload")
	}
	after, e := count(ctx, pool, `SELECT count(*) FROM lifecycle_storage_call WHERE operation IN('READ_PINNED','PROCESSING_PUT')`)
	if e != nil || after != before {
		return errors.New("reservation-first business caused new GET/output")
	}
	n, e := count(ctx, pool, `SELECT count(*) FROM lifecycle_reader_pin p WHERE NOT EXISTS(SELECT 1 FROM lifecycle_reader_termination t WHERE t.pin_id=p.id)`)
	if e != nil || n != 0 {
		return errors.New("business pins did not drain")
	}
	counts := map[string]int64{}
	for name, query := range map[string]string{"requirements": "SELECT count(*) FROM lifecycle_fixture_reference_requirement", "releases": "SELECT count(*) FROM lifecycle_fixture_reference_release", "calls": "SELECT count(*) FROM lifecycle_storage_call", "observations": "SELECT count(*) FROM lifecycle_storage_observation", "typed_costs": "SELECT count(*) FROM cost_allocation WHERE lifecycle_storage_call_id IS NOT NULL", "pins": "SELECT count(*) FROM lifecycle_reader_pin", "stops": "SELECT count(*) FROM lifecycle_reader_termination", "provider_send_claims": "SELECT count(*) FROM lifecycle_provider_send_claim"} {
		v, e := count(ctx, pool, query)
		if e != nil {
			return e
		}
		counts[name] = int64(v)
	}
	return write(c, "reference-business-races.json", map[string]any{"run_id": c.RunID, "source_scope": targets, "plan": plan, "inventory_plan": inventory, "provider_deletion_authorized": false, "actual_delete": "NOT_RUN", "cleanup": 0, "counts": counts, "native_processing_baseline": completed, "in_flight_processing_after_restriction": failed, "direct_issued": issued.Operation, "direct_transfer": transfer, "dispatches_after_owner_stop": dispatches, "reservation_first_GET_and_output_delta": after - before, "LC11": "PASS_NON_DELETE_NATIVE_BUSINESS_ADMISSION", "LC12": "PASS_NON_DELETE_CERTIFIED_DIRECT_TCP_TRANSFER", "full_D1": "NOT_RUN"})
}
