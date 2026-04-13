#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
PERF_DIR="$ROOT_DIR/perf"
COMPOSE_FILE="$ROOT_DIR/docker-compose.perf.yml"
MODE="${1:-full}"
HOST_MOCK_PID=""
HOST_MOCK_LOG=""

load_env() {
  set -a
  # shellcheck source=/dev/null
  source "$PERF_DIR/env.defaults"
  if [ -f "$PERF_DIR/.env.local" ]; then
    # shellcheck source=/dev/null
    source "$PERF_DIR/.env.local"
  fi
  # shellcheck source=/dev/null
  source "$PERF_DIR/profiles/${PROFILE}.env"
  set +a
}

load_env

PROJECT_NAME="${PERF_COMPOSE_PROJECT_NAME}"
RUNTIME_DIR="$PERF_DIR/runtime"
RESULT_TIMESTAMP="$(date -u +%Y%m%d-%H%M%S)"
RESULT_DIR="${RESULT_DIR:-$PERF_DIR/results/${RESULT_TIMESTAMP}-${PROFILE}}"
MANIFEST_PATH="$RUNTIME_DIR/seed-manifest.json"
LOG_DIR="$RESULT_DIR/logs"
K6_DIR="$RESULT_DIR/k6"
EVAL_DIR="$RESULT_DIR/evaluations"
SQL_DIR="$RESULT_DIR/sql"

PERF_DATABASE_URL="postgres://${PERF_POSTGRES_USER}:${PERF_POSTGRES_PASSWORD}@localhost:${PERF_DB_PORT}/${PERF_POSTGRES_DB}?sslmode=disable"
PERF_UPSTREAM_URL="http://host.docker.internal:${PERF_MOCK_PORT}"
PERF_APP_URL="http://localhost:${PERF_APP_PORT}"
PERF_K6_BASE_URL="http://ampmanager:16823"

mkdir -p "$RUNTIME_DIR" "$RESULT_DIR" "$LOG_DIR" "$K6_DIR" "$EVAL_DIR" "$SQL_DIR"

cleanup() {
  if [ -n "$HOST_MOCK_PID" ] && kill -0 "$HOST_MOCK_PID" >/dev/null 2>&1; then
    kill "$HOST_MOCK_PID" >/dev/null 2>&1 || true
    wait "$HOST_MOCK_PID" >/dev/null 2>&1 || true
  fi
  stop_stale_mock_upstream
}

trap cleanup EXIT INT TERM

compose() {
  COMPOSE_PROJECT_NAME="$PROJECT_NAME" docker compose -f "$COMPOSE_FILE" "$@"
}

wait_for_http() {
  local url="$1"
  local label="$2"
  local timeout="${3:-60}"

  for _ in $(seq 1 "$timeout"); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done

  echo "[error] timed out waiting for ${label} at ${url}" >&2
  exit 1
}

wait_for_service_healthy() {
  local service="$1"
  local timeout="${2:-60}"
  local container_id
  container_id="$(compose ps -q "$service")"
  if [ -z "$container_id" ]; then
    echo "[error] missing container for service ${service}" >&2
    exit 1
  fi

  for _ in $(seq 1 "$timeout"); do
    local health
    health="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$container_id")"
    if [ "$health" = "healthy" ] || [ "$health" = "none" ]; then
      return
    fi
    sleep 1
  done

  echo "[error] timed out waiting for service ${service} to become healthy" >&2
  exit 1
}

stop_stale_mock_upstream() {
  if command -v lsof >/dev/null 2>&1; then
    local pids
    pids="$(lsof -ti "tcp:${PERF_MOCK_PORT}" || true)"
    if [ -n "$pids" ]; then
      # shellcheck disable=SC2086
      kill $pids >/dev/null 2>&1 || true
      sleep 1
    fi
  fi
}

record_meta() {
  cat >"$RESULT_DIR/meta.json" <<EOF
{
  "started_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "profile": "${PROFILE}",
  "user_count": ${USER_COUNT},
  "active_key_pool": ${ACTIVE_KEY_POOL},
  "app_url": "${PERF_APP_URL}",
  "upstream_url": "${PERF_UPSTREAM_URL}",
  "request_detail_enabled": ${REQUEST_DETAIL_ENABLED}
}
EOF
}

