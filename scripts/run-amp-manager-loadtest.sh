#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

APP_URL="${APP_URL:-http://127.0.0.1:16823}"
DB_TYPE="${DB_TYPE:-sqlite}"
SQLITE_PATH="${SQLITE_PATH:-$REPO_ROOT/data/data.db}"
DATABASE_URL="${DATABASE_URL:-}"
STAMP="$(date +%Y%m%d-%H%M%S)"

port_in_use() {
  local port="$1"
  (echo >/dev/tcp/127.0.0.1/"$port") >/dev/null 2>&1 && return 0
  if command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1 && return 0
  fi
  return 1
}

pick_free_port() {
  local start="$1"
  local port="$start"
  while port_in_use "$port"; do
    port=$((port + 1))
  done
  echo "$port"
}

FAKE_PORT="${FAKE_PORT:-$(pick_free_port 28080)}"
UPSTREAM_URL="${UPSTREAM_URL:-http://127.0.0.1:${FAKE_PORT}}"
UPSTREAM_API_KEY="${UPSTREAM_API_KEY:-}"
USER_COUNT="${USER_COUNT:-6000}"
USER_PREFIX="${USER_PREFIX:-lt-user-${STAMP}}"
USER_PASSWORD="${USER_PASSWORD:-loadtest123}"
BALANCE_USD_OPTIONS="${BALANCE_USD_OPTIONS:-1000000}"
CONCURRENCY_OPTIONS="${CONCURRENCY_OPTIONS:-64,96,128}"
SUBSCRIPTION_SHARE="${SUBSCRIPTION_SHARE:-0}"
MANIFEST_PATH="${MANIFEST_PATH:-$REPO_ROOT/perf/runtime/seed-manifest-${STAMP}.json}"
REPORT_DIR="${REPORT_DIR:-$REPO_ROOT/perf/results}"
RUN_DURATION_SEC="${RUN_DURATION_SEC:-120}"
REPORT_PATH="${REPORT_PATH:-$REPORT_DIR/amp-manager-loadtest-${STAMP}.json}"
FAKE_LOG_PATH="${FAKE_LOG_PATH:-$REPORT_DIR/amp-manager-loadtest-${STAMP}-fake-upstream.log}"

mkdir -p "$REPORT_DIR"

cd "$REPO_ROOT"

cleanup() {
  if [[ -n "${FAKE_PID:-}" ]] && kill -0 "$FAKE_PID" 2>/dev/null; then
    kill "$FAKE_PID" 2>/dev/null || true
    wait "$FAKE_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

echo "== start fake upstream =="
go run ./cmd/fake-llm-responses -port "$FAKE_PORT" >"$FAKE_LOG_PATH" 2>&1 &
FAKE_PID=$!
for _ in $(seq 1 30); do
  if curl -sf "http://127.0.0.1:${FAKE_PORT}/healthz" >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$FAKE_PID" 2>/dev/null; then
    echo "fake upstream failed to start"
    cat "$FAKE_LOG_PATH" || true
    exit 1
  fi
  sleep 1
done
if ! curl -sf "http://127.0.0.1:${FAKE_PORT}/healthz" >/dev/null 2>&1; then
  echo "fake upstream health check failed"
  cat "$FAKE_LOG_PATH" || true
  exit 1
fi

echo "== seed users and keys =="
SEED_CMD=(
  go run ./cmd/seed-loadtest-users
  -db-type "$DB_TYPE"
  -count "$USER_COUNT"
  -user-prefix "$USER_PREFIX"
  -password "$USER_PASSWORD"
  -balance-usd-options "$BALANCE_USD_OPTIONS"
  -concurrency-options "$CONCURRENCY_OPTIONS"
  -subscription-share "$SUBSCRIPTION_SHARE"
  -manifest "$MANIFEST_PATH"
  -app-url "$APP_URL"
  -upstream-url "$UPSTREAM_URL"
)

if [[ "$DB_TYPE" == "sqlite" ]]; then
  SEED_CMD+=(-sqlite-path "$SQLITE_PATH")
else
  SEED_CMD+=(-database-url "$DATABASE_URL")
fi

if [[ -n "$UPSTREAM_API_KEY" ]]; then
  SEED_CMD+=(-upstream-api-key "$UPSTREAM_API_KEY")
fi

"${SEED_CMD[@]}"

echo "== run loadtest =="
go run ./cmd/loadtest-amp-manager \
  -manifest "$MANIFEST_PATH" \
  -app-url "$APP_URL" \
  -duration-sec "$RUN_DURATION_SEC" \
  -output "$REPORT_PATH"

echo "manifest_path=$MANIFEST_PATH"
echo "report_path=$REPORT_PATH"
echo "fake_log_path=$FAKE_LOG_PATH"
