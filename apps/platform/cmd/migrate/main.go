package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/config"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/migration"
)

func main() {
	direction := flag.String("direction", "up", "migration direction: up or down")
	dir := flag.String("dir", "migrations", "migration directory")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx := context.Background()

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

	runner := migration.NewRunner(db, *dir)
	switch *direction {
	case "up":
		err = runner.Up(ctx)
	case "down":
		err = runner.Down(ctx)
	default:
		err = fmt.Errorf("unsupported migration direction %q", *direction)
	}
	if err != nil {
		logger.Error("migration failed", "direction", *direction, "error", err)
		os.Exit(1)
	}
	logger.Info("migration complete", "direction", *direction)
}
