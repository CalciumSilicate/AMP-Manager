import { useCallback, useEffect, useMemo, useState } from 'react'
import { Activity, Clock3, RefreshCw, Server, TimerReset, Waves } from 'lucide-react'

import { getStatusMonitorDashboard, type StatusMonitorDashboardItem, type StatusMonitorPeriod, type StatusMonitorState } from '@/api/statusMonitor'
import { AdminPageShell, AdminSurface, AdminToolbarRow } from '@/components/admin/AdminPageShell'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { motion, staggerContainer, staggerItem } from '@/lib/motion'
import { formatDateTimeWithSeconds } from '@/lib/formatters'

const PERIOD_OPTIONS: { value: StatusMonitorPeriod; label: string }[] = [
  { value: '7d', label: '7 天' },
  { value: '15d', label: '15 天' },
  { value: '30d', label: '30 天' },
]

const STATUS_LABELS: Record<StatusMonitorState, string> = {
  operational: '正常',
  degraded: '波动',
  error: '错误',
  failed: '失败',
  unknown: '未知',
}

const HISTORY_CLASS: Record<StatusMonitorState, string> = {
  operational: 'bg-emerald-500',
  degraded: 'bg-amber-400',
  error: 'bg-red-500',
  failed: 'bg-rose-700',
  unknown: 'bg-muted',
}

const BADGE_CLASS: Record<StatusMonitorState, string> = {
  operational: 'border-emerald-200 bg-emerald-50 text-emerald-700',
  degraded: 'border-amber-200 bg-amber-50 text-amber-700',
  error: 'border-red-200 bg-red-50 text-red-700',
  failed: 'border-rose-200 bg-rose-50 text-rose-700',
  unknown: 'border-border bg-muted/50 text-muted-foreground',
}

const TARGET_LABELS = {
  service_proxy: '服务层',
  channel_direct: '上游层',
  custom_http: '自定义',
} as const

