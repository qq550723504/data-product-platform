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

	complianceapp "github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/application"
	complianceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/infrastructure"
	compliancehttp "github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/transport/http"
	contractapp "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/application"
	contractinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/infrastructure"
	contracthttp "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	datasethttp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/transport/http"
	entityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/application"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	entitysplink "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/splink"
	entityhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	metadataapp "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/application"
	metadatadomain "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
	metadatainfra "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/openmetadata"
	metadatahttp "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/config"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/queue"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	productapp "github.com/qq550723504/data-product-platform/apps/platform/internal/product/application"
	productinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
	producthttp "github.com/qq550723504/data-product-platform/apps/platform/internal/product/transport/http"
	qualityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/application"
	qualityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/infrastructure"
	qualityhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/readmodel"
	readmodelhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/readmodel/transport/http"
	resourceapp "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/application"
	resourceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
	resourcehttp "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/transport/http"
	rightsapp "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/application"
	rightsinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
	rightshttp "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/traceability"
	traceabilityhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/traceability/transport/http"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
	workflowhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/transport/http"
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

	// Freeze every event's handler obligation as it is recorded, so the
	// obligation belongs to the event rather than to whichever worker profile
	// dispatches it later. The governance profile is fixed for this process.
	obligationRouter, err := routing.NewRouter(cfg.OpenMetadata.Enabled)
	if err != nil {
		logger.Error("build outbox routing table", "error", err)
		os.Exit(1)
	}
	outbox.ConfigureAppendObligation(obligationRouter)
	logger.Info("outbox routing obligation configured",
		"routing_version", obligationRouter.Version(),
		"governance_projection", cfg.OpenMetadata.Enabled,
	)

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
	readModelHandler := readmodelhttp.NewHandler(readmodel.NewRepository(db))

	resourceRepo := resourceinfra.NewPostgresRepository()
	resourceHandler := resourcehttp.NewHandler(resourceapp.NewCreateService(txManager, resourceRepo))

	datasetRepo := datasetinfra.NewPostgresRepository(db)
	datasetWriter := datasetapp.NewUploadVersionService(txManager, datasetRepo, objectStore)
	datasetHandler := datasethttp.NewHandler(
		datasetapp.NewCreateDatasetService(txManager, datasetRepo, resourceRepo),
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
	if cfg.Splink.Enabled {
		splinkClient, err := entitysplink.NewClient(entitysplink.Config{
			BaseURL:               cfg.Splink.BaseURL,
			Token:                 cfg.Splink.Token,
			ExpectedEngineVersion: cfg.Splink.ExpectedEngineVersion,
			ModelRef:              cfg.Splink.ModelRef,
			ModelVersion:          cfg.Splink.ModelVersion,
			PolicyRef:             cfg.Splink.PolicyRef,
			PolicyVersion:         cfg.Splink.PolicyVersion,
			Timeout:               time.Duration(cfg.Splink.TimeoutSeconds) * time.Second,
		}, nil)
		if err != nil {
			logger.Error("create Splink candidate engine", "error", err)
			os.Exit(1)
		}
		health, err := splinkClient.Probe(ctx)
		if err != nil {
			logger.Error("probe Splink candidate engine", "error", err)
			os.Exit(1)
		}
		entityService.UseCandidateGenerator(splinkClient)
		logger.Info(
			"Splink candidate engine enabled",
			"base_url", cfg.Splink.BaseURL,
			"engine_version", health.EngineVersion,
			"model_ref", cfg.Splink.ModelRef,
			"model_version", cfg.Splink.ModelVersion,
			"policy_ref", cfg.Splink.PolicyRef,
			"policy_version", cfg.Splink.PolicyVersion,
		)
	}
	entityHandler := entityhttp.NewHandler(entityService, entityRepo)

	workflowRepo := workflowinfra.NewPostgresRepository(db)
	workflowVersionService := workflowapp.NewWorkflowVersionService(txManager, workflowRepo)
	executionService := workflowapp.NewExecutionService(txManager, workflowRepo)
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

	qualityRepo := qualityinfra.NewPostgresRepository(db)
	qualityService := qualityapp.NewService(cfg.IndustryPackRoot, txManager, datasetRepo, qualityRepo, objectStore)
	qualityHandler := qualityhttp.NewHandler(qualityService, qualityRepo)

	complianceRepo := complianceinfra.NewPostgresRepository(db)
	complianceService := complianceapp.NewService(cfg.IndustryPackRoot, txManager, datasetRepo, complianceRepo, objectStore)
	complianceHandler := compliancehttp.NewHandler(complianceService, complianceRepo)

	metadataRepo := metadatainfra.NewPostgresRepository(db)
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
			metadataRepo,
			productRepo,
			metadataEngine,
		)
	}
	metadataHandler := metadatahttp.NewHandler(metadataService, metadataRepo)

	traceabilityHandler := traceabilityhttp.NewHandler(
		evidence.NewQueryRepository(db),
		cost.NewQueryRepository(db),
		traceability.NewRepository(db),
	)

	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpserver.NewMux(
			readModelHandler.Register,
			resourceHandler.Register,
			datasetHandler.Register,
			entityHandler.Register,
			workflowHandler.Register,
			productHandler.Register,
			productHandler.RegisterValidation,
			productHandler.RegisterPublish,
			rightsHandler.Register,
			contractHandler.Register,
			qualityHandler.Register,
			complianceHandler.Register,
			metadataHandler.Register,
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
