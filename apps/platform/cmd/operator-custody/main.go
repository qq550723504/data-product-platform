// operator-custody is a separately authorized, private operator composition.
// It never starts services, migrates a DB, issues STS or creates protected copies.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/minio/minio-go/v7/pkg/credentials"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
)

func main() {
	profile := flag.String("profile", "", "reviewed private operator profile (no inline credentials)")
	digest := flag.String("profile-sha256", "", "reviewed profile bytes SHA256; identity, not execution authority")
	key := flag.String("idempotency-key", "", "explicit acquisition identity; incomplete replay never reads again")
	flag.Parse()
	if err := run(*profile, *digest, *key); err != nil {
		fmt.Fprintln(os.Stderr, "custody refused/unknown; preserve private files and Core receipts; no automatic retry")
		os.Exit(1)
	}
}

func run(profile, digest, key string) error {
	b, err := pilot.ReadCustodyPrivateFile(profile, 128<<10)
	if err != nil {
		return lc.ErrCustodyScope
	}
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != digest {
		return lc.ErrCustodyScope
	}
	var cfg struct {
		Plan                 lc.CustodyPlan `json:"plan"`
		CredentialFile       string         `json:"credentialFile"`
		CredentialFileSHA256 string         `json:"credentialFileSha256"`
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if dec.Decode(&cfg) != nil || dec.Decode(&struct{}{}) != io.EOF || cfg.Plan.Validate() != nil {
		return lc.ErrCustodyScope
	}
	secret, err := pilot.ReadCustodyPrivateFile(cfg.CredentialFile, 128<<10)
	if err != nil {
		return lc.ErrCustodyScope
	}
	sh := sha256.Sum256(secret)
	if hex.EncodeToString(sh[:]) != cfg.CredentialFileSHA256 {
		return lc.ErrCustodyScope
	}
	var native struct{ RootKey, RootSecret, StorageEndpoint string }
	if json.Unmarshal(secret, &native) != nil || native.RootKey == "" || native.RootSecret == "" || native.StorageEndpoint != cfg.Plan.Endpoint {
		return lc.ErrCustodyScope
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := database.Open(ctx, os.Getenv("POSTGRES_DSN"))
	if err != nil {
		return lc.ErrCustodyScope
	}
	defer pool.Close()
	var schema int64
	if pool.QueryRow(ctx, `SELECT max(version) FROM schema_migration`).Scan(&schema) != nil || schema < 71 {
		return lc.ErrCustodyScope
	}
	router, err := routing.NewRouter(false)
	if err != nil {
		return lc.ErrCustodyScope
	}
	outbox.ConfigureAppendObligation(router)
	s, err := lc.NewOperatorCustody(pool, cfg.Plan)
	if err != nil {
		return lc.ErrCustodyScope
	}
	a, err := storage.NewOperatorCustodyAdapter(cfg.Plan, credentials.NewStaticV4(native.RootKey, native.RootSecret, ""))
	if err != nil {
		return lc.ErrCustodyScope
	}
	manifestHash, err := pilot.ExportOperatorCustody(ctx, s, cfg.Plan, key, a)
	if err != nil {
		return lc.ErrCustodyInterrupted
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "ACCEPTED", "acquisitionId": s.ID(), "workspaceId": cfg.Plan.WorkspaceID, "runId": cfg.Plan.RunID, "manifestSha256": manifestHash, "businessVersionChanged": false})
}
