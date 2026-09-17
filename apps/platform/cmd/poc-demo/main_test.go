package main

import (
	"testing"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/config"
)

func TestDemoConfigRefusesOtherTargets(t *testing.T) {
	base := config.Config{Environment: "poc-demo", PostgresDSN: "postgres://postgres:demo-only@postgres:5432/dpp_demo?sslmode=disable", Redis: config.RedisConfig{Addr: "redis:6379"}, Storage: config.StorageConfig{Endpoint: "minio:9000", Bucket: "dpp-demo"}}
	if err := validateConfig(base, "SYNTHETIC_LOCAL_ONLY"); err != nil {
		t.Fatal(err)
	}
	for _, alter := range []func(*config.Config){
		func(c *config.Config) { c.Environment = "production" },
		func(c *config.Config) { c.PostgresDSN = "postgres://postgres@prod:5432/dpp_demo" },
		func(c *config.Config) { c.PostgresDSN = "postgres://postgres@postgres:5432/customer" },
		func(c *config.Config) { c.Redis.Addr = "prod:6379" },
		func(c *config.Config) { c.Redis.DB = 1 },
		func(c *config.Config) { c.Storage.Endpoint = "prod:9000" },
		func(c *config.Config) { c.Storage.Bucket = "customer" },
		func(c *config.Config) { c.Hop.Enabled = true },
		func(c *config.Config) { c.Splink.Enabled = true },
		func(c *config.Config) { c.OpenMetadata.Enabled = true },
	} {
		candidate := base
		alter(&candidate)
		if validateConfig(candidate, "SYNTHETIC_LOCAL_ONLY") == nil {
			t.Fatalf("accepted a non-demo target: %+v", candidate)
		}
	}
	if validateConfig(base, "") == nil {
		t.Fatal("missing opt-in was accepted")
	}
}

func TestDemoCommandsDoNotPublishOrReview(t *testing.T) {
	for _, command := range []string{"publish", "confirm", "reject", "reset", "", "prepare;other"} {
		if allowedCommand(command) {
			t.Fatalf("unexpected command %q", command)
		}
	}
}
