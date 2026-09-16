package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/config"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/queue"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}

	db, err := database.Open(ctx, cfg.PostgresDSN)
	if err != nil {
		logger.Error("open postgres", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	server := queue.NewServer(queue.Config{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	mux := asynq.NewServeMux()
	mux.HandleFunc(queue.TaskHealthPing, func(_ context.Context, task *asynq.Task) error {
		logger.Info("worker health ping consumed", "task_type", task.Type())
		return nil
	})

	errCh := make(chan error, 2)
	go func() {
		logger.Info("asynq worker started", "redis", cfg.Redis.Addr, "env", cfg.Environment)
		if err := server.Run(mux); err != nil {
			errCh <- err
		}
	}()

	publisher := outbox.NewPublisher(db, time.Second)
	go func() {
		logger.Info("outbox publisher started")
		err := publisher.Run(ctx, func(_ context.Context, event outbox.PublishedEvent) error {
			logger.Info(
				"domain event published",
				"event_id", event.ID,
				"event_type", event.EventType,
				"aggregate_type", event.AggregateType,
				"aggregate_id", event.AggregateID,
				"attempt", event.Attempts,
			)
			return nil
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("worker shutdown requested")
	case err := <-errCh:
		logger.Error("worker stopped unexpectedly", "error", err)
		stop()
	}

	server.Shutdown()
}
