import { memo, startTransition, useDeferredValue, useState, useEffect, useRef, useMemo, useCallback } from 'react'
import {
  getRequestLogs, getAdminRequestLogs, getAdminDistinctModels, getAdminDistinctKeys, getDistinctKeys, getDistinctModels,
  RequestLog, DistinctAPIKey,
} from '@/api/amp'
import { connectRequestLogsWS } from '@/api/requestLogsWS'
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

interface Props {
  isAdmin: boolean
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
      <TableCell>
        {(log.channelName || log.provider) ? (
          <div className="flex flex-wrap items-center gap-1.5">
            <Badge variant="outline" className="text-xs">{log.channelName || log.provider}</Badge>
            {formatTransportLabel(log.downstreamTransport) && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Badge variant="outline" className="cursor-help text-[10px] uppercase tracking-[0.12em] text-muted-foreground">
                    D {formatTransportLabel(log.downstreamTransport)}
                  </Badge>
                </TooltipTrigger>
                <TooltipContent side="bottom" className="max-w-72 text-xs">
                  Downstream：AMP Manager 到客户端这一侧的传输方式。
                </TooltipContent>
              </Tooltip>
            )}
            {formatTransportLabel(log.upstreamTransport) && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Badge variant="outline" className="cursor-help text-[10px] uppercase tracking-[0.12em] text-muted-foreground">
                    U {formatTransportLabel(log.upstreamTransport)}
                  </Badge>
                </TooltipTrigger>
                <TooltipContent side="bottom" className="max-w-72 text-xs">
                  Upstream：AMP Manager 到上游模型服务这一侧的传输方式。
                </TooltipContent>
              </Tooltip>
            )}
            {log.transportFallbackReason && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Badge variant="secondary" className="cursor-help text-[10px] uppercase tracking-[0.12em]">
                    Fallback
                  </Badge>
                </TooltipTrigger>
                <TooltipContent side="bottom" className="max-w-72 text-xs">
                  {log.transportFallbackReason}
                </TooltipContent>
              </Tooltip>
            )}
          </div>
        ) : (
          <span className="text-muted-foreground">-</span>
        )}
      </TableCell>
      <TableCell>
        {log.thinkingLevel ? (
          <Badge variant="secondary" className="text-xs">{log.thinkingLevel}</Badge>
        ) : (
          <span className="text-muted-foreground">-</span>
        )}
      </TableCell>
      <TableCell>
        <Badge variant="outline">{log.method}</Badge>
      </TableCell>
      <TableCell>
        {isAdmin ? (
          <button
            onClick={() => onOpenDetail(log.id)}
            className="cursor-pointer hover:opacity-80 transition-opacity"
            title="点击查看请求详情"
          >
            <StatusBadge status={log.statusCode} />
          </button>
        ) : (
          <StatusBadge status={log.statusCode} />
        )}
      </TableCell>
      <TableCell className="text-right text-muted-foreground">
        {renderDuration(log.ttfbMs)}
      </TableCell>
      <TableCell className="text-right text-muted-foreground">
        {typeof log.tps === 'number' ? `${log.tps.toFixed(1)}/s` : '-'}
      </TableCell>
      <TableCell className="text-right text-muted-foreground">
        {renderDuration(log.latencyMs)}
      </TableCell>
      <TableCell className="text-right"><Num value={log.inputTokens} /></TableCell>
      <TableCell className="text-right"><Num value={log.outputTokens} /></TableCell>
      <TableCell className="text-right"><Num value={log.cacheReadInputTokens} /></TableCell>
      <TableCell className="text-right"><Num value={log.cacheCreationInputTokens} /></TableCell>
      <TableCell className="text-right text-muted-foreground">
        {log.costUsd ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="cursor-help underline decoration-dotted underline-offset-4">{`$${log.costUsd}`}</span>
            </TooltipTrigger>
            <TooltipContent side="bottom" className="max-w-80 text-xs">
              <div className="space-y-1.5">
                <p className="font-medium">计费标准</p>
                <div className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
                  <span className="text-muted-foreground">输入</span>
                  <span className="font-mono">{formatPricePerMillion(log.inputCostPerToken)}</span>
                  <span className="text-muted-foreground">输出</span>
                  <span className="font-mono">{formatPricePerMillion(log.outputCostPerToken)}</span>
                  <span className="text-muted-foreground">缓存读</span>
                  <span className="font-mono">{formatPricePerMillion(log.cacheReadInputPerToken)}</span>
                  <span className="text-muted-foreground">缓存写</span>
                  <span className="font-mono">{formatPricePerMillion(log.cacheCreationInputPerToken)}</span>
                </div>
              </div>
            </TooltipContent>
          </Tooltip>
        ) : '-'}
      </TableCell>
      <TableCell className="text-right">
        {typeof log.rateMultiplier === 'number' ? (
          <Badge variant="outline" className="font-mono">
            {formatDecimal(log.rateMultiplier, 2)}x
          </Badge>
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

  const [filters, setFilters] = useState<FilterValues>({ userId: '', apiKeyId: '', model: '', statuses: [], from: '', to: '' })

  const [users, setUsers] = useState<UserInfo[]>([])
  const [models, setModels] = useState<string[]>([])
  const [keys, setKeys] = useState<DistinctAPIKey[]>([])

  const [autoRefresh, setAutoRefresh] = useState(true)
  const wsCloseRef = useRef<(() => void) | null>(null)
  const wsBufRef = useRef<RequestLog[]>([])
  const flushTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const logsRef = useRef<RequestLog[]>([])
  const totalRef = useRef(0)
  const knownLogIDsRef = useRef<Set<string>>(new Set())
  const filtersRef = useRef<FilterValues>({ userId: '', apiKeyId: '', model: '', statuses: [], from: '', to: '' })
  const abortControllerRef = useRef<AbortController | null>(null)

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

  useEffect(() => {
    if (liveInsertEnabled) {
      const flushBuf = () => {
        const batch = wsBufRef.current
        wsBufRef.current = []
        flushTimerRef.current = null
        if (batch.length === 0) return

        let next = [...logsRef.current]
        let newCount = 0
        let removedCount = 0
        const activeFilters = filtersRef.current
        for (const log of batch) {
          const matchesFilters = matchesRequestLogFilters(log, activeFilters)
          const idx = next.findIndex(l => l.id === log.id)
          if (idx >= 0) {
            if (matchesFilters) {
              next[idx] = log
            } else {
              next.splice(idx, 1)
              knownLogIDsRef.current.delete(log.id)
              removedCount++
            }
          } else if (matchesFilters) {
            next.unshift(log)
            if (!knownLogIDsRef.current.has(log.id)) {
              knownLogIDsRef.current.add(log.id)
              newCount++
            }
          }
        }

        next = next.slice(0, pageSize)
        logsRef.current = next
        startTransition(() => {
          setLogs(next)

          if (newCount > 0 || removedCount > 0) {
            totalRef.current += newCount - removedCount
            setTotal(totalRef.current)
          }
        })
      }

      const close = connectRequestLogsWS(
        (newLog) => {
          wsBufRef.current.push(newLog)
          if (flushTimerRef.current === null) {
            flushTimerRef.current = setTimeout(flushBuf, 100)
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
      wsBufRef.current = []
      if (wsCloseRef.current) {
        wsCloseRef.current()
        wsCloseRef.current = null
      }
    }
  }, [liveInsertEnabled, pageSize])

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
      const params = {
        page,
        pageSize,
        model: filters.model || undefined,
        statusCodes: filters.statuses.length ? filters.statuses.map((status) => Number.parseInt(status, 10)) : undefined,
        from: filters.from ? localToISO(filters.from) : undefined,
        to: filters.to ? localToISO(filters.to) : undefined,
      }
      const result = isAdmin
        ? await getAdminRequestLogs({ ...params, userId: filters.userId || undefined, apiKeyId: filters.apiKeyId || undefined }, controller.signal)
        : await getRequestLogs({ ...params, apiKeyId: filters.apiKeyId || undefined }, controller.signal)
      if (controller.signal.aborted) return

      const items = result.items || []
      logsRef.current = items
      knownLogIDsRef.current = new Set(items.map(item => item.id))
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
  }, [isAdmin, page, pageSize, filters])

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
  const tableBodyKey = [
    page,
    pageSize,
    filters.userId,
    filters.apiKeyId,
    filters.model,
    filters.statuses.join(','),
    filters.from,
    filters.to,
  ].join(':')

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
                <CardDescription>共 {total} 条记录</CardDescription>
              </div>
              <div className="flex items-center gap-3">
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
                <div className={`relative overflow-y-auto overflow-x-hidden max-h-[calc(100vh-320px)] min-h-[400px] rounded-md border transition-opacity ${fetching ? 'opacity-50 pointer-events-none' : ''}`}>
                <Table>
                  <TableHeader className="sticky top-0 z-10 bg-background">
                    <TableRow>
                      <TableHead>时间</TableHead>
                      {isAdmin && <TableHead>用户</TableHead>}
                      <TableHead>模型</TableHead>
                      <TableHead>渠道</TableHead>
                      <TableHead>思维等级</TableHead>
                      <TableHead>方法</TableHead>
                      <TableHead>状态</TableHead>
                      <TableHead className="text-right">TTFB</TableHead>
                      <TableHead className="text-right">TPS</TableHead>
                      <TableHead className="text-right">用时</TableHead>
                      <TableHead className="text-right">
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span className="cursor-help underline decoration-dotted underline-offset-4">输入</span>
                          </TooltipTrigger>
                          <TooltipContent side="bottom">
                            <p>这里的输入 Tokens 已扣除缓存读取命中的部分。</p>
                          </TooltipContent>
                        </Tooltip>
                      </TableHead>
                      <TableHead className="text-right">输出</TableHead>
                      <TableHead className="text-right">缓存读</TableHead>
                      <TableHead className="text-right">缓存写</TableHead>
                      <TableHead className="text-right">成本</TableHead>
                      <TableHead className="text-right">倍率</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody key={tableBodyKey}>
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

      {isAdmin && (
        <LogDetailModal
          logId={selectedLogId}
          open={detailModalOpen}
          onOpenChange={setDetailModalOpen}
        />
      )}
    </motion.div>
  )
}
