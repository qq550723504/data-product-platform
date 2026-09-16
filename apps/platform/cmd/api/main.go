package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	contractapp "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/application"
	contractinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/infrastructure"
	contracthttp "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	datasethttp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/transport/http"
	entityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/application"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	entityhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/config"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/queue"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	productapp "github.com/qq550723504/data-product-platform/apps/platform/internal/product/application"
	productinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
	producthttp "github.com/qq550723504/data-product-platform/apps/platform/internal/product/transport/http"
	resourceapp "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/application"
	resourceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
	resourcehttp "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/transport/http"
	rightsapp "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/application"
	rightsinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
	rightshttp "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/transport/http"
	traceabilityhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/traceability/transport/http"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
	workflowhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/transport/http"
	workflowqueue "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/transport/queue"
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

	objectStore, err := storage.New(
		cfg.Storage.Endpoint,
		cfg.Storage.AccessKey,
		cfg.Storage.SecretKey,
		cfg.Storage.Bucket,
		cfg.Storage.UseSSL,
	)
	if err != nil {
		logger.Error("create object storage client", "error", err)
		os.Exit(1)
	}
	if err := objectStore.EnsureBucket(ctx); err != nil {
		logger.Error("ensure object storage bucket", "error", err)
		os.Exit(1)
	}

	queueClient := queue.NewClient(queue.Config{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer queueClient.Close()
	if err := queue.EnqueueHealthPing(ctx, queueClient); err != nil {
		logger.Error("verify Redis queue", "error", err)
		os.Exit(1)
	}

	txManager := transaction.NewManager(db)

	resourceRepo := resourceinfra.NewPostgresRepository()
	resourceHandler := resourcehttp.NewHandler(resourceapp.NewCreateService(txManager, resourceRepo))

	datasetRepo := datasetinfra.NewPostgresRepository(db)
	datasetWriter := datasetapp.NewUploadVersionService(txManager, datasetRepo, objectStore)
	datasetHandler := datasethttp.NewHandler(
		datasetapp.NewCreateDatasetService(txManager, datasetRepo),
		datasetWriter,
		datasetapp.NewInvalidateVersionService(txManager, datasetRepo),
		datasetRepo,
	)

	entityRepo := entityinfra.NewPostgresRepository(db)
	entityService := entityapp.NewMatchService(
		cfg.IndustryPackRoot,
		txManager,
		entityRepo,
		datasetRepo,
		datasetWriter,
		objectStore,
	)
	entityHandler := entityhttp.NewHandler(entityService, entityRepo)

	workflowRepo := workflowinfra.NewPostgresRepository(db)
	workflowVersionService := workflowapp.NewWorkflowVersionService(txManager, workflowRepo)
	workflowQueueClient := workflowqueue.NewClient(queueClient)
	executionService := workflowapp.NewExecutionService(txManager, workflowRepo, workflowQueueClient)
	workflowHandler := workflowhttp.NewHandler(workflowVersionService, executionService, workflowRepo)

	productRepo := productinfra.NewPostgresRepository(db)
	productService := productapp.NewService(txManager, productRepo)
	productHandler := producthttp.NewHandler(productService, productRepo)

	rightsRepo := rightsinfra.NewPostgresRepository(db)
	rightsService := rightsapp.NewService(txManager, rightsRepo)
	rightsHandler := rightshttp.NewHandler(rightsService, rightsRepo)

	contractRepo := contractinfra.NewPostgresRepository(db)
	contractService := contractapp.NewService(txManager, contractRepo)
	contractHandler := contracthttp.NewHandler(contractService, contractRepo)

	traceabilityHandler := traceabilityhttp.NewHandler(
		evidence.NewQueryRepository(db),
		cost.NewQueryRepository(db),
	)

	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpserver.NewMux(
			resourceHandler.Register,
			datasetHandler.Register,
			entityHandler.Register,
			workflowHandler.Register,
			productHandler.Register,
			rightsHandler.Register,
			contractHandler.Register,
			traceabilityHandler.Register,
		),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("platform API started", "addr", cfg.HTTPAddr, "env", cfg.Environment)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown requested")
	case err := <-errCh:
		logger.Error("HTTP server stopped unexpectedly", "error", err)
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown", "error", err)
	}
}
