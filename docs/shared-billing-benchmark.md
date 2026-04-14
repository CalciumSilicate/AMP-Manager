# Shared-Billing Benchmark

## 目的

用仓库内置的 `cmd/loadtest-responses` 在真实 `PostgreSQL + Redis` 环境下验证 shared-billing runtime 的关键 knobs，包括：

- `streamBatchSize`
- `reconcileBatchSize`
- `expiryBatchSize`
- `projectorWorkers`
- `projectorClaimIdleSec`

## 前置条件

至少需要：

- 可用的 PostgreSQL
- 可用的 Redis
- 本仓库能正常执行 `go run ./cmd/loadtest-responses`

本仓库开发环境可以直接使用：

```bash
docker compose -f docker-compose.dev.yml up -d postgres redis
```

默认连接参数：

```bash
export DATABASE_URL='postgres://postgres:mysecretpassword@localhost:5432/ampmanager?sslmode=disable'
export REDIS_URL='redis://localhost:6379/0'
```

## 单次运行

最小 shared-billing smoke：

```bash
go run ./cmd/loadtest-responses \
  -profile smoke-shared-billing \
  -stage-seconds 2 \
  -stages 100,300 \
  -database-url "$DATABASE_URL" \
  -redis-url "$REDIS_URL" \
  -redis-prefix "pf-smoke-$(date +%s)"
```

## 矩阵运行

比较 projector worker 数：

```bash
go run ./cmd/loadtest-responses \
  -profile benchmark-shared-billing \
  -stage-seconds 3 \
  -stages 100,300,1000 \
  -database-url "$DATABASE_URL" \
  -redis-url "$REDIS_URL" \
  -redis-prefix "pf-workers-$(date +%s)" \
  -matrix-projector-workers 1,2,4 \
  -output "${TMPDIR:-/tmp}/pf-workers-matrix.json"
```

比较 batch size：

```bash
go run ./cmd/loadtest-responses \
  -profile benchmark-shared-billing \
  -stage-seconds 3 \
  -stages 100,300,1000 \
  -database-url "$DATABASE_URL" \
  -redis-url "$REDIS_URL" \
  -redis-prefix "pf-batch-$(date +%s)" \
  -projector-workers 4 \
  -matrix-stream-batch-sizes 50,100,200 \
  -output "${TMPDIR:-/tmp}/pf-batch-matrix.json"
```

比较 reclaim idle：

```bash
go run ./cmd/loadtest-responses \
  -profile benchmark-shared-billing \
  -stage-seconds 3 \
  -stages 100,300,1000 \
  -database-url "$DATABASE_URL" \
  -redis-url "$REDIS_URL" \
  -redis-prefix "pf-claim-$(date +%s)" \
  -projector-workers 4 \
  -matrix-projector-claim-idle-secs 15,30,60 \
  -output "${TMPDIR:-/tmp}/pf-claim-matrix.json"
```

## 输出解读

终端会输出：

- 每组 run 的 knobs
- 每个 stage 的 `e2e_p95/e2e_p99/adm_p95/set_p95/prj_p95/rcl_p95`
- `rclf` reclaim failures
- `rclm` reclaim claimed
- comparative summary

JSON 报告：

- 单次运行写单个 `benchmarkReport`
- 矩阵模式写 `matrixReport`

重点先看：

- `worst_stage_e2e_p95`
- `avg_throughput_rps`
- `error_rate`
- `validation_failures`
- `project_failures / reclaim_failures / reconcile_failures`

## 当前短跑观察

基于当前仓库的短时 shared-billing matrix，`projectorWorkers=1/2/4` 和 `streamBatchSize=50/100/200` 尚未拉开明显差距。这通常说明：

- 当前瓶颈还不在 projector 并发度本身
- 更可能仍在请求主路径或整体单机吞吐

所以后续调优应优先继续关注：

- admission / settle 同步热路径
- 更长时段、更高并发的 shared-billing benchmark
- 真实多实例环境下的观察
