import { useState, useEffect, useMemo } from 'react'
import { getDashboard, DashboardData, DashboardCacheHitRate } from '@/api/dashboard'
import { getBillingState, updateBillingPriority, resetBillingDaily, BillingStateResponse } from '@/api/billing'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { CircularProgress } from '@/components/CircularProgress'
import { Num } from '@/components/Num'
import { formatDateTime, formatDateTimeWithSeconds, formatDecimal, formatGroupedNumericString } from '@/lib/formatters'
import { navigateDashboard } from '@/lib/dashboard-navigation'
import { motion, AnimatePresence, staggerContainer, staggerItem } from '@/lib/motion'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import {
  type ChartConfig,
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
} from '@/components/ui/chart'
import {
  Area, AreaChart, Bar, BarChart, Pie, PieChart,
  CartesianGrid, XAxis, YAxis, Cell, Label,
} from 'recharts'
import {
  Wallet, TrendingUp, Zap, ArrowUpRight, ArrowDownRight,
  RefreshCw, Activity, DollarSign, Hash, AlertTriangle,
  DatabaseZap, CreditCard, Shield, ArrowRightLeft, Check, Loader2,
  RotateCcw,
} from 'lucide-react'

const trendChartConfig = {
  cost: { label: '成本 (USD)', color: 'hsl(var(--chart-2))' },
  requests: { label: '请求数', color: 'hsl(var(--chart-1))' },
} satisfies ChartConfig

const modelChartConfig = {
  requests: { label: '请求数' },
  model1: { label: '模型 1', color: 'hsl(var(--chart-1))' },
  model2: { label: '模型 2', color: 'hsl(var(--chart-2))' },
  model3: { label: '模型 3', color: 'hsl(var(--chart-3))' },
  model4: { label: '模型 4', color: 'hsl(var(--chart-4))' },
  model5: { label: '模型 5', color: 'hsl(var(--chart-5))' },
} satisfies ChartConfig

const MODEL_COLORS = [
  'hsl(var(--chart-1))',
  'hsl(var(--chart-2))',
  'hsl(var(--chart-3))',
  'hsl(var(--chart-4))',
  'hsl(var(--chart-5))',
]

const LIMIT_TYPE_LABELS: Record<string, string> = {
  daily: '日限制',
  weekly: '周限制',
  monthly: '月限制',
  rolling_5h: '5小时滚动',
  total: '总量限制',
}

const WINDOW_MODE_LABELS: Record<string, string> = {
  fixed: '固定窗口',
  sliding: '滑动窗口',
}

