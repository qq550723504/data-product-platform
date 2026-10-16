package main

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	parkindicator "github.com/qq550723504/data-product-platform/apps/platform/internal/industrypack/park/indicator"
	pinnedworkflow "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/adapters/workflow"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	platformqueue "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/queue"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/native"
	workflowqueue "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/transport/queue"
)

type remainingTaskResult struct {
	Execution uuid.UUID
	TaskID    string
	Error     string
	At        time.Time
}
type remainingQueue struct {
	client         *workflowqueue.Client
	server         *asynq.Server
	done           chan remainingTaskResult
	mu             sync.Mutex
	history        []remainingTaskResult
	parked, resume chan struct{}
	resumeOnce     sync.Once
}

func startRemainingQueue(ctx context.Context, r remainingConfig, pool *pgxpool.Pool, handler *workflowqueue.Handler, pause uuid.UUID, allowed ...uuid.UUID) (*remainingQueue, error) {
	c := r.Base
	if c.RedisAddr != "dpp-lifecycle307-808a6960-remaining-redis:6379" {
		return nil, errors.New("fixed isolated Redis required")
	}
	cfg := platformqueue.Config{Addr: c.RedisAddr}
	q := &remainingQueue{client: workflowqueue.NewClient(platformqueue.NewClient(cfg)), server: platformqueue.NewServer(cfg), done: make(chan remainingTaskResult, 16)}
	q.parked = make(chan struct{})
	q.resume = make(chan struct{})
	ids := map[uuid.UUID]bool{}
	for _, id := range allowed {
		ids[id] = true
	}
	mux := asynq.NewServeMux()
	mux.HandleFunc(workflowqueue.TaskExecute, func(x context.Context, t *asynq.Task) error {
		id, err := workflowqueue.ExecutionID(t)
		task, ok := asynq.GetTaskID(x)
		if err != nil || !ok || task == "" || !ids[id] {
			return errors.New("unexpected real queue delivery")
		}
		if id == pause {
			q.mu.Lock()
			first := len(q.history) == 0
			q.mu.Unlock()
			if first {
				if e := write(c, "remaining-P2-queue-entry.json", map[string]any{"execution": id, "task_id": task, "before_handler_and_pins": true, "at": time.Now().UTC()}); e != nil {
					return e
				}
				close(q.parked)
				select {
				case <-q.resume:
				case <-x.Done():
					return x.Err()
				}
			}
		}
		err = handler.Handle(x, t)
		result := remainingTaskResult{Execution: id, TaskID: task, At: time.Now().UTC()}
		if err != nil {
			result.Error = err.Error()
		}
		if e := write(c, "remaining-task-"+task+".json", result); e != nil {
			return e
		}
		q.mu.Lock()
		q.history = append(q.history, result)
		q.mu.Unlock()
		q.done <- result
		return err
	})
	if err := q.server.Start(mux); err != nil {
		q.client.Close()
		return nil, err
	}
	return q, nil
}
func (q *remainingQueue) release() { q.resumeOnce.Do(func() { close(q.resume) }) }
func (q *remainingQueue) close()   { q.release(); q.server.Shutdown(); q.client.Close() }
func (q *remainingQueue) wait(ctx context.Context, id uuid.UUID) error {
	select {
	case result := <-q.done:
		if result.Execution != id || result.Error != "" {
			return errors.New("real queue task failed or returned wrong execution")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func remainingEngine(ctx context.Context, r remainingConfig, pool *pgxpool.Pool, cs []domain.Copy, s remainingState, barrier bool) (*workflowqueue.Handler, *referenceInputBarrier, error) {
	o, _, _, pins, err := remainingServices(r, pool, cs)
	if err != nil {
		return nil, nil, err
	}
	source, err := remainingCopyKey(cs, "fixture/p1.csv")
	if err != nil {
		return nil, nil, err
	}
	writer, err := remainingWriter(ctx, r, pool, o, pins, source, s.Plane)
	if err != nil {
		return nil, nil, err
	}
	tx := transaction.NewManager(pool)
	wf := workflowinfra.NewPostgresRepository(pool)
	engine := native.NewEngine("/run/repo/industry-packs", tx, datasetinfra.NewPostgresRepository(pool), entityinfra.NewPostgresRepository(pool), wf, writer, planeCache{}, parkindicator.NewCalculator()).WithInputReader(pins)
	var b *referenceInputBarrier
	if barrier {
		b = &referenceInputBarrier{pins: pins, target: source.DatasetVersionID, entered: make(chan struct{}), release: make(chan struct{})}
		engine = engine.WithInputReader(b)
	}
	h := workflowqueue.NewHandler(workflowapp.NewExecutionService(tx, wf), wf, &pinnedworkflow.PinnedEngine{Pins: pins, Inner: engine}).WithNativeExecutionLocker(tx)
	return h, b, nil
}
func remainingBaseline(ctx context.Context, r remainingConfig, pool *pgxpool.Pool, original []domain.Copy) error {
	var s remainingState
	if err := readRemaining(filepath.Join(r.Base.OutputDirectory, "remaining-state.json"), &s); err != nil {
		return err
	}
	cs, err := planeCopies(ctx, r.Base, pool)
	if err != nil {
		return err
	}
	h, _, err := remainingEngine(ctx, r, pool, cs, s, false)
	if err != nil {
		return err
	}
	q, err := startRemainingQueue(ctx, r, pool, h, uuid.Nil, s.P0)
	if err != nil {
		return err
	}
	defer q.close()
	if err = q.client.EnqueueExecution(ctx, s.P0); err != nil {
		return err
	}
	if err = q.wait(ctx, s.P0); err != nil {
		return err
	}
	ex, err := workflowinfra.NewPostgresRepository(pool).GetExecution(ctx, s.P0)
	if err != nil || ex.Status != workflowdomain.ExecutionSucceeded || ex.OutputDatasetVersionID == nil {
		return errors.New("real native baseline did not succeed")
	}
	return write(r.Base, "remaining-native-baseline.json", map[string]any{"execution": ex, "tasks": q.history, "delete": 0, "native_engine": true, "real_asynq": true})
}
