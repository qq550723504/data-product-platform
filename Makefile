.PHONY: help dev test lint fmt migrate-up migrate-down

help:
	@echo "Targets:"
	@echo "  dev          Run local development stack (to be implemented)"
	@echo "  test         Run project tests"
	@echo "  lint         Run linters"
	@echo "  fmt          Format source code"
	@echo "  migrate-up   Apply database migrations"
	@echo "  migrate-down Roll back database migrations"

dev:
	@echo "TODO: add local development command"

test:
	@echo "TODO: add tests once code is initialized"

lint:
	@echo "TODO: add linters once code is initialized"

fmt:
	@echo "TODO: add formatters once code is initialized"

migrate-up:
	@echo "TODO: wire migration tool"

migrate-down:
	@echo "TODO: wire migration tool"
