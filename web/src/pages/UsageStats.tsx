import { useState, useEffect, useRef, useMemo, useCallback } from 'react'
import { getUsageSummary, getAdminUsageSummary, getAdminDistinctModels, getDistinctModels, UsageSummary } from '@/api/amp'
import { listUsers, UserInfo } from '@/api/users'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Num } from '@/components/Num'
import { UsageSummaryCards } from '@/components/UsageSummaryCards'
import { LogFilterBar, FilterValues, localToISO } from '@/components/LogFilterBar'
import { motion } from '@/lib/motion'
import { PageSizeSlider } from '@/components/PageSizeSlider'

interface Props {
  isAdmin: boolean
}

type SummaryGroupBy = 'day' | 'model' | 'user'
type RefreshInterval = 5 | 10 | 30 | 60

export default function UsageStats({ isAdmin }: Props) {
  const [summary, setSummary] = useState<UsageSummary[]>([])
  const [summaryGroupBy, setSummaryGroupBy] = useState<SummaryGroupBy>('day')
  const [error, setError] = useState('')

  const [filters, setFilters] = useState<FilterValues>({ userId: '', apiKeyId: '', model: '', from: '', to: '' })

  const [users, setUsers] = useState<UserInfo[]>([])
  const [models, setModels] = useState<string[]>([])

  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)

  const [autoRefresh, setAutoRefresh] = useState(false)
  const [refreshInterval, setRefreshInterval] = useState<RefreshInterval>(10)
  const refreshTimerRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const summaryAbortControllerRef = useRef<AbortController | null>(null)

  const userIdToUsername = useMemo(() => {
    const map = new Map<string, string>()
    users.forEach(u => map.set(u.id, u.username))
    return map
  }, [users])

  useEffect(() => {
    if (isAdmin) {
      Promise.all([listUsers(), getAdminDistinctModels()])
        .then(([usersRes, modelsRes]) => {
          setUsers(usersRes || [])
          setModels(modelsRes.models || [])
        })
        .catch(console.error)
    } else {
      getDistinctModels()
        .then(res => setModels(res.models || []))
        .catch(console.error)
    }
  }, [isAdmin])

  useEffect(() => {
    return () => {
      if (summaryAbortControllerRef.current) summaryAbortControllerRef.current.abort()
    }
  }, [])

  const loadSummary = useCallback(async () => {
    if (summaryAbortControllerRef.current) summaryAbortControllerRef.current.abort()
    const controller = new AbortController()
    summaryAbortControllerRef.current = controller

    setError('')
    try {
      const params = {
        groupBy: summaryGroupBy,
        model: filters.model || undefined,
        from: filters.from ? localToISO(filters.from) : undefined,
        to: filters.to ? localToISO(filters.to) : undefined,
      }
      const result = isAdmin
        ? await getAdminUsageSummary({ ...params, userId: filters.userId || undefined }, controller.signal)
        : await getUsageSummary(params, controller.signal)
      if (controller.signal.aborted) return
      setSummary(result.items || [])
    } catch (err) {
      if (controller.signal.aborted) return
      setError(err instanceof Error ? err.message : '加载失败')
    }
  }, [isAdmin, summaryGroupBy, filters])

  useEffect(() => {
    loadSummary()
  }, [loadSummary])

  useEffect(() => {
    if (autoRefresh) {
      refreshTimerRef.current = setInterval(loadSummary, refreshInterval * 1000)
    } else {
      if (refreshTimerRef.current) {
        clearInterval(refreshTimerRef.current)
        refreshTimerRef.current = null
      }
    }
    return () => {
      if (refreshTimerRef.current) clearInterval(refreshTimerRef.current)
    }
  }, [autoRefresh, refreshInterval, loadSummary])

  const handleFilterChange = (newFilters: FilterValues) => {
    setFilters(newFilters)
    setPage(1)
  }

  const handlePageSizeChange = (newSize: number) => {
    setPageSize(newSize)
    setPage(1)
  }

  const getSummaryDisplayName = (groupKey: string): string => {
    if (summaryGroupBy === 'user') {
      return userIdToUsername.get(groupKey) || groupKey
    }
    return groupKey
  }

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="space-y-6">
      <motion.div initial={{ opacity: 0, y: -20 }} animate={{ opacity: 1, y: 0 }} transition={{ type: 'spring', bounce: 0.2, duration: 0.6 }}>
        <h2 className="text-2xl font-bold tracking-tight">使用量统计</h2>
        <p className="text-muted-foreground">
          {isAdmin ? '管理员视图：查看所有用户的 Token 使用量和成本统计' : '查看 Token 使用量和成本统计'}
        </p>
      </motion.div>

      <LogFilterBar
        isAdmin={isAdmin}
        users={users}
        keys={[]}
        models={models}
        values={filters}
        onChange={handleFilterChange}
      />

      {error && (
        <div className="rounded-md bg-red-50 p-4 text-red-700">{error}</div>
      )}

      <UsageSummaryCards summary={summary} />

      <motion.div initial={{ opacity: 0, y: 30 }} animate={{ opacity: 1, y: 0 }} transition={{ type: 'spring', bounce: 0.2, duration: 0.6, delay: 0.15 }}>
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <div>
                <CardTitle>使用量明细</CardTitle>
                <CardDescription>
                  {isAdmin ? '按时间、模型或用户分组的 Token 使用量' : '按时间或模型分组的 Token 使用量'}
                </CardDescription>
              </div>
              <div className="flex items-center gap-3">
                <Select value={summaryGroupBy} onValueChange={(v) => setSummaryGroupBy(v as SummaryGroupBy)}>
                  <SelectTrigger className="w-32">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="day">按日期</SelectItem>
                    <SelectItem value="model">按模型</SelectItem>
                    {isAdmin && <SelectItem value="user">按用户</SelectItem>}
                  </SelectContent>
                </Select>
                <div className="flex items-center gap-2 border-l pl-3">
                  <Switch id="stats-auto-refresh" checked={autoRefresh} onCheckedChange={setAutoRefresh} />
                  <Label htmlFor="stats-auto-refresh" className="text-sm">自动刷新</Label>
                  {autoRefresh && (
                    <Select value={String(refreshInterval)} onValueChange={(v) => setRefreshInterval(Number(v) as RefreshInterval)}>
                      <SelectTrigger className="w-20">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="5">5秒</SelectItem>
                        <SelectItem value="10">10秒</SelectItem>
                        <SelectItem value="30">30秒</SelectItem>
                        <SelectItem value="60">60秒</SelectItem>
                      </SelectContent>
                    </Select>
                  )}
                </div>
                <Button variant="outline" onClick={loadSummary}>刷新</Button>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            {summary.length === 0 ? (
              <p className="text-center text-muted-foreground py-4">暂无数据</p>
            ) : (
              <>
              <div className="relative overflow-auto max-h-[calc(100vh-420px)] min-h-[300px] rounded-md border">
              <Table>
                <TableHeader className="sticky top-0 z-10 bg-background">
                  <TableRow>
                    <TableHead>{summaryGroupBy === 'day' ? '日期' : summaryGroupBy === 'model' ? '模型' : '用户'}</TableHead>
                    <TableHead className="text-right">请求数</TableHead>
                    <TableHead className="text-right">输入(去缓存)</TableHead>
                    <TableHead className="text-right">输出</TableHead>
                    <TableHead className="text-right">缓存读</TableHead>
                    <TableHead className="text-right">缓存写</TableHead>
                    <TableHead className="text-right">成本</TableHead>
                    <TableHead className="text-right">错误</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {summary.slice((page - 1) * pageSize, page * pageSize).map((s, i) => (
                    <TableRow key={i}>
                      <TableCell className="font-medium">{getSummaryDisplayName(s.groupKey)}</TableCell>
                      <TableCell className="text-right"><Num value={s.requestCount} /></TableCell>
                      <TableCell className="text-right"><Num value={s.inputTokensSum} /></TableCell>
                      <TableCell className="text-right"><Num value={s.outputTokensSum} /></TableCell>
                      <TableCell className="text-right"><Num value={s.cacheReadInputTokensSum} /></TableCell>
                      <TableCell className="text-right"><Num value={s.cacheCreationInputTokensSum} /></TableCell>
                      <TableCell className="text-right text-green-600 dark:text-green-400">
                        {s.costUsdSum ? `$${s.costUsdSum}` : '-'}
                      </TableCell>
                      <TableCell className="text-right">
                        {s.errorCount > 0 ? (
                          <Badge variant="destructive">{s.errorCount}</Badge>
                        ) : (
                          <span className="text-muted-foreground">0</span>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
              </div>
              {(() => {
                const totalPages = Math.max(1, Math.ceil(summary.length / pageSize))
                return (
                  <div className="flex items-center justify-between mt-4">
                    <div className="flex items-center gap-4">
                      <p className="text-sm text-muted-foreground">
                        第 {page} 页，共 {totalPages} 页（{summary.length} 条）
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
                )
              })()}
              </>
            )}
          </CardContent>
        </Card>
      </motion.div>
    </motion.div>
  )
}
