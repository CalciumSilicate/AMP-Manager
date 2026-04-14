import { authFetch } from './client'
import type { UserSubscriptionResponse } from './subscription'

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
    const payload = data as { error?: string; message?: string } | undefined
    throw new Error(payload?.error || payload?.message || `请求失败 (${res.status})`)
  }

  return data as T
}

export type RedeemCodeMode = 'single_use' | 'shared'
export type RedeemCodeStatus = 'active' | 'disabled' | 'consumed'
export type RedeemRedemptionStatus = 'success' | 'rejected'

export interface RedeemCampaign {
  id: string
  name: string
  description: string
  codeMode: RedeemCodeMode
  sharedCode?: string
  sharedCodeMask?: string
  subscriptionPlanId: string
  subscriptionPlanName: string
  subscriptionDurationDays: number
  balanceMicros: number
  totalRedemptionsLimit: number
  redeemedCount: number
  perUserLimit: number
  startsAt: string | null
  endsAt: string | null
  enabled: boolean
  codeCount: number
  batchCount: number
  createdAt: string
  updatedAt: string
}

export interface RedeemCampaignRequest {
  name: string
  description: string
  codeMode: RedeemCodeMode
  sharedCode?: string
  subscriptionPlanId: string
  subscriptionDurationDays: number
  balanceMicros: number
  totalRedemptionsLimit: number
  perUserLimit: number
  startsAt?: string
  endsAt?: string
  enabled: boolean
}

export interface RedeemBatch {
  id: string
  campaignId: string
  campaignName: string
  name: string
  prefix: string
  codeCount: number
  codeLength: number
  createdAt: string
  updatedAt: string
}

export interface RedeemBatchRequest {
  name: string
  prefix: string
  codeCount: number
  codeLength: number
}

export interface RedeemCode {
  id: string
  campaignId: string
  campaignName: string
  batchId?: string | null
  batchName?: string
  codeMode: RedeemCodeMode
  codeValue: string
  codeMask: string
  status: RedeemCodeStatus
  maxRedemptions: number
  redeemedCount: number
  lastRedeemedAt: string | null
  createdAt: string
  updatedAt: string
}

export interface RedeemRedemption {
  id: string
  campaignId?: string | null
  campaignName: string
  codeId?: string | null
  userId: string
  username: string
  codeMask: string
  subscriptionPlanId: string
  subscriptionPlanName: string
  subscriptionDurationDays: number
  balanceMicros: number
  status: RedeemRedemptionStatus
  failureReason: string
  grantedSubscriptionId: string
  grantedExpiresAt: string | null
  balanceAfterMicros: number
  createdAt: string
}

export interface RedeemResult {
  message: string
  campaign: RedeemCampaign | null
  redemption: RedeemRedemption | null
  currentSubscription?: UserSubscriptionResponse | null
  balanceMicros: number
  balanceUsd: string
}

export async function redeemCode(code: string): Promise<RedeemResult> {
  return fetchJson<RedeemResult>(`${API_BASE}/me/redeem`, {
    method: 'POST',
    body: JSON.stringify({ code }),
  })
}

export async function listMyRedeemRecords(): Promise<RedeemRedemption[]> {
  const data = await fetchJson<{ items: RedeemRedemption[] }>(`${API_BASE}/me/redeem/records`)
  return data.items || []
}

export async function listRedeemCampaigns(): Promise<RedeemCampaign[]> {
  const data = await fetchJson<{ items: RedeemCampaign[] }>(`${API_BASE}/admin/redeem/campaigns`)
  return data.items || []
}

export async function createRedeemCampaign(payload: RedeemCampaignRequest): Promise<RedeemCampaign> {
  return fetchJson<RedeemCampaign>(`${API_BASE}/admin/redeem/campaigns`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function updateRedeemCampaign(id: string, payload: RedeemCampaignRequest): Promise<RedeemCampaign> {
  return fetchJson<RedeemCampaign>(`${API_BASE}/admin/redeem/campaigns/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export async function deleteRedeemCampaign(id: string): Promise<void> {
  await fetchJson<{ message: string }>(`${API_BASE}/admin/redeem/campaigns/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export async function listRedeemBatches(campaignId = ''): Promise<RedeemBatch[]> {
  const query = campaignId ? `?campaignId=${encodeURIComponent(campaignId)}` : ''
  const data = await fetchJson<{ items: RedeemBatch[] }>(`${API_BASE}/admin/redeem/batches${query}`)
  return data.items || []
}

export async function createRedeemBatch(campaignId: string, payload: RedeemBatchRequest): Promise<{ batch: RedeemBatch; codes: string[] }> {
  return fetchJson<{ batch: RedeemBatch; codes: string[] }>(`${API_BASE}/admin/redeem/campaigns/${encodeURIComponent(campaignId)}/batches`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function downloadRedeemBatchCSV(batchId: string): Promise<Blob> {
  const res = await authFetch(`${API_BASE}/admin/redeem/batches/${encodeURIComponent(batchId)}/export`)
  if (!res.ok) {
    let message = '导出失败'
    try {
      const payload = await res.json()
      message = payload.error || message
    } catch {
      // ignore
    }
    throw new Error(message)
  }
  return res.blob()
}

export interface RedeemCodeFilters {
  campaignId?: string
  batchId?: string
  status?: RedeemCodeStatus | ''
  keyword?: string
  limit?: number
}

export async function listRedeemCodes(filters: RedeemCodeFilters = {}): Promise<RedeemCode[]> {
  const params = new URLSearchParams()
  if (filters.campaignId) params.set('campaignId', filters.campaignId)
  if (filters.batchId) params.set('batchId', filters.batchId)
  if (filters.status) params.set('status', filters.status)
  if (filters.keyword) params.set('keyword', filters.keyword)
  if (filters.limit) params.set('limit', String(filters.limit))
  const query = params.toString()
  const data = await fetchJson<{ items: RedeemCode[] }>(`${API_BASE}/admin/redeem/codes${query ? `?${query}` : ''}`)
  return data.items || []
}

export async function setRedeemCodeEnabled(id: string, enabled: boolean): Promise<void> {
  await fetchJson<{ message: string }>(`${API_BASE}/admin/redeem/codes/${encodeURIComponent(id)}/enabled`, {
    method: 'PATCH',
    body: JSON.stringify({ enabled }),
  })
}

export interface RedeemRedemptionFilters {
  campaignId?: string
  status?: RedeemRedemptionStatus | ''
  username?: string
  limit?: number
}

export async function listRedeemRedemptions(filters: RedeemRedemptionFilters = {}): Promise<RedeemRedemption[]> {
  const params = new URLSearchParams()
  if (filters.campaignId) params.set('campaignId', filters.campaignId)
  if (filters.status) params.set('status', filters.status)
  if (filters.username) params.set('username', filters.username)
  if (filters.limit) params.set('limit', String(filters.limit))
  const query = params.toString()
  const data = await fetchJson<{ items: RedeemRedemption[] }>(`${API_BASE}/admin/redeem/redemptions${query ? `?${query}` : ''}`)
  return data.items || []
}
