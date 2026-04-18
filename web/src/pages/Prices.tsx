import { useState, useEffect, useMemo, useCallback, KeyboardEvent, useRef } from 'react'
import { motion, tableStaggerContainer, tableRowVariants } from '@/lib/motion'
import {
  listPrices,
  getPriceStats,
  refreshPrices,
  listPriceContextRules,
  updatePriceContextRules,
  ModelPrice,
  ModelPriceContextRule,
  ModelPriceContextRuleRequest,
  PriceStats,
} from '../api/billing'
import { Button } from '@/components/ui/button'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Table,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { OverflowCopyText } from '@/components/OverflowCopyText'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { PageSizeSlider } from '@/components/PageSizeSlider'

function formatPrice(costPerToken: number): string {
  if (!costPerToken || costPerToken === 0) return '-'
  const perMillion = costPerToken * 1_000_000
  if (perMillion >= 1) {
    return `$${perMillion.toFixed(2)}`
  }
  if (perMillion >= 0.01) {
    return `$${perMillion.toFixed(3)}`
  }
  return `$${perMillion.toFixed(4)}`
}

const providerVariants: Record<string, 'default' | 'secondary' | 'destructive' | 'outline'> = {
  anthropic: 'default',
  openai: 'secondary',
  google: 'outline',
  gemini: 'outline',
  deepseek: 'default',
  azure: 'secondary',
}

const MESSAGE_AUTO_DISMISS_DELAY = 5000

interface EditableContextRule {
  id: string
  ruleName: string
  minTokens: string
  maxTokens: string
  inputPerMillion: string
  outputPerMillion: string
  cacheReadPerMillion: string
  cacheCreationPerMillion: string
}

function toPerMillionString(costPerToken: number): string {
  if (!costPerToken) return ''
  return (costPerToken * 1_000_000).toString()
}

function fromPerMillionString(value: string): number {
  const parsed = Number.parseFloat(value || '0')
  if (Number.isNaN(parsed) || parsed < 0) return 0
  return parsed / 1_000_000
}

function toEditableContextRule(rule: ModelPriceContextRule): EditableContextRule {
  return {
    id: rule.id,
    ruleName: rule.ruleName,
    minTokens: String(rule.minTokens),
    maxTokens: rule.maxTokens ? String(rule.maxTokens) : '',
    inputPerMillion: toPerMillionString(rule.inputCostPerToken),
    outputPerMillion: toPerMillionString(rule.outputCostPerToken),
    cacheReadPerMillion: toPerMillionString(rule.cacheReadInputPerToken),
    cacheCreationPerMillion: toPerMillionString(rule.cacheCreationPerToken),
  }
}

function cloneEditableContextRules(rules: EditableContextRule[]): EditableContextRule[] {
  return rules.map((rule) => ({ ...rule }))
}

function createEmptyContextRule(): EditableContextRule {
  return {
    id: crypto.randomUUID(),
    ruleName: '',
    minTokens: '',
    maxTokens: '',
    inputPerMillion: '',
    outputPerMillion: '',
    cacheReadPerMillion: '',
    cacheCreationPerMillion: '',
  }
}

function normalizeEditableContextRule(rule: EditableContextRule) {
  return {
    ruleName: rule.ruleName.trim(),
    minTokens: rule.minTokens.replace(/\D/g, ''),
    maxTokens: rule.maxTokens.replace(/\D/g, ''),
    inputPerMillion: rule.inputPerMillion,
    outputPerMillion: rule.outputPerMillion,
    cacheReadPerMillion: rule.cacheReadPerMillion,
    cacheCreationPerMillion: rule.cacheCreationPerMillion,
  }
}

function areEditableContextRulesEqual(left: EditableContextRule[], right: EditableContextRule[]): boolean {
  if (left.length !== right.length) return false
  return JSON.stringify(left.map(normalizeEditableContextRule)) === JSON.stringify(right.map(normalizeEditableContextRule))
}

