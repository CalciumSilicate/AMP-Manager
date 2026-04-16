import type { LimitType, WindowMode, SubscriptionStatus } from './subscription'
import { authFetch } from './client'

const API_BASE = '/api'

// 统一的 fetch + JSON 解析 helper
async function fetchJson<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await authFetch(url, options)
  
  let data: unknown
  let text = ''
  
  try {
    data = await res.json()
  } catch {
    try {
      text = await res.text()
    } catch {
      text = ''
    }
  }
  
  if (!res.ok) {
    const errorObj = data as { error?: string; message?: string } | undefined
    const errorMessage = errorObj?.error || errorObj?.message || 
      `请求失败 (${res.status})${text ? ': ' + text.slice(0, 100) : ''}`
    throw new Error(errorMessage)
  }
  
  return data as T
}

export interface ModelPrice {
  model: string
  provider?: string | null
  source: string
  inputCostPerToken: number
  outputCostPerToken: number
  cacheReadInputPerToken: number
  cacheCreationPerToken: number
  updatedAt: string
}

export interface ModelPriceContextRule {
  id: string
  model: string
  ruleName: string
  minTokens: number
  maxTokens?: number | null
  inputCostPerToken: number
  outputCostPerToken: number
  cacheReadInputPerToken: number
  cacheCreationPerToken: number
  sortOrder: number
  createdAt: string
  updatedAt: string
}

export interface ModelPriceContextRuleRequest {
  ruleName: string
  minTokens: number
  maxTokens?: number | null
  inputCostPerToken: number
  outputCostPerToken: number
  cacheReadInputPerToken: number
  cacheCreationPerToken: number
  sortOrder: number
}

export interface PriceListResponse {
  items: ModelPrice[]
  total: number
}

export interface PriceStats {
  modelCount: number
  source: string
  fetchedAt: string
}

// 获取价格列表
export async function listPrices(): Promise<PriceListResponse> {
  return fetchJson<PriceListResponse>(`${API_BASE}/admin/prices`)
}

// 获取价格服务状态
export async function getPriceStats(): Promise<PriceStats> {
  return fetchJson<PriceStats>(`${API_BASE}/admin/prices/stats`)
}

// 手动刷新价格
export async function refreshPrices(): Promise<{ message: string; modelCount: number; fetchedAt: string }> {
  return fetchJson(`${API_BASE}/admin/prices/refresh`, {
    method: 'POST',
  })
}

export async function listPriceContextRules(model: string): Promise<{ items: ModelPriceContextRule[] }> {
  return fetchJson<{ items: ModelPriceContextRule[] }>(`${API_BASE}/admin/prices/context-rules?model=${encodeURIComponent(model)}`)
}

export async function updatePriceContextRules(model: string, rules: ModelPriceContextRuleRequest[]): Promise<{ items: ModelPriceContextRule[] }> {
  return fetchJson<{ items: ModelPriceContextRule[] }>(`${API_BASE}/admin/prices/context-rules?model=${encodeURIComponent(model)}`, {
    method: 'PUT',
    body: JSON.stringify({ rules }),
  })
}

// --- User Billing API Types ---

export interface WindowRemaining {
  limitType: LimitType
  windowMode: WindowMode
  limitMicros: number
  usedMicros: number
  leftMicros: number
  windowStart: string
  windowEnd: string
}

export interface BillingStateSubscription {
  id: string
  userId: string
  planId: string
  planName: string
  startsAt: string
  expiresAt: string | null
  status: SubscriptionStatus
  limits: {
    id: string
    planId: string
    limitType: LimitType
    windowMode: WindowMode
    limitMicros: number
    fixedResetTime?: string | null
    createdAt: string
    updatedAt: string
  }[]
  createdAt: string
  updatedAt: string
}

export interface BillingStateResponse {
  balanceMicros: number
  balanceUsd: string
  subscription: BillingStateSubscription | null
  windows: WindowRemaining[] | null
  primarySource: 'subscription' | 'balance'
  secondarySource: 'subscription' | 'balance'
}

export interface UserBillingSetting {
  userId: string
  primarySource: 'subscription' | 'balance'
  secondarySource: 'subscription' | 'balance'
  createdAt: string
  updatedAt: string
}

// --- User Billing API ---

export async function getBillingState(): Promise<BillingStateResponse> {
  return fetchJson<BillingStateResponse>(`${API_BASE}/me/billing/state`)
}

export async function updateBillingPriority(primarySource: 'subscription' | 'balance'): Promise<UserBillingSetting> {
  return fetchJson<UserBillingSetting>(`${API_BASE}/me/billing/priority`, {
    method: 'PUT',
    body: JSON.stringify({ primarySource }),
  })
}

export async function getMySubscription(): Promise<{ subscription: BillingStateSubscription | null }> {
  return fetchJson<{ subscription: BillingStateSubscription | null }>(`${API_BASE}/me/subscription`)
}
