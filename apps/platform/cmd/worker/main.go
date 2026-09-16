package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/hibiken/asynq"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/config"
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

	errCh := make(chan error, 1)
	go func() {
		logger.Info("platform worker started", "redis", cfg.Redis.Addr, "env", cfg.Environment)
		if err := server.Run(mux); err != nil {
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
