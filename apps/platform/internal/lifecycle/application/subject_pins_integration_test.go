package application

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	deliveryapp "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/application"
	deliverydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/domain"
	deliveryinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
)

// Database protocol tests: SQL-only native port and deterministic gate are not
// provider/business acceptance. Native compositions are verified separately.
func subjectExecution(t *testing.T, pool *pgxpool.Pool, w uuid.UUID, c domain.Copy, o *OccupancyService) (workflowdomain.Execution, *workflowapp.ExecutionService) {
	t.Helper()
	ctx := context.Background()
	wf, wv := uuid.New(), uuid.New()
	var dataset uuid.UUID
	if e := pool.QueryRow(ctx, `SELECT dataset_id FROM dataset_version WHERE id=$1`, c.DatasetVersionID).Scan(&dataset); e != nil {
		t.Fatal(e)
	}
	if _, e := pool.Exec(ctx, `INSERT INTO workflow(id,workspace_id,code,name) VALUES($1,$2,$3,'SQL-only input ownership')`, wf, w, wf.String()); e != nil {
		t.Fatal(e)
	}
	if _, e := pool.Exec(ctx, `INSERT INTO workflow_version(id,workflow_id,version,definition_ref,definition_sha256,definition) VALUES($1,$2,'1','sql-only',repeat('a',64),'{}')`, wv, wf); e != nil {
		t.Fatal(e)
	}
	output := uuid.New()
	if _, e := pool.Exec(ctx, `INSERT INTO dataset(id,workspace_id,code,name,dataset_type) VALUES($1,$2,$3,'SQL-only processing output','CURATED')`, output, w, output.String()); e != nil {
		t.Fatal(e)
	}
	svc := workflowapp.NewExecutionService(o.tx, workflowinfra.NewPostgresRepository(pool))
	caller := o.authority.(*occupancyAuthority).caller
	ex, e := svc.Create(ctx, workflowapp.CreateExecutionCommand{WorkspaceID: w, WorkflowVersionID: wv, OutputDatasetID: output, TargetPeriod: "2026-10", Inputs: []workflowdomain.InputBinding{{Name: "fixture", DatasetVersionID: c.DatasetVersionID}}, IdempotencyKey: "subject-" + uuid.NewString(), ActorID: &caller.ActorID})
	if e != nil {
		t.Fatal(e)
	}
	ex, e = svc.StartWithReferenceCheck(ctx, ex.ID, "native:"+ex.ID.String(), "sql-only")
	if e != nil {
		t.Fatal(e)
	}
	return ex, svc
}
func TestProcessingPinOwnsActualInputHandleUntilEngineStops(t *testing.T) {
	pool, w, c, o, reader, port, plan := newReaderFixture(t)
	ex, _ := subjectExecution(t, pool, w, c, o)
	next, _ := subjectExecution(t, pool, w, c, o)
	pins := NewFixtureSubjectPins(reader, map[uuid.UUID]uuid.UUID{c.DatasetVersionID: c.ID})
	entered, release := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(readerContext(o))
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- pins.ExecuteProcessing(ctx, w, ex.ID, func(ctx context.Context) error {
			r, e := pins.ReadVersion(ctx, c.DatasetVersionID)
			if e != nil {
				return e
			}
			defer r.Close()
			b, e := io.ReadAll(r)
			if e != nil || string(b) != string(readerTestBytes) {
				return errors.New("input read mismatch")
			}
			close(entered)
			<-release
			return ctx.Err()
		})
	}()
	<-entered
	if occupancyFor(t, o, w, plan.ID, c.ID).ActiveReaders != 1 || port.calls.Load() != 1 {
		t.Fatal("processing input lacks durable occupancy")
	}
	if _, e := o.RestrictContentUse(context.Background(), RestrictContentUseCommand{CommandHeader: CommandHeader{w, "processing-current-restriction"}, DatasetVersionID: c.DatasetVersionID, BasisRef: "SQL-only"}); e != nil {
		t.Fatal(e)
	}
	cancel()
	if occupancyFor(t, o, w, plan.ID, c.ID).ActiveReaders != 1 {
		t.Fatal("cancel released live processing owner")
	}
	close(release)
	if e := <-done; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if occupancyFor(t, o, w, plan.ID, c.ID).ActiveReaders != 0 || count(t, pool, `SELECT count(*) FROM lifecycle_reader_pin WHERE kind='FIXTURE_PROCESSING'`) != 1 {
		t.Fatal("owned stop failed to drain processing pin")
	}
	if e := pins.ExecuteProcessing(readerContext(o), w, ex.ID, func(context.Context) error { t.Error("same execution reopened engine"); return nil }); !errors.Is(e, domain.ErrTerminationUnknown) {
		t.Fatal(e)
	}
	if e := pins.ExecuteProcessing(readerContext(o), w, next.ID, func(context.Context) error { t.Error("restricted input opened engine"); return nil }); e == nil {
		t.Fatal("restriction failed")
	}
	if port.calls.Load() != 1 {
		t.Fatal("processing recovery/restriction caused new physical read")
	}
}
func TestProcessingUnclosedOrUnknownOwnerKeepsPin(t *testing.T) {
	for _, mode := range []string{"handle-open", "unknown-native-stop", "output-unknown"} {
		t.Run(mode, func(t *testing.T) {
			pool, w, c, o, reader, port, plan := newReaderFixture(t)
			ex, _ := subjectExecution(t, pool, w, c, o)
			pins := NewFixtureSubjectPins(reader, map[uuid.UUID]uuid.UUID{c.DatasetVersionID: c.ID})
			port.unknownStop = mode == "unknown-native-stop"
			e := pins.ExecuteProcessing(readerContext(o), w, ex.ID, func(ctx context.Context) error {
				r, e := pins.ReadVersion(ctx, c.DatasetVersionID)
				if e != nil {
					return e
				}
				if mode != "handle-open" {
					r.Close()
				}
				if mode == "output-unknown" {
					pins.MarkProcessingOutputUnknown(ctx)
				}
				return nil
			})
			if !errors.Is(e, domain.ErrTerminationUnknown) || occupancyFor(t, o, w, plan.ID, c.ID).ActiveReaders != 1 {
				t.Fatal("unknown owner released", e)
			}
		})
	}
}

