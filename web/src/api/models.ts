import { authFetch } from './client'

const API_BASE = '/api'

export interface AvailableModel {
  modelId: string
  displayName: string
  channelId: string
  channelType: 'openai' | 'claude' | 'gemini'
  channelName: string
}

export interface FetchModelsResult {
  message: string
  results: Record<string, number>
}

export async function listAvailableModels(): Promise<AvailableModel[]> {
  const res = await authFetch(`${API_BASE}/models`)
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error(data.error || '获取模型列表失败')
  }
  const data = await res.json()
  return data.models || []
}

export async function fetchChannelModels(channelId: string): Promise<{ message: string; count: number }> {
  const res = await authFetch(`${API_BASE}/admin/channels/${channelId}/fetch-models`, {
    method: 'POST',
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error(data.error || '获取模型失败')
  }
  return res.json()
}

export interface ChannelModel2 {
  id: string
  channelId: string
  modelId: string
  displayName: string
  createdAt: string
}

export async function getChannelModels(channelId: string): Promise<ChannelModel2[]> {
  const res = await authFetch(`${API_BASE}/admin/channels/${channelId}/models`)
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error(data.error || '获取渠道模型失败')
  }
  const data = await res.json()
  return data.models || []
}

export async function fetchAllModels(): Promise<FetchModelsResult> {
  const res = await authFetch(`${API_BASE}/admin/models/fetch-all`, {
    method: 'POST',
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error(data.error || '获取模型失败')
  }
  return res.json()
}
