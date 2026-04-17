import { Fragment, memo, startTransition, useDeferredValue, useState, useEffect, useRef, useMemo, useCallback } from 'react'
import {
  getRequestLogs, getAdminRequestLogs, getAdminDistinctModels, getAdminDistinctKeys, getDistinctKeys, getDistinctModels,
  RequestLog, DistinctAPIKey,
} from '@/api/amp'
import { connectRequestLogsWS } from '@/api/requestLogsWS'
import { getPublicSiteConfig } from '@/api/system'
import { listUsers, UserInfo } from '@/api/users'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card'
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
} from '@/components/ui/tooltip'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatDateTimeWithSeconds, formatDecimal } from '@/lib/formatters'
import { Num } from '@/components/Num'
import { StatusBadge } from '@/components/StatusBadge'
import { LogDetailModal } from '@/components/LogDetailModal'
import { LogFilterBar, FilterValues, localToISO } from '@/components/LogFilterBar'
import { motion } from '@/lib/motion'
import { PageSizeSlider } from '@/components/PageSizeSlider'
import { cn } from '@/lib/utils'

interface Props {
  isAdmin: boolean
}

function getLiveFlushDelay(pageSize: number) {
  if (pageSize >= 100) return 1500
  if (pageSize >= 50) return 1000
  return 500
}

function matchesRequestLogFilters(log: RequestLog, filters: FilterValues) {
  if (filters.userId && log.userId !== filters.userId) {
    return false
  }
  if (filters.apiKeyId && log.apiKeyId !== filters.apiKeyId) {
    return false
  }
  if (filters.model) {
    const filterModel = filters.model.toLowerCase()
    const mappedModel = log.mappedModel?.toLowerCase()
    const originalModel = log.originalModel?.toLowerCase()
    if (mappedModel !== filterModel && originalModel !== filterModel) {
      return false
    }
  }
  if (filters.channel) {
    const channelText = (log.channelName || log.provider || '').toLowerCase()
    if (!channelText.includes(filters.channel.toLowerCase())) {
      return false
    }
  }
  if (filters.sessionId) {
    const sessionText = (log.sessionId || '').toLowerCase()
    if (!sessionText.includes(filters.sessionId.toLowerCase())) {
      return false
    }
  }
  if (filters.statuses.length > 0 && !filters.statuses.includes(String(log.statusCode))) {
    return false
  }
  if (filters.from) {
    const from = new Date(filters.from)
    const createdAt = new Date(log.createdAt)
    if (!Number.isNaN(from.getTime()) && !Number.isNaN(createdAt.getTime()) && createdAt < from) {
      return false
    }
  }
  if (filters.to) {
    const to = new Date(filters.to)
    const createdAt = new Date(log.createdAt)
    if (!Number.isNaN(to.getTime()) && !Number.isNaN(createdAt.getTime()) && createdAt > to) {
      return false
    }
  }
  return true
}

function getDefaultSessionSearchWindow() {
  const to = new Date()
  const from = new Date(to.getTime() - 6 * 60 * 60 * 1000)

  return {
    from: from.toISOString(),
    to: to.toISOString(),
  }
}

function formatTransportLabel(transport?: string) {
  if (!transport) return null
  if (transport.toLowerCase() === 'websocket') return 'WS'
  if (transport.toLowerCase() === 'http') return 'HTTP'
  return transport
}

function renderDuration(valueMs?: number) {
  if (typeof valueMs !== 'number') return '-'

  const compact = valueMs >= 1000
    ? `${Number((valueMs / 1000).toFixed(1)).toString()}s`
    : `${valueMs}ms`

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-block cursor-default">{compact}</span>
      </TooltipTrigger>
      <TooltipContent side="bottom">
        <p>{valueMs}ms</p>
      </TooltipContent>
    </Tooltip>
  )
}

function formatPricePerMillion(costPerToken?: number) {
  if (costPerToken === undefined || costPerToken === null) return '-'
  const perMillion = costPerToken * 1_000_000
  if (perMillion >= 1) return `$${perMillion.toFixed(2)} / 1M`
  if (perMillion >= 0.01) return `$${perMillion.toFixed(3)} / 1M`
  return `$${perMillion.toFixed(4)} / 1M`
}