reset_stack_if_requested() {
  if [ "${RESET_ENV}" = "true" ]; then
    compose down -v --remove-orphans >/dev/null 2>&1 || true
  fi
}

ensure_ampmanager_image() {
  if docker image inspect "$AMPMANAGER_IMAGE" >/dev/null 2>&1; then
    return
  fi
  docker pull "$AMPMANAGER_IMAGE"
}

start_host_mock_upstream() {
  stop_stale_mock_upstream
  HOST_MOCK_LOG="$LOG_DIR/mock-upstream.log"
  local mock_binary="$RUNTIME_DIR/perf-mock-upstream"
  go build -o "$mock_binary" ./cmd/perf-mock-upstream
  PERF_MOCK_ADDR=":${PERF_MOCK_PORT}" \
  PERF_UPSTREAM_API_KEY="$PERF_UPSTREAM_API_KEY" \
  PERF_MOCK_MODELS="${UPSTREAM_CHAT_MODEL},${UPSTREAM_RESPONSES_MODEL}" \
  PERF_MOCK_NONSTREAM_DELAY_MS="$PERF_MOCK_NONSTREAM_DELAY_MS" \
  PERF_MOCK_STREAM_FIRST_BYTE_DELAY_MS="$PERF_MOCK_STREAM_FIRST_BYTE_DELAY_MS" \
  PERF_MOCK_STREAM_CHUNK_DELAY_MS="$PERF_MOCK_STREAM_CHUNK_DELAY_MS" \
  PERF_MOCK_STREAM_CHUNKS="$PERF_MOCK_STREAM_CHUNKS" \
  PERF_MOCK_CHAT_PROMPT_TOKENS="$PERF_MOCK_CHAT_PROMPT_TOKENS" \
  PERF_MOCK_CHAT_COMPLETION_TOKENS="$PERF_MOCK_CHAT_COMPLETION_TOKENS" \
  PERF_MOCK_RESPONSES_INPUT_TOKENS="$PERF_MOCK_RESPONSES_INPUT_TOKENS" \
  PERF_MOCK_RESPONSES_OUTPUT_TOKENS="$PERF_MOCK_RESPONSES_OUTPUT_TOKENS" \
  PERF_MOCK_ERROR_RATE="$PERF_MOCK_ERROR_RATE" \
  PERF_MOCK_RETRYABLE_RATE="$PERF_MOCK_RETRYABLE_RATE" \
  PERF_MOCK_RETRYABLE_STATUS="$PERF_MOCK_RETRYABLE_STATUS" \
    "$mock_binary" >"$HOST_MOCK_LOG" 2>&1 &
  HOST_MOCK_PID="$!"
  wait_for_http "http://localhost:${PERF_MOCK_PORT}/healthz" "mock upstream"
}

start_prerequisites() {
  ensure_ampmanager_image
  compose up -d postgres
  wait_for_service_healthy postgres 90
  start_host_mock_upstream
}

seed_database() {
  rm -f "$MANIFEST_PATH"
  go run ./cmd/perfseed \
    --db-type postgres \
    --database-url "$PERF_DATABASE_URL" \
    --user-count "$USER_COUNT" \
    --user-prefix "$PERF_USER_PREFIX" \
    --user-password "$PERF_USER_PASSWORD" \
    --admin-username "$PERF_ADMIN_USERNAME" \
    --admin-password "$PERF_ADMIN_PASSWORD" \
    --balance-micros "$BALANCE_MICROS" \
    --upstream-url "$PERF_UPSTREAM_URL" \
    --upstream-api-key "$PERF_UPSTREAM_API_KEY" \
    --request-detail-enabled="${REQUEST_DETAIL_ENABLED}" \
    --public-chat-model "$PUBLIC_CHAT_MODEL" \
    --public-responses-model "$PUBLIC_RESPONSES_MODEL" \
    --upstream-chat-model "$UPSTREAM_CHAT_MODEL" \
    --upstream-responses-model "$UPSTREAM_RESPONSES_MODEL" \
    --output "$MANIFEST_PATH"
}

start_app() {
  compose up -d ampmanager
  wait_for_http "${PERF_APP_URL}/" "ampmanager"
}

