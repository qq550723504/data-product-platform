// One-time additional fixtures only in the exact owned independent database.
package main

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/tests/acceptancefixture"
	"log"
	"os"
)

func main() {
	if os.Getenv("ACCEPTANCE_IDENTITY_PREPARE_ACK") != "NEW_ISOLATED_1A_ONLY" {
		log.Fatal("explicit independent fixture acknowledgement required")
	}
	ctx := context.Background()
	cfg, e := pgxpool.ParseConfig(os.Getenv("ACCEPTANCE_IDENTITY_PREPARE_DSN"))
	must(e)
	if cfg.ConnConfig.Database != "acceptance_1a_20261024" {
		log.Fatal("exact independent database required")
	}
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	must(e)
	defer db.Close()
	tx, e := db.Begin(ctx)
	must(e)
	defer tx.Rollback(ctx)
	must(acceptancefixture.AppendBasisMaterials(ctx, tx))
	must(tx.Commit(ctx))
	log.Print("Synthetic basis evidence/relations prepared; no receipt, delivery or execution command")
}
func must(e error) {
	if e != nil {
		log.Fatal("isolated basis fixture preparation failed; no partial commit")
	}
}