function formatMultiplierValue(value?: number) {
  if (typeof value !== 'number') return '-'
  return `${formatDecimal(value, 2)}x`
}

function multiplierBadgeClass(value?: number) {
  if (typeof value !== 'number') return ''
  if (value >= 10) return 'border-rose-200 bg-rose-50 text-rose-700 hover:bg-rose-100'
  if (value >= 3) return 'border-amber-200 bg-amber-50 text-amber-700 hover:bg-amber-100'
  if (value > 1) return 'border-emerald-200 bg-emerald-50 text-emerald-700 hover:bg-emerald-100'
  if (value === 1) return 'border-slate-200 bg-slate-50 text-slate-700 hover:bg-slate-100'
  return 'border-violet-200 bg-violet-50 text-violet-700 hover:bg-violet-100'
}

function buildPricingTitle(log: RequestLog) {
  const modelLabel = log.pricingModel || log.mappedModel || log.originalModel || '模型'
  if (log.pricingRuleName) {
    return `${modelLabel}（${log.pricingRuleName}）计费标准`
  }
  return `${modelLabel} 计费标准`
}

type RequestFormatKey = 'chat_completions' | 'responses' | 'messages' | 'generate_content'

function detectRequestFormatKeyFromPath(path?: string): RequestFormatKey | null {
  if (!path) return null

  const normalizedPath = path.toLowerCase()
  if (normalizedPath.includes('/v1/chat/completions')) return 'chat_completions'
  if (normalizedPath.includes('/v1/responses')) return 'responses'
  if (normalizedPath.includes('/v1/messages')) return 'messages'
  if (normalizedPath.includes('/v1beta/models/') || normalizedPath.includes('/v1beta1/publishers/google/models/')) {
    return 'generate_content'
  }

  return null
}

function normalizeRequestFormatKey(format?: string, path?: string): RequestFormatKey | null {
  switch (format?.trim().toLowerCase()) {
    case 'openai':
    case 'openai-chat':
    case 'chat_completions':
      return 'chat_completions'
    case 'openai-responses':
    case 'responses':
      return 'responses'
    case 'claude':
    case 'messages':
      return 'messages'
    case 'gemini':
    case 'generate_content':
      return 'generate_content'
    default:
      return detectRequestFormatKeyFromPath(path)
  }
}

function formatRequestFormatLabel(format?: string) {
  switch (normalizeRequestFormatKey(format)) {
    case 'chat_completions':
      return 'Chat Completions'
    case 'responses':
      return 'Responses'
    case 'messages':
      return 'Anthropic Messages'
    case 'generate_content':
      return 'Gemini'
    default:
      return format || ''
  }
}

function requestFormatBadgeClass(format?: string, path?: string) {
  switch (normalizeRequestFormatKey(format, path)) {
    case 'chat_completions':
      return 'border-emerald-300 bg-emerald-50 text-emerald-700 hover:bg-emerald-100'
    case 'responses':
      return 'border-sky-300 bg-sky-50 text-sky-700 hover:bg-sky-100'
    case 'messages':
      return 'border-amber-300 bg-amber-50 text-amber-700 hover:bg-amber-100'
    case 'generate_content':
      return 'border-violet-300 bg-violet-50 text-violet-700 hover:bg-violet-100'
    default:
      return ''
  }
}

function translationPathLabel(log: RequestLog) {
  if (!log.requestFormat || !log.upstreamFormat || log.requestFormat === log.upstreamFormat) {
    return null
  }
  return `${formatRequestFormatLabel(log.requestFormat)} -> ${formatRequestFormatLabel(log.upstreamFormat)}`
}

function formatMethodLabel(method?: string) {
  if (!method) return '-'
  if (method.toLowerCase() === 'websocket') return 'WebSocket'
  return method.toUpperCase()
}

function methodBadgeClass(log: Pick<RequestLog, 'method' | 'transportFallbackReason'>) {
  if (log.method?.toLowerCase() !== 'websocket') return ''
  if (log.transportFallbackReason) {
    return 'font-medium border-rose-300 bg-rose-50 text-rose-700 hover:bg-rose-100'
  }
  return 'font-medium'
}

