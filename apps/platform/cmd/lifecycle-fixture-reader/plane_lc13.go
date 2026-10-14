package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	parkindicator "github.com/qq550723504/data-product-platform/apps/platform/internal/industrypack/park/indicator"
	pinnedworkflow "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/adapters/workflow"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	platformqueue "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/queue"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/native"
	workflowqueue "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/transport/queue"
)

type lc13State struct {
	One, Two uuid.UUID
	UseUntil time.Time
}

// The fault is after the real pinned reader has durably journaled a native GET.
// os.Exit deliberately skips ExecuteProcessing's owner-stop deferred cleanup.
type lc13CrashReader struct {
	pins      *lc.FixtureSubjectPins
	c         config
	execution uuid.UUID
}

func (r lc13CrashReader) ReadVersion(ctx context.Context, id uuid.UUID) (io.ReadCloser, error) {
	h, e := r.pins.ReadVersion(ctx, id)
	if e != nil {
		return nil, e
	}
	if e = write(r.c, "lc13-crash-"+r.execution.String()+".json", map[string]any{"execution": r.execution, "input": id, "after_native_get": true, "at": time.Now().UTC(), "exit": 95, "owner_stop": false}); e != nil {
		return nil, e
	}
	os.Exit(95)
	return h, nil
}

// Select one fixture subject only after the real stale-query has applied its
// default cutoff. This does not alter the recovery clock or persisted lease.
type lc13SelectedRepository struct {
	*workflowinfra.PostgresRepository
	selected uuid.UUID
}

func (r lc13SelectedRepository) ListStaleNativeExecutionIDs(ctx context.Context, cutoff time.Time, cursor uuid.UUID, limit int) ([]uuid.UUID, error) {
	ids, e := r.PostgresRepository.ListStaleNativeExecutionIDs(ctx, cutoff, cursor, limit)
	if e != nil {
		return nil, e
	}
	for _, id := range ids {
		if id == r.selected {
			return []uuid.UUID{id}, nil
		}
	}
	return nil, nil
}

type lc13ObservedEngine struct {
	inner     workflowapp.ProcessingEngine
	priorPins bool
	calls     int
}

