package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	metadataapp "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/application"
	metadatadomain "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
	metadatainfra "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/openmetadata"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/config"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/queue"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	productinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowhop "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/hop"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
	nativeengine "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/native"
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

	server := queue.NewServer(queue.Config{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	txManager := transaction.NewManager(db)
	datasetRepo := datasetinfra.NewPostgresRepository(db)
	entityRepo := entityinfra.NewPostgresRepository(db)
	workflowRepo := workflowinfra.NewPostgresRepository(db)
	datasetWriter := datasetapp.NewUploadVersionService(txManager, datasetRepo, objectStore)
	executionService := workflowapp.NewExecutionService(txManager, workflowRepo, nil)
	processingEngine := nativeengine.NewEngine(
		cfg.IndustryPackRoot,
		txManager,
		datasetRepo,
		entityRepo,
		workflowRepo,
		datasetWriter,
		objectStore,
	)

	managedBridges := make([]workflowapp.ManagedExecutionBridge, 0, 1)
	var managedReconciler *workflowapp.ManagedReconciler
	if cfg.Hop.Enabled {
		hopClient, err := workflowhop.NewClient(cfg.Hop.BaseURL, cfg.Hop.Username, cfg.Hop.Password, nil)
		if err != nil {
			logger.Error("create Apache Hop client", "error", err)
			os.Exit(1)
		}
		artifactRoot := filepath.Dir(filepath.Clean(cfg.IndustryPackRoot))
		hopBridge, err := workflowhop.NewBridge(hopClient, artifactRoot, txManager, datasetRepo, datasetWriter, objectStore)
		if err != nil {
			logger.Error("create Apache Hop bridge", "error", err)
			os.Exit(1)
		}
		managedBridges = append(managedBridges, hopBridge)
		managedReconciler = workflowapp.NewManagedReconciler(executionService, workflowRepo, hopBridge)
		logger.Info("Apache Hop managed execution enabled", "base_url", cfg.Hop.BaseURL, "artifact_root", artifactRoot)
	}
	workflowTaskHandler := workflowqueue.NewHandler(executionService, workflowRepo, processingEngine, managedBridges...)

	var metadataService *metadataapp.Service
	if cfg.OpenMetadata.Enabled {
		metadataEngine, err := openmetadata.NewClient(cfg.OpenMetadata.BaseURL, cfg.OpenMetadata.Token, nil)
		if err != nil {
			logger.Error("create OpenMetadata client", "error", err)
			os.Exit(1)
		}
		metadataService = metadataapp.NewService(
			metadatadomain.ProviderOpenMetadata,
			cfg.OpenMetadata.Domain,
			txManager,
			metadatainfra.NewPostgresRepository(db),
			productinfra.NewPostgresRepository(db),
			metadataEngine,
		)
		logger.Info("OpenMetadata governance projection enabled", "base_url", cfg.OpenMetadata.BaseURL, "domain", cfg.OpenMetadata.Domain)
	}

	mux := asynq.NewServeMux()
	mux.HandleFunc(queue.TaskHealthPing, func(_ context.Context, task *asynq.Task) error {
		logger.Info("worker health ping consumed", "task_type", task.Type())
		return nil
	})
	mux.HandleFunc(workflowqueue.TaskExecute, func(ctx context.Context, task *asynq.Task) error {
		err := workflowTaskHandler.Handle(ctx, task)
		if err != nil {
			logger.Error("workflow execution task failed", "task_type", task.Type(), "error", err)
		}
		return err
	})

	errCh := make(chan error, 2)
	go func() {
		logger.Info("asynq worker started", "redis", cfg.Redis.Addr, "env", cfg.Environment)
		if err := server.Run(mux); err != nil {
			errCh <- err
		}
	}()

	if managedReconciler != nil {
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			logger.Info("managed execution reconciler started")
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := managedReconciler.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
						// Remote engines are independent runtimes. A transient status/finalization
						// failure must not stop the worker; the next tick retries reconciliation.
						logger.Warn("managed execution reconciliation failed", "error", err)
					}
				}
			}
		}()
	}

	publisher := outbox.NewPublisher(db, outbox.Config{
		PollInterval: time.Second,
		ConsumerName: "metadata-projection",
		Logger:       logger,
	})
	go func() {
		logger.Info("outbox publisher started")
		err := publisher.Run(ctx, func(eventCtx context.Context, event outbox.PublishedEvent) error {
			if metadataService != nil {
				if err := metadataService.HandleOutboxEvent(eventCtx, event); err != nil {
					logger.Error(
						"governance projection failed",
						"event_id", event.ID,
						"event_type", event.EventType,
						"aggregate_id", event.AggregateID,
						"attempt", event.Attempts,
						"error", err,
					)
					return err
				}
			}
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