function TransportMetaTooltip({
  method,
  downstreamTransport,
  upstreamTransport,
  transportFallbackReason,
}: {
  method?: string
  downstreamTransport?: string
  upstreamTransport?: string
  transportFallbackReason?: string
}) {
  const metaItems = [
    { label: '方法', value: formatMethodLabel(method) },
    { label: '下游', value: formatTransportLabel(downstreamTransport) || '-' },
    { label: '上游', value: formatTransportLabel(upstreamTransport) || '-' },
  ]

  return (
    <TooltipContent side="bottom" className="max-w-80 border bg-popover px-3 py-2 text-xs text-popover-foreground shadow-md">
      <div className="space-y-2">
        <div className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5">
          {metaItems.map((item) => (
            <Fragment key={item.label}>
              <span className="text-[11px] font-medium text-foreground/70">{item.label}</span>
              <span className="text-[11px] font-medium text-foreground">{item.value}</span>
            </Fragment>
          ))}
        </div>
        {transportFallbackReason ? (
          <div className="space-y-1 border-t border-border/80 pt-2">
            <p className="text-[11px] font-medium text-foreground/70">Fallback</p>
            <p className="break-words text-[11px] leading-relaxed text-foreground">{transportFallbackReason}</p>
          </div>
        ) : null}
      </div>
    </TooltipContent>
  )
}