export default function Overview() {
  const [data, setData] = useState<DashboardData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [billingState, setBillingState] = useState<BillingStateResponse | null>(null)
  const [billingLoading, setBillingLoading] = useState(true)
  const [priorityPopoverOpen, setPriorityPopoverOpen] = useState(false)
  const [prioritySaving, setPrioritySaving] = useState(false)
  const [billingResetting, setBillingResetting] = useState(false)

  const handlePriorityChange = async (value: 'subscription' | 'balance') => {
    if (value === billingState?.primarySource) {
      setPriorityPopoverOpen(false)
      return
    }
    setPrioritySaving(true)
    try {
      await updateBillingPriority(value)
      const billing = await getBillingState()
      setBillingState(billing)
      setPriorityPopoverOpen(false)
    } catch {
      // silently fail
    } finally {
      setPrioritySaving(false)
    }
  }

  const handleResetBillingDaily = async () => {
    if (!billingState?.dailyReset) return
    if (!billingState.dailyReset.allowed) return
    if (!window.confirm('确定要重置今日计费吗？本次操作会清空当前固定日额度已用量，并缩短 1 天订阅时长。')) {
      return
    }

    setBillingResetting(true)
    try {
      const result = await resetBillingDaily()
      setBillingState(result.state)
    } catch (err) {
      window.alert(err instanceof Error ? err.message : '重置今日计费失败')
    } finally {
      setBillingResetting(false)
    }
  }

  const loadDashboard = async () => {
    setLoading(true)
    setBillingLoading(true)
    setError('')
    setBillingState(null)

    void getBillingState()
      .then((billing) => {
        setBillingState(billing)
      })
      .catch(() => {
        setBillingState(null)
      })
      .finally(() => {
        setBillingLoading(false)
      })

    try {
      const result = await getDashboard()
      setData(result)
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadDashboard()
  }, [])

  const trendData = useMemo(() => {
    if (!data?.dailyTrend?.length) return []
    return data.dailyTrend.map(d => ({
      date: d.date.slice(5),
      cost: parseFloat(d.costUsd),
      requests: d.requests,
    }))
  }, [data?.dailyTrend])

  const modelBarData = useMemo(() => {
    if (!data?.topModels?.length) return []
    return data.topModels.map((m, i) => {
      const key = `model${i}`
      return {
        key,
        name: m.model.length > 20 ? m.model.slice(0, 18) + '…' : m.model,
        fullName: m.model,
        requests: m.requestCount,
        cost: parseFloat(m.costUsd),
        fill: `var(--color-${key})`,
      }
    })
  }, [data?.topModels])

  if (loading && !data) {
    return (
      <div className="flex items-center justify-center h-64">
        <RefreshCw className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error && !data) {
    return (
      <div className="flex flex-col items-center justify-center h-64 gap-4">
        <p className="text-destructive">{error}</p>
        <Button variant="outline" onClick={loadDashboard}>重试</Button>
      </div>
    )
  }

  if (!data) return null

  const balanceUsd = parseFloat(data.balance.balanceUsd)
  const todayCost = parseFloat(data.today.costUsd)
  const weekCost = parseFloat(data.week.costUsd)
  const concurrencyDisplay = `并发 ${data.balance.currentConcurrency} / ${data.balance.concurrencyLimit > 0 ? data.balance.concurrencyLimit : '∞'}`
  const formatUsd = (value: number, digits: number) => `$${formatDecimal(value, digits)}`
  const formatWindowUsd = (micros: number) => formatDecimal(micros / 1e6, 2)
  const getWindowMeta = (windowMode: string, limitType: string, windowStart: string, windowEnd: string) => {
    if (limitType === 'total') {
      return '总额度'
    }
    if (windowMode === 'fixed') {
      return `下次重置 ${formatDateTime(windowEnd)}`
    }
    return `统计窗口 ${formatDateTime(windowStart)} ~ 现在`
  }

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="space-y-6">
      {/* Header */}
      <motion.div
        initial={{ opacity: 0, y: -20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ type: 'spring', bounce: 0.2, duration: 0.6 }}
        className="flex items-center justify-between"
      >
        <div>
          <h2 className="text-2xl font-bold tracking-tight">概览</h2>
          <p className="text-muted-foreground">账户用量和费用一览</p>
        </div>
        <Button variant="outline" size="sm" onClick={loadDashboard} disabled={loading}>
          <RefreshCw className={`h-4 w-4 mr-2 ${loading ? 'animate-spin' : ''}`} />
          刷新
        </Button>
      </motion.div>

      {/* Balance + Today/Week/Month stats */}
      <motion.div
        variants={staggerContainer}
        initial="hidden"
        animate="visible"
        className="grid gap-4 md:grid-cols-2 lg:grid-cols-4"
      >
        <motion.div variants={staggerItem} whileHover={{ scale: 1.03, y: -4 }} whileTap={{ scale: 0.98 }} className="h-full">
          <Card className="h-full bg-gradient-to-br from-blue-500/10 to-cyan-500/10 border-blue-500/20">
            <CardHeader className="pb-2">
              <div className="flex items-center justify-between gap-2">
                <CardDescription className="flex items-center gap-1.5">
                  <Wallet className="h-4 w-4" />
                  账户余额
                </CardDescription>
                <Badge variant="outline" className="bg-background/70">{concurrencyDisplay}</Badge>
              </div>
              <CardTitle className="text-3xl text-blue-600 dark:text-blue-400">
                {formatUsd(balanceUsd, 2)}
              </CardTitle>
            </CardHeader>
          </Card>
        </motion.div>

        <motion.div variants={staggerItem} whileHover={{ scale: 1.03, y: -4 }} whileTap={{ scale: 0.98 }} className="h-full">
          <Card className="h-full">
            <CardHeader className="pb-2">
              <div className="flex items-center justify-between">
                <CardDescription className="flex items-center gap-1.5">
                  <Zap className="h-4 w-4" />
                  今日用量
                </CardDescription>
                {data.today.errorCount > 0 && (
                  <Badge variant="destructive" className="text-[10px]">
                    <Num value={data.today.errorCount} compact={false} /> 错误
                  </Badge>
                )}
              </div>
              <CardTitle className="text-2xl"><Num value={data.today.requestCount} compact={false} /></CardTitle>
            </CardHeader>
            <CardContent className="pb-3">
              <p className="text-xs text-muted-foreground flex items-center gap-1">
                <DollarSign className="h-3 w-3" />
                {formatUsd(todayCost, 4)}
              </p>
            </CardContent>
          </Card>
        </motion.div>

        <motion.div variants={staggerItem} whileHover={{ scale: 1.03, y: -4 }} whileTap={{ scale: 0.98 }} className="h-full">
          <Card className="h-full">
            <CardHeader className="pb-2">
              <CardDescription className="flex items-center gap-1.5">
                <TrendingUp className="h-4 w-4" />
                近 7 天
              </CardDescription>
              <CardTitle className="text-2xl"><Num value={data.week.requestCount} compact={false} /></CardTitle>
            </CardHeader>
            <CardContent className="pb-3">
              <p className="text-xs text-muted-foreground flex items-center gap-1">
                <DollarSign className="h-3 w-3" />
                {formatUsd(weekCost, 4)}
              </p>
            </CardContent>
          </Card>
        </motion.div>

        <motion.div variants={staggerItem} whileHover={{ scale: 1.03, y: -4 }} whileTap={{ scale: 0.98 }} className="h-full">
          <Card className="h-full bg-gradient-to-br from-green-500/10 to-emerald-500/10 border-green-500/20">
            <CardHeader className="pb-2">
              <CardDescription className="flex items-center gap-1.5">
                <Activity className="h-4 w-4" />
                近 30 天
              </CardDescription>
              <CardTitle className="text-2xl"><Num value={data.month.requestCount} compact={false} /></CardTitle>
            </CardHeader>
            <CardContent className="pb-3">
              <p className="text-xs text-green-600 dark:text-green-400 flex items-center gap-1 font-medium">
                <DollarSign className="h-3 w-3" />
                {formatUsd(parseFloat(data.month.costUsd), 4)}
              </p>
            </CardContent>
          </Card>
        </motion.div>
      </motion.div>

      {/* Token summary */}
      <motion.div
        variants={staggerContainer}
        initial="hidden"
        animate="visible"
        className="grid gap-4 md:grid-cols-4"
      >
        {[
          { label: '输入 Tokens (去缓存, 30天)', value: data.month.inputTokensSum, icon: ArrowUpRight, color: 'text-orange-500' },
          { label: '输出 Tokens (30天)', value: data.month.outputTokensSum, icon: ArrowDownRight, color: 'text-purple-500' },
          { label: '总请求 (30天)', value: data.month.requestCount, icon: Hash, color: 'text-blue-500' },
          { label: '错误数 (30天)', value: data.month.errorCount, icon: AlertTriangle, color: data.month.errorCount > 0 ? 'text-red-500' : 'text-muted-foreground' },
        ].map((item) => (
          <motion.div
            key={item.label}
            variants={staggerItem}
            whileHover={{ scale: 1.03, y: -4 }}
            whileTap={{ scale: 0.98 }}
            className="h-full"
          >
            <Card className="h-full">
              <CardHeader className="pb-2">
                <CardDescription className="flex items-center gap-1.5">
                  <item.icon className={`h-4 w-4 ${item.color}`} />
                  {item.label}
                </CardDescription>
                <CardTitle className="text-xl"><Num value={item.value} compact={false} /></CardTitle>
              </CardHeader>
            </Card>
          </motion.div>
        ))}
      </motion.div>

      {/* Billing Status + Daily Cost Trend */}
      <div className="grid gap-6 lg:grid-cols-2">
        <motion.div
          initial={{ opacity: 0, y: 30, scale: 0.95 }}
          animate={{ opacity: 1, y: 0, scale: 1 }}
          transition={{ type: 'spring', bounce: 0.25, duration: 0.7, delay: 0.15 }}
        >
          <Card>
            <CardHeader className="pb-2">
              <div className="flex items-center justify-between gap-3">
                <div className="min-w-0">
                  <CardTitle className="text-base flex items-center gap-2">
                    <CreditCard className="h-4 w-4" />
                    计费状态
                  </CardTitle>
                </div>
                {billingLoading ? (
                  <div className="h-9 w-40 rounded-md loading-shimmer" />
                ) : billingState ? (
                  <div className="flex flex-wrap items-center justify-end gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-9 rounded-md px-3"
                      onClick={() => navigateDashboard('purchase-center')}
                    >
                      <CreditCard className="mr-2 h-4 w-4" />
                      购买订阅
                    </Button>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <span className="inline-flex">
                          <Button
                            variant="outline"
                            size="sm"
                            className="h-9 rounded-md px-3"
                            onClick={() => void handleResetBillingDaily()}
                            disabled={!billingState.dailyReset.allowed || billingResetting}
                          >
                            <RotateCcw className={`mr-2 h-4 w-4 ${billingResetting ? 'animate-spin' : ''}`} />
                            重置今日计费
                          </Button>
                        </span>
                      </TooltipTrigger>
                      <TooltipContent side="bottom" className="space-y-1 border bg-popover px-3 py-2 text-xs text-popover-foreground shadow-md">
                        <p>{billingState.dailyReset.message}</p>
                        <p>今日已重置 {billingState.dailyReset.usedToday}/{billingState.dailyReset.dailyLimit}</p>
                      </TooltipContent>
                    </Tooltip>
                    <Popover open={priorityPopoverOpen} onOpenChange={setPriorityPopoverOpen}>
                      <PopoverTrigger asChild>
                        <Button variant="outline" size="sm" className="h-9 rounded-md px-3">
                          <ArrowRightLeft className="mr-2 h-4 w-4" />
                          {billingState.primarySource === 'subscription' ? '订阅优先' : '余额优先'}
                        </Button>
                      </PopoverTrigger>
                      <PopoverContent align="end" className="w-64 p-2">
                        <div className="space-y-1">
                          <p className="px-2 py-1 text-xs font-medium text-muted-foreground">切换扣费规则</p>
                          {([
                            { value: 'subscription' as const, label: '订阅优先', desc: '先用订阅额度，不足再扣余额' },
                            { value: 'balance' as const, label: '余额优先', desc: '先扣余额，不足再用订阅额度' },
                          ]).map((option) => {
                            const isActive = billingState.primarySource === option.value
                            return (
                              <motion.button
                                key={option.value}
                                onClick={() => handlePriorityChange(option.value)}
                                disabled={prioritySaving}
                                className={`flex w-full items-center gap-3 rounded-md px-2 py-2.5 text-left text-sm transition-colors ${
                                  isActive ? 'bg-primary/10 text-primary' : 'text-foreground hover:bg-accent'
                                }`}
                                whileHover={{ x: 2 }}
                                whileTap={{ scale: 0.98 }}
                                transition={{ type: 'spring', bounce: 0.3, duration: 0.3 }}
                              >
                                <div className="min-w-0 flex-1">
                                  <div className="font-medium">{option.label}</div>
                                  <div className="text-[11px] text-muted-foreground">{option.desc}</div>
                                </div>
                                <AnimatePresence mode="wait">
                                  {prioritySaving && billingState.primarySource !== option.value ? (
                                    <motion.div
                                      key="loader"
                                      initial={{ opacity: 0, scale: 0.5 }}
                                      animate={{ opacity: 1, scale: 1 }}
                                      exit={{ opacity: 0, scale: 0.5 }}
                                    >
                                      <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
                                    </motion.div>
                                  ) : isActive ? (
                                    <motion.div
                                      key="check"
                                      initial={{ opacity: 0, scale: 0.5 }}
                                      animate={{ opacity: 1, scale: 1 }}
                                      exit={{ opacity: 0, scale: 0.5 }}
                                      transition={{ type: 'spring', bounce: 0.5, duration: 0.4 }}
                                    >
                                      <Check className="h-4 w-4 text-primary" />
                                    </motion.div>
                                  ) : null}
                                </AnimatePresence>
                              </motion.button>
                            )
                          })}
                        </div>
                      </PopoverContent>
                    </Popover>
                  </div>
                ) : (
                  <Badge variant="outline">暂不可用</Badge>
                )}
              </div>
            </CardHeader>
            <CardContent className="space-y-2.5">
              {billingLoading ? (
                <div className="space-y-2.5">
                  <div className="grid gap-4 border-b pb-2.5 sm:grid-cols-[minmax(0,1fr)_200px]">
                    <div className="space-y-2">
                      <div className="h-3 w-16 rounded loading-shimmer" />
                      <div className="h-6 w-40 rounded loading-shimmer" />
                      <div className="h-4 w-28 rounded loading-shimmer" />
                    </div>
                    <div className="space-y-2 sm:justify-self-end sm:text-right">
                      <div className="h-3 w-12 rounded loading-shimmer sm:ml-auto" />
                      <div className="h-7 w-32 rounded loading-shimmer sm:ml-auto" />
                      <div className="h-3 w-24 rounded loading-shimmer sm:ml-auto" />
                    </div>
                  </div>
                  {[0, 1].map((item) => (
                    <div key={item} className="flex items-center justify-between gap-4 py-0.5">
                      <div className="min-w-0 flex-1 space-y-2">
                        <div className="h-4 w-24 rounded loading-shimmer" />
                        <div className="h-3 w-36 rounded loading-shimmer" />
                        <div className="h-3 w-44 rounded loading-shimmer" />
                      </div>
                      <div className="h-10 w-10 rounded-full loading-shimmer shrink-0" />
                    </div>
                  ))}
                </div>
              ) : billingState ? (
                <div className="space-y-2.5">
                  <div className="grid gap-4 border-b pb-2.5 sm:grid-cols-[minmax(0,1fr)_200px]">
                    <div className="space-y-2">
                      <div className="space-y-1.5">
                        <div className="flex flex-wrap items-center gap-2">
                          <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">订阅状态</p>
                          {billingState.subscription ? (
                            <Badge variant={billingState.subscription.status === 'active' ? 'default' : 'secondary'}>
                              {billingState.subscription.status === 'active' ? '生效中' : billingState.subscription.status}
                            </Badge>
                          ) : null}
                        </div>
                        <div className="flex items-center gap-2">
                          <Shield className="h-4 w-4 text-purple-500" />
                          <span className="text-lg font-semibold">
                            {billingState.subscription ? billingState.subscription.planName : '未订阅'}
                          </span>
                        </div>
                        <p className="text-sm text-muted-foreground">
                          {billingState.subscription?.expiresAt
                            ? `到期 ${formatDateTimeWithSeconds(billingState.subscription.expiresAt)}`
                            : billingState.subscription
                              ? '永不过期'
                              : '-'}
                        </p>
                      </div>
                    </div>
                    <div className="space-y-2 sm:justify-self-end sm:text-right">
                      <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">余额</p>
                      <div className="flex items-center gap-2 sm:justify-end">
                        <Wallet className="h-4 w-4 text-blue-500" />
                        <span className="text-2xl font-semibold">
                          ${formatGroupedNumericString(parseFloat(billingState.balanceUsd).toFixed(2))}
                        </span>
                      </div>
                      <div className="space-y-1">
                        <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">扣费顺序</p>
                        <p className="text-sm font-medium">
                          {billingState.primarySource === 'subscription' ? '订阅 → 余额' : '余额 → 订阅'}
                        </p>
                      </div>
                    </div>
                  </div>
                  {billingState.subscription && billingState.windows && billingState.windows.length > 0 ? (
                    <div className="space-y-2">
                      {billingState.windows.map((w, i) => {
                        const usedPct = w.limitMicros > 0 ? (w.usedMicros / w.limitMicros) * 100 : 0
                        return (
                          <div key={i} className="flex items-center justify-between gap-4 py-0.5">
                            <div className="min-w-0 space-y-1">
                              <div className="flex items-center gap-2">
                                <p className="text-sm font-medium">
                                  {LIMIT_TYPE_LABELS[w.limitType] || w.limitType}
                                </p>
                                <span className="text-[11px] text-muted-foreground">
                                  {WINDOW_MODE_LABELS[w.windowMode] || w.windowMode}
                                </span>
                              </div>
                              <p className="text-[11px] text-muted-foreground">
                                剩余 ${formatWindowUsd(w.leftMicros)} / 限额 ${formatWindowUsd(w.limitMicros)}
                              </p>
                              <div className="text-[11px] text-muted-foreground">
                                {getWindowMeta(w.windowMode, w.limitType, w.windowStart, w.windowEnd)}
                              </div>
                            </div>
                            <CircularProgress
                              value={usedPct}
                              size={40}
                              strokeWidth={4}
                              label={`${Math.round(Math.min(usedPct, 100))}%`}
                              className="shrink-0"
                              indicatorClassName="stroke-purple-500"
                              trackClassName="stroke-border"
                            />
                          </div>
                        )
                      })}
                    </div>
                  ) : (
                    <p className="text-sm text-muted-foreground">当前没有可展示的订阅额度。</p>
                  )}
                </div>
              ) : (
                <p className="text-sm text-muted-foreground">计费状态暂不可用。</p>
              )}
            </CardContent>
          </Card>
        </motion.div>

        <motion.div
          initial={{ opacity: 0, y: 30, scale: 0.97 }}
          animate={{ opacity: 1, y: 0, scale: 1 }}
          transition={{ type: 'spring', bounce: 0.25, duration: 0.7, delay: 0.22 }}
        >
          <Card className="h-full">
            <CardHeader className="pb-3">
              <CardTitle className="text-base flex items-center gap-2">
                <DollarSign className="h-4 w-4 text-emerald-500" />
                每日成本趋势
              </CardTitle>
              <CardDescription>近 14 天成本变化 (USD)</CardDescription>
            </CardHeader>
            <CardContent>
              {trendData.length === 0 ? (
                <p className="text-center text-muted-foreground py-12">暂无数据</p>
              ) : (
                <ChartContainer config={trendChartConfig} className="h-[216px] w-full">
                  <AreaChart accessibilityLayer data={trendData} margin={{ top: 8, right: 12, left: -4, bottom: 0 }}>
                    <defs>
                      <linearGradient id="fillCost" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="5%" stopColor="var(--color-cost)" stopOpacity={0.4} />
                        <stop offset="95%" stopColor="var(--color-cost)" stopOpacity={0.02} />
                      </linearGradient>
                    </defs>
                    <CartesianGrid vertical={false} />
                    <XAxis
                      dataKey="date"
                      tickLine={false}
                      axisLine={false}
                      tickMargin={8}
                      tickFormatter={v => v}
                    />
                    <YAxis
                      tickLine={false}
                      axisLine={false}
                      tickMargin={4}
                      tickFormatter={v => `$${v}`}
                    />
                    <ChartTooltip
                      cursor={false}
                      content={
                        <ChartTooltipContent
                          indicator="dot"
                          formatter={(value) => `$${Number(value).toFixed(4)}`}
                        />
                      }
                    />
                    <Area
                      type="monotoneX"
                      dataKey="cost"
                      stroke="var(--color-cost)"
                      strokeWidth={2}
                      fill="url(#fillCost)"
                      dot={{ r: 3, fill: 'var(--color-cost)', strokeWidth: 0 }}
                      activeDot={{ r: 5, strokeWidth: 2 }}
                    />
                  </AreaChart>
                </ChartContainer>
              )}
            </CardContent>
          </Card>
        </motion.div>
      </div>

      {/* Top Models + Daily Requests */}
      <div className="grid gap-6 lg:grid-cols-2">
        <motion.div
          initial={{ opacity: 0, y: 30, scale: 0.95 }}
          animate={{ opacity: 1, y: 0, scale: 1 }}
          transition={{ type: 'spring', bounce: 0.25, duration: 0.7, delay: 0.3 }}
        >
          <Card className="h-full">
            <CardHeader className="pb-3">
              <CardTitle className="text-base">热门模型排行（30天）</CardTitle>
              <CardDescription>按请求次数和成本排名</CardDescription>
            </CardHeader>
            <CardContent>
              {modelBarData.length === 0 ? (
                <p className="text-center text-muted-foreground py-12">暂无数据</p>
              ) : (
                <ChartContainer config={modelChartConfig} className="h-[216px] w-full">
                  <BarChart
                    accessibilityLayer
                    data={modelBarData}
                    layout="vertical"
                    margin={{ top: 4, right: 16, left: 4, bottom: 0 }}
                  >
                    <CartesianGrid horizontal={false} />
                    <XAxis type="number" tickLine={false} axisLine={false} tickMargin={4} />
                    <YAxis
                      type="category"
                      dataKey="name"
                      tickLine={false}
                      axisLine={false}
                      width={120}
                      tick={{ fontSize: 11 }}
                    />
                    <ChartTooltip
                      cursor={false}
                      content={
                        <ChartTooltipContent
                          indicator="dot"
                          formatter={(value, _name, item) => {
                            const d = item.payload as { fullName: string; cost: number }
                            return (
                              <div className="space-y-0.5">
                                <div className="font-mono text-xs">{d.fullName}</div>
                                <div>请求: {Number(value).toLocaleString()}</div>
                                <div>成本: <span className="text-green-600 dark:text-green-400">${d.cost.toFixed(4)}</span></div>
                              </div>
                            )
                          }}
                        />
                      }
                    />
                    <Bar
                      dataKey="requests"
                      radius={[0, 6, 6, 0]}
                      maxBarSize={28}
                    >
                      {modelBarData.map((_entry, i) => (
                        <Cell key={i} fill={MODEL_COLORS[i % MODEL_COLORS.length]} />
                      ))}
                    </Bar>
                  </BarChart>
                </ChartContainer>
              )}
            </CardContent>
          </Card>
        </motion.div>

        <motion.div
          initial={{ opacity: 0, y: 30, scale: 0.97 }}
          animate={{ opacity: 1, y: 0, scale: 1 }}
          transition={{ type: 'spring', bounce: 0.25, duration: 0.7, delay: 0.38 }}
        >
          <Card className="h-full">
            <CardHeader className="pb-3">
              <CardTitle className="text-base flex items-center gap-2">
                <Activity className="h-4 w-4 text-blue-500" />
                每日请求量
              </CardTitle>
              <CardDescription>近 14 天请求数变化</CardDescription>
            </CardHeader>
            <CardContent>
              {trendData.length === 0 ? (
                <p className="text-center text-muted-foreground py-12">暂无数据</p>
              ) : (
                <ChartContainer config={trendChartConfig} className="h-[216px] w-full">
                  <BarChart accessibilityLayer data={trendData} margin={{ top: 8, right: 12, left: -4, bottom: 0 }}>
                    <defs>
                      <linearGradient id="fillRequests" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="0%" stopColor="var(--color-requests)" stopOpacity={0.9} />
                        <stop offset="100%" stopColor="var(--color-requests)" stopOpacity={0.4} />
                      </linearGradient>
                    </defs>
                    <CartesianGrid vertical={false} />
                    <XAxis
                      dataKey="date"
                      tickLine={false}
                      axisLine={false}
                      tickMargin={8}
                    />
                    <YAxis
                      tickLine={false}
                      axisLine={false}
                      tickMargin={4}
                    />
                    <ChartTooltip
                      cursor={false}
                      content={<ChartTooltipContent indicator="dot" />}
                    />
                    <Bar
                      dataKey="requests"
                      fill="url(#fillRequests)"
                      radius={[6, 6, 0, 0]}
                      maxBarSize={36}
                    />
                  </BarChart>
                </ChartContainer>
              )}
            </CardContent>
          </Card>
        </motion.div>
      </div>

      {/* Cache Hit Rates */}
      <motion.div
        initial={{ opacity: 0, y: 30, scale: 0.97 }}
        animate={{ opacity: 1, y: 0, scale: 1 }}
        transition={{ type: 'spring', bounce: 0.25, duration: 0.7, delay: 0.45 }}
      >
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <DatabaseZap className="h-4 w-4" />
              缓存命中率（30天）
            </CardTitle>
            <CardDescription>按模型提供商分类的缓存使用情况</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="grid gap-4 md:grid-cols-3">
              {(['Claude', 'OpenAI', 'Gemini'] as const).map((providerName, providerIdx) => {
                const rate = (data.cacheHitRates || []).find((r: DashboardCacheHitRate) => r.provider === providerName)
                const hitRate = rate ? parseFloat(rate.hitRate) : 0
                const hasData = !!rate

                const providerColors: Record<string, { bg: string; text: string; border: string; ring: string }> = {
                  'Claude': {
                    bg: 'from-orange-500/10 to-amber-500/10',
                    text: 'text-orange-600 dark:text-orange-400',
                    border: 'border-orange-500/20',
                    ring: 'hsl(var(--chart-5))',
                  },
                  'OpenAI': {
                    bg: 'from-emerald-500/10 to-green-500/10',
                    text: 'text-emerald-600 dark:text-emerald-400',
                    border: 'border-emerald-500/20',
                    ring: 'hsl(var(--chart-2))',
                  },
                  'Gemini': {
                    bg: 'from-blue-500/10 to-indigo-500/10',
                    text: 'text-blue-600 dark:text-blue-400',
                    border: 'border-blue-500/20',
                    ring: 'hsl(var(--chart-1))',
                  },
                }

                const colors = providerColors[providerName]

                const cacheChartConfig = {
                  hit: { label: '命中', color: colors.ring },
                  miss: { label: '未命中', color: 'hsl(var(--muted))' },
                } satisfies ChartConfig

                const ringData = hasData
                  ? [
                      { name: 'hit', value: hitRate, fill: colors.ring },
                      { name: 'miss', value: Math.max(100 - hitRate, 0), fill: 'hsl(var(--muted))' },
                    ]
                  : []

                return (
                  <motion.div
                    key={providerName}
                    initial={{ opacity: 0, scale: 0.9, y: 20 }}
                    animate={{ opacity: 1, scale: 1, y: 0 }}
                    transition={{ type: 'spring', bounce: 0.3, duration: 0.6, delay: 0.5 + providerIdx * 0.08 }}
                    whileHover={{ scale: 1.02, y: -3 }}
                  >
                    <Card className={`bg-gradient-to-br ${colors.bg} ${colors.border}`}>
                      <CardHeader className="pb-2">
                        <div className="flex items-center justify-between">
                          <CardTitle className={`text-sm font-semibold ${colors.text}`}>
                            {providerName}
                          </CardTitle>
                          {hasData && (
                            <Badge variant="outline" className="text-[10px] font-mono">
                              <Num value={rate!.requestCount} compact={false} /> 请求
                            </Badge>
                          )}
                        </div>
                      </CardHeader>
                      <CardContent className="pb-4">
                        <div className="flex min-h-[112px] items-center gap-4">
                          {hasData ? (
                            <>
                              <ChartContainer config={cacheChartConfig} className="w-20 h-20 shrink-0">
                              <PieChart>
                                <Pie
                                  data={ringData}
                                  dataKey="value"
                                  nameKey="name"
                                  innerRadius={22}
                                  outerRadius={36}
                                  startAngle={90}
                                  endAngle={-270}
                                  strokeWidth={0}
                                  paddingAngle={2}
                                >
                                  {ringData.map((entry, i) => (
                                    <Cell key={i} fill={entry.fill} />
                                  ))}
                                  <Label
                                    content={({ viewBox }) => {
                                      if (viewBox && 'cx' in viewBox && 'cy' in viewBox) {
                                        return (
                                          <text x={viewBox.cx} y={viewBox.cy} textAnchor="middle" dominantBaseline="middle">
                                            <tspan className={`text-sm font-bold fill-foreground`}>
                                              {rate!.hitRate}%
                                            </tspan>
                                          </text>
                                        )
                                      }
                                    }}
                                  />
                                </Pie>
                              </PieChart>
                              </ChartContainer>
                              <div className="flex-1 space-y-2">
                              <div className="flex items-baseline gap-1.5">
                                <span className={`text-2xl font-bold ${colors.text}`}>
                                  {rate!.hitRate}%
                                </span>
                                <span className="text-[10px] text-muted-foreground">命中率</span>
                              </div>
                              <div className="grid grid-cols-2 gap-x-3 gap-y-0.5 text-[11px] text-muted-foreground">
                                <span>缓存读取</span>
                                <span className="text-right font-mono"><Num value={rate!.cacheReadTokens} compact={false} /></span>
                                <span>缓存写入</span>
                                <span className="text-right font-mono"><Num value={rate!.cacheCreationTokens} compact={false} /></span>
                                <span>总输入 Tokens</span>
                                <span className="text-right font-mono"><Num value={rate!.totalInputTokens} compact={false} /></span>
                              </div>
                              </div>
                            </>
                          ) : (
                            <>
                              <div className="flex h-20 w-20 shrink-0 items-center justify-center rounded-full border border-dashed border-border/80 text-xs text-muted-foreground">
                                --
                              </div>
                              <div className="flex-1 space-y-2">
                                <div className="flex items-baseline gap-1.5">
                                  <span className="text-2xl font-bold text-muted-foreground">--</span>
                                  <span className="text-[10px] text-muted-foreground">命中率</span>
                                </div>
                                <div className="grid grid-cols-2 gap-x-3 gap-y-0.5 text-[11px] text-muted-foreground">
                                  <span>缓存读取</span>
                                  <span className="text-right font-mono">-</span>
                                  <span>缓存写入</span>
                                  <span className="text-right font-mono">-</span>
                                  <span>总输入 Tokens</span>
                                  <span className="text-right font-mono">-</span>
                                </div>
                              </div>
                            </>
                          )}
                        </div>
                      </CardContent>
                    </Card>
                  </motion.div>
                )
              })}
            </div>
          </CardContent>
        </Card>
      </motion.div>
    </motion.div>
  )
}
