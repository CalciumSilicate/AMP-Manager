import { authFetch } from './client'

const API_BASE = '/api'

export interface DashboardPeriodStats {
  requestCount: number
  inputTokensSum: number
  outputTokensSum: number
  costMicros: number
  costUsd: string
  errorCount: number
}

export interface DashboardTopModel {
  model: string
  requestCount: number
  costMicros: number
  costUsd: string
}

export interface DashboardDailyTrend {
  date: string
  costMicros: number
  costUsd: string
  requests: number
}

export interface AdminDashboardThroughputPoint {
  minute: string
  qps1m: number
  rpm5m: number
  tpm5m: number
  concurrency1m: number
}

export interface AdminDashboardTimingPoint {
  bucket: string
  avgMs: number
  p50Ms: number
  p90Ms: number
  p99Ms: number
  sampleCnt: number
}

export interface DashboardCacheHitRate {
  provider: string
  totalInputTokens: number
  cacheReadTokens: number
  cacheCreationTokens: number
  requestCount: number
  hitRate: string
}

export interface DashboardData {
  balance: {
    balanceMicros: number
    balanceUsd: string
    currentConcurrency: number
    concurrencyLimit: number
  }
  today: DashboardPeriodStats
  week: DashboardPeriodStats
  month: DashboardPeriodStats
  topModels: DashboardTopModel[]
  dailyTrend: DashboardDailyTrend[]
  cacheHitRates: DashboardCacheHitRate[]
}

export async function getDashboard(): Promise<DashboardData> {
  const res = await authFetch(`${API_BASE}/me/dashboard`)
  if (!res.ok) throw new Error('获取仪表盘数据失败')
  return res.json()
}

export interface AdminDashboardData {
  balance: {
    totalBalanceMicros: number
    totalBalanceUsd: string
    userCount: number
    currentConcurrency: number
  }
  today: DashboardPeriodStats
  week: DashboardPeriodStats
  month: DashboardPeriodStats
  topModels: DashboardTopModel[]
  dailyTrend: DashboardDailyTrend[]
  throughputWindow: string
  throughputTrend: AdminDashboardThroughputPoint[]
  ttfbTrend: AdminDashboardTimingPoint[]
  durationTrend: AdminDashboardTimingPoint[]
  cacheHitRates: DashboardCacheHitRate[]
}

export interface AdminDashboardSummaryData {
  balance: {
    totalBalanceMicros: number
    totalBalanceUsd: string
    userCount: number
    currentConcurrency: number
  }
  today: DashboardPeriodStats
  week: DashboardPeriodStats
  month: DashboardPeriodStats
  topModels: DashboardTopModel[]
  dailyTrend: DashboardDailyTrend[]
}

export interface AdminDashboardTrendsData {
  throughputWindow: string
  throughputTrend: AdminDashboardThroughputPoint[]
  ttfbTrend: AdminDashboardTimingPoint[]
  durationTrend: AdminDashboardTimingPoint[]
}

const inflightRequests = new Map<string, Promise<unknown>>()

async function dedupedJSONRequest<T>(key: string, url: string): Promise<T> {
  const existing = inflightRequests.get(key) as Promise<T> | undefined
  if (existing) {
    return existing
  }
  const request = (async () => {
    const res = await authFetch(url)
    if (!res.ok) throw new Error('获取管理员仪表盘数据失败')
    return res.json() as Promise<T>
  })()
  inflightRequests.set(key, request)
  try {
    return await request
  } finally {
    inflightRequests.delete(key)
  }
}

export async function getAdminDashboardSummary(): Promise<AdminDashboardSummaryData> {
  return dedupedJSONRequest<AdminDashboardSummaryData>('admin-dashboard-summary', `${API_BASE}/admin/dashboard/summary`)
}

export async function getAdminDashboardTrends(throughputWindow = '1h'): Promise<AdminDashboardTrendsData> {
  return dedupedJSONRequest<AdminDashboardTrendsData>(
    `admin-dashboard-trends:${throughputWindow}`,
    `${API_BASE}/admin/dashboard/trends?throughputWindow=${encodeURIComponent(throughputWindow)}`,
  )
}

export async function getAdminDashboardCacheHit(): Promise<{ cacheHitRates: DashboardCacheHitRate[] }> {
  return dedupedJSONRequest<{ cacheHitRates: DashboardCacheHitRate[] }>('admin-dashboard-cache-hit', `${API_BASE}/admin/dashboard/cache-hit`)
}

export async function getAdminDashboard(throughputWindow = '1h'): Promise<AdminDashboardData> {
  const [summary, trends, cacheHit] = await Promise.all([
    getAdminDashboardSummary(),
    getAdminDashboardTrends(throughputWindow),
    getAdminDashboardCacheHit(),
  ])

  return {
    ...summary,
    ...trends,
    cacheHitRates: cacheHit.cacheHitRates,
  }
}