const RequestLogRow = memo(function RequestLogRow({
  log,
  isAdmin,
  userIdToUsername,
  onOpenDetail,
}: {
  log: RequestLog
  isAdmin: boolean
  userIdToUsername: Map<string, string>
  onOpenDetail: (logId: string) => void
}) {
  const userDisplay = log.username || userIdToUsername.get(log.userId) || `${log.userId.slice(0, 8)}...`
  const keyDisplay = log.apiKeyName ? `${log.apiKeyName}${log.apiKeyPrefix ? ` (${log.apiKeyPrefix})` : ''}` : (log.apiKeyPrefix || log.apiKeyId || '-')
  const translationPath = translationPathLabel(log)
  const channelBadgeTone = requestFormatBadgeClass(log.requestFormat, log.path)
  const canOpenDetail = isAdmin || log.statusCode >= 400
  const sessionDisplay = log.sessionId || '-'

  return (
    <TableRow>
      <TableCell className="text-xs text-muted-foreground whitespace-nowrap">
        {formatDateTimeWithSeconds(log.createdAt)}
      </TableCell>
      {isAdmin && (
        <TableCell className="text-xs max-w-24">
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="cursor-default truncate block">{userDisplay}</span>
            </TooltipTrigger>
            <TooltipContent side="bottom" className="bg-popover text-popover-foreground border shadow-md px-3 py-2 text-xs space-y-1">
              <div className="flex items-center gap-1.5">
                <span className="text-muted-foreground">用户</span>
                <span className="font-medium">{log.username || userIdToUsername.get(log.userId) || log.userId}</span>
              </div>
              <div className="flex items-center gap-1.5">
                <span className="text-muted-foreground">Key</span>
                <span className="font-medium">{keyDisplay}</span>
              </div>
            </TooltipContent>
          </Tooltip>
        </TableCell>
      )}
      <TableCell className="max-w-40">
        {log.sessionId ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="block cursor-default truncate font-mono text-xs text-foreground" title={sessionDisplay}>
                {sessionDisplay}
              </span>
            </TooltipTrigger>
            <TooltipContent side="bottom" className="border bg-popover px-3 py-2 text-xs text-popover-foreground shadow-md">
              <span className="font-mono">{sessionDisplay}</span>
            </TooltipContent>
          </Tooltip>
        ) : (
          <span className="text-xs text-muted-foreground">-</span>
        )}
      </TableCell>
      <TableCell>
        <div className="flex flex-col">
          <span className="font-medium text-sm truncate max-w-32" title={log.mappedModel || log.originalModel}>
            {log.mappedModel || log.originalModel || '-'}
          </span>
          {log.mappedModel && log.originalModel && log.mappedModel !== log.originalModel && (
            <span className="text-xs text-muted-foreground truncate max-w-32" title={log.originalModel}>
              ← {log.originalModel}
            </span>
          )}
        </div>
      </TableCell>
      <TableCell className="whitespace-nowrap">
        {(log.channelName || log.provider) ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <div className="flex flex-nowrap items-center gap-1.5">
                <Badge
                  variant="outline"
                  className={cn(
                    'max-w-[10rem] cursor-help truncate text-xs whitespace-nowrap',
                    channelBadgeTone,
                  )}
                >
                  {log.channelName || log.provider}
                </Badge>
              </div>
            </TooltipTrigger>
            <TooltipContent side="bottom" className="max-w-80 border bg-popover px-3 py-2 text-xs text-popover-foreground shadow-md">
              <div className="space-y-2">
                <div className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5">
                  <span className="text-[11px] font-medium text-foreground/70">渠道</span>
                  <span className="text-[11px] font-medium text-foreground">{log.channelName || log.provider}</span>
                  {translationPath ? (
                    <>
                      <span className="text-[11px] font-medium text-foreground/70">翻译</span>
                      <span className="text-[11px] font-medium text-sky-700">{translationPath}</span>
                    </>
                  ) : null}
                </div>
              </div>
            </TooltipContent>
          </Tooltip>
        ) : (
          <span className="text-muted-foreground">-</span>
        )}
      </TableCell>
      <TableCell className="whitespace-nowrap">
        {log.thinkingLevel ? (
          <Badge variant="secondary" className="text-xs whitespace-nowrap">{log.thinkingLevel}</Badge>
        ) : (
          <span className="text-muted-foreground">-</span>
        )}
      </TableCell>
      <TableCell className="whitespace-nowrap">
        <Tooltip>
          <TooltipTrigger asChild>
            <Badge
              variant="outline"
              className={cn(
                'cursor-help whitespace-nowrap',
                methodBadgeClass(log),
              )}
            >
              {formatMethodLabel(log.method)}
            </Badge>
          </TooltipTrigger>
          <TransportMetaTooltip
            method={log.method}
            downstreamTransport={log.downstreamTransport}
            upstreamTransport={log.upstreamTransport}
            transportFallbackReason={log.transportFallbackReason}
          />
        </Tooltip>
      </TableCell>
      <TableCell className="whitespace-nowrap">
        {canOpenDetail ? (
          <button
            onClick={() => onOpenDetail(log.id)}
            className="cursor-pointer whitespace-nowrap hover:opacity-80 transition-opacity"
            title={isAdmin ? '点击查看请求详情' : '点击查看失败请求详情'}
          >
            <StatusBadge status={log.statusCode} />
          </button>
        ) : (
          <StatusBadge status={log.statusCode} />
        )}
      </TableCell>
      <TableCell className="whitespace-nowrap text-right text-muted-foreground">
        {renderDuration(log.ttfbMs)}
      </TableCell>
      <TableCell className="whitespace-nowrap text-right text-muted-foreground">
        {typeof log.tps === 'number' ? `${log.tps.toFixed(1)}/s` : '-'}
      </TableCell>
      <TableCell className="whitespace-nowrap text-right text-muted-foreground">
        {renderDuration(log.latencyMs)}
      </TableCell>
      <TableCell className="whitespace-nowrap text-right"><Num value={log.inputTokens} /></TableCell>
      <TableCell className="whitespace-nowrap text-right"><Num value={log.outputTokens} /></TableCell>
      <TableCell className="whitespace-nowrap text-right"><Num value={log.cacheReadInputTokens} /></TableCell>
      <TableCell className="whitespace-nowrap text-right"><Num value={log.cacheCreationInputTokens} /></TableCell>
      <TableCell className="whitespace-nowrap text-right text-muted-foreground">
        {log.costUsd ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="cursor-help underline decoration-dotted underline-offset-4">{`$${log.costUsd}`}</span>
              </TooltipTrigger>
              <TooltipContent side="bottom" className="max-w-80 border bg-popover px-3 py-2 text-xs text-popover-foreground shadow-md">
                <div className="space-y-2">
                  <p className="break-words text-[11px] font-semibold tracking-[0.02em] text-foreground">{buildPricingTitle(log)}</p>
                  <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5">
                    <span className="text-[11px] font-medium text-foreground/70">输入</span>
                    <span className="font-mono text-[11px] font-medium text-foreground">{formatPricePerMillion(log.inputCostPerToken)}</span>
                    <span className="text-[11px] font-medium text-foreground/70">输出</span>
                    <span className="font-mono text-[11px] font-medium text-foreground">{formatPricePerMillion(log.outputCostPerToken)}</span>
                    <span className="text-[11px] font-medium text-foreground/70">缓存读</span>
                    <span className="font-mono text-[11px] font-medium text-foreground">{formatPricePerMillion(log.cacheReadInputPerToken)}</span>
                    <span className="text-[11px] font-medium text-foreground/70">缓存写</span>
                    <span className="font-mono text-[11px] font-medium text-foreground">{formatPricePerMillion(log.cacheCreationInputPerToken)}</span>
                  </div>
                </div>
              </TooltipContent>
            </Tooltip>
        ) : '-'}
      </TableCell>
      <TableCell className="whitespace-nowrap text-right">
        {typeof log.rateMultiplier === 'number' ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <Badge variant="outline" className={cn('cursor-help whitespace-nowrap font-mono', multiplierBadgeClass(log.rateMultiplier))}>
                  {formatDecimal(log.rateMultiplier, 2)}x
                </Badge>
              </TooltipTrigger>
            <TooltipContent side="bottom" className="max-w-80 border bg-popover px-3 py-2 text-xs text-popover-foreground shadow-md">
              <div className="space-y-2">
                <p className="text-[11px] font-semibold tracking-[0.02em] text-foreground">倍率拆分</p>
                <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5">
                  <span className="text-[11px] font-medium text-foreground/70">渠道倍率</span>
                  <span className="font-mono text-[11px] font-medium text-foreground">{formatMultiplierValue(log.channelRateMultiplier)}</span>
                  <span className="text-[11px] font-medium text-foreground/70">分组倍率</span>
                  <span className="font-mono text-[11px] font-medium text-foreground">{formatMultiplierValue(log.groupRateMultiplier)}</span>
                  <span className="text-[11px] font-medium text-foreground/70">特殊计费</span>
                  <span className="font-mono text-[11px] font-medium text-foreground">
                    {log.specialRateReason
                      ? `${log.specialRateReason} ${formatMultiplierValue(log.specialRateMultiplier)}`
                      : formatMultiplierValue(log.specialRateMultiplier)}
                  </span>
                  <span className="text-[11px] font-medium text-foreground/70">总倍率</span>
                  <span className="font-mono text-[11px] font-medium text-foreground">{formatMultiplierValue(log.rateMultiplier)}</span>
                </div>
              </div>
            </TooltipContent>
          </Tooltip>
        ) : (
          <span className="text-muted-foreground">-</span>
        )}
      </TableCell>
    </TableRow>
  )
})

