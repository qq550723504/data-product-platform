#!/usr/bin/env bash
set -euo pipefail
if [[ "${LIVE_BROWSER_ACCEPTANCE:-}" != "1" ]]; then
  echo 'Set LIVE_BROWSER_ACCEPTANCE=1 to run the disposable live integration suite.' >&2
  exit 2
fi
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ARTIFACTS="$ROOT/.artifacts/live-browser"
mkdir -p "$ARTIFACTS"
printf '{"verified":false,"status":"started"}\n' > "$ARTIFACTS/verification.json"
# Do not inherit a production service target from a developer shell.
export APP_ENV=browser-acceptance APP_HTTP_ADDR=127.0.0.1:18080
export POSTGRES_DSN='postgres://postgres:postgres@127.0.0.1:15432/dpp_browser_live?sslmode=disable'
export TEST_POSTGRES_DSN="$POSTGRES_DSN"
export REDIS_ADDR=127.0.0.1:16379 REDIS_DB=0 REDIS_PASSWORD=''
export OBJECT_STORAGE_ENDPOINT=127.0.0.1:19000 OBJECT_STORAGE_BUCKET=dpp-browser-live
export OBJECT_STORAGE_ACCESS_KEY=minio OBJECT_STORAGE_SECRET_KEY=minio123 OBJECT_STORAGE_USE_SSL=false
export OPENMETADATA_ENABLED=false HOP_ENABLED=false SPLINK_ENABLED=false
export INDUSTRY_PACK_ROOT="$ROOT/industry-packs"
PROJECT="dpp-browser-${GITHUB_RUN_ID:-local}-$$"
COMPOSE=(docker compose -p "$PROJECT" -f "$ROOT/tests/live-browser/compose.yml")
cleanup() {
  status=$?
  trap - EXIT
  "${COMPOSE[@]}" logs --no-color > "$ARTIFACTS/services.log" 2>&1 || true
  "${COMPOSE[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
  exit "$status"
}
trap cleanup EXIT
"${COMPOSE[@]}" config --quiet
"${COMPOSE[@]}" up -d --wait --wait-timeout 90
for image in postgres:16-alpine redis:7-alpine quay.io/minio/minio:RELEASE.2025-04-22T22-12-26Z; do
  docker image inspect "$image" --format '{{json .RepoDigests}}'
done > "$ARTIFACTS/images.txt"
git -C "$ROOT" rev-parse HEAD > "$ARTIFACTS/source-sha.txt"
cd "$ROOT/apps/platform"
go run ./cmd/migrate -direction up -dir ../../migrations
go build -o "$ARTIFACTS/platform-api" ./cmd/api
go build -o "$ARTIFACTS/platform-worker" ./cmd/worker
# Pipefail keeps assertion/compile errors red even though logs are preserved.
go test -count=1 -run '^TestBrowserLiveCorePOC$' -timeout 9m -v ./internal/acceptance 2>&1 | tee "$ARTIFACTS/core-live.log"
