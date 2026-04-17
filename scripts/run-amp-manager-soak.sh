#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

STAMP="$(date +%Y%m%d-%H%M%S)"

DEFAULT_APP_URL="http://127.0.0.1:16823"
APP_URL="${APP_URL-}"
MODE="${MODE:-seed-once}"
START_FAKE="${START_FAKE:-auto}"
BUILD_TOOLS="${BUILD_TOOLS:-1}"
STOP_ON_ERROR="${STOP_ON_ERROR:-0}"
RESEED="${RESEED:-1}"
SLEEP_SEC="${SLEEP_SEC:-0}"
ITERATION_LIMIT="${ITERATION_LIMIT:-0}"

DB_TYPE="${DB_TYPE:-sqlite}"
SQLITE_PATH="${SQLITE_PATH:-$REPO_ROOT/data/data.db}"
DATABASE_URL="${DATABASE_URL:-}"

TRANSPORT="${TRANSPORT:-sse}"
WS_SHARE="${WS_SHARE:-0.3}"
RUN_DURATION_SEC="${RUN_DURATION_SEC:-120}"
REQUEST_TIMEOUT_SEC="${REQUEST_TIMEOUT_SEC:-240}"

USER_COUNT="${USER_COUNT:-6000}"
USER_PREFIX="${USER_PREFIX:-soak-user-${STAMP}}"
USER_PASSWORD="${USER_PASSWORD:-loadtest123}"
BALANCE_USD_OPTIONS="${BALANCE_USD_OPTIONS:-1000000}"
CONCURRENCY_OPTIONS="${CONCURRENCY_OPTIONS:-64,96,128}"
SUBSCRIPTION_SHARE="${SUBSCRIPTION_SHARE:-0}"
SUPPORTED_MODELS="${SUPPORTED_MODELS:-gpt-5.4,gpt-5.3-codex,gpt-5.2,gpt-5.4-mini}"
SUBSCRIPTION_DAY_OPTIONS="${SUBSCRIPTION_DAY_OPTIONS:-30,90,180}"
SUBSCRIPTION_PLAN_IDS="${SUBSCRIPTION_PLAN_IDS:-}"
UPSTREAM_API_KEY="${UPSTREAM_API_KEY:-}"

SINGLE_USERS="${SINGLE_USERS:-1000}"
BURST_USERS="${BURST_USERS:-500}"
BURST_MIN="${BURST_MIN:-2}"
BURST_MAX="${BURST_MAX:-4}"
HOT_USERS="${HOT_USERS:-100}"
HOT_MIN="${HOT_MIN:-20}"
HOT_MAX="${HOT_MAX:-50}"

RUNTIME_DIR="${RUNTIME_DIR:-$REPO_ROOT/perf/runtime}"
BIN_DIR="${BIN_DIR:-$RUNTIME_DIR/bin}"
MANIFEST_PATH="${MANIFEST_PATH:-$RUNTIME_DIR/seed-manifest-soak-${STAMP}.json}"
REPORT_DIR="${REPORT_DIR:-$REPO_ROOT/perf/results}"
REPORT_PREFIX="${REPORT_PREFIX:-amp-manager-soak}"
FAKE_LOG_PATH="${FAKE_LOG_PATH:-$REPORT_DIR/${REPORT_PREFIX}-${STAMP}-fake-upstream.log}"

LOADTEST_BIN="$BIN_DIR/loadtest-amp-manager"
SEED_BIN="$BIN_DIR/seed-loadtest-users"
FAKE_BIN="$BIN_DIR/fake-llm-responses"

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

fail() {
  echo "error: $*" >&2
  exit 1
}