export default function PricesPage() {
  const [prices, setPrices] = useState<ModelPrice[]>([])
  const [stats, setStats] = useState<PriceStats | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')
  const [searchTerm, setSearchTerm] = useState('')
  const [providerFilter, setProviderFilter] = useState<string>('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [rulesDialogModel, setRulesDialogModel] = useState<ModelPrice | null>(null)
  const [rulesDraftModelName, setRulesDraftModelName] = useState<string | null>(null)
  const [loadedRulesSnapshot, setLoadedRulesSnapshot] = useState<EditableContextRule[]>([])
  const [contextRulesDraft, setContextRulesDraft] = useState<EditableContextRule[]>([])
  const [rulesLoading, setRulesLoading] = useState(false)
  const [rulesSaving, setRulesSaving] = useState(false)
  const rulesRequestIdRef = useRef(0)

  const loadData = useCallback(async (signal?: AbortSignal) => {
    try {
      const [pricesData, statsData] = await Promise.all([
        listPrices(),
        getPriceStats(),
      ])
      if (signal?.aborted) return
      setPrices(pricesData.items || [])
      setStats(statsData)
      setError('')
    } catch (err) {
      if (signal?.aborted) return
      setError(err instanceof Error ? err.message : '加载失败')
      setSuccess('')
    } finally {
      if (!signal?.aborted) {
        setLoading(false)
      }
    }
  }, [])

  useEffect(() => {
    const abortController = new AbortController()
    loadData(abortController.signal)
    return () => {
      abortController.abort()
    }
  }, [loadData])

  useEffect(() => {
    if (!success) return
    const timer = setTimeout(() => setSuccess(''), MESSAGE_AUTO_DISMISS_DELAY)
    return () => clearTimeout(timer)
  }, [success])

  useEffect(() => {
    if (!error) return
    const timer = setTimeout(() => setError(''), MESSAGE_AUTO_DISMISS_DELAY)
    return () => clearTimeout(timer)
  }, [error])

  const handleRefresh = async () => {
    setRefreshing(true)
    setError('')
    setSuccess('')
    try {
      const result = await refreshPrices()
      setSuccess(`${result.message}，共 ${result.modelCount} 个模型`)
      await loadData()
    } catch (err) {
      setError(err instanceof Error ? err.message : '刷新失败')
    } finally {
      setRefreshing(false)
    }
  }

  const handleBadgeKeyDown = (e: KeyboardEvent, callback: () => void) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      callback()
    }
  }

  const providers = useMemo(() => {
    const set = new Set(prices.map(p => p.provider).filter(Boolean))
    return Array.from(set).sort()
  }, [prices])

  const filteredPrices = useMemo(() => {
    return prices.filter(p => {
      const matchesSearch = !searchTerm || 
        p.model.toLowerCase().includes(searchTerm.toLowerCase()) ||
        p.provider?.toLowerCase().includes(searchTerm.toLowerCase())
      const matchesProvider = !providerFilter || p.provider === providerFilter
      return matchesSearch && matchesProvider
    })
  }, [prices, searchTerm, providerFilter])

  // 当筛选条件变化时重置页码
  useEffect(() => {
    setPage(1)
  }, [searchTerm, providerFilter])

  const totalPages = Math.max(1, Math.ceil(filteredPrices.length / pageSize))
  const paginatedPrices = useMemo(() => {
    const start = (page - 1) * pageSize
    return filteredPrices.slice(start, start + pageSize)
  }, [filteredPrices, page, pageSize])

  const providerStats = useMemo(() => {
    const counts: Record<string, number> = {}
    prices.forEach(p => {
      const provider = p.provider || 'unknown'
      counts[provider] = (counts[provider] || 0) + 1
    })
    return counts
  }, [prices])

  const handlePageSizeChange = (newSize: number) => {
    setPageSize(newSize)
    setPage(1)
  }

  const clearRulesDialogState = useCallback(() => {
    rulesRequestIdRef.current += 1
    setRulesDialogModel(null)
    setRulesDraftModelName(null)
    setLoadedRulesSnapshot([])
    setContextRulesDraft([])
    setRulesLoading(false)
  }, [])

  const isRulesDirty = useMemo(() => {
    if (!rulesDraftModelName) return false
    return !areEditableContextRulesEqual(loadedRulesSnapshot, contextRulesDraft)
  }, [contextRulesDraft, loadedRulesSnapshot, rulesDraftModelName])

  const loadRulesForModel = useCallback(async (price: ModelPrice) => {
    const requestId = rulesRequestIdRef.current + 1
    rulesRequestIdRef.current = requestId

    setRulesDialogModel(price)
    setRulesDraftModelName(price.model)
    setRulesLoading(true)
    setLoadedRulesSnapshot([])
    setContextRulesDraft([])
    try {
      const result = await listPriceContextRules(price.model)
      if (requestId !== rulesRequestIdRef.current) return

      const snapshot = (result.items || []).map(toEditableContextRule)
      setLoadedRulesSnapshot(snapshot)
      setContextRulesDraft(cloneEditableContextRules(snapshot))
      setError('')
    } catch (err) {
      if (requestId !== rulesRequestIdRef.current) return
      setError(err instanceof Error ? err.message : '加载上下文规则失败')
    } finally {
      if (requestId === rulesRequestIdRef.current) {
        setRulesLoading(false)
      }
    }
  }, [])

  const requestCloseRulesDialog = useCallback(() => {
    if (rulesSaving) return false
    if (isRulesDirty && !window.confirm('关闭将丢弃未保存的上下文规则，确定继续吗？')) {
      return false
    }
    clearRulesDialogState()
    return true
  }, [clearRulesDialogState, isRulesDirty, rulesSaving])

  const openRulesDialog = async (price: ModelPrice) => {
    const nextModelName = price.model
    const isSameModel = rulesDraftModelName === nextModelName

    if (isSameModel) {
      setRulesDialogModel(price)
      if (rulesLoading || isRulesDirty) {
        return
      }
    } else if (rulesDraftModelName && isRulesDirty) {
      const shouldDiscard = window.confirm(`切换到 ${nextModelName} 将丢弃当前未保存的上下文规则，确定继续吗？`)
      if (!shouldDiscard) return
    }

    await loadRulesForModel(price)
  }

  const addContextRule = () => {
    setContextRulesDraft((prev) => [...prev, createEmptyContextRule()])
  }

  const updateContextRule = (id: string, field: keyof EditableContextRule, value: string) => {
    setContextRulesDraft((prev) => prev.map((rule) => rule.id === id ? { ...rule, [field]: value } : rule))
  }

  const removeContextRule = (id: string) => {
    setContextRulesDraft((prev) => prev.filter((rule) => rule.id !== id))
  }

  const handleSaveContextRules = async () => {
    const modelName = rulesDraftModelName || rulesDialogModel?.model
    if (!modelName) return
    setRulesSaving(true)
    try {
      const payload: ModelPriceContextRuleRequest[] = contextRulesDraft.map((rule, index) => ({
        ruleName: rule.ruleName.trim(),
        minTokens: Number.parseInt(rule.minTokens || '0', 10) || 0,
        maxTokens: rule.maxTokens.trim() ? Number.parseInt(rule.maxTokens, 10) : undefined,
        inputCostPerToken: fromPerMillionString(rule.inputPerMillion),
        outputCostPerToken: fromPerMillionString(rule.outputPerMillion),
        cacheReadInputPerToken: fromPerMillionString(rule.cacheReadPerMillion),
        cacheCreationPerToken: fromPerMillionString(rule.cacheCreationPerMillion),
        sortOrder: index,
      }))
      await updatePriceContextRules(modelName, payload)
      setSuccess(`已更新 ${modelName} 的上下文规则`)
      clearRulesDialogState()
    } catch (err) {
      setError(err instanceof Error ? err.message : '保存上下文规则失败')
    } finally {
      setRulesSaving(false)
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-muted-foreground">加载中...</div>
      </div>
    )
  }

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="space-y-6">
      <motion.div initial={{ opacity: 0, y: -20 }} animate={{ opacity: 1, y: 0 }} transition={{ type: 'spring', bounce: 0.2, duration: 0.6 }}>
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-bold tracking-tight">模型价格表</h1>
            <p className="text-muted-foreground">
              LiteLLM 模型价格，用于计算请求成本
            </p>
          </div>
          <Button onClick={handleRefresh} disabled={refreshing}>
            {refreshing ? '刷新中...' : '刷新价格'}
          </Button>
        </div>
      </motion.div>

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {success && (
        <Alert>
          <AlertDescription>{success}</AlertDescription>
        </Alert>
      )}

      {/* 统计卡片 */}
      <div className="grid gap-4 md:grid-cols-4">
        <motion.div initial={{ opacity: 0, y: 20, scale: 0.9 }} animate={{ opacity: 1, y: 0, scale: 1 }} transition={{ type: 'spring', bounce: 0.35, duration: 0.6, delay: 0 * 0.08 }} whileHover={{ scale: 1.05, y: -4 }}>
          <Card>
            <CardHeader className="pb-2">
              <CardDescription>总模型数</CardDescription>
              <CardTitle className="text-2xl">{prices.length}</CardTitle>
            </CardHeader>
          </Card>
        </motion.div>
        <motion.div initial={{ opacity: 0, y: 20, scale: 0.9 }} animate={{ opacity: 1, y: 0, scale: 1 }} transition={{ type: 'spring', bounce: 0.35, duration: 0.6, delay: 1 * 0.08 }} whileHover={{ scale: 1.05, y: -4 }}>
          <Card>
            <CardHeader className="pb-2">
              <CardDescription>数据来源</CardDescription>
              <CardTitle className="text-2xl">{stats?.source || '-'}</CardTitle>
            </CardHeader>
          </Card>
        </motion.div>
        <motion.div initial={{ opacity: 0, y: 20, scale: 0.9 }} animate={{ opacity: 1, y: 0, scale: 1 }} transition={{ type: 'spring', bounce: 0.35, duration: 0.6, delay: 2 * 0.08 }} whileHover={{ scale: 1.05, y: -4 }}>
          <Card>
            <CardHeader className="pb-2">
              <CardDescription>更新时间</CardDescription>
              <CardTitle className="text-lg">
                {stats?.fetchedAt ? new Date(stats.fetchedAt).toLocaleString() : '-'}
              </CardTitle>
            </CardHeader>
          </Card>
        </motion.div>
        <motion.div initial={{ opacity: 0, y: 20, scale: 0.9 }} animate={{ opacity: 1, y: 0, scale: 1 }} transition={{ type: 'spring', bounce: 0.35, duration: 0.6, delay: 3 * 0.08 }} whileHover={{ scale: 1.05, y: -4 }}>
          <Card>
            <CardHeader className="pb-2">
              <CardDescription>Provider 数</CardDescription>
              <CardTitle className="text-2xl">{providers.length}</CardTitle>
            </CardHeader>
          </Card>
        </motion.div>
      </div>

      {/* Provider 分布 */}
      <Card>
        <CardHeader>
          <CardTitle>Provider 分布</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap gap-2">
            <Badge
              variant={!providerFilter ? 'default' : 'outline'}
              className="cursor-pointer"
              role="button"
              tabIndex={0}
              aria-pressed={!providerFilter}
              onClick={() => setProviderFilter('')}
              onKeyDown={(e) => handleBadgeKeyDown(e, () => setProviderFilter(''))}
            >
              全部 ({prices.length})
            </Badge>
            {Object.entries(providerStats)
              .sort((a, b) => b[1] - a[1])
              .slice(0, 15)
              .map(([provider, count]) => {
                const isSelected = providerFilter === provider
                return (
                  <motion.div key={provider} initial={{ opacity: 0, scale: 0.8 }} animate={{ opacity: 1, scale: 1 }} transition={{ type: 'spring', bounce: 0.4, duration: 0.4 }} whileHover={{ scale: 1.15 }} whileTap={{ scale: 0.9 }} style={{ display: 'inline-block' }}>
                    <Badge
                      variant={isSelected ? 'default' : (providerVariants[provider] || 'outline')}
                      className="cursor-pointer"
                      role="button"
                      tabIndex={0}
                      aria-pressed={isSelected}
                      onClick={() => setProviderFilter(isSelected ? '' : provider)}
                      onKeyDown={(e) => handleBadgeKeyDown(e, () => setProviderFilter(isSelected ? '' : provider))}
                    >
                      {provider} ({count})
                    </Badge>
                  </motion.div>
                )
              })}
          </div>
        </CardContent>
      </Card>

      {/* 搜索和过滤 */}
      <div className="flex gap-4">
        <Input
          type="search"
          placeholder="搜索模型名称..."
          value={searchTerm}
          onChange={e => setSearchTerm(e.target.value)}
          className="max-w-sm"
        />
        <div className="text-muted-foreground self-center">
          显示 {filteredPrices.length} / {prices.length} 个模型
        </div>
      </div>

      {/* 价格表 */}
      <motion.div initial={{ opacity: 0, y: 30 }} animate={{ opacity: 1, y: 0 }} transition={{ type: 'spring', bounce: 0.2, duration: 0.6, delay: 0.1 }}>
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <div>
                <CardTitle>价格列表</CardTitle>
                <CardDescription>共 {filteredPrices.length} 条记录</CardDescription>
              </div>
              <Button variant="outline" onClick={() => void loadData()}>刷新</Button>
            </div>
          </CardHeader>
          <CardContent>
            {filteredPrices.length === 0 ? (
              <div className="py-8 text-center text-muted-foreground">
                <p className="text-lg">未找到匹配的模型</p>
                <p className="text-sm mt-1">
                  {searchTerm && `搜索: "${searchTerm}"`}
                  {searchTerm && providerFilter && ' · '}
                  {providerFilter && `Provider: ${providerFilter}`}
                </p>
                <Button
                  variant="link"
                  className="mt-2"
                  onClick={() => {
                    setSearchTerm('')
                    setProviderFilter('')
                  }}
                >
                  清除筛选条件
                </Button>
              </div>
            ) : (
              <>
                <div className="relative overflow-auto max-h-[calc(100vh-320px)] min-h-[400px] rounded-md border">
                  <Table>
                    <TableHeader className="sticky top-0 z-10 bg-background">
                      <TableRow>
                        <TableHead>模型</TableHead>
                        <TableHead>Provider</TableHead>
                        <TableHead className="text-right">输入 ($/1M)</TableHead>
                        <TableHead className="text-right">输出 ($/1M)</TableHead>
                        <TableHead className="text-right">缓存读取</TableHead>
                        <TableHead className="text-right">缓存创建</TableHead>
                        <TableHead>来源</TableHead>
                        <TableHead className="text-right">操作</TableHead>
                      </TableRow>
                    </TableHeader>
                    <motion.tbody variants={tableStaggerContainer} initial="hidden" animate="visible" key={`${page}-${pageSize}-${searchTerm}-${providerFilter}`}>
                      {paginatedPrices.map((price) => (
                        <motion.tr key={`${price.provider ?? 'unknown'}:${price.model}`} variants={tableRowVariants} className="border-b transition-colors hover:bg-muted/50 data-[state=selected]:bg-muted">
                          <TableCell className="max-w-[260px]">
                            <OverflowCopyText
                              text={price.model}
                              className="max-w-[260px] font-mono text-sm"
                            />
                          </TableCell>
                          <TableCell>
                            <Badge variant={providerVariants[price.provider ?? ''] || 'outline'}>
                              {price.provider || '-'}
                            </Badge>
                          </TableCell>
                          <TableCell className="text-right font-mono">
                            {formatPrice(price.inputCostPerToken)}
                          </TableCell>
                          <TableCell className="text-right font-mono">
                            {formatPrice(price.outputCostPerToken)}
                          </TableCell>
                          <TableCell className="text-right font-mono text-muted-foreground">
                            {formatPrice(price.cacheReadInputPerToken)}
                          </TableCell>
                          <TableCell className="text-right font-mono text-muted-foreground">
                            {formatPrice(price.cacheCreationPerToken)}
                          </TableCell>
                          <TableCell>
                            <Badge variant="outline" className="text-xs">
                              {price.source}
                            </Badge>
                          </TableCell>
                          <TableCell className="text-right">
                            <Button variant="outline" size="sm" onClick={() => void openRulesDialog(price)}>
                              编辑
                            </Button>
                          </TableCell>
                        </motion.tr>
                      ))}
                    </motion.tbody>
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

      <Dialog
        open={!!rulesDialogModel}
        onOpenChange={(open) => {
          if (!open) {
            requestCloseRulesDialog()
          }
        }}
      >
        <DialogContent className="max-w-5xl">
          <DialogHeader>
            <DialogTitle>编辑上下文规则</DialogTitle>
            <DialogDescription>{rulesDialogModel?.model || '-'} 达到指定上下文区间后，将按规则价格计费；未命中规则时回退默认价格。</DialogDescription>
          </DialogHeader>
          <div className="space-y-3 py-2">
            <div className="flex items-center justify-between">
              <p className="text-sm text-muted-foreground">区间采用 [最小值, 最大值)；最大值留空表示无上限。</p>
              <Button variant="outline" size="sm" onClick={addContextRule}>添加规则</Button>
            </div>
            {rulesLoading ? (
              <div className="py-8 text-center text-muted-foreground">加载中...</div>
            ) : contextRulesDraft.length === 0 ? (
              <div className="rounded-md border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">
                暂无上下文规则
              </div>
            ) : (
              <div className="space-y-3">
                {contextRulesDraft.map((rule) => (
                  <div key={rule.id} className="grid gap-3 rounded-lg border border-border/70 p-4 md:grid-cols-4">
                    <Input value={rule.ruleName} onChange={(e) => updateContextRule(rule.id, 'ruleName', e.target.value)} placeholder="规则名，例如 长上下文" />
                    <Input value={rule.minTokens} onChange={(e) => updateContextRule(rule.id, 'minTokens', e.target.value.replace(/\D/g, ''))} placeholder="最小 tokens" />
                    <Input value={rule.maxTokens} onChange={(e) => updateContextRule(rule.id, 'maxTokens', e.target.value.replace(/\D/g, ''))} placeholder="最大 tokens（可空）" />
                    <Button variant="ghost" size="sm" className="justify-self-end" onClick={() => removeContextRule(rule.id)}>删除</Button>
                    <Input value={rule.inputPerMillion} onChange={(e) => updateContextRule(rule.id, 'inputPerMillion', e.target.value)} placeholder="输入 $/1M" />
                    <Input value={rule.outputPerMillion} onChange={(e) => updateContextRule(rule.id, 'outputPerMillion', e.target.value)} placeholder="输出 $/1M" />
                    <Input value={rule.cacheReadPerMillion} onChange={(e) => updateContextRule(rule.id, 'cacheReadPerMillion', e.target.value)} placeholder="缓存读 $/1M" />
                    <Input value={rule.cacheCreationPerMillion} onChange={(e) => updateContextRule(rule.id, 'cacheCreationPerMillion', e.target.value)} placeholder="缓存写 $/1M" />
                  </div>
                ))}
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => requestCloseRulesDialog()}>取消</Button>
            <Button onClick={() => void handleSaveContextRules()} disabled={rulesSaving}>
              {rulesSaving ? '保存中...' : '保存规则'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </motion.div>
  )
}
