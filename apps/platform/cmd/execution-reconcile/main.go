package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/config"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
)

func main() {
	apply := flag.Bool("apply", false, "create idempotent outbox dispatch records; default is report-only")
	limit := flag.Int("limit", 100, "maximum number of QUEUED executions to inspect")
	flag.Parse()
	if *limit <= 0 {
		fmt.Fprintln(os.Stderr, "-limit must be positive")
		os.Exit(2)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}
	router, err := routing.NewRouter(cfg.OpenMetadata.Enabled)
	if err != nil {
		logger.Error("build outbox routing table", "error", err)
		os.Exit(1)
	}
	outbox.ConfigureAppendObligation(router)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	pool, err := database.Open(ctx, cfg.PostgresDSN)
	if err != nil {
		logger.Error("open postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	reconciler := workflowapp.NewQueuedExecutionReconciler(transaction.NewManager(pool), workflowinfra.NewPostgresRepository(pool))
	reports, err := reconciler.Run(ctx, *apply, *limit)
	if err != nil {
		logger.Error("reconcile queued executions", "error", err, "apply", *apply)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(map[string]any{
		"apply":   *apply,
		"count":   len(reports),
		"reports": reports,
	}); err != nil {
		logger.Error("write reconciliation report", "error", err)
		os.Exit(1)
	}
}