func (r *lc13ObservedEngine) Execute(ctx context.Context, req workflowapp.ProcessingRequest) (workflowapp.ProcessingResult, error) {
	r.calls++
	result, e := r.inner.Execute(ctx, req)
	r.priorPins = errors.Is(e, domain.ErrTerminationUnknown)
	return result, e
}
func lc13Counts(ctx context.Context, pool *pgxpool.Pool) (json.RawMessage, error) {
	var b []byte
	e := pool.QueryRow(ctx, `SELECT json_build_object('reads',(SELECT count(*) FROM lifecycle_storage_call WHERE operation='READ_PINNED'),'puts',(SELECT count(*) FROM lifecycle_storage_call WHERE operation='PROCESSING_PUT'),'pins',(SELECT count(*) FROM lifecycle_reader_pin),'stops',(SELECT count(*) FROM lifecycle_reader_termination),'outputs',(SELECT count(*) FROM lifecycle_processing_output_ready),'costs',(SELECT count(*) FROM cost_event),'send_claims',(SELECT count(*) FROM lifecycle_provider_send_claim),'absence',(SELECT count(*) FROM lifecycle_copy_absence_receipt),'completion',(SELECT count(*) FROM lifecycle_plan_completion))`).Scan(&b)
	return b, e
}
func lc13NoNewIO(before, after json.RawMessage) error {
	var a, b map[string]int
	if json.Unmarshal(before, &a) != nil || json.Unmarshal(after, &b) != nil {
		return errors.New("bad LC13 counts")
	}
	for _, k := range []string{"reads", "puts", "pins", "stops", "outputs", "send_claims", "absence", "completion"} {
		if a[k] != b[k] {
			return fmt.Errorf("LC13 %s changed", k)
		}
	}
	return nil
}
func planeLC13(ctx context.Context, c config, pool *pgxpool.Pool, original []domain.Copy, mode string) error {
	if c.ReferenceDemand || c.RedisAddr != "dpp-lifecycle307-"+c.RunID.String()[:8]+"-redis:6379" {
		return lc.ErrStorageScope
	}
	state, e := loadPlaneState(c)
	if e != nil {
		return e
	}
	cs, e := planeCopies(ctx, c, pool)
	if e != nil {
		return e
	}
	o, policy, _, pins, e := planeServices(ctx, c, pool, cs)
	if e != nil {
		return e
	}
	tx := transaction.NewManager(pool)
	repo := workflowinfra.NewPostgresRepository(pool)
	svc := workflowapp.NewExecutionService(tx, repo)
	if mode == "lc13-init" {
		until := time.Now().UTC().Add(2 * time.Minute).Truncate(time.Microsecond)
		if _, e = policy.BindFixtureCopyPolicy(ctx, lc.BindCopyPolicyCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "lc13-beta-policy"}, DatasetVersionID: original[1].DatasetVersionID, UseUntil: until, RestoreUntil: c.ExpiresAt}); e != nil {
			return e
		}
		create := func(key string) (workflowdomain.Execution, error) {
			return svc.Create(ctx, workflowapp.CreateExecutionCommand{WorkspaceID: c.WorkspaceID, WorkflowVersionID: state.WorkflowVersionID, OutputDatasetID: state.OutputDatasetID, TargetPeriod: pilot.PlanePeriod, Inputs: []workflowdomain.InputBinding{{Name: "enterprise_raw", DatasetVersionID: original[0].DatasetVersionID}, {Name: "enterprise_resolution", DatasetVersionID: state.StandardizedVersionID}, {Name: "lease_raw", DatasetVersionID: original[1].DatasetVersionID}, {Name: "energy_raw", DatasetVersionID: original[1].DatasetVersionID}}, ActorID: &c.ActorID, IdempotencyKey: key})
		}
		one, e := create("lc13-crash-one")
		if e != nil {
			return e
		}
		two, e := create("lc13-crash-two")
		if e != nil {
			return e
		}
		return write(c, "lc13-state.json", lc13State{one.ID, two.ID, until})
	}
	var subjects lc13State
	b, e := os.ReadFile(filepath.Join(c.OutputDirectory, "lc13-state.json"))
	if e != nil {
		return e
	}
	if e = json.Unmarshal(b, &subjects); e != nil {
		return e
	}
	writer, e := planeWriter(ctx, c, pool, o, pins, original, state)
	if e != nil {
		return e
	}
	engine := native.NewEngine("/run/repo/industry-packs", tx, datasetinfra.NewPostgresRepository(pool), entityinfra.NewPostgresRepository(pool), repo, writer, planeCache{}, parkindicator.NewCalculator()).WithInputReader(pins)
	selected := subjects.One
	if mode == "lc13-crash-two" {
		selected = subjects.Two
	}
	if mode == "lc13-crash-one" || mode == "lc13-crash-two" {
		engine.WithInputReader(lc13CrashReader{pins, c, selected})
	}
	wrapped := &pinnedworkflow.PinnedEngine{Pins: pins, Inner: engine}
	handler := workflowqueue.NewHandler(svc, repo, wrapped).WithNativeExecutionLocker(tx)
	if mode == "lc13-crash-one" || mode == "lc13-crash-two" || mode == "lc13-orphan" {
		q := platformqueue.Config{Addr: c.RedisAddr}
		client := workflowqueue.NewClient(platformqueue.NewClient(q))
		defer client.Close()
		before, e := lc13Counts(ctx, pool)
		if e != nil {
			return e
		}
		if mode != "lc13-orphan" {
			if e = client.EnqueueExecution(ctx, selected); e != nil {
				return e
			}
		}
		server := platformqueue.NewServer(q)
		mux := asynq.NewServeMux()
		delivered := make(chan uuid.UUID, 16)
		mux.HandleFunc(workflowqueue.TaskExecute, func(x context.Context, t *asynq.Task) error {
			id, e := workflowqueue.ExecutionID(t)
			if e != nil {
				return e
			}
			e = handler.Handle(x, t)
			if e == nil {
				delivered <- id
			}
			return e
		})
		if e = server.Start(mux); e != nil {
			return e
		}
		defer server.Shutdown()
		if mode != "lc13-orphan" {
			<-ctx.Done()
			return ctx.Err()
		}
		// First wait for both crashed tasks themselves to be redelivered. Do not
		// enqueue a replacement and label it orphan recovery.
		seen := map[uuid.UUID]int{}
		for seen[subjects.One] == 0 || seen[subjects.Two] == 0 {
			select {
			case id := <-delivered:
				seen[id]++
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		for _, id := range []uuid.UUID{subjects.One, subjects.Two} {
			if e = client.EnqueueExecution(ctx, id); e != nil {
				return e
			}
		}
		for seen[subjects.One] < 2 || seen[subjects.Two] < 2 {
			select {
			case id := <-delivered:
				seen[id]++
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		after, e := lc13Counts(ctx, pool)
		if e != nil {
			return e
		}
		if e = lc13NoNewIO(before, after); e != nil {
			return e
		}
		return write(c, "lc13-orphan.json", map[string]any{"real_asynq_deliveries": seen, "before": before, "after": after, "native_io_delta": 0, "outbox_dispatch": "NOT_RUN", "at": time.Now().UTC()})
	}
	if mode == "lc13-lease-recovery" {
		if time.Now().UTC().Before(subjects.UseUntil) {
			return errors.New("natural reader expiry not reached")
		}
		before, e := lc13Counts(ctx, pool)
		if e != nil {
			return e
		}
		var outcomes []any
		for _, id := range []uuid.UUID{subjects.One, subjects.Two} {
			ex, e := repo.GetExecution(ctx, id)
			if e != nil {
				return e
			}
			if ex.StartedAt == nil || time.Now().UTC().Before(ex.StartedAt.Add(10*time.Minute+time.Second)) {
				return errors.New("natural default recovery lease not reached")
			}
			if id == subjects.Two {
				if _, e = o.RestrictContentUse(ctx, lc.RestrictContentUseCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "lc13-restriction"}, DatasetVersionID: original[0].DatasetVersionID, BasisRef: "CONTROLLED_NON_DELETE_LC13"}); e != nil {
					return e
				}
			}
			observed := &lc13ObservedEngine{inner: wrapped}
			r := workflowapp.NewNativeReconciler(tx, svc, lc13SelectedRepository{repo, id}, datasetinfra.NewPostgresRepository(pool), observed)
			if e = r.RunOnce(ctx); e != nil {
				return e
			}
			done, e := repo.GetExecution(ctx, id)
			if e != nil {
				return e
			}
			if done.Status != workflowdomain.ExecutionFailed || done.OutputDatasetVersionID != nil {
				return errors.New("unsafe LC13 terminal result")
			}
			if id == subjects.One && (!observed.priorPins || observed.calls != 1) {
				return errors.New("unknown pins did not reject native recovery")
			}
			replayBefore, e := lc13Counts(ctx, pool)
			if e != nil {
				return e
			}
			if e = r.RunOnce(ctx); e != nil {
				return e
			}
			replayAfter, e := lc13Counts(ctx, pool)
			if e != nil {
				return e
			}
			if string(replayBefore) != string(replayAfter) {
				return errors.New("recovery terminal replay mutated counts")
			}
			outcomes = append(outcomes, map[string]any{"execution": done, "prior_pin_rejection": observed.priorPins, "engine_wrapper_calls": observed.calls, "restriction_present": id == subjects.Two, "current_reader_gate_reached": false})
		}
		// A new explicit business retry is checked independently of the old
		// execution-pin guard. A rejection does not claim a native input read.
		_, retryErr := svc.Retry(ctx, workflowapp.RetryExecutionCommand{ExecutionID: subjects.Two, IdempotencyKey: "lc13-restricted-explicit-retry", ActorID: &c.ActorID})
		if retryErr == nil {
			return errors.New("restricted explicit retry admitted references")
		}
		after, e := lc13Counts(ctx, pool)
		if e != nil {
			return e
		}
		if e = lc13NoNewIO(before, after); e != nil {
			return e
		}
		return write(c, "lc13-recovery.json", map[string]any{"outcomes": outcomes, "before": before, "after": after, "reader_use_until": subjects.UseUntil, "observed_at": time.Now().UTC(), "clock_or_status_rewrite": false, "pins_released": 0, "retry_rejected": true, "recovery_cost": "Core recovery activity and estimated engine invocation; separate from native storage calls"})
	}
	if mode == "lc13-final" {
		return planeExport(ctx, c, pool)
	}
	return errors.New("unsupported LC13 mode")
}
