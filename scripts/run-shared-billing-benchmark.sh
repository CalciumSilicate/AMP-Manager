#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

PROFILE="${PROFILE:-benchmark-shared-billing}"
STAGE_SECONDS="${STAGE_SECONDS:-3}"
STAGES="${STAGES:-100,300,1000}"
DATABASE_URL="${DATABASE_URL:-postgres://postgres:mysecretpassword@localhost:5432/ampmanager?sslmode=disable}"
REDIS_URL="${REDIS_URL:-redis://localhost:6379/0}"
REDIS_PREFIX="${REDIS_PREFIX:-pf-bench-$(date +%s)}"
LABEL="${LABEL:-}"
OUTPUT_PATH="${OUTPUT_PATH:-${TMPDIR:-/tmp}/amp-shared-billing-$(date +%Y%m%d-%H%M%S).json}"

MATRIX_STREAM_BATCH_SIZES="${MATRIX_STREAM_BATCH_SIZES:-}"
MATRIX_RECONCILE_BATCH_SIZES="${MATRIX_RECONCILE_BATCH_SIZES:-}"
MATRIX_EXPIRY_BATCH_SIZES="${MATRIX_EXPIRY_BATCH_SIZES:-}"
MATRIX_PROJECTOR_WORKERS="${MATRIX_PROJECTOR_WORKERS:-1,2,4}"
MATRIX_PROJECTOR_CLAIM_IDLE_SECS="${MATRIX_PROJECTOR_CLAIM_IDLE_SECS:-}"

PROJECTOR_WORKERS="${PROJECTOR_WORKERS:-0}"
PROJECTOR_CLAIM_IDLE_SEC="${PROJECTOR_CLAIM_IDLE_SEC:-0}"
STREAM_BATCH_SIZE="${STREAM_BATCH_SIZE:-100}"
RECONCILE_BATCH_SIZE="${RECONCILE_BATCH_SIZE:-0}"
EXPIRY_BATCH_SIZE="${EXPIRY_BATCH_SIZE:-0}"

mkdir -p "$(dirname "$OUTPUT_PATH")"

cd "$REPO_ROOT"

CMD=(
  go run ./cmd/loadtest-responses
  -profile "$PROFILE"
  -stage-seconds "$STAGE_SECONDS"
  -stages "$STAGES"
  -database-url "$DATABASE_URL"
  -redis-url "$REDIS_URL"
  -redis-prefix "$REDIS_PREFIX"
  -stream-batch-size "$STREAM_BATCH_SIZE"
  -reconcile-batch-size "$RECONCILE_BATCH_SIZE"
  -expiry-batch-size "$EXPIRY_BATCH_SIZE"
  -projector-workers "$PROJECTOR_WORKERS"
  -projector-claim-idle-sec "$PROJECTOR_CLAIM_IDLE_SEC"
  -output "$OUTPUT_PATH"
)

if [[ -n "$LABEL" ]]; then
  CMD+=(-label "$LABEL")
fi
if [[ -n "$MATRIX_STREAM_BATCH_SIZES" ]]; then
  CMD+=(-matrix-stream-batch-sizes "$MATRIX_STREAM_BATCH_SIZES")
fi
if [[ -n "$MATRIX_RECONCILE_BATCH_SIZES" ]]; then
  CMD+=(-matrix-reconcile-batch-sizes "$MATRIX_RECONCILE_BATCH_SIZES")
fi
if [[ -n "$MATRIX_EXPIRY_BATCH_SIZES" ]]; then
  CMD+=(-matrix-expiry-batch-sizes "$MATRIX_EXPIRY_BATCH_SIZES")
fi
if [[ -n "$MATRIX_PROJECTOR_WORKERS" ]]; then
  CMD+=(-matrix-projector-workers "$MATRIX_PROJECTOR_WORKERS")
fi
if [[ -n "$MATRIX_PROJECTOR_CLAIM_IDLE_SECS" ]]; then
  CMD+=(-matrix-projector-claim-idle-secs "$MATRIX_PROJECTOR_CLAIM_IDLE_SECS")
fi

printf 'profile=%s stages=%s output=%s\n' "$PROFILE" "$STAGES" "$OUTPUT_PATH"
printf 'redis_prefix=%s matrix_workers=%s matrix_stream=%s matrix_claim_idle=%s\n' \
  "$REDIS_PREFIX" "$MATRIX_PROJECTOR_WORKERS" "$MATRIX_STREAM_BATCH_SIZES" "$MATRIX_PROJECTOR_CLAIM_IDLE_SECS"

"${CMD[@]}"

printf 'report_written=%s\n' "$OUTPUT_PATH"