collect_logs() {
  compose logs --no-color ampmanager >"$LOG_DIR/ampmanager.log" || true
  compose logs --no-color postgres >"$LOG_DIR/postgres.log" || true
  if [ -n "$HOST_MOCK_LOG" ] && [ -f "$HOST_MOCK_LOG" ]; then
    cp "$HOST_MOCK_LOG" "$LOG_DIR/mock-upstream.log.copy" >/dev/null 2>&1 || true
  fi
}

collect_sql_snapshots() {
  compose exec -T postgres psql -U "$PERF_POSTGRES_USER" -d "$PERF_POSTGRES_DB" -c "
    SELECT endpoint,
           status,
           COUNT(*) AS request_count,
           ROUND(AVG(latency_ms)::numeric, 2) AS avg_latency_ms,
           MAX(latency_ms) AS max_latency_ms,
           SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) AS error_count,
           COALESCE(SUM(cost_micros), 0) AS total_cost_micros
    FROM request_logs
    GROUP BY endpoint, status
    ORDER BY endpoint, status;
  " >"$SQL_DIR/request_logs_summary.txt"

  compose exec -T postgres psql -U "$PERF_POSTGRES_USER" -d "$PERF_POSTGRES_DB" -c "
    SELECT state,
           wait_event_type,
           wait_event,
           COUNT(*) AS sessions
    FROM pg_stat_activity
    WHERE datname = '${PERF_POSTGRES_DB}'
    GROUP BY state, wait_event_type, wait_event
    ORDER BY sessions DESC, state;
  " >"$SQL_DIR/pg_stat_activity.txt"

  compose exec -T postgres psql -U "$PERF_POSTGRES_USER" -d "$PERF_POSTGRES_DB" -c "
    SELECT datname,
           numbackends,
           xact_commit,
           xact_rollback,
           tup_returned,
           tup_fetched,
           tup_inserted,
           tup_updated,
           tup_deleted,
           blk_read_time,
           blk_write_time
    FROM pg_stat_database
    WHERE datname = '${PERF_POSTGRES_DB}';
  " >"$SQL_DIR/pg_stat_database.txt"
}

service_health() {
  local service
  : >"$RESULT_DIR/service-health.txt"
  for service in postgres ampmanager; do
    local container_id
    container_id="$(compose ps -q "$service")"
    if [ -z "$container_id" ]; then
      echo "${service}: missing container" >>"$RESULT_DIR/service-health.txt"
      return 1
    fi

    local running
    local oom_killed
    local exit_code
    running="$(docker inspect -f '{{.State.Running}}' "$container_id")"
    oom_killed="$(docker inspect -f '{{.State.OOMKilled}}' "$container_id")"
    exit_code="$(docker inspect -f '{{.State.ExitCode}}' "$container_id")"
    echo "${service}: running=${running} oom_killed=${oom_killed} exit_code=${exit_code}" >>"$RESULT_DIR/service-health.txt"
    if [ "$running" != "true" ] || [ "$oom_killed" != "false" ]; then
      return 1
    fi
  done

  if ! curl -fsS "http://localhost:${PERF_MOCK_PORT}/healthz" >/dev/null 2>&1; then
    echo "mock-upstream: host process not running" >>"$RESULT_DIR/service-health.txt"
    return 1
  fi
  echo "mock-upstream: health endpoint reachable on port ${PERF_MOCK_PORT}" >>"$RESULT_DIR/service-health.txt"
  return 0
}

run_k6() {
  local script_name="$1"
  local scenario="$2"
  local label="$3"
  shift 3

  local safe_label
  safe_label="$(echo "$label" | tr ' /' '__')"
  local summary_path="$K6_DIR/${scenario}-${safe_label}.json"
  local log_path="$LOG_DIR/${scenario}-${safe_label}.log"
  local eval_path="$EVAL_DIR/${scenario}-${safe_label}.json"
  local summary_rel="${summary_path#$ROOT_DIR/}"

  set +e
  compose run --rm -T \
    -u "$(id -u):$(id -g)" \
    -e BASE_URL="$PERF_K6_BASE_URL" \
    -e MANIFEST_PATH="/workspace/perf/runtime/seed-manifest.json" \
    -e ACTIVE_KEY_POOL="$ACTIVE_KEY_POOL" \
    -e PROMPT_CHARS="$PROMPT_CHARS" \
    -e K6_HTTP_TIMEOUT="$K6_HTTP_TIMEOUT" \
    "$@" \
    k6 \
    run --summary-export "/workspace/${summary_rel}" "/workspace/perf/k6/${script_name}" \
    >"$log_path" 2>&1
  local k6_exit=$?
  set -e

  if [ ! -f "$summary_path" ]; then
    echo "[error] missing k6 summary for ${scenario} ${label}" >&2
    exit 1
  fi

  if ! service_health; then
    echo "[warn] service health check failed after ${scenario} ${label}" >&2
  fi

  set +e
  go run ./cmd/perfreport evaluate-step \
    --scenario "$scenario" \
    --label "$label" \
    --summary "$summary_path" \
    --output "$eval_path"
  local eval_exit=$?
  set -e

  if [ "$k6_exit" -ne 0 ]; then
    echo "[warn] k6 exited with ${k6_exit} for ${scenario} ${label}" >&2
  fi

  if [ "$eval_exit" -eq 0 ]; then
    return 0
  fi
  if [ "$eval_exit" -eq 10 ]; then
    return 10
  fi
  exit "$eval_exit"
}