resolve_config() {
  case "$MODE" in
    seed-once|reuse) ;;
    *) fail "MODE must be seed-once or reuse" ;;
  esac

  case "$TRANSPORT" in
    sse|ws|mixed) ;;
    *) fail "TRANSPORT must be sse, ws, or mixed" ;;
  esac

  if [[ "$START_FAKE" == "auto" ]]; then
    if [[ "$MODE" == "seed-once" ]]; then
      START_FAKE="1"
    else
      START_FAKE="0"
    fi
  fi

  if [[ "$START_FAKE" != "0" && "$START_FAKE" != "1" ]]; then
    fail "START_FAKE must be 0, 1, or auto"
  fi

  if [[ "$START_FAKE" == "1" ]]; then
    FAKE_PORT="${FAKE_PORT:-$(pick_free_port 28080)}"
    UPSTREAM_URL="http://127.0.0.1:${FAKE_PORT}"
  else
    FAKE_PORT="${FAKE_PORT:-}"
    UPSTREAM_URL="${UPSTREAM_URL:-}"
  fi

  if [[ "$MODE" == "reuse" && ! -f "$MANIFEST_PATH" ]]; then
    fail "MANIFEST_PATH does not exist: $MANIFEST_PATH"
  fi

  if [[ "$MODE" == "seed-once" && -z "$APP_URL" ]]; then
    APP_URL="$DEFAULT_APP_URL"
  fi

  if [[ "$MODE" == "seed-once" && "$START_FAKE" == "0" && -z "$UPSTREAM_URL" ]]; then
    fail "UPSTREAM_URL is required when MODE=seed-once and START_FAKE=0"
  fi
}

build_binary() {
  local package_path="$1"
  local output_path="$2"
  if [[ "$BUILD_TOOLS" == "1" || ! -x "$output_path" ]]; then
    echo "== build $(basename "$output_path") =="
    go build -o "$output_path" "$package_path"
  fi
}

prepare_tools() {
  mkdir -p "$RUNTIME_DIR" "$BIN_DIR" "$REPORT_DIR"
  build_binary "./cmd/loadtest-amp-manager" "$LOADTEST_BIN"
  if [[ "$MODE" == "seed-once" ]]; then
    build_binary "./cmd/seed-loadtest-users" "$SEED_BIN"
  fi
  if [[ "$START_FAKE" == "1" ]]; then
    build_binary "./cmd/fake-llm-responses" "$FAKE_BIN"
  fi
}

