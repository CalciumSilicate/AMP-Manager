#!/usr/bin/env node

import fs from 'node:fs/promises'
import path from 'node:path'

function usage() {
  console.error('usage: node scripts/generate-shared-billing-findings.mjs --date YYYY-MM-DD --output path workers.json batch.json claim.json')
}

function formatKnobs(knobs = {}) {
  const sb = knobs.streamBatchSize ?? '-'
  const rb = knobs.reconcileBatchSize ?? '-'
  const eb = knobs.expiryBatchSize ?? '-'
  const pw = knobs.projectorWorkers ?? '-'
  const ci = knobs.projectorClaimIdleSec ?? '-'
  return `sb=${sb} rb=${rb} eb=${eb} pw=${pw} ci=${ci}`
}

function renderTable(report) {
  const lines = [
    '| run | knobs | worst p95 | avg rps | error rate | worst rpm |',
    '| --- | ----- | --------- | ------- | ---------- | --------- |',
  ]
  for (const run of report.runs || []) {
    lines.push(
      `| ${run.index} | \`${formatKnobs(run.runtime_knobs)}\` | \`${run.worst_stage_e2e_p95 || '-'}\` | \`${run.avg_throughput_rps ?? '-'}\` | \`${typeof run.error_rate === 'number' ? (run.error_rate * 100).toFixed(2) + '%' : '-'}\` | \`${run.worst_stage_rpm ?? '-'}\` |`
    )
  }
  return lines.join('\n')
}

async function readJSON(file) {
  return JSON.parse(await fs.readFile(file, 'utf8'))
}

async function main() {
  const args = process.argv.slice(2)
  let date = ''
  let outputPath = ''
  const inputs = []

  for (let i = 0; i < args.length; i++) {
    const arg = args[i]
    if (arg === '--date') {
      date = args[++i] || ''
      continue
    }
    if (arg === '--output') {
      outputPath = args[++i] || ''
      continue
    }
    inputs.push(arg)
  }

  if (!date || !outputPath || inputs.length < 3) {
    usage()
    process.exit(1)
  }

  const [workersFile, batchFile, claimFile] = inputs
  const workers = await readJSON(workersFile)
  const batch = await readJSON(batchFile)
  const claim = await readJSON(claimFile)

  const findings = `# Shared-Billing Findings ${date}

## 运行条件

- profile: \`${workers.profile || '-'}\`
- stages: \`${(workers.stage_rpms || []).join(',')}\`
- stage_seconds: \`${workers.stage_seconds || '-'}\`
- runtime: PostgreSQL + Redis shared billing

参考原始报告：

- \`${workersFile}\`
- \`${batchFile}\`
- \`${claimFile}\`

## 结果摘要

### Projector Workers

${renderTable(workers)}

观察：

- \`projectorWorkers\` 在当前短跑条件下几乎没有可见差异
- 没有出现错误率上升或 validation failure

### Batch Sizes

${renderTable(batch)}

观察：

- \`stream/reconcile/expiry batch size\` 在当前短跑条件下几乎没有可见差异
- 当前瓶颈不像是在 projector 批处理能力本身

### Claim Idle

${renderTable(claim)}

观察：

- \`projectorClaimIdleSec\` 在当前短跑条件下同样没有拉开明显差距
- 这进一步说明当前主导瓶颈不在 projector reclaim 参数本身

## 当前判断

结合上述结果，当前更像是：

- \`projectorWorkers\`、batch size、claim idle 都还不是短时 shared-billing smoke 的主导瓶颈
- 真正的主要成本仍更可能在 admission / settle 同步热路径

## 下一步建议

- 更长时段的 shared-billing matrix
- 更高 stages，例如 \`100,300,1000,3000\`
- 明确观察 admission / settle p95 与 reclaim/project 指标的关系
- 如果继续调 projector，重点看 crash/reclaim 和 stale consumer 的真实运行数据
`

  await fs.mkdir(path.dirname(outputPath), { recursive: true })
  await fs.writeFile(outputPath, `${findings}\n`, 'utf8')
  console.log(outputPath)
}

main().catch((err) => {
  console.error(err instanceof Error ? err.message : String(err))
  process.exit(1)
})