type sqlDirectGate struct{}

func (sqlDirectGate) EvaluateDirectData(_ context.Context, _ pgx.Tx, r deliveryapp.DirectDataGateRequest) (deliveryapp.DirectDataGateResult, error) {
	cap := r.RequestedExpiresAt
	return deliveryapp.DirectDataGateResult{Evaluation: deliverydomain.GateEvaluation{Allowed: true, PrincipalRef: r.PrincipalRef, EffectiveConsumerRef: r.EffectiveConsumerRef, FreshCapExpiresAt: &cap}}, nil
}

type heldTransferPins struct {
	pins             *FixtureSubjectPins
	entered, release chan struct{}
}

func (h heldTransferPins) AdmitDirectDataTransfer(ctx context.Context, tx pgx.Tx, op deliverydomain.Operation) (uuid.UUID, error) {
	id, e := h.pins.AdmitDirectDataTransfer(ctx, tx, op)
	if e != nil {
		return id, e
	}
	close(h.entered)
	select {
	case <-h.release:
		return id, nil
	case <-ctx.Done():
		return uuid.Nil, ctx.Err()
	}
}
func TestDirectIssuedAndTransferPinCommitTogetherBeforeAnyBytes(t *testing.T) {
	pool, w, c, o, reader, port, plan := newReaderFixture(t)
	pins := NewFixtureSubjectPins(reader, map[uuid.UUID]uuid.UUID{c.DatasetVersionID: c.ID})
	ctx := readerContext(o)
	profile := uuid.New()
	if _, e := pool.Exec(ctx, `INSERT INTO certification_profile(id,workspace_id,profile_ref,code,name,version,content_sha256,content_snapshot,purpose_mode,action_mode,consumer_mode,delivery_mode,quality_gate_required,rights_required,compliance_required,contract_required,traceability_required,evidence_required) VALUES($1,$2,$3,$3,'SQL-only direct profile','1',encode(digest(convert_to('sql-only','UTF8'),'sha256'),'hex'),'sql-only','ANY','ANY','ANY','ANY',false,false,false,false,false,false)`, profile, w, profile.String()); e != nil {
		t.Fatal(e)
	}
	held := heldTransferPins{pins, make(chan struct{}), make(chan struct{})}
	svc := deliveryapp.NewDirectDataService(o.tx, deliveryinfra.NewPostgresRepository(pool), sqlDirectGate{}, datasetinfra.NewPostgresRepository(pool)).WithTransferPins(held)
	caller := o.authority.(*occupancyAuthority).caller
	cmd := deliveryapp.DirectDataCommand{WorkspaceID: w, DatasetVersionID: c.DatasetVersionID, ProfileID: profile, PrincipalRef: caller.Subject, EffectiveConsumerRef: "SQL_ONLY_CONSUMER", Purpose: "FIXTURE", Action: "READ", ScopeType: "ALL_RESOURCE", IdempotencyKey: "owned-delivery"}
	result := make(chan deliveryapp.DirectDataResult, 1)
	failure := make(chan error, 1)
	go func() { r, e := svc.Deliver(ctx, cmd); result <- r; failure <- e }()
	<-held.entered
	if port.calls.Load() != 0 || count(t, pool, `SELECT count(*) FROM lifecycle_reader_pin`) != 0 || count(t, pool, `SELECT count(*) FROM delivery_operation`) != 0 {
		t.Fatal("pin, issued or bytes escaped before commit")
	}
	close(held.release)
	r := <-result
	if e := <-failure; e != nil {
		t.Fatal(e)
	}
	if !r.PayloadReady || r.TransferPinID == uuid.Nil || occupancyFor(t, o, w, plan.ID, c.ID).ActiveReaders != 1 {
		t.Fatal("committed transfer pin missing", r)
	}
	var cap, use time.Time
	if e := pool.QueryRow(ctx, `SELECT d.fresh_cap_expires_at,p.use_until FROM delivery_operation d JOIN lifecycle_reader_pin p ON p.delivery_operation_id=d.id WHERE d.id=$1`, r.Operation.ID).Scan(&cap, &use); e != nil || use.After(cap) {
		t.Fatal("transfer cap widened", e)
	}
	writer := &blockingReaderWriter{entered: make(chan struct{}), release: make(chan struct{}), fail: true}
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, e := reader.ReadPinnedTo(streamCtx, w, r.TransferPinID, writer); done <- e }()
	<-writer.entered
	cancel()
	if occupancyFor(t, o, w, plan.ID, c.ID).ActiveReaders != 1 {
		t.Fatal("issued transfer cancel released live writer")
	}
	close(writer.release)
	if e := <-done; !errors.Is(e, io.ErrClosedPipe) {
		t.Fatal(e)
	}
	replay, e := svc.Deliver(ctx, cmd)
	if !errors.Is(e, deliveryapp.ErrDirectDataReplayRequiresNewAttempt) || replay.PayloadReady || replay.TransferPinID != uuid.Nil || port.calls.Load() != 1 {
		t.Fatal("issued same-key replay opened payload", e)
	}
	if _, e = reader.ReadPinnedTo(ctx, w, r.TransferPinID, io.Discard); e == nil {
		t.Fatal("terminated transfer was reusable")
	}
	var status string
	if e = pool.QueryRow(ctx, `SELECT status FROM delivery_operation WHERE id=$1`, r.Operation.ID).Scan(&status); e != nil || status != "ISSUED" {
		t.Fatal("transfer changed terminal issued history")
	}
}
