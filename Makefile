.PHONY: help dev-up dev-down dev-logs api worker test lint fmt tidy migrate-up migrate-down

COMPOSE_FILE := deploy/docker-compose/docker-compose.yml
PLATFORM_DIR := apps/platform

help:
	@echo "Targets:"
	@echo "  dev-up       Start PostgreSQL, Redis and MinIO"
	@echo "  dev-down     Stop local infrastructure"
	@echo "  dev-logs     Follow local infrastructure logs"
	@echo "  api          Run the Go API process"
	@echo "  worker       Run the Go worker process"
	@echo "  test         Run Go tests"
	@echo "  lint         Run go vet"
	@echo "  fmt          Format Go source"
	@echo "  tidy         Update Go module metadata"
	@echo "  migrate-up   Apply pending database migrations"
	@echo "  migrate-down Roll back the latest database migration"

dev-up:
	docker compose -f $(COMPOSE_FILE) up -d

dev-down:
	docker compose -f $(COMPOSE_FILE) down

dev-logs:
	docker compose -f $(COMPOSE_FILE) logs -f

api:
	cd $(PLATFORM_DIR) && go run ./cmd/api

worker:
	cd $(PLATFORM_DIR) && go run ./cmd/worker

test:
	cd $(PLATFORM_DIR) && go test ./...

lint:
	cd $(PLATFORM_DIR) && go vet ./...

fmt:
	cd $(PLATFORM_DIR) && gofmt -w $$(find . -name '*.go' -type f)

tidy:
	cd $(PLATFORM_DIR) && go mod tidy

migrate-up:
	cd $(PLATFORM_DIR) && go run ./cmd/migrate -direction up -dir ../../migrations

migrate-down:
	cd $(PLATFORM_DIR) && go run ./cmd/migrate -direction down -dir ../../migrations
