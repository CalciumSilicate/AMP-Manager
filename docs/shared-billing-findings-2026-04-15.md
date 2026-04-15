# Shared-Billing Findings 2026-04-15

## 运行条件

- profile: `benchmark-shared-billing`
- stages: `100,300,1000`
- stage_seconds: `3`
- runtime: PostgreSQL + Redis shared billing

参考原始报告：

- `/home/arch/cal/workspace/tmp/pf-dev-shared-workers-matrix-refresh-20260415-080149.json`
- `/home/arch/cal/workspace/tmp/pf-dev-shared-batch-matrix-refresh-20260415-080149.json`
- `/home/arch/cal/workspace/tmp/pf-dev-shared-summary-full-20260415-081206.md`

## 结果摘要

### Projector Workers

| run | knobs | worst p95 | avg rps | error rate | worst rpm |
| --- | ----- | --------- | ------- | ---------- | --------- |
| 1 | `sb=100 rb=100 eb=100 pw=1 ci=0` | `5.227s` | `3.69` | `0.00%` | `100` |
| 2 | `sb=100 rb=100 eb=100 pw=2 ci=0` | `5.223s` | `3.69` | `0.00%` | `100` |
| 3 | `sb=100 rb=100 eb=100 pw=4 ci=0` | `5.221s` | `3.69` | `0.00%` | `100` |

观察：

- `projectorWorkers=1/2/4` 在当前短跑条件下几乎没有可见差异
- 没有出现错误率上升或 validation failure
- 最差 p95 始终落在 `~5.22s`

### Batch Sizes

| run | knobs | worst p95 | avg rps | error rate | worst rpm |
| --- | ----- | --------- | ------- | ---------- | --------- |
| 1 | `sb=50 rb=50 eb=50 pw=4 ci=0` | `5.227s` | `3.69` | `0.00%` | `100` |
| 2 | `sb=100 rb=100 eb=100 pw=4 ci=0` | `5.22s` | `3.69` | `0.00%` | `100` |
| 3 | `sb=200 rb=200 eb=200 pw=4 ci=0` | `5.22s` | `3.69` | `0.00%` | `100` |

观察：

- `stream/reconcile/expiry batch size = 50/100/200` 也几乎没有可见差异
- 当前瓶颈不像是在 projector 批处理能力本身


### Claim Idle

| run | knobs | worst p95 | avg rps | error rate | worst rpm |
| --- | ----- | --------- | ------- | ---------- | --------- |
| 1 | `sb=100 rb=100 eb=100 pw=4 ci=15` | `5.221s` | `3.69` | `0.00%` | `100` |
| 2 | `sb=100 rb=100 eb=100 pw=4 ci=30` | `5.221s` | `3.69` | `0.00%` | `100` |
| 3 | `sb=100 rb=100 eb=100 pw=4 ci=60` | `5.222s` | `3.69` | `0.00%` | `100` |

观察：

- `projectorClaimIdleSec = 15/30/60` 在当前短跑条件下同样没有拉开明显差距
- 这进一步说明当前主导瓶颈不在 projector reclaim 参数本身

## 当前判断

结合上述结果，当前更像是：

- `projectorWorkers` 和 batch size 已经不是短时 shared-billing smoke 的主导瓶颈
- 真正的主要成本仍更可能在 admission / settle 同步热路径
- 如果继续调 projector，应该用更长时段、更高并发、更贴近真实业务体量的矩阵压测来验证，而不是继续依赖短跑结果

## 下一步建议

优先级更高的方向：

- 更长时段的 shared-billing matrix
- 更高 stages，例如 `100,300,1000,3000`
- 明确观察 admission / settle p95 与 reclaim/project 指标的关系
- 如果要继续调 projector，重点看 crash/reclaim 和 stale consumer 的真实运行数据，而不是只看 worker 数
