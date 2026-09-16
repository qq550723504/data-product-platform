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

	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	datasethttp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/transport/http"
	entityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/application"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	entityhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/config"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/queue"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	resourceapp "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/application"
	resourceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
	resourcehttp "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/transport/http"
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

	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpserver.NewMux(
			resourceHandler.Register,
			datasetHandler.Register,
			entityHandler.Register,
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
