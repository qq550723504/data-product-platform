package main

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	lifecyclequeue "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/transport/queue"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	platformqueue "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/queue"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

type selectedRecovery struct {
	w, id uuid.UUID
	inner *lc.RecoveryService
}

func (s selectedRecovery) RunRecovery(ctx context.Context, w, id uuid.UUID) error {
	if w != s.w || id != s.id {
		return lc.ErrStorageScope
	}
	return s.inner.RunRecovery(ctx, w, id)
}

type selectedEnqueuer struct {
	w, id uuid.UUID
	inner *lifecyclequeue.Client
}

func (s selectedEnqueuer) EnqueueRecovery(ctx context.Context, w, id uuid.UUID) error {
	if w != s.w || id != s.id {
		return lc.ErrStorageScope
	}
	return s.inner.EnqueueRecovery(ctx, w, id)
}
func recoverThroughQueue(ctx context.Context, pool *pgxpool.Pool, l loaded, stage string, work uuid.UUID, exact *lc.ExactExecution, inspector lc.StorageInspector) error {
	expected := "dpp-lifecycle307-" + l.Scope.Run.String()[:8] + "-remaining-recovery-redis:6379"
	if l.F.RedisAddr != expected {
		return errDenied
	}
	cfg := platformqueue.Config{Addr: l.F.RedisAddr}
	client := platformqueue.NewClient(cfg)
	defer client.Close()
	queue := lifecyclequeue.NewClient(client)
	service := lc.NewRecoveryService(exact, inspector, transaction.NewManager(pool), queue, []uuid.UUID{l.Scope.Workspace})
	router, e := routing.NewRouter(false)
	if e != nil {
		return e
	}
	unsupported := func(context.Context, outbox.PublishedEvent) error { return lc.ErrStorageUnsupported }
	dispatcher, e := outbox.NewDispatcher(pool, outbox.Config{ConsumerName: execution + ":" + stage, Logger: slog.New(slog.DiscardHandler)}, router,
		outbox.HandlerRegistration{Name: routing.HandlerExecutionQueue, Handle: unsupported},
		outbox.HandlerRegistration{Name: routing.HandlerExternalPublicationQueue, Handle: unsupported},
		outbox.HandlerRegistration{Name: routing.HandlerLifecycleRecoveryQueue, Handle: lifecyclequeue.OutboxHandler(selectedEnqueuer{l.Scope.Workspace, work, queue})})
	if e != nil {
		return e
	}
	published := false
	for n := 0; n < 2000; n++ {
		if e = dispatcher.DispatchOnce(ctx); e != nil {
			return e
		}
		var status string
		if e = pool.QueryRow(ctx, `SELECT status FROM outbox_event WHERE aggregate_id=$1 AND event_type=$2`, work, lc.ReconciliationRequestedEvent).Scan(&status); e != nil {
			return e
		}
		if status == "PUBLISHED" {
			published = true
			break
		}
	}
	if !published {
		return errors.New("selected durable recovery outbox obligation not published")
	}
	done := make(chan error, 1)
	var taskID string
	handler := lifecyclequeue.Handle(selectedRecovery{l.Scope.Workspace, work, service})
	mux := asynq.NewServeMux()
	mux.HandleFunc(lifecyclequeue.TaskRecover, func(x context.Context, t *asynq.Task) error {
		id, ok := asynq.GetTaskID(x)
		if !ok || id != "lifecycle/recovery/"+work.String() {
			return lc.ErrStorageScope
		}
		taskID = id
		_, e := readOnly(stage)
		if e == nil {
			e = handler(x, t)
		}
		done <- e
		return e
	})
	server := platformqueue.NewServer(cfg)
	if e = server.Start(mux); e != nil {
		return e
	}
	defer server.Shutdown()
	select {
	case e = <-done:
		if e != nil {
			return e
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	return save("/run/output/"+stage+"-queue.json", map[string]any{"work": work, "outbox": "PUBLISHED", "task_id": taskID, "actual_asynq_worker": true, "native_delete": 0, "selected_only": true})
}
