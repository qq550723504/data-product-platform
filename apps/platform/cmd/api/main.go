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

	"github.com/qq550723504/data-product-platform/apps/platform/internal/agentquality"
	agentqualityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/agentquality/infrastructure"
	agentqualityhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/agentquality/transport/http"
	agenttaskapp "github.com/qq550723504/data-product-platform/apps/platform/internal/agenttask/application"
	agenttaskinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/agenttask/infrastructure"
	agenttaskhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/agenttask/transport/http"
	annotationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/application"
	annotationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/infrastructure"
	annotationhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/transport/http"
	certificationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/application"
	certificationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/infrastructure"
	certificationhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/transport/http"
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
	deliveryapp "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/application"
	deliveryinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/infrastructure"
	deliveryhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/transport/http"
	entityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/application"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	entitysplink "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/splink"
	entityhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	externalpilot "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/pilot"
	goldapp "github.com/qq550723504/data-product-platform/apps/platform/internal/gold/application"
	goldinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/gold/infrastructure"
	goldhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/gold/transport/http"
	ingestionpilot "github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/pilot"
	lifecyclepilot "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	metadataapp "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/application"
	metadatainfra "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/openmetadata"
	metadatahttp "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/config"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	platformprincipal "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/queue"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	productapp "github.com/qq550723504/data-product-platform/apps/platform/internal/product/application"
	productinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
	producthttp "github.com/qq550723504/data-product-platform/apps/platform/internal/product/transport/http"
	qualityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/application"
	qualitygx "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/gx"
	qualityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/infrastructure"
	remediationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/remediation/application"
	remediationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/remediation/infrastructure"
	remediationhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/remediation/transport/http"
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

	var objectStore storage.RuntimeStore
	if cfg.Storage.WorkspaceProfile != "" {
		objectStore, err = lifecyclepilot.BuildWorkspaceObjects(db, cfg.Storage.WorkspaceProfile)
	} else {
		var bootstrap *storage.Store
		bootstrap, err = storage.New(cfg.Storage.Endpoint, cfg.Storage.AccessKey, cfg.Storage.SecretKey, cfg.Storage.Bucket, cfg.Storage.UseSSL)
		if err == nil {
			err = bootstrap.EnsureBucket(ctx)
		}
		objectStore = bootstrap
	}
	if err != nil {
		logger.Error("configure object storage runtime", "error", "storage binding unavailable")
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

	// Queue delivery is asynchronous via the worker/outbox. Redis outages
	// must not remove PostgreSQL-backed API command handling from service.
	readiness := func(ctx context.Context) error {
		return db.Ping(ctx)
	}

	txManager := transaction.NewManager(db)
	readModelRepo := readmodel.NewRepository(db)
	readModelHandler := readmodelhttp.NewHandler(readModelRepo)

	resourceRepo := resourceinfra.NewPostgresRepository()
	resourceHandler := resourcehttp.NewHandler(resourceapp.NewCreateService(txManager, resourceRepo))

	datasetRepo := datasetinfra.NewPostgresRepository(db)
	datasetWriter := datasetapp.NewUploadVersionService(txManager, datasetRepo, objectStore)
	if lease, ok := objectStore.(datasetapp.VersionPublicationLease); ok {
		datasetWriter.WithPublicationLease(lease)
	}
	datasetHandler := datasethttp.NewHandler(
		datasetapp.NewCreateDatasetService(txManager, datasetRepo, resourceRepo),
		datasetWriter,
		datasetapp.NewInvalidateVersionService(txManager, datasetRepo),
		datasetapp.NewFailVersionService(txManager, datasetRepo),
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
	humanDecisionResolver, err := platformprincipal.NewStaticResolver(
		cfg.HumanDecisionAPI.Enabled,
		cfg.HumanDecisionAPI.Token,
		cfg.HumanDecisionAPI.Subject,
		cfg.HumanDecisionAPI.ActorID,
		cfg.HumanDecisionAPI.WorkspaceIDs,
		[]string{platformprincipal.CapabilityHumanDecision},
	)
	if err != nil {
		logger.Error("configure human decision principal boundary", "error", err)
		os.Exit(1)
	}
	entityHandler := entityhttp.NewHandler(entityService, entityRepo, humanDecisionResolver)

	annotationRepo := annotationinfra.NewRepository(db)
	annotationService := annotationapp.NewService(txManager, annotationRepo, nil)
	goldRepo := goldinfra.NewPostgresRepository(db)
	annotationHandler := annotationhttp.NewHandler(annotationService, humanDecisionResolver)

	workflowRepo := workflowinfra.NewPostgresRepository(db)
	workflowVersionService := workflowapp.NewWorkflowVersionService(txManager, workflowRepo)
	executionService := workflowapp.NewExecutionService(txManager, workflowRepo)
	workflowHandler := workflowhttp.NewHandler(workflowVersionService, executionService, workflowRepo)
	goldService := goldapp.NewService(
		txManager,
		goldRepo,
		workflowRepo,
		annotationRepo,
		datasetRepo,
		datasetWriter,
		objectStore,
	)
	goldHandler := goldhttp.NewHandler(goldService)

	productRepo := productinfra.NewPostgresRepository(db)
	productService := productapp.NewService(txManager, productRepo)
	productHandler := producthttp.NewHandler(productService, productRepo)

	rightsRepo := rightsinfra.NewPostgresRepository(db)
	rightsService := rightsapp.NewService(txManager, rightsRepo)
	rightsAuthorizer, err := rightshttp.NewStaticAuthorizer(cfg.RightsAPI.Token, cfg.RightsAPI.ActorID, cfg.RightsAPI.WorkspaceIDs)
	if err != nil {
		logger.Error("configure rights API authorization", "error", err)
		os.Exit(1)
	}
	rightsHandler := rightshttp.NewHandler(rightsService, rightsRepo, rightsAuthorizer)
	workflowHandler.Authorize = rightsAuthorizer.Authorize
	entityHandler.Authorize = rightsAuthorizer.Authorize

	contractRepo := contractinfra.NewPostgresRepository(db)
	contractService := contractapp.NewService(txManager, contractRepo)
	contractHandler := contracthttp.NewHandler(contractService, contractRepo)

	qualityRepo := qualityinfra.NewPostgresRepository(db)
	qualityService := qualityapp.NewService(cfg.IndustryPackRoot, txManager, datasetRepo, qualityRepo, objectStore, evidence.NewQueryRepository(db)).
		ConfigureGold(goldRepo, annotationService)
	if cfg.GX.Enabled {
		gxClient, err := qualitygx.NewClient(qualitygx.Config{
			BaseURL:               cfg.GX.BaseURL,
			Token:                 cfg.GX.Token,
			ExpectedEngineVersion: cfg.GX.ExpectedEngineVersion,
			Timeout:               time.Duration(cfg.GX.TimeoutSeconds) * time.Second,
		}, nil)
		if err != nil {
			logger.Error("create GX quality engine", "error", err)
			os.Exit(1)
		}
		if err := gxClient.Probe(ctx); err != nil {
			logger.Error("probe GX quality engine", "error", err)
			os.Exit(1)
		}
		if err := qualityService.RegisterEngine(gxClient); err != nil {
			logger.Error("register GX quality engine", "error", err)
			os.Exit(1)
		}
		logger.Info("GX quality engine enabled",
			"base_url", cfg.GX.BaseURL,
			"engine_version", cfg.GX.ExpectedEngineVersion,
		)
	}
	qualityHandler := qualityhttp.NewHandlerWithCost(qualityService, qualityRepo, evidence.NewQueryRepository(db), cost.NewQueryRepository(db))
	qualityHandler.Principal = humanDecisionResolver
	contractHandler.Principal = humanDecisionResolver
	readModelHandler.Principal = humanDecisionResolver
	remediationRepo := remediationinfra.NewRepository(db)
	var remediationService *remediationapp.Service
	agentTemplate := agentquality.Template{Inputs: []agentquality.Input{}}
	if ref := os.Getenv("QUALITY_REMEDIATION_PLAN_REF"); ref != "" {
		plan, err := remediationapp.LoadPlan(cfg.IndustryPackRoot, ref)
		if err != nil {
			logger.Error("configure remediation plan", "error", err)
			os.Exit(1)
		}
		remediationService = &remediationapp.Service{Tx: txManager, Repo: remediationRepo, Quality: qualityService, Executions: executionService, Workflows: workflowRepo, Plan: plan}
		agentTemplate = agentquality.Template{Configured: true, Name: plan.Metadata.Name, Version: plan.Metadata.Version, Hash: plan.Hash, WorkflowHash: plan.WorkflowHash, Inputs: []agentquality.Input{}}
		for _, input := range plan.Inputs {
			agentTemplate.Inputs = append(agentTemplate.Inputs, agentquality.Input{Name: input.Name, DatasetType: input.DatasetType, Required: input.Required})
		}
	}
	remediationHandler := remediationhttp.NewHandler(remediationService, remediationRepo, humanDecisionResolver)
	agentTaskHandler := &agenttaskhttp.Handler{Principal: humanDecisionResolver, WriteEnabled: cfg.AgentTasksWriteEnabled,
		Service: &agenttaskapp.Service{Tx: txManager, Repo: &agenttaskinfra.Repository{Pool: db}, Remediation: remediationService}}
	agentQualityHandler := &agentqualityhttp.Handler{Principal: humanDecisionResolver, Service: &agentquality.Service{
		Reader: agentqualityinfra.Reader{Catalog: readModelRepo, DatasetRepo: datasetRepo}, Template: agentTemplate,
	}}

	certificationProfileRepo := certificationinfra.NewProfileRepository(db)
	certificationRepo := certificationinfra.NewCertificationRepository(db)
	certificationService := certificationapp.NewCertificationService(txManager, certificationProfileRepo, certificationRepo, certificationapp.NewReferenceEvidenceResolver())
	certificationEligibility := certificationapp.NewEligibilityService(certificationService, datasetRepo, rightsRepo)
	certificationHandler := certificationhttp.NewHandler(certificationService, certificationEligibility, datasetRepo)
	certificationHandler.Authorize = rightsAuthorizer.Authorize
	certificationHandler.Principal = humanDecisionResolver
	certificationHandler.Rights = rightsService

	deliveryRepo := deliveryinfra.NewPostgresRepository(db)
	deliveryGate := deliveryapp.NewCertificationDirectDataGate(certificationEligibility)
	directDataService := deliveryapp.NewDirectDataService(txManager, deliveryRepo, deliveryGate, datasetRepo)
	deliveryPrincipalResolver, err := deliveryhttp.NewStaticPrincipalResolver(
		cfg.DeliveryAPI.Enabled,
		cfg.DeliveryAPI.Token,
		cfg.DeliveryAPI.PrincipalRef,
		cfg.DeliveryAPI.ConsumerRef,
		cfg.DeliveryAPI.WorkspaceIDs,
	)
	if err != nil {
		logger.Error("configure direct delivery trusted caller boundary", "error", err)
		os.Exit(1)
	}
	deliveryHandler := deliveryhttp.NewHandler(directDataService, deliveryPrincipalResolver, objectStore)

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
			openmetadata.Provider,
			cfg.OpenMetadata.Domain,
			txManager,
			metadataRepo,
			productRepo,
			metadataEngine,
		)
		if err := openmetadata.ConfigureSourceSchema(metadataService, db, os.Getenv("OPENMETADATA_SCHEMA_PROFILE")); err != nil {
			logger.Error("configure finite source schema observer", "error", err)
			os.Exit(1)
		}
	}
	metadataHandler := metadatahttp.NewHandler(metadataService, metadataRepo)
	metadataHandler.Authorize = rightsAuthorizer.Authorize
	registerIngestion := func(*http.ServeMux) {}
	if path := os.Getenv("INGESTION_PROFILE"); path != "" {
		h, err := ingestionpilot.Build(db, path)
		if err != nil {
			logger.Error("ingestion profile invalid")
			os.Exit(1)
		}
		registerIngestion = h.Register
	}

	traceabilityHandler := traceabilityhttp.NewHandler(traceability.NewRepository(db))
	traceabilityHandler.Principal = humanDecisionResolver
	registerSourceAdmission, sourceAdmissionErr := sourceAdmissionRoutes(db)
	if sourceAdmissionErr != nil {
		logger.Error("configure fork-approved source admission")
		os.Exit(1)
	}
	externalHandler, err := externalpilot.Build(db)
	if err != nil {
		logger.Error("configure external integration principal boundary", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpserver.NewMuxWithReadiness(readiness,
			readModelHandler.Register,
			resourceHandler.Register,
			datasetHandler.Register,
			entityHandler.Register,
			annotationHandler.Register,
			workflowHandler.Register,
			goldHandler.Register,
			productHandler.Register,
			productHandler.RegisterValidation,
			productHandler.RegisterPublish,
			rightsHandler.Register,
			contractHandler.Register,
			qualityHandler.Register,
			agentQualityHandler.Register,
			agentTaskHandler.Register,
			remediationHandler.Register,
			certificationHandler.Register,
			deliveryHandler.Register,
			complianceHandler.Register,
			metadataHandler.Register,
			registerIngestion,
			registerSourceAdmission,
			traceabilityHandler.Register,
			externalHandler.Register,
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
