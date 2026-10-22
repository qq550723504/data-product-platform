// workspace-object-copy adds one verified physical location for an existing
// version. It accepts explicit custody bytes, never root/source credentials.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
)

func main() {
	profile := flag.String("profile", "", "private workspace object profile")
	version := flag.String("version", "", "existing DatasetVersion UUID")
	file := flag.String("content-file", "", "private file containing the unchanged custody bytes")
	key := flag.String("idempotency-key", "", "stable explicit copy command identity")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	id, err := uuid.Parse(*version)
	if err != nil || id == uuid.Nil || !filepath.IsAbs(*file) || *key == "" || *profile == "" {
		fail()
	}
	info, err := os.Stat(*file)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 128<<20 {
		fail()
	}
	data, err := os.ReadFile(*file)
	if err != nil {
		fail()
	}
	pool, err := database.Open(ctx, os.Getenv("POSTGRES_DSN"))
	if err != nil {
		fail()
	}
	defer pool.Close()
	router, err := routing.NewRouter(false)
	if err != nil {
		fail()
	}
	outbox.ConfigureAppendObligation(router)
	service, err := pilot.BuildWorkspaceObjects(pool, *profile)
	if err != nil {
		fail()
	}
	uri, err := service.AddRetainedCopy(ctx, id, *key, data)
	if err != nil {
		fail()
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "VERIFIED", "datasetVersionId": id, "physicalCopyURI": uri, "businessVersionChanged": false})
}
func fail() {
	fmt.Fprintln(os.Stderr, "workspace copy refused/unknown; retain command and inspect immutable receipts")
	os.Exit(1)
}