export default function RequestLogs({ isAdmin }: Props) {
  const [logs, setLogs] = useState<RequestLog[]>([])
  const [loading, setLoading] = useState(true)
  const [fetching, setFetching] = useState(false)
  const hasLoadedRef = useRef(false)
  const [error, setError] = useState('')
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [pageSize, setPageSize] = useState(20)
  const [sessionSearchMinChars, setSessionSearchMinChars] = useState(3)

  const [filters, setFilters] = useState<FilterValues>({ userId: '', apiKeyId: '', sessionId: '', model: '', channel: '', statuses: [], from: '', to: '' })

  const [users, setUsers] = useState<UserInfo[]>([])
  const [models, setModels] = useState<string[]>([])
  const [keys, setKeys] = useState<DistinctAPIKey[]>([])

  const [autoRefresh, setAutoRefresh] = useState(true)
  const wsCloseRef = useRef<(() => void) | null>(null)
  const wsBufRef = useRef<Map<string, RequestLog>>(new Map())
  const pendingLogsRef = useRef<Map<string, RequestLog>>(new Map())
  const flushTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const logsRef = useRef<RequestLog[]>([])
  const totalRef = useRef(0)
  const knownLogIDsRef = useRef<Set<string>>(new Set())
  const filtersRef = useRef<FilterValues>({ userId: '', apiKeyId: '', sessionId: '', model: '', channel: '', statuses: [], from: '', to: '' })
  const abortControllerRef = useRef<AbortController | null>(null)
  const [pendingCount, setPendingCount] = useState(0)

  const [selectedLogId, setSelectedLogId] = useState<string | null>(null)
  const [detailModalOpen, setDetailModalOpen] = useState(false)

  const userIdToUsername = useMemo(() => {
    const map = new Map<string, string>()
    users.forEach(u => map.set(u.id, u.username))
    return map
  }, [users])

  const deferredLogs = useDeferredValue(logs)
  const liveInsertEnabled = isAdmin && autoRefresh && page === 1

  useEffect(() => {
    if (isAdmin) {
      Promise.all([listUsers(), getAdminDistinctModels()])
        .then(([usersRes, modelsRes]) => {
          setUsers(usersRes || [])
          setModels(modelsRes.models || [])
        })
        .catch(console.error)
      return
    }

    getDistinctModels()
      .then((modelsRes) => {
        setModels(modelsRes.models || [])
      })
      .catch(console.error)
  }, [isAdmin])

  useEffect(() => {
    getPublicSiteConfig()
      .then((config) => {
        const minChars = config.sessionSticky?.logSearchMinChars
        if (typeof minChars === 'number' && Number.isFinite(minChars) && minChars > 0) {
          setSessionSearchMinChars(Math.max(1, Math.floor(minChars)))
        }
      })
      .catch(() => {
        setSessionSearchMinChars(3)
      })
  }, [])

  useEffect(() => {
    if (isAdmin) {
      getAdminDistinctKeys(filters.userId || undefined)
        .then(res => setKeys(res.keys || []))
        .catch(console.error)
      return
    }

    getDistinctKeys()
      .then(res => setKeys(res.keys || []))
      .catch(console.error)
  }, [isAdmin, filters.userId])

  useEffect(() => {
    logsRef.current = logs
  }, [logs])

  useEffect(() => {
    totalRef.current = total
  }, [total])

  useEffect(() => {
    filtersRef.current = filters
  }, [filters])

  const liveFlushDelay = getLiveFlushDelay(pageSize)
  const normalizedSessionId = filters.sessionId.trim()
  const shouldApplyImplicitSessionWindow = Boolean(normalizedSessionId) && !filters.from && !filters.to

  const applyPendingLogs = useCallback(() => {
    if (pendingLogsRef.current.size === 0) return

    const pendingBatch = Array.from(pendingLogsRef.current.values()).sort((a, b) => (
      new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
    ))
    pendingLogsRef.current.clear()
    setPendingCount(0)

    const next = [...pendingBatch, ...logsRef.current]
      .filter((log, index, arr) => arr.findIndex((item) => item.id === log.id) === index)
      .slice(0, pageSize)

    logsRef.current = next
    knownLogIDsRef.current = new Set(next.map((item) => item.id))
    startTransition(() => {
      setLogs(next)
    })
  }, [pageSize])

  useEffect(() => {
    const wsBuf = wsBufRef.current
    const pendingLogs = pendingLogsRef.current

    if (liveInsertEnabled) {
      const flushBuf = () => {
        const batch = Array.from(wsBuf.values())
        wsBuf.clear()
        flushTimerRef.current = null
        if (batch.length === 0) return

        let next = [...logsRef.current]
        let newCount = 0
        let removedCount = 0
        let visibleChanged = false
        const activeFilters = filtersRef.current
        for (const log of batch) {
          const matchesFilters = matchesRequestLogFilters(log, activeFilters)
          const idx = next.findIndex(l => l.id === log.id)
          if (idx >= 0) {
            if (matchesFilters) {
              next[idx] = log
              visibleChanged = true
            } else {
              next.splice(idx, 1)
              knownLogIDsRef.current.delete(log.id)
              removedCount++
              visibleChanged = true
            }
          } else if (matchesFilters) {
            if (!pendingLogs.has(log.id)) {
              newCount++
            }
            pendingLogs.set(log.id, log)
          }
        }

        if (visibleChanged) {
          next = next.slice(0, pageSize)
          logsRef.current = next
        }

        startTransition(() => {
          if (visibleChanged) {
            setLogs(next)
          }
          setPendingCount(pendingLogs.size)
          if (newCount > 0 || removedCount > 0) {
            totalRef.current += newCount - removedCount
            setTotal(totalRef.current)
          }
        })
      }

      const close = connectRequestLogsWS(
        (newLog) => {
          wsBuf.set(newLog.id, newLog)
          if (flushTimerRef.current === null) {
            flushTimerRef.current = setTimeout(flushBuf, liveFlushDelay)
          }
        },
        () => {
          wsCloseRef.current = null
        },
      )
      wsCloseRef.current = close
    } else {
      if (wsCloseRef.current) {
        wsCloseRef.current()
        wsCloseRef.current = null
      }
    }
    return () => {
      if (flushTimerRef.current !== null) {
        clearTimeout(flushTimerRef.current)
        flushTimerRef.current = null
      }
      wsBuf.clear()
      pendingLogs.clear()
      setPendingCount(0)
      if (wsCloseRef.current) {
        wsCloseRef.current()
        wsCloseRef.current = null
      }
    }
  }, [liveInsertEnabled, liveFlushDelay, pageSize])

  useEffect(() => {
    return () => {
      if (abortControllerRef.current) abortControllerRef.current.abort()
    }
  }, [])

  const loadData = useCallback(async () => {
    if (abortControllerRef.current) abortControllerRef.current.abort()
    const controller = new AbortController()
    abortControllerRef.current = controller

    if (!hasLoadedRef.current) setLoading(true)
    setFetching(true)
    setError('')
    try {
      if (normalizedSessionId && normalizedSessionId.length < sessionSearchMinChars) {
        setLogs([])
        setTotal(0)
        setError(`Session ID 至少 ${sessionSearchMinChars} 个字符`)
        return
      }

      const sessionWindow = shouldApplyImplicitSessionWindow ? getDefaultSessionSearchWindow() : null
      const params = {
        page,
        pageSize,
        sessionId: normalizedSessionId || undefined,
        model: filters.model || undefined,
        channel: filters.channel || undefined,
        statusCodes: filters.statuses.length ? filters.statuses.map((status) => Number.parseInt(status, 10)) : undefined,
        from: filters.from ? localToISO(filters.from) : sessionWindow?.from,
        to: filters.to ? localToISO(filters.to) : sessionWindow?.to,
      }
      const result = isAdmin
        ? await getAdminRequestLogs({ ...params, userId: filters.userId || undefined, apiKeyId: filters.apiKeyId || undefined }, controller.signal)
        : await getRequestLogs({ ...params, apiKeyId: filters.apiKeyId || undefined }, controller.signal)
      if (controller.signal.aborted) return

      const items = result.items || []
      logsRef.current = items
      knownLogIDsRef.current = new Set(items.map(item => item.id))
      pendingLogsRef.current.clear()
      setPendingCount(0)
      setLogs(items)

      totalRef.current = result.total
      setTotal(result.total)
    } catch (err) {
      if (controller.signal.aborted) return
      setError(err instanceof Error ? err.message : '加载失败')
    } finally {
      if (!controller.signal.aborted) {
        setLoading(false)
        setFetching(false)
        hasLoadedRef.current = true
      }
    }
  }, [filters, isAdmin, normalizedSessionId, page, pageSize, sessionSearchMinChars, shouldApplyImplicitSessionWindow])

  useEffect(() => {
    loadData()
  }, [loadData])

  const handleFilterChange = (newFilters: FilterValues) => {
    setFilters(newFilters)
    setPage(1)
  }

  const handlePageSizeChange = (newSize: number) => {
    setPageSize(newSize)
    setPage(1)
  }

  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  const handleOpenDetail = useCallback((logId: string) => {
    setSelectedLogId(logId)
    setDetailModalOpen(true)
  }, [])

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="space-y-6">
      <motion.div initial={{ opacity: 0, y: -20 }} animate={{ opacity: 1, y: 0 }} transition={{ type: 'spring', bounce: 0.2, duration: 0.6 }}>
        <h2 className="text-2xl font-bold tracking-tight">请求日志</h2>
        <p className="text-muted-foreground">
          {isAdmin ? '管理员视图：查看所有用户的 API 请求历史' : '查看 API 请求历史'}
        </p>
      </motion.div>

      <LogFilterBar
        isAdmin={isAdmin}
        users={users}
        keys={keys}
        models={models}
        values={filters}
        onChange={handleFilterChange}
        sessionSearchMinChars={sessionSearchMinChars}
      />

      {error && (
        <div className="rounded-md bg-red-50 p-4 text-red-700">{error}</div>
      )}

      <motion.div initial={{ opacity: 0, y: 30 }} animate={{ opacity: 1, y: 0 }} transition={{ type: 'spring', bounce: 0.2, duration: 0.6, delay: 0.1 }}>
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <div>
                <CardTitle>请求记录</CardTitle>
                <CardDescription>共 {total} 条记录{shouldApplyImplicitSessionWindow ? ' · Session 搜索按最近 6 小时查询' : ''}</CardDescription>
              </div>
              <div className="flex items-center gap-3">
                {isAdmin && pendingCount > 0 ? (
                  <div className="flex items-center gap-2">
                    <Badge variant="secondary">新 {pendingCount}</Badge>
                    <Button variant="outline" size="sm" onClick={applyPendingLogs}>
                      插入
                    </Button>
                  </div>
                ) : null}
                {isAdmin ? (
                  <div className="flex items-center gap-2 border-l pl-3">
                    <Switch id="auto-refresh" checked={autoRefresh} onCheckedChange={setAutoRefresh} />
                    <Label htmlFor="auto-refresh" className="text-sm">自动刷新</Label>
                  </div>
                ) : null}
                <Button variant="outline" onClick={loadData}>刷新</Button>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            {loading ? (
              <p className="text-center text-muted-foreground py-8">加载中...</p>
            ) : logs.length === 0 && !fetching ? (
              <p className="text-center text-muted-foreground py-8">暂无请求记录</p>
            ) : (
              <>
                <div className={`relative overflow-auto max-h-[calc(100vh-320px)] min-h-[400px] rounded-md border transition-opacity ${fetching ? 'opacity-50 pointer-events-none' : ''}`}>
                <Table className="min-w-full w-max" containerClassName="overflow-visible">
                  <TableHeader className="sticky top-0 z-10 bg-background">
                    <TableRow>
                      <TableHead className="whitespace-nowrap">时间</TableHead>
                      {isAdmin && <TableHead className="whitespace-nowrap">用户</TableHead>}
                      <TableHead className="whitespace-nowrap">Session</TableHead>
                      <TableHead className="whitespace-nowrap">模型</TableHead>
                      <TableHead className="whitespace-nowrap">渠道</TableHead>
                      <TableHead className="whitespace-nowrap">思维等级</TableHead>
                      <TableHead className="whitespace-nowrap">方法</TableHead>
                      <TableHead className="whitespace-nowrap">状态</TableHead>
                      <TableHead className="whitespace-nowrap text-right">TTFB</TableHead>
                      <TableHead className="whitespace-nowrap text-right">TPS</TableHead>
                      <TableHead className="whitespace-nowrap text-right">用时</TableHead>
                      <TableHead className="whitespace-nowrap text-right">
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span className="cursor-help underline decoration-dotted underline-offset-4">输入</span>
                          </TooltipTrigger>
                          <TooltipContent side="bottom">
                            <p>这里的输入 Tokens 已扣除缓存读取命中的部分。</p>
                          </TooltipContent>
                        </Tooltip>
                      </TableHead>
                      <TableHead className="whitespace-nowrap text-right">输出</TableHead>
                      <TableHead className="whitespace-nowrap text-right">缓存读</TableHead>
                      <TableHead className="whitespace-nowrap text-right">缓存写</TableHead>
                      <TableHead className="whitespace-nowrap text-right">成本</TableHead>
                      <TableHead className="whitespace-nowrap text-right">倍率</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {deferredLogs.map((log) => (
                      <RequestLogRow
                        key={log.id}
                        log={log}
                        isAdmin={isAdmin}
                        userIdToUsername={userIdToUsername}
                        onOpenDetail={handleOpenDetail}
                      />
                    ))}
                  </TableBody>
                </Table>
                </div>

                <div className="flex items-center justify-between mt-4">
                  <div className="flex items-center gap-4">
                    <p className="text-sm text-muted-foreground">
                      第 {page} 页，共 {totalPages} 页
                    </p>
                    <PageSizeSlider value={pageSize} onChange={handlePageSizeChange} />
                  </div>
                  <div className="flex gap-2">
                    <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage(p => p - 1)}>
                      上一页
                    </Button>
                    <Button variant="outline" size="sm" disabled={page >= totalPages} onClick={() => setPage(p => p + 1)}>
                      下一页
                    </Button>
                  </div>
                </div>
              </>
            )}
          </CardContent>
        </Card>
      </motion.div>

      <LogDetailModal
        isAdmin={isAdmin}
        logId={selectedLogId}
        open={detailModalOpen}
        onOpenChange={setDetailModalOpen}
      />
    </motion.div>
  )
}