cleanup() {
  if [[ -n "${FAKE_PID:-}" ]] && kill -0 "$FAKE_PID" 2>/dev/null; then
    kill "$FAKE_PID" 2>/dev/null || true
    wait "$FAKE_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

start_fake_upstream() {
  if [[ "$START_FAKE" != "1" ]]; then
    return
  fi

  echo "== start fake upstream =="
  "$FAKE_BIN" -port "$FAKE_PORT" >"$FAKE_LOG_PATH" 2>&1 &
  FAKE_PID=$!

  for _ in $(seq 1 30); do
    if curl -sf "${UPSTREAM_URL}/healthz" >/dev/null 2>&1; then
      return
    fi
    if ! kill -0 "$FAKE_PID" 2>/dev/null; then
      echo "fake upstream failed to start" >&2
      cat "$FAKE_LOG_PATH" || true
      exit 1
    fi
    sleep 1
  done

  echo "fake upstream health check failed" >&2
  cat "$FAKE_LOG_PATH" || true
  exit 1
}

seed_users_once() {
  if [[ "$MODE" != "seed-once" ]]; then
    return
  fi

  if [[ "$RESEED" != "1" && -f "$MANIFEST_PATH" ]]; then
    echo "== reuse existing manifest =="
    return
  fi

  echo "== seed users and keys =="
  local seed_cmd=(
    "$SEED_BIN"
    -db-type "$DB_TYPE"
    -count "$USER_COUNT"
    -user-prefix "$USER_PREFIX"
    -password "$USER_PASSWORD"
    -manifest "$MANIFEST_PATH"
    -supported-models "$SUPPORTED_MODELS"
    -balance-usd-options "$BALANCE_USD_OPTIONS"
    -concurrency-options "$CONCURRENCY_OPTIONS"
    -subscription-day-options "$SUBSCRIPTION_DAY_OPTIONS"
    -subscription-share "$SUBSCRIPTION_SHARE"
  )

  if [[ "$DB_TYPE" == "sqlite" ]]; then
    seed_cmd+=(-sqlite-path "$SQLITE_PATH")
  else
    seed_cmd+=(-database-url "$DATABASE_URL")
  fi

  if [[ -n "$UPSTREAM_URL" ]]; then
    seed_cmd+=(-upstream-url "$UPSTREAM_URL")
  fi

  if [[ -n "$APP_URL" ]]; then
    seed_cmd+=(-app-url "$APP_URL")
  fi

  if [[ -n "$UPSTREAM_API_KEY" ]]; then
    seed_cmd+=(-upstream-api-key "$UPSTREAM_API_KEY")
  fi

  if [[ -n "$SUBSCRIPTION_PLAN_IDS" ]]; then
    seed_cmd+=(-subscription-plan-ids "$SUBSCRIPTION_PLAN_IDS")
  fi

  "${seed_cmd[@]}"
}

capture_fake_stats() {
  local output_path="$1"
  if [[ "$START_FAKE" != "1" ]]; then
    return
  fi
  if curl -sf "${UPSTREAM_URL}/debug/stats" >"$output_path"; then
    echo "fake_stats_path=$output_path"
    return
  fi
  rm -f "$output_path" 2>/dev/null || true
}

run_iteration() {
  local iteration="$1"
  local cycle_stamp
  local report_path
  local log_path
  local stats_path
  local status

  cycle_stamp="$(date +%Y%m%d-%H%M%S)"
  report_path="$REPORT_DIR/${REPORT_PREFIX}-${cycle_stamp}.json"
  log_path="$REPORT_DIR/${REPORT_PREFIX}-${cycle_stamp}.log"
  stats_path="$REPORT_DIR/${REPORT_PREFIX}-${cycle_stamp}-fake-stats.json"

  local loadtest_cmd=(
    "$LOADTEST_BIN"
    -manifest "$MANIFEST_PATH"
    -transport "$TRANSPORT"
    -duration-sec "$RUN_DURATION_SEC"
    -request-timeout-sec "$REQUEST_TIMEOUT_SEC"
    -single-users "$SINGLE_USERS"
    -burst-users "$BURST_USERS"
    -burst-min "$BURST_MIN"
    -burst-max "$BURST_MAX"
    -hot-users "$HOT_USERS"
    -hot-min "$HOT_MIN"
    -hot-max "$HOT_MAX"
    -output "$report_path"
  )

  if [[ "$TRANSPORT" == "mixed" ]]; then
    loadtest_cmd+=(-ws-share "$WS_SHARE")
  fi

  if [[ -n "$APP_URL" ]]; then
    loadtest_cmd+=(-app-url "$APP_URL")
  fi

  echo "== iteration ${iteration} =="
  echo "started_at=$(date -Is)"
  echo "report_path=$report_path"

  set +e
  "${loadtest_cmd[@]}" 2>&1 | tee "$log_path"
  status=${PIPESTATUS[0]}
  set -e

  echo "log_path=$log_path"
  capture_fake_stats "$stats_path"

  if (( status == 0 )); then
    echo "iteration_status=ok"
    return
  fi

  echo "iteration_status=failed exit_code=${status}"
  if [[ "$STOP_ON_ERROR" == "1" ]]; then
    exit "$status"
  fi
}

main() {
  local iteration=1
  local completed=0

  cd "$REPO_ROOT"
  resolve_config
  prepare_tools
  start_fake_upstream
  seed_users_once

  echo "mode=$MODE"
  if [[ -n "$APP_URL" ]]; then
    echo "app_url=$APP_URL"
  else
    echo "app_url=(from manifest)"
  fi
  echo "transport=$TRANSPORT"
  echo "manifest_path=$MANIFEST_PATH"
  echo "report_dir=$REPORT_DIR"
  if [[ "$START_FAKE" == "1" ]]; then
    echo "upstream_url=$UPSTREAM_URL"
    echo "fake_log_path=$FAKE_LOG_PATH"
  fi

  while :; do
    if (( ITERATION_LIMIT > 0 && iteration > ITERATION_LIMIT )); then
      break
    fi

    run_iteration "$iteration"
    completed="$iteration"
    iteration=$((iteration + 1))

    if (( SLEEP_SEC > 0 )) && (( ITERATION_LIMIT == 0 || iteration <= ITERATION_LIMIT )); then
      echo "sleep_sec=$SLEEP_SEC"
      sleep "$SLEEP_SEC"
    fi
  done

  echo "completed_iterations=$completed"
}

main "$@"