export default function StatusMonitor() {
  const [period, setPeriod] = useState<StatusMonitorPeriod>('7d')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [data, setData] = useState<Awaited<ReturnType<typeof getStatusMonitorDashboard>> | null>(null)

  const loadDashboard = useCallback(async (nextPeriod = period) => {
    try {
      setLoading(true)
      setError('')
      const result = await getStatusMonitorDashboard(nextPeriod)
      setData(result)
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : '加载状态监控面板失败')
    } finally {
      setLoading(false)
    }
  }, [period])

  useEffect(() => {
    void loadDashboard(period)
  }, [loadDashboard, period])

  const nextUpdateAt = useMemo(() => {
    if (!data?.lastUpdated || !data.pollIntervalSec) return null
    const value = new Date(data.lastUpdated)
    if (Number.isNaN(value.getTime())) return null
    value.setSeconds(value.getSeconds() + data.pollIntervalSec)
    return value.toISOString()
  }, [data?.lastUpdated, data?.pollIntervalSec])

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
      <AdminPageShell
        title="状态监控"
        description="查看服务层、上游层和自定义目标的最新探测状态、历史条带和周期可用率。"
        actions={
          <Button variant="outline" onClick={() => void loadDashboard()} disabled={loading}>
            <RefreshCw className={`mr-2 h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
            刷新
          </Button>
        }
        width="7xl"
      >
        {error ? (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}

        <AdminSurface className="overflow-hidden">
          <AdminToolbarRow className="gap-4">
            <div className="space-y-1">
              <div className="flex items-center gap-2">
                <Badge variant="outline" className={BADGE_CLASS[data?.overallStatus || 'unknown']}>
                  {STATUS_LABELS[data?.overallStatus || 'unknown']}
                </Badge>
                <span className="text-sm text-muted-foreground">共 {data?.summaryCounts.total || 0} 个监控项</span>
              </div>
              <div className="flex flex-wrap items-center gap-4 text-sm text-muted-foreground">
                <span className="inline-flex items-center gap-1.5">
                  <Clock3 className="h-4 w-4" />
                  上次探测 {data?.lastUpdated ? formatDateTimeWithSeconds(data.lastUpdated) : '暂无'}
                </span>
                <span className="inline-flex items-center gap-1.5">
                  <TimerReset className="h-4 w-4" />
                  轮询周期 {data?.pollIntervalSec ? `${Math.round(data.pollIntervalSec / 60)} 分钟` : '-'}
                </span>
                <span className="inline-flex items-center gap-1.5">
                  <Waves className="h-4 w-4" />
                  下次预计 {nextUpdateAt ? formatDateTimeWithSeconds(nextUpdateAt) : '等待首次探测'}
                </span>
              </div>
            </div>
            <div className="flex items-center gap-2">
              {PERIOD_OPTIONS.map((option) => (
                <Button
                  key={option.value}
                  variant={period === option.value ? 'default' : 'outline'}
                  size="sm"
                  onClick={() => setPeriod(option.value)}
                >
                  {option.label}
                </Button>
              ))}
            </div>
          </AdminToolbarRow>

          <div className="grid gap-3 border-t border-border/70 px-5 py-5 md:grid-cols-2 xl:grid-cols-5">
            <SummaryCard label="正常" value={data?.summaryCounts.operational || 0} accent="text-emerald-600" />
            <SummaryCard label="波动" value={data?.summaryCounts.degraded || 0} accent="text-amber-600" />
            <SummaryCard label="错误" value={data?.summaryCounts.error || 0} accent="text-red-600" />
            <SummaryCard label="失败" value={data?.summaryCounts.failed || 0} accent="text-rose-700" />
            <SummaryCard label="未知" value={data?.summaryCounts.unknown || 0} accent="text-muted-foreground" />
          </div>
        </AdminSurface>

        {loading && !data ? (
          <div className="py-16 text-center text-sm text-muted-foreground">状态监控面板加载中...</div>
        ) : null}

        {!loading && data && data.groups.length === 0 ? (
          <div className="rounded-xl border border-dashed px-4 py-12 text-center text-sm text-muted-foreground">
            还没有启用中的状态监控项。
          </div>
        ) : null}

        {data ? (
          <motion.div variants={staggerContainer} initial="hidden" animate="visible" className="space-y-6">
            {data.groups.map((group) => (
              <motion.section key={group.groupName} variants={staggerItem} className="space-y-3">
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <h3 className="text-sm font-semibold uppercase tracking-[0.16em] text-muted-foreground">{group.groupName}</h3>
                    <p className="text-sm text-muted-foreground">{group.items.length} 个监控项</p>
                  </div>
                </div>
                <div className="grid gap-4 xl:grid-cols-2">
                  {group.items.map((item) => (
                    <StatusCard key={item.id} item={item} period={period} />
                  ))}
                </div>
              </motion.section>
            ))}
          </motion.div>
        ) : null}
      </AdminPageShell>
    </motion.div>
  )
}

function SummaryCard({ label, value, accent }: { label: string; value: number; accent?: string }) {
  return (
    <div className="rounded-2xl border border-border/70 bg-background/80 px-4 py-4">
      <div className="text-xs uppercase tracking-[0.18em] text-muted-foreground">{label}</div>
      <div className={`mt-3 text-3xl font-semibold tracking-tight ${accent || ''}`}>{value}</div>
    </div>
  )
}

function StatusCard({ item, period }: { item: StatusMonitorDashboardItem; period: StatusMonitorPeriod }) {
  const history = item.history.length > 0 ? item.history : Array.from({ length: 60 }, (_, index) => ({
    status: 'unknown' as const,
    checkedAt: `${index}`,
  }))

  return (
    <motion.article
      variants={staggerItem}
      whileHover={{ y: -3, scale: 1.01 }}
      transition={{ type: 'spring', bounce: 0.2, duration: 0.35 }}
      className="overflow-hidden rounded-[28px] border border-border/70 bg-gradient-to-br from-background via-background to-muted/35 shadow-sm"
    >
      <div className="space-y-5 px-5 py-5">
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-2">
            <div className="flex items-center gap-3">
              <div className="flex h-12 w-12 items-center justify-center rounded-2xl border border-border/70 bg-background/90 text-foreground">
                <Activity className="h-5 w-5" />
              </div>
              <div>
                <h4 className="text-xl font-semibold tracking-tight text-foreground">{item.name}</h4>
                <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                  <Badge variant="outline" className="rounded-full">{item.channelName || TARGET_LABELS[item.targetType]}</Badge>
                  {item.model ? <span>{item.model}</span> : null}
                </div>
              </div>
            </div>
          </div>
          <Badge variant="outline" className={`rounded-full px-3 py-1 text-sm ${BADGE_CLASS[item.latest.status]}`}>
            {STATUS_LABELS[item.latest.status]}
          </Badge>
        </div>

        <div className="grid gap-3 sm:grid-cols-2">
          <MetricTile icon={TimerReset} label="对话延迟" value={formatMs(item.latest.latencyMs)} />
          <MetricTile icon={Waves} label="端点 PING" value={formatMs(item.latest.ttfbMs)} />
        </div>

        <div className="grid gap-3 rounded-2xl border border-border/70 bg-background/75 px-4 py-4 sm:grid-cols-2">
          <div className="space-y-1">
            <div className="text-xs uppercase tracking-[0.16em] text-muted-foreground">Endpoint</div>
            <div className="line-clamp-2 text-sm text-foreground">{item.endpointLabel || '-'}</div>
          </div>
          <div className="space-y-1">
            <div className="text-xs uppercase tracking-[0.16em] text-muted-foreground">Latest</div>
            <div className="text-sm text-foreground">{item.latest.checkedAt ? formatDateTimeWithSeconds(item.latest.checkedAt) : '暂无'}</div>
            <div className="text-xs text-muted-foreground">{item.latest.message || '暂无消息'}</div>
          </div>
        </div>

        <div className="flex items-end justify-between gap-4">
          <div>
            <div className="text-xs uppercase tracking-[0.16em] text-muted-foreground">Availability ({period})</div>
            <div className="mt-1 text-2xl font-semibold tracking-tight text-emerald-600">
              {item.availability.availabilityPct.toFixed(2)}%
            </div>
            <div className="text-xs text-muted-foreground">
              {item.availability.operationalCount}/{item.availability.totalChecks} 成功
            </div>
          </div>
          <div className="text-right text-xs text-muted-foreground">
            <div>HISTORY ({item.history.length || 60} PTS)</div>
            <div className="mt-1">最新探测靠右</div>
          </div>
        </div>

        <div className="space-y-2">
          <div className="flex items-center justify-between text-[11px] uppercase tracking-[0.18em] text-muted-foreground">
            <span>Past</span>
            <span>Now</span>
          </div>
          <div className="flex items-center gap-[3px]">
            {history.map((point, index) => (
              <span
                key={`${point.checkedAt}-${index}`}
                className={`h-7 flex-1 rounded-full ${HISTORY_CLASS[point.status]}`}
                title={point.checkedAt}
              />
            ))}
          </div>
        </div>
      </div>
    </motion.article>
  )
}

function MetricTile({
  icon: Icon,
  label,
  value,
}: {
  icon: typeof Server
  label: string
  value: string
}) {
  return (
    <div className="rounded-2xl border border-border/70 bg-background/80 px-4 py-4">
      <div className="flex items-center gap-2 text-xs uppercase tracking-[0.16em] text-muted-foreground">
        <Icon className="h-4 w-4" />
        {label}
      </div>
      <div className="mt-3 text-3xl font-semibold tracking-tight text-foreground">{value}</div>
    </div>
  )
}

function formatMs(value: number) {
  return value > 0 ? `${value} ms` : '-'
}
