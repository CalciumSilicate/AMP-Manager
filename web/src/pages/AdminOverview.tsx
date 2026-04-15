import { useState, useEffect, useMemo } from 'react'
import { getAdminDashboard, AdminDashboardData, DashboardCacheHitRate } from '@/api/dashboard'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Num } from '@/components/Num'
import { formatDecimal, formatGroupedNumericString } from '@/lib/formatters'
import { motion, staggerContainer, staggerItem } from '@/lib/motion'
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
  DatabaseZap, Users,
} from 'lucide-react'

const trendChartConfig = {
  cost: { label: '成本 (USD)', color: 'hsl(var(--chart-2))' },
  requests: { label: '请求数', color: 'hsl(var(--chart-1))' },
} satisfies ChartConfig

const throughputChartConfig = {
  qps1m: { label: '1 分钟平均 QPS', color: 'hsl(var(--chart-1))' },
  rpm5m: { label: '5 分钟平均 RPM', color: 'hsl(var(--chart-2))' },
  tpm5m: { label: '5 分钟平均 TPM', color: 'hsl(var(--chart-3))' },
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

export default function AdminOverview() {
  const [data, setData] = useState<AdminDashboardData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [trendView, setTrendView] = useState<'cost' | 'requests'>('cost')
  const [throughputView, setThroughputView] = useState<'qps1m' | 'rpm5m' | 'tpm5m'>('qps1m')

  const loadDashboard = async () => {
    setLoading(true)
    setError('')
    try {
      const result = await getAdminDashboard()
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

  const throughputData = useMemo(() => {
    if (!data?.throughputTrend?.length) return []
    return data.throughputTrend.map((point) => {
      const minute = new Date(point.minute)
      const label = Number.isNaN(minute.getTime())
        ? point.minute
        : minute.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
      const tooltipLabel = Number.isNaN(minute.getTime())
        ? point.minute
        : minute.toLocaleString([], {
            month: '2-digit',
            day: '2-digit',
            hour: '2-digit',
            minute: '2-digit',
          })

      return {
        ...point,
        label,
        tooltipLabel,
      }
    })
  }, [data?.throughputTrend])

  const modelPieData = useMemo(() => {
    if (!data?.topModels?.length) return []
    return data.topModels.map((m, i) => {
      const key = `model${i}`
      return {
        key,
        name: m.model.length > 24 ? m.model.slice(0, 22) + '…' : m.model,
        fullName: m.model,
        value: m.requestCount,
        cost: parseFloat(m.costUsd),
        fill: `var(--color-${key})`,
      }
    })
  }, [data?.topModels])

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

  const totalModelRequests = useMemo(() => {
    return modelPieData.reduce((sum, m) => sum + m.value, 0)
  }, [modelPieData])

  const modelPieConfig = useMemo(() => {
    const config: ChartConfig = { value: { label: '请求数' } }
    modelPieData.forEach((m, i) => {
      config[m.key] = {
        label: m.name,
        color: MODEL_COLORS[i % MODEL_COLORS.length],
      }
    })
    return config
  }, [modelPieData])

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

  const totalBalanceUsd = parseFloat(data.balance.totalBalanceUsd)
  const todayCost = parseFloat(data.today.costUsd)
  const weekCost = parseFloat(data.week.costUsd)
  const formatUsd = (value: number, digits: number) => `$${formatDecimal(value, digits)}`
  const trendViewMeta = {
    cost: {
      description: '近 14 天全局成本变化 (USD)',
      color: 'var(--color-cost)',
      gradientId: 'adminFillCost',
    },
    requests: {
      description: '近 14 天全局请求数变化',
      color: 'var(--color-requests)',
      gradientId: 'adminFillRequests',
    },
  } as const
  const throughputViewMeta = {
    qps1m: {
      label: '1 分钟平均 QPS',
      description: '近 24 小时，每分钟请求数 / 60',
      digits: 2,
      unit: 'req/s',
    },
    rpm5m: {
      label: '5 分钟平均 RPM',
      description: '近 24 小时，5 分钟窗口平均每分钟请求',
      digits: 2,
      unit: 'req/min',
    },
    tpm5m: {
      label: '5 分钟平均 TPM',
      description: '近 24 小时，5 分钟窗口平均每分钟 Tokens',
      digits: 1,
      unit: 'tok/min',
    },
  } as const

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
          <h2 className="text-2xl font-bold tracking-tight">管理员概览</h2>
          <p className="text-muted-foreground">系统全局用量和费用汇总</p>
        </div>
        <Button variant="outline" size="sm" onClick={loadDashboard} disabled={loading}>
          <RefreshCw className={`h-4 w-4 mr-2 ${loading ? 'animate-spin' : ''}`} />
          刷新
        </Button>
      </motion.div>

      {/* Balance + UserCount + Today/Week/Month */}
      <motion.div
        variants={staggerContainer}
        initial="hidden"
        animate="visible"
        className="grid gap-4 md:grid-cols-2 lg:grid-cols-5"
      >
        <motion.div variants={staggerItem} whileHover={{ scale: 1.03, y: -4 }} whileTap={{ scale: 0.98 }} className="h-full">
          <Card className="h-full bg-gradient-to-br from-blue-500/10 to-cyan-500/10 border-blue-500/20">
            <CardHeader className="pb-2">
              <CardDescription className="flex items-center gap-1.5">
                <Wallet className="h-4 w-4" />
                全部用户总余额
              </CardDescription>
              <CardTitle className="text-3xl text-blue-600 dark:text-blue-400">
                {formatUsd(totalBalanceUsd, 2)}
              </CardTitle>
            </CardHeader>
          </Card>
        </motion.div>

        <motion.div variants={staggerItem} whileHover={{ scale: 1.03, y: -4 }} whileTap={{ scale: 0.98 }} className="h-full">
          <Card className="h-full bg-gradient-to-br from-purple-500/10 to-violet-500/10 border-purple-500/20">
            <CardHeader className="pb-2">
              <CardDescription className="flex items-center gap-1.5">
                <Users className="h-4 w-4" />
                注册用户数
              </CardDescription>
              <CardTitle className="text-3xl text-purple-600 dark:text-purple-400">
                <Num value={data.balance.userCount} compact={false} />
              </CardTitle>
            </CardHeader>
            <CardContent className="pb-3">
              <p className="text-xs text-muted-foreground">
                人均余额 ${formatGroupedNumericString((data.balance.userCount > 0 ? (totalBalanceUsd / data.balance.userCount) : 0).toFixed(2))}
              </p>
            </CardContent>
          </Card>
        </motion.div>

        <motion.div variants={staggerItem} whileHover={{ scale: 1.03, y: -4 }} whileTap={{ scale: 0.98 }} className="h-full">
          <Card className="h-full">
            <CardHeader className="pb-2">
              <div className="flex items-center justify-between">
                <CardDescription className="flex items-center gap-1.5">
                  <Zap className="h-4 w-4" />
                  今日全局
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
          { label: '输入 Tokens (去缓存, 全局30天)', value: data.month.inputTokensSum, icon: ArrowUpRight, color: 'text-orange-500' },
          { label: '输出 Tokens (全局30天)', value: data.month.outputTokensSum, icon: ArrowDownRight, color: 'text-purple-500' },
          { label: '总请求 (全局30天)', value: data.month.requestCount, icon: Hash, color: 'text-blue-500' },
          { label: '错误数 (全局30天)', value: data.month.errorCount, icon: AlertTriangle, color: data.month.errorCount > 0 ? 'text-red-500' : 'text-muted-foreground' },
        ].map((item) => (
          <motion.div key={item.label} variants={staggerItem} whileHover={{ scale: 1.03, y: -4 }}>
            <Card>
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

      {/* Trend + Throughput */}
      <div className="grid gap-6 lg:grid-cols-2">
        <motion.div
          initial={{ opacity: 0, y: 30, scale: 0.97 }}
          animate={{ opacity: 1, y: 0, scale: 1 }}
          transition={{ type: 'spring', bounce: 0.25, duration: 0.7, delay: 0.2 }}
        >
          <Card>
            <CardHeader>
              <div className="flex items-start justify-between gap-3">
                <div>
                  <CardTitle className="text-base flex items-center gap-2">
                    <TrendingUp className="h-4 w-4 text-emerald-500" />
                    每日趋势（全局）
                  </CardTitle>
                  <CardDescription>{trendViewMeta[trendView].description}</CardDescription>
                </div>
                <div className="flex items-center gap-1">
                  <Button size="sm" variant={trendView === 'cost' ? 'default' : 'outline'} onClick={() => setTrendView('cost')}>
                    成本
                  </Button>
                  <Button size="sm" variant={trendView === 'requests' ? 'default' : 'outline'} onClick={() => setTrendView('requests')}>
                    请求量
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent>
              {trendData.length === 0 ? (
                <p className="text-center text-muted-foreground py-12">暂无数据</p>
              ) : trendView === 'cost' ? (
                <ChartContainer config={trendChartConfig} className="h-[240px] w-full">
                  <AreaChart accessibilityLayer data={trendData} margin={{ top: 8, right: 12, left: -4, bottom: 0 }}>
                    <defs>
                      <linearGradient id="adminFillCost" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="5%" stopColor="var(--color-cost)" stopOpacity={0.4} />
                        <stop offset="95%" stopColor="var(--color-cost)" stopOpacity={0.02} />
                      </linearGradient>
                    </defs>
                    <CartesianGrid vertical={false} />
                    <XAxis dataKey="date" tickLine={false} axisLine={false} tickMargin={8} />
                    <YAxis tickLine={false} axisLine={false} tickMargin={4} tickFormatter={v => `$${v}`} />
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
                      fill="url(#adminFillCost)"
                      dot={{ r: 3, fill: 'var(--color-cost)', strokeWidth: 0 }}
                      activeDot={{ r: 5, strokeWidth: 2 }}
                    />
                  </AreaChart>
                </ChartContainer>
              ) : (
                <ChartContainer config={trendChartConfig} className="h-[240px] w-full">
                  <BarChart accessibilityLayer data={trendData} margin={{ top: 8, right: 12, left: -4, bottom: 0 }}>
                    <defs>
                      <linearGradient id="adminFillRequests" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="0%" stopColor="var(--color-requests)" stopOpacity={0.9} />
                        <stop offset="100%" stopColor="var(--color-requests)" stopOpacity={0.4} />
                      </linearGradient>
                    </defs>
                    <CartesianGrid vertical={false} />
                    <XAxis dataKey="date" tickLine={false} axisLine={false} tickMargin={8} />
                    <YAxis tickLine={false} axisLine={false} tickMargin={4} />
                    <ChartTooltip cursor={false} content={<ChartTooltipContent indicator="dot" />} />
                    <Bar dataKey="requests" fill="url(#adminFillRequests)" radius={[6, 6, 0, 0]} maxBarSize={36} />
                  </BarChart>
                </ChartContainer>
              )}
            </CardContent>
          </Card>
        </motion.div>

        <motion.div
          initial={{ opacity: 0, y: 30, scale: 0.97 }}
          animate={{ opacity: 1, y: 0, scale: 1 }}
          transition={{ type: 'spring', bounce: 0.25, duration: 0.7, delay: 0.28 }}
        >
          <Card>
            <CardHeader>
              <div className="flex items-start justify-between gap-3">
                <div>
                  <CardTitle className="text-base flex items-center gap-2">
                    <Activity className="h-4 w-4 text-blue-500" />
                    实时吞吐趋势（全局）
                  </CardTitle>
                  <CardDescription>{throughputViewMeta[throughputView].description}</CardDescription>
                </div>
                <div className="flex items-center gap-1">
                  <Button size="sm" variant={throughputView === 'qps1m' ? 'default' : 'outline'} onClick={() => setThroughputView('qps1m')}>
                    1m QPS
                  </Button>
                  <Button size="sm" variant={throughputView === 'rpm5m' ? 'default' : 'outline'} onClick={() => setThroughputView('rpm5m')}>
                    5m RPM
                  </Button>
                  <Button size="sm" variant={throughputView === 'tpm5m' ? 'default' : 'outline'} onClick={() => setThroughputView('tpm5m')}>
                    5m TPM
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent>
              {throughputData.length === 0 ? (
                <p className="text-center text-muted-foreground py-12">暂无数据</p>
              ) : (
                <ChartContainer config={throughputChartConfig} className="h-[240px] w-full">
                  <AreaChart accessibilityLayer data={throughputData} margin={{ top: 8, right: 12, left: -4, bottom: 0 }}>
                    <defs>
                      <linearGradient id="adminFillThroughput" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="5%" stopColor={`var(--color-${throughputView})`} stopOpacity={0.35} />
                        <stop offset="95%" stopColor={`var(--color-${throughputView})`} stopOpacity={0.02} />
                      </linearGradient>
                    </defs>
                    <CartesianGrid vertical={false} />
                    <XAxis dataKey="label" tickLine={false} axisLine={false} tickMargin={8} minTickGap={28} />
                    <YAxis tickLine={false} axisLine={false} tickMargin={4} tickFormatter={(value) => formatDecimal(Number(value), throughputViewMeta[throughputView].digits)} />
                    <ChartTooltip
                      cursor={false}
                      content={
                        <ChartTooltipContent
                          indicator="dot"
                          labelFormatter={(_value, payload) => {
                            const item = payload?.[0]?.payload as { tooltipLabel?: string } | undefined
                            return item?.tooltipLabel || ''
                          }}
                          formatter={(value) => `${formatDecimal(Number(value), throughputViewMeta[throughputView].digits)} ${throughputViewMeta[throughputView].unit}`}
                        />
                      }
                    />
                    <Area
                      type="monotoneX"
                      dataKey={throughputView}
                      stroke={`var(--color-${throughputView})`}
                      strokeWidth={2}
                      fill="url(#adminFillThroughput)"
                      dot={false}
                      activeDot={{ r: 4, strokeWidth: 2 }}
                    />
                  </AreaChart>
                </ChartContainer>
              )}
            </CardContent>
          </Card>
        </motion.div>
      </div>

      {/* Model Donut Pie + Horizontal Bar */}
      <div className="grid gap-6 lg:grid-cols-2">
        <motion.div
          initial={{ opacity: 0, y: 30, scale: 0.95 }}
          animate={{ opacity: 1, y: 0, scale: 1 }}
          transition={{ type: 'spring', bounce: 0.25, duration: 0.7, delay: 0.35 }}
        >
          <Card className="h-full">
            <CardHeader className="pb-2">
              <CardTitle className="text-base">模型请求分布（全局）</CardTitle>
              <CardDescription>30 天内全局各模型请求占比</CardDescription>
            </CardHeader>
            <CardContent>
              {modelPieData.length === 0 ? (
                <p className="text-center text-muted-foreground py-12">暂无数据</p>
              ) : (
                <div className="flex items-center gap-6">
                  <ChartContainer config={modelPieConfig} className="w-[180px] h-[180px] shrink-0">
                    <PieChart>
                      <ChartTooltip
                        cursor={false}
                        content={
                          <ChartTooltipContent
                            nameKey="key"
                            hideLabel
                            formatter={(value, _name, item) => {
                              const d = item.payload as { fullName: string; value: number; cost: number }
                              const pct = totalModelRequests > 0 ? ((d.value / totalModelRequests) * 100).toFixed(1) : '0'
                              return (
                                <div className="space-y-0.5">
                                  <div className="font-medium text-xs">{d.fullName}</div>
                                  <div>请求: {Number(value).toLocaleString()} ({pct}%)</div>
                                  <div>成本: <span className="text-green-600 dark:text-green-400">${d.cost.toFixed(4)}</span></div>
                                </div>
                              )
                            }}
                          />
                        }
                      />
                      <Pie
                        data={modelPieData}
                        dataKey="value"
                        nameKey="key"
                        innerRadius={50}
                        outerRadius={80}
                        paddingAngle={3}
                        strokeWidth={2}
                        stroke="hsl(var(--background))"
                      >
                        <Label
                          content={({ viewBox }) => {
                            if (viewBox && 'cx' in viewBox && 'cy' in viewBox) {
                              return (
                                <text x={viewBox.cx} y={viewBox.cy} textAnchor="middle" dominantBaseline="middle">
                                  <tspan x={viewBox.cx} y={viewBox.cy} className="fill-foreground text-2xl font-bold">
                                    {totalModelRequests.toLocaleString()}
                                  </tspan>
                                  <tspan x={viewBox.cx} y={(viewBox.cy || 0) + 18} className="fill-muted-foreground text-[10px]">
                                    总请求
                                  </tspan>
                                </text>
                              )
                            }
                          }}
                        />
                      </Pie>
                    </PieChart>
                  </ChartContainer>
                  <div className="flex-1 space-y-2 min-w-0">
                    {modelPieData.map((m, i) => {
                      const pct = totalModelRequests > 0 ? ((m.value / totalModelRequests) * 100).toFixed(1) : '0'
                      return (
                        <div key={m.key} className="flex items-center gap-2 text-sm" title={m.fullName}>
                          <div
                            className="w-2.5 h-2.5 rounded-full shrink-0"
                            style={{ backgroundColor: MODEL_COLORS[i % MODEL_COLORS.length] }}
                          />
                          <span className="truncate min-w-0 flex-1 text-muted-foreground text-xs">
                            {m.fullName}
                          </span>
                          <span className="font-mono text-xs tabular-nums shrink-0">
                            {pct}%
                          </span>
                          <span className="font-mono text-xs tabular-nums text-muted-foreground shrink-0">
                            {m.value.toLocaleString()}
                          </span>
                        </div>
                      )
                    })}
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        </motion.div>

        <motion.div
          initial={{ opacity: 0, y: 30, scale: 0.95 }}
          animate={{ opacity: 1, y: 0, scale: 1 }}
          transition={{ type: 'spring', bounce: 0.25, duration: 0.7, delay: 0.4 }}
        >
          <Card className="h-full">
            <CardHeader>
              <CardTitle className="text-base">热门模型排行（全局30天）</CardTitle>
              <CardDescription>按请求次数和成本排名 Top 10</CardDescription>
            </CardHeader>
            <CardContent>
              {modelBarData.length === 0 ? (
                <p className="text-center text-muted-foreground py-12">暂无数据</p>
              ) : (
                <ChartContainer config={modelChartConfig} className="h-[300px] w-full">
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
                    <Bar dataKey="requests" radius={[0, 6, 6, 0]} maxBarSize={24}>
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
              缓存命中率（全局30天）
            </CardTitle>
            <CardDescription>按模型提供商分类的全局缓存使用情况</CardDescription>
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
                                            <tspan className="text-sm font-bold fill-foreground">
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
