// The controlled B2 API uses the same application/HTTP path as live tests.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
)

func main() {
	// Database secret is referenced by a private file, never a command argument.
	dsn, err := os.ReadFile(os.Getenv("INGESTION_CORE_DSN_FILE"))
	if err != nil {
		log.Fatal("Core credential file unavailable")
	}
	pool, err := pgxpool.New(context.Background(), string(dsn))
	if err != nil {
		log.Fatal("Core configuration invalid")
	}
	defer pool.Close()
	router, err := routing.NewRouter(false)
	if err != nil {
		log.Fatal("routing invalid")
	}
	outbox.ConfigureAppendObligation(router)
	h, err := pilot.Build(pool, os.Getenv("INGESTION_PROFILE"))
	if err != nil {
		log.Fatal("ingestion profile invalid")
	}
	address := os.Getenv("INGESTION_LISTEN")
	if address == "" {
		address = "127.0.0.1:8098"
	}
	server := &http.Server{Addr: address, Handler: httpserver.NewMuxWithReadiness(pool.Ping, h.Register), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second}
	log.Print("controlled ingestion API listening")
	if err = server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal("ingestion API stopped")
	}
}
