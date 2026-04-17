import { authFetch } from './client'

const API_BASE = '/api'

async function fetchJson<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await authFetch(url, options)
  let data: unknown
  try {
    data = await res.json()
  } catch {
    data = undefined
  }
  if (!res.ok) {
    const payload = data as { error?: string } | undefined
    throw new Error(payload?.error || `请求失败 (${res.status})`)
  }
  return data as T
}

export type CouponCodeMode = 'shared' | 'single_use'
export type CouponDiscountType = 'fixed_amount' | 'percentage'

export interface CouponConfig {
  enabled: boolean
}

export interface CouponCampaign {
  id: string
  name: string
  description: string
  codeMode: CouponCodeMode
  sharedCode: string
  discountType: CouponDiscountType
  fixedDiscountCnyCent: number
  percentOffBps: number
  maxDiscountCnyCent: number
  totalUsageLimit: number
  usedCount: number
  perUserLimit: number
  startsAt?: string | null
  endsAt?: string | null
  enabled: boolean
  createdAt: string
  updatedAt: string
}

export interface CouponCampaignRequest {
  name: string
  description: string
  codeMode: CouponCodeMode
  sharedCode?: string
  discountType: CouponDiscountType
  fixedDiscountCnyCent: number
  percentOffBps: number
  maxDiscountCnyCent: number
  totalUsageLimit: number
  perUserLimit: number
  startsAt?: string
  endsAt?: string
  enabled: boolean
}

export interface CouponBatch {
  id: string
  campaignId: string
  name: string
  prefix: string
  codeCount: number
  codeLength: number
  createdAt: string
  updatedAt: string
}

export interface CouponCode {
  id: string
  campaignId: string
  campaignName: string
  batchId?: string
  batchName?: string
  codeMode: CouponCodeMode
  codeValue: string
  codeMask: string
  discountType: CouponDiscountType
  fixedDiscountCnyCent: number
  percentOffBps: number
  maxDiscountCnyCent: number
  status: 'active' | 'disabled' | 'consumed'
  maxUsages: number
  usedCount: number
  perUserLimit: number
  startsAt?: string | null
  endsAt?: string | null
  createdAt: string
  updatedAt: string
}

export interface CouponUsage {
  id: string
  campaignId: string
  campaignName: string
  codeId: string
  codeMask: string
  userId: string
  username: string
  orderId: string
  orderNo: string
  status: 'applied' | 'reversed'
  discountCnyCent: number
  finalAmountCnyCent: number
  createdAt: string
  updatedAt: string
}

export async function getCouponConfig(): Promise<CouponConfig> {
  return fetchJson<CouponConfig>(`${API_BASE}/admin/coupons/config`)
}

export async function updateCouponConfig(payload: CouponConfig): Promise<CouponConfig> {
  const data = await fetchJson<{ config: CouponConfig }>(`${API_BASE}/admin/coupons/config`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
  return data.config
}

export async function listCouponCampaigns(): Promise<CouponCampaign[]> {
  const data = await fetchJson<{ items: CouponCampaign[] }>(`${API_BASE}/admin/coupons/campaigns`)
  return data.items || []
}

export async function createCouponCampaign(payload: CouponCampaignRequest): Promise<CouponCampaign> {
  return fetchJson<CouponCampaign>(`${API_BASE}/admin/coupons/campaigns`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function updateCouponCampaign(id: string, payload: CouponCampaignRequest): Promise<CouponCampaign> {
  return fetchJson<CouponCampaign>(`${API_BASE}/admin/coupons/campaigns/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export async function deleteCouponCampaign(id: string): Promise<void> {
  await fetchJson<{ message: string }>(`${API_BASE}/admin/coupons/campaigns/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export async function createCouponBatch(campaignId: string, payload: { name: string; prefix: string; codeCount: number; codeLength: number }): Promise<{ batch: CouponBatch; codes: string[] }> {
  return fetchJson<{ batch: CouponBatch; codes: string[] }>(`${API_BASE}/admin/coupons/campaigns/${encodeURIComponent(campaignId)}/batches`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function listCouponCodes(filters: { campaignId?: string; status?: string; keyword?: string; limit?: number } = {}): Promise<CouponCode[]> {
  const params = new URLSearchParams()
  if (filters.campaignId) params.set('campaignId', filters.campaignId)
  if (filters.status) params.set('status', filters.status)
  if (filters.keyword) params.set('keyword', filters.keyword)
  if (filters.limit) params.set('limit', String(filters.limit))
  const query = params.toString()
  const data = await fetchJson<{ items: CouponCode[] }>(`${API_BASE}/admin/coupons/codes${query ? `?${query}` : ''}`)
  return data.items || []
}

export async function setCouponCodeEnabled(id: string, enabled: boolean): Promise<void> {
  await fetchJson<{ message: string }>(`${API_BASE}/admin/coupons/codes/${encodeURIComponent(id)}/enabled`, {
    method: 'PATCH',
    body: JSON.stringify({ enabled }),
  })
}

export async function listCouponUsages(limit = 200): Promise<CouponUsage[]> {
  const data = await fetchJson<{ items: CouponUsage[] }>(`${API_BASE}/admin/coupons/usages?limit=${limit}`)
  return data.items || []
}
