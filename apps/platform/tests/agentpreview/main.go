// Isolated acceptance host: real Agent application + HTTP, no other Commands,
// engine adapters, model clients, worker, Redis or object-store runtime.
package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/agentquality"
	agentqualityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/agentquality/infrastructure"
	agentqualityhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/agentquality/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/agenttask/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/agenttask/infrastructure"
	agenthttp "github.com/qq550723504/data-product-platform/apps/platform/internal/agenttask/transport/http"
	certificationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/application"
	certificationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/infrastructure"
	certificationhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	qualityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/infrastructure"
	remediation "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/remediation/application"
	remediationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/remediation/infrastructure"
	qualityhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/readmodel"
	readmodelhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/readmodel/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/traceability"
	traceabilityhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/traceability/transport/http"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	if os.Getenv("AGENT_Q2_PREVIEW_ACK") != "ISOLATED_PLAN_ONLY" {
		log.Fatal("explicit isolated preview acknowledgement required")
	}
	root, err := os.Getwd()
	must(err)
	ctx := context.Background()
	cfg, writeEnabled, err := previewConfig(os.Getenv("AGENT_Q2_PREVIEW_DSN"), os.Getenv("AGENT_TASKS_WRITE_ENABLED"))
	must(err)
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	must(err)
	defer db.Close()
	must(db.Ping(ctx))
	resolver, err := principal.NewStaticResolver(true, "agent-q2-core-only", "agent-q2-operator", "22222222-2222-4222-8222-222222222222", []string{"11111111-1111-4111-8111-111111111111"}, []string{principal.CapabilityHumanDecision})
	must(err)
	template, err := remediation.LoadPlan(filepath.Join(root, "industry-packs"), "park/remediation/energy-quarantine-v1.yaml")
	must(err)
	router, err := routing.NewRouter(false)
	must(err)
	outbox.ConfigureAppendObligation(router)
	handler := agenthttp.Handler{Principal: resolver, WriteEnabled: writeEnabled, Service: &application.Service{Tx: transaction.NewManager(db), Repo: &infrastructure.Repository{Pool: db}, Remediation: &remediation.Service{Plan: template, Repo: remediationinfra.NewRepository(db)}}}
	mux := http.NewServeMux()
	handler.Register(mux)
	contextTemplate := agentquality.Template{Configured: true, Name: template.Metadata.Name, Version: template.Metadata.Version, Hash: template.Hash, WorkflowHash: template.WorkflowHash, Inputs: []agentquality.Input{}}
	for _, input := range template.Inputs {
		contextTemplate.Inputs = append(contextTemplate.Inputs, agentquality.Input{Name: input.Name, DatasetType: input.DatasetType, Required: input.Required})
	}
	contextHandler := agentqualityhttp.Handler{Principal: resolver, Service: &agentquality.Service{Reader: agentqualityinfra.Reader{Catalog: readmodel.NewRepository(db), DatasetRepo: datasetinfra.NewPostgresRepository(db)}, Template: contextTemplate}}
	contextHandler.Register(mux)
	qualityHandler := qualityhttp.NewHandlerWithCost(nil, qualityinfra.NewPostgresRepository(db), evidence.NewQueryRepository(db), cost.NewQueryRepository(db))
	qualityHandler.Principal = resolver
	qualityHandler.RegisterReadOnly(mux)
	certificationService := certificationapp.NewCertificationService(nil, certificationinfra.NewProfileRepository(db), certificationinfra.NewCertificationRepository(db), nil)
	certificationHandler := certificationhttp.NewHandler(certificationService, nil, datasetinfra.NewPostgresRepository(db))
	certificationHandler.Principal = resolver
	certificationHandler.RegisterHistoryReadOnly(mux)
	releaseHandler := readmodelhttp.NewHandler(readmodel.NewRepository(db))
	releaseHandler.Principal = resolver
	releaseHandler.RegisterReleaseHistoryReadOnly(mux)
	traceHandler := traceabilityhttp.NewHandler(traceability.NewRepository(db))
	traceHandler.Principal = resolver
	traceHandler.Register(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	log.Print("Isolated real Agent planning API on 127.0.0.1:18497; dispatch disabled")
	log.Fatal(http.ListenAndServe("127.0.0.1:18497", mux))
}
func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func previewConfig(dsn, writeValue string) (*pgxpool.Config, bool, error) {
	if dsn == "" {
		return nil, false, fmt.Errorf("explicit independent database DSN required")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, false, fmt.Errorf("invalid preview database configuration")
	}
	if !strings.HasPrefix(cfg.ConnConfig.Database, "agent_q2_acceptance_") {
		return nil, false, fmt.Errorf("dedicated acceptance database required")
	}
	enabled := false
	if writeValue != "" {
		enabled, err = strconv.ParseBool(writeValue)
		if err != nil {
			return nil, false, fmt.Errorf("invalid preview write setting")
		}
	}
	if !enabled {
		cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	}
	return cfg, enabled, nil
}