run_smoke() {
  run_k6 smoke.js smoke smoke \
    -e SMOKE_DURATION="$SMOKE_DURATION"
}

run_nonstream() {
  local step
  IFS=',' read -r -a steps <<<"$NONSTREAM_STEPS_RPM"
  for step in "${steps[@]}"; do
    set +e
    run_k6 nonstream-step.js nonstream "${step}-rpm" \
      -e STEP_RPM="$step" \
      -e STEP_DURATION="$NONSTREAM_STEP_DURATION" \
      -e K6_PREALLOCATED_VUS="$K6_PREALLOCATED_VUS" \
      -e K6_MAX_VUS="$K6_MAX_VUS"
    local exit_code=$?
    set -e
    if [ "$exit_code" -ne 0 ]; then
      if [ "$exit_code" -eq 10 ]; then
        break
      fi
      exit "$exit_code"
    fi
  done
}

run_stream() {
  local step
  IFS=',' read -r -a steps <<<"$STREAM_STEPS_CONCURRENCY"
  for step in "${steps[@]}"; do
    set +e
    run_k6 stream-step.js stream "${step}-concurrency" \
      -e STEP_CONCURRENCY="$step" \
      -e STEP_DURATION="$STREAM_STEP_DURATION"
    local exit_code=$?
    set -e
    if [ "$exit_code" -ne 0 ]; then
      if [ "$exit_code" -eq 10 ]; then
        break
      fi
      exit "$exit_code"
    fi
  done
}

run_mixed() {
  local step
  IFS=',' read -r -a steps <<<"$MIXED_STEPS_RPM"
  for step in "${steps[@]}"; do
    set +e
    run_k6 mixed-breakpoint.js mixed "${step}-rpm" \
      -e STEP_RPM="$step" \
      -e STEP_DURATION="$MIXED_STEP_DURATION" \
      -e MIXED_STREAM_SHARE="$MIXED_STREAM_SHARE" \
      -e K6_PREALLOCATED_VUS="$K6_PREALLOCATED_VUS" \
      -e K6_MAX_VUS="$K6_MAX_VUS"
    local exit_code=$?
    set -e
    if [ "$exit_code" -ne 0 ]; then
      if [ "$exit_code" -eq 10 ]; then
        break
      fi
      exit "$exit_code"
    fi
  done
}

write_report() {
  collect_logs
  collect_sql_snapshots
  go run ./cmd/perfreport write-report \
    --results-dir "$RESULT_DIR" \
    --output "$RESULT_DIR/REPORT.md"
}

main() {
  record_meta
  reset_stack_if_requested
  start_prerequisites
  seed_database
  start_app

  case "$MODE" in
    smoke)
      run_smoke
      ;;
    nonstream)
      run_nonstream
      ;;
    stream)
      run_stream
      ;;
    mixed)
      run_mixed
      ;;
    full)
      run_smoke
      run_nonstream
      run_stream
      run_mixed
      ;;
    *)
      echo "usage: ./perf/run.sh [full|smoke|nonstream|stream|mixed]" >&2
      exit 1
      ;;
  esac

  write_report
  echo "report written to ${RESULT_DIR}/REPORT.md"
  if [ "${KEEP_STACK}" != "true" ]; then
    compose down >/dev/null 2>&1 || true
  fi
}

main
