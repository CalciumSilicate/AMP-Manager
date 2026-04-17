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

export interface InviteConfig {
  enabled: boolean
  configComplete: boolean
  inviterRewardMicros: number
  inviteeRewardMicros: number
  minFirstPaidCnyCent: number
}

export interface InviteSummary {
  enabled: boolean
  configComplete: boolean
  inviteCode: string
  invitedUsers: number
  pendingInvites: number
  rewardedInvites: number
  totalRewardMicros: number
  invitedByUserId: string
  invitedByUsername: string
  boundInviteCode: string
}

export interface InviteRewardEvent {
  id: string
  relationId: string
  beneficiaryUserId: string
  beneficiaryUsername: string
  beneficiaryRole: 'inviter' | 'invitee'
  orderNo: string
  status: 'granted' | 'reversed'
  amountMicros: number
  orderPaidAmountCnyCent: number
  createdAt: string
}

export interface InviteStats {
  enabled: boolean
  configComplete: boolean
  relationsTotal: number
  pendingTotal: number
  rewardedTotal: number
  grantedRewardMicros: number
  reversedRewardMicros: number
}

export interface InviteRelation {
  id: string
  inviterUserId: string
  inviterUsername: string
  inviterCode: string
  inviteeUserId: string
  inviteeUsername: string
  status: 'pending' | 'rewarded'
  firstPaidOrderNo: string
  firstPaidOrderKind: string
  firstPaidAmountCnyCent: number
  firstPaidAt?: string | null
  rewardedAt?: string | null
  lastReversedAt?: string | null
  createdAt: string
}

export async function getMyInviteSummary(): Promise<InviteSummary> {
  return fetchJson<InviteSummary>(`${API_BASE}/me/invite/summary`)
}

export async function listMyInviteRewards(): Promise<InviteRewardEvent[]> {
  const data = await fetchJson<{ items: InviteRewardEvent[] }>(`${API_BASE}/me/invite/rewards`)
  return data.items || []
}

export async function getInviteConfig(): Promise<InviteConfig> {
  return fetchJson<InviteConfig>(`${API_BASE}/admin/invite/config`)
}

export async function updateInviteConfig(payload: Omit<InviteConfig, 'configComplete'>): Promise<InviteConfig> {
  const data = await fetchJson<{ config: InviteConfig }>(`${API_BASE}/admin/invite/config`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
  return data.config
}

export async function getInviteStats(): Promise<InviteStats> {
  return fetchJson<InviteStats>(`${API_BASE}/admin/invite/stats`)
}

export async function listInviteRelations(): Promise<InviteRelation[]> {
  const data = await fetchJson<{ items: InviteRelation[] }>(`${API_BASE}/admin/invite/relations`)
  return data.items || []
}

export async function listInviteRewardEvents(): Promise<InviteRewardEvent[]> {
  const data = await fetchJson<{ items: InviteRewardEvent[] }>(`${API_BASE}/admin/invite/reward-events`)
  return data.items || []
}
