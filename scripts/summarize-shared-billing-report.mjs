#!/usr/bin/env node

import fs from 'node:fs/promises'
import path from 'node:path'

function usage() {
  console.error('usage: node scripts/summarize-shared-billing-report.mjs [--output path] report.json [report2.json ...]')
}

function formatKnobs(knobs = {}) {
  const sb = knobs.streamBatchSize ?? '-'
  const rb = knobs.reconcileBatchSize ?? '-'
  const eb = knobs.expiryBatchSize ?? '-'
  const pw = knobs.projectorWorkers ?? '-'
  const ci = knobs.projectorClaimIdleSec ?? '-'
  return `sb=${sb} rb=${rb} eb=${eb} pw=${pw} ci=${ci}`
}

function toPercent(value) {
  if (typeof value !== 'number') return '-'
  return `${(value * 100).toFixed(2)}%`
}

function summarizeMatrixReport(report, sourceName) {
  const lines = []
  lines.push(`## ${sourceName}`)
  lines.push('')
  lines.push(`- profile: \`${report.profile}\``)
  lines.push(`- mode: \`${report.mode}\``)
  lines.push(`- stages: \`${(report.stage_rpms || []).join(',')}\``)
  lines.push(`- stage_seconds: \`${report.stage_seconds}\``)
  lines.push(`- started_at: \`${report.started_at}\``)
  lines.push(`- completed_at: \`${report.completed_at}\``)
  lines.push('')
  lines.push('| run | label | knobs | worst p95 | avg rps | error rate | worst rpm | validation |')
  lines.push('| --- | ----- | ----- | --------- | ------- | ---------- | --------- | ---------- |')
  for (const run of report.runs || []) {
    lines.push(
      `| ${run.index} | ${run.label || '-'} | \`${formatKnobs(run.runtime_knobs)}\` | ${run.worst_stage_e2e_p95 || '-'} | ${run.avg_throughput_rps ?? '-'} | ${toPercent(run.error_rate)} | ${run.worst_stage_rpm ?? '-'} | ${run.validation_failures ?? '-'} |`
    )
  }
  lines.push('')
  return lines.join('\n')
}

function summarizeSingleReport(report, sourceName) {
  const lines = []
  lines.push(`## ${sourceName}`)
  lines.push('')
  lines.push(`- profile: \`${report.metadata?.profile || '-'}\``)
  lines.push(`- label: \`${report.metadata?.label || '-'}\``)
  lines.push(`- mode: \`${report.metadata?.mode || '-'}\``)
  lines.push(`- knobs: \`${formatKnobs(report.metadata?.runtime_knobs)}\``)
  lines.push(`- worst p95: \`${report.summary?.worst_stage_e2e_p95 || '-'}\``)
  lines.push(`- avg rps: \`${report.summary?.avg_throughput_rps ?? '-'}\``)
  lines.push(`- error rate: \`${toPercent(report.summary?.error_rate)}\``)
  lines.push(`- validation failures: \`${report.summary?.validation_failures ?? '-'}\``)
  lines.push('')
  return lines.join('\n')
}

async function main() {
  const args = process.argv.slice(2)
  let outputPath = ''
  const inputs = []

  for (let i = 0; i < args.length; i++) {
    const arg = args[i]
    if (arg === '--output') {
      outputPath = args[i + 1] || ''
      i++
      continue
    }
    inputs.push(arg)
  }

  if (inputs.length === 0) {
    usage()
    process.exit(1)
  }

  const sections = ['# Shared-Billing Benchmark Summary', '']
  for (const input of inputs) {
    const raw = await fs.readFile(input, 'utf8')
    const report = JSON.parse(raw)
    const sourceName = path.basename(input)
    if (Array.isArray(report.runs) && Array.isArray(report.reports)) {
      sections.push(summarizeMatrixReport(report, sourceName))
    } else if (report.metadata && report.summary) {
      sections.push(summarizeSingleReport(report, sourceName))
    } else {
      sections.push(`## ${sourceName}\n\n- unsupported report shape\n`)
    }
  }

  const markdown = `${sections.join('\n')}\n`
  if (outputPath) {
    await fs.mkdir(path.dirname(outputPath), { recursive: true })
    await fs.writeFile(outputPath, markdown, 'utf8')
  } else {
    process.stdout.write(markdown)
  }
}

main().catch((err) => {
  console.error(err instanceof Error ? err.message : String(err))
  process.exit(1)
})
