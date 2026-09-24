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
if ! "${COMPOSE[@]}" up -d --wait --wait-timeout 120; then
  "${COMPOSE[@]}" logs --no-color label-studio || true
  exit 1
fi

LABEL_STUDIO_TOKEN="$("${COMPOSE[@]}" exec -T label-studio \
  python3 /label-studio/label_studio/manage.py shell -c \
  "from users.models import User; from rest_framework.authtoken.models import Token; u=User.objects.get(email='live@example.com'); print(Token.objects.get_or_create(user=u)[0].key)" \
  | tail -n 1 | tr -d '\r')"
test -n "$LABEL_STUDIO_TOKEN"
export TEST_LABEL_STUDIO_URL=http://127.0.0.1:18082
export TEST_LABEL_STUDIO_TOKEN="$LABEL_STUDIO_TOKEN"
for image in postgres:16-alpine redis:7-alpine quay.io/minio/minio:RELEASE.2025-04-22T22-12-26Z \
  heartexlabs/label-studio@sha256:aa461572e8f9d86a1bf9520c1db620204e86160fd2f80dd7e9d40ac84a8828ea; do
  docker image inspect "$image" --format '{{json .RepoDigests}}'
done > "$ARTIFACTS/images.txt"
git -C "$ROOT" rev-parse HEAD > "$ARTIFACTS/source-sha.txt"
cd "$ROOT/apps/platform"
go run ./cmd/migrate -direction up -dir ../../migrations
go build -o "$ARTIFACTS/platform-api" ./cmd/api
go build -o "$ARTIFACTS/platform-worker" ./cmd/worker
export LIVE_PLATFORM_WORKER="$ARTIFACTS/platform-worker"
export LIVE_BROWSER_ARTIFACTS="$ARTIFACTS"
# Pipefail keeps assertion/compile errors red even though logs are preserved.
go test -count=1 -run '^TestLabelStudioLiveReference$' -timeout 2m -v ./internal/annotation/labelstudio 2>&1 | tee "$ARTIFACTS/label-studio-live.log"

go test -count=1 -run '^TestBrowserLiveCorePOC$' -timeout 9m -v ./internal/acceptance 2>&1 | tee "$ARTIFACTS/core-live.log"

go test -count=1 -run '^TestBrowserCSVIngest$' -timeout 6m -v ./internal/acceptance 2>&1 | tee "$ARTIFACTS/ingest-live.log"

go test -count=1 -run '^TestLabelStudioLiveCoreResultReviewAndSnapshot$' -timeout 3m -v ./internal/annotation/labelstudio 2>&1 | tee "$ARTIFACTS/label-studio-core-live.log"

go test -count=1 -run '^TestLiveGoldWorkerBuildAndFormalQuality$' -timeout 3m -v ./internal/gold/application 2>&1 | tee "$ARTIFACTS/gold-worker-quality-live.log"

# Re-run existing Go regressions after the live slice, with all live-only
# provider/worker opt-ins removed so the long live tests are not executed twice
# or concurrently with fault-injection suites sharing PostgreSQL/Redis.
unset TEST_LABEL_STUDIO_URL TEST_LABEL_STUDIO_TOKEN LIVE_PLATFORM_WORKER LIVE_BROWSER_ARTIFACTS
LIVE_BROWSER_ACCEPTANCE=0 go test -count=1 ./... 2>&1 | tee "$ARTIFACTS/go-regressions.log"
