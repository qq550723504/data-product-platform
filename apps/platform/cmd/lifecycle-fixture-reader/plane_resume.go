package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	parkindicator "github.com/qq550723504/data-product-platform/apps/platform/internal/industrypack/park/indicator"
	pinnedworkflow "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/adapters/workflow"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	platformqueue "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/queue"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/native"
	workflowqueue "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/transport/queue"
	"os"
	"path/filepath"
	"time"
)

func resumePins(ctx context.Context, pool *pgxpool.Pool, w, execution uuid.UUID) ([]domain.ReaderPin, error) {
	rows, e := pool.Query(ctx, `SELECT id FROM lifecycle_reader_pin WHERE workspace_id=$1 AND execution_id=$2 ORDER BY id`, w, execution)
	if e != nil {
		return nil, e
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return nil, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	var pins []domain.ReaderPin
	tx, e := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	repo := &infrastructure.PostgresRepository{}
	for _, id := range ids {
		p, e := repo.GetReaderPin(ctx, tx, w, id)
		if e != nil {
			return nil, e
		}
		pins = append(pins, p)
	}
	return pins, tx.Commit(ctx)
}
func planeResume(ctx context.Context, c config, pool *pgxpool.Pool, original []domain.Copy, mode string) error {
	if !c.LC13Continuation || c.ReferenceDemand || c.RedisAddr != "dpp-lifecycle307-"+c.RunID.String()[:8]+"-redis:6379" {
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
	tx := transaction.NewManager(pool)
	repo := workflowinfra.NewPostgresRepository(pool)
	svc := workflowapp.NewExecutionService(tx, repo)
	o, policy, _, pins, e := planeServices(ctx, c, pool, cs)
	if e != nil {
		return e
	}
	if mode == "resume-init" {
		if _, e = policy.BindFixtureCopyPolicy(ctx, lc.BindCopyPolicyCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "resume-beta-policy"}, DatasetVersionID: original[1].DatasetVersionID, UseUntil: c.ExpiresAt, RestoreUntil: c.ExpiresAt}); e != nil {
			return e
		}
		ex, e := svc.Create(ctx, workflowapp.CreateExecutionCommand{WorkspaceID: c.WorkspaceID, WorkflowVersionID: state.WorkflowVersionID, OutputDatasetID: state.OutputDatasetID, TargetPeriod: pilot.PlanePeriod, Inputs: []workflowdomain.InputBinding{{Name: "enterprise_raw", DatasetVersionID: original[0].DatasetVersionID}, {Name: "enterprise_resolution", DatasetVersionID: state.StandardizedVersionID}, {Name: "lease_raw", DatasetVersionID: original[1].DatasetVersionID}, {Name: "energy_raw", DatasetVersionID: original[1].DatasetVersionID}}, ActorID: &c.ActorID, IdempotencyKey: "resume-original"})
		if e != nil {
			return e
		}
		return write(c, "resume-original.json", ex)
	}
	var ex workflowdomain.Execution
	b, e := os.ReadFile(filepath.Join(c.OutputDirectory, "resume-original.json"))
	if e != nil {
		return e
	}
	if e = json.Unmarshal(b, &ex); e != nil {
		return e
	}
	if mode == "resume-membership" {
		ps, e := resumePins(ctx, pool, c.WorkspaceID, ex.ID)
		if e != nil {
			return e
		}
		if len(ps) != 3 {
			return domain.ErrTerminationUnknown
		}
		b, _ := json.Marshal(ps)
		h := sha256.Sum256(b)
		return write(c, "resume-pin-membership.json", map[string]any{"pins": ps, "sha256": hex.EncodeToString(h[:]), "execution": ex.ID})
	}
	if mode == "resume-stop" {
		var signed pilot.SignedClosedReferenceExecutor
		b, e := os.ReadFile("/run/private/owner-stop.json")
		if e != nil {
			return e
		}
		if e = json.Unmarshal(b, &signed); e != nil {
			return e
		}
		key, e := hex.DecodeString(c.OwnerStopPublicKey)
		if e != nil || len(key) != ed25519.PublicKeySize {
			return domain.ErrTerminationUnknown
		}
		v := pilot.ProcessingStopVerifier{Key: key, Expected: c.OwnerStopIdentity, Proof: signed}
		if _, e = v.Validate(time.Now().UTC()); e != nil {
			return e
		}
		current, e := repo.GetExecution(ctx, ex.ID)
		if e != nil {
			return e
		}
		if current.Status == workflowdomain.ExecutionRunning {
			if _, e = svc.Fail(ctx, ex.ID, "NATIVE_OWNER_STOP_CONFIRMED", "trusted closed native owner exited", nil, "lc13-owner-stop"); e != nil {
				return e
			}
		} else if current.Status != workflowdomain.ExecutionFailed {
			return domain.ErrTerminationUnknown
		}
		o, _, _, _, e = planeServicesWithRecovery(ctx, c, pool, cs, v)
		if e != nil {
			return e
		}
		ps, e := resumePins(ctx, pool, c.WorkspaceID, ex.ID)
		if e != nil {
			return e
		}
		var stops []lc.TerminateFixtureReaderCommand
		for _, p := range ps {
			stops = append(stops, lc.TerminateFixtureReaderCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID}, PinID: p.ID, Generation: p.Generation, Outcome: domain.ReaderCancelled})
		}
		cmd := lc.TerminateProcessingReadersCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "resume-owner-atomic-stop"}, ExecutionID: ex.ID, Stops: stops}
		result, e := o.TerminateProcessingReaders(ctx, cmd)
		if e != nil {
			return e
		}
		replay, e := o.TerminateProcessingReaders(ctx, cmd)
		if e != nil || !replay.Replayed || replay.ID != result.ID {
			return errors.New("owner stop replay mismatch")
		}
		return write(c, "resume-stop-verified.json", map[string]any{"result": result, "replay": replay, "terminated_pins": len(stops), "execution": ex.ID, "signed_proof": signed, "public_key": c.OwnerStopPublicKey})
	}
	selected := ex.ID
	if mode == "resume-retry" {
		var active int
		if e = pool.QueryRow(ctx, `SELECT count(*) FROM lifecycle_reader_pin p WHERE execution_id=$1 AND NOT EXISTS(SELECT 1 FROM lifecycle_reader_termination t WHERE t.pin_id=p.id)`, ex.ID).Scan(&active); e != nil {
			return e
		}
		if active != 0 {
			return domain.ErrTerminationUnknown
		}
		retry, e := svc.Retry(ctx, workflowapp.RetryExecutionCommand{ExecutionID: ex.ID, IdempotencyKey: "resume-explicit-retry", ActorID: &c.ActorID})
		if e != nil {
			return e
		}
		if retry.RetryOfExecutionID == nil || *retry.RetryOfExecutionID != ex.ID || retry.ID == ex.ID {
			return errors.New("retry history mismatch")
		}
		selected = retry.ID
	}
	if mode != "resume-crash" && mode != "resume-retry" {
		return errors.New("unknown resume mode")
	}
	writer, e := planeWriter(ctx, c, pool, o, pins, original, state)
	if e != nil {
		return e
	}
	engine := native.NewEngine("/run/repo/industry-packs", tx, datasetinfra.NewPostgresRepository(pool), entityinfra.NewPostgresRepository(pool), repo, writer, planeCache{}, parkindicator.NewCalculator()).WithInputReader(pins)
	if mode == "resume-crash" {
		engine.WithInputReader(lc13CrashReader{pins, c, selected})
	}
	handler := workflowqueue.NewHandler(svc, repo, &pinnedworkflow.PinnedEngine{Pins: pins, Inner: engine}).WithNativeExecutionLocker(tx)
	q := platformqueue.Config{Addr: c.RedisAddr}
	client := workflowqueue.NewClient(platformqueue.NewClient(q))
	defer client.Close()
	if e = client.EnqueueExecution(ctx, selected); e != nil {
		return e
	}
	server := platformqueue.NewServer(q)
	mux := asynq.NewServeMux()
	done := make(chan uuid.UUID, 8)
	mux.HandleFunc(workflowqueue.TaskExecute, func(x context.Context, t *asynq.Task) error {
		id, e := workflowqueue.ExecutionID(t)
		if e != nil {
			return e
		}
		e = handler.Handle(x, t)
		if e == nil {
			done <- id
		}
		return e
	})
	if e = server.Start(mux); e != nil {
		return e
	}
	defer server.Shutdown()
	for {
		select {
		case id := <-done:
			if mode == "resume-retry" && id == selected {
				completed, e := repo.GetExecution(ctx, selected)
				if e != nil {
					return e
				}
				if completed.Status != workflowdomain.ExecutionSucceeded || completed.OutputDatasetVersionID == nil {
					return errors.New("retry did not produce native ready output")
				}
				return write(c, "resume-native-success.json", completed)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
