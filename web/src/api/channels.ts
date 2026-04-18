import { authFetch } from './client'

const API_BASE = '/api/admin/channels'
const DEFAULT_TRANSLATOR: ChannelTranslator = {
  compatible: false,
  responses: false,
  messages: false,
  gemini: false,
}

async function handleResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: '请求失败' }))
    throw new Error(error.error || '请求失败')
  }
  return response.json()
}

export type ChannelType = 'gemini' | 'claude' | 'openai'
export type ChannelEndpoint = 'chat_completions' | 'responses' | 'messages' | 'generate_content'

export interface ChannelTranslator {
  compatible: boolean
  responses: boolean
  messages: boolean
  gemini: boolean
}

export interface ChannelModel {
  name: string
  alias?: string
}

export interface Channel {
  id: string
  type: ChannelType
  endpoint: ChannelEndpoint
  name: string
  baseUrl: string
  apiKeySet: boolean
  enabled: boolean
  weight: number
  priority: number
  rateMultiplier: number
  groupIds: string[]
  groupNames: string[]
  models: ChannelModel[]
  modelWhitelist: boolean
  simulateCli: boolean
  simulateUa: boolean
  simulateSystemPrompt: boolean
  traditionalChinese: boolean
  copilotApi: boolean
  codexWebsocketEnabled: boolean
  headers: Record<string, string>
  translator: ChannelTranslator
  createdAt: string
  updatedAt: string
}

export interface ChannelRequest {
  type: ChannelType
  endpoint?: ChannelEndpoint
  name: string
  baseUrl: string
  apiKey?: string
  enabled: boolean
  weight: number
  priority: number
  rateMultiplier: number
  groupIds?: string[]
  models?: ChannelModel[]
  modelWhitelist?: boolean
  simulateCli?: boolean
  simulateUa?: boolean
  simulateSystemPrompt?: boolean
  traditionalChinese?: boolean
  copilotApi?: boolean
  codexWebsocketEnabled?: boolean
  headers?: Record<string, string>
  translator?: ChannelTranslator
}

export interface TestChannelResult {
  success: boolean
  message: string
  latencyMs?: number
  ttfbMs?: number
  statusCode?: number
}

export interface TestChannelRequest {
  format: ChannelEndpoint
  model: string
  prompt: string
  instructions: string
  thinkingEffort?: string
}

function normalizeChannelModels(input: unknown): ChannelModel[] {
  if (!Array.isArray(input)) {
    return []
  }
  return input.map((item) => {
    const model = item as Partial<ChannelModel> | null
    return {
      name: typeof model?.name === 'string' ? model.name : '',
      alias: typeof model?.alias === 'string' ? model.alias : '',
    }
  })
}

function normalizeChannel(channel: Channel): Channel {
  const headers = channel.headers && typeof channel.headers === 'object'
    ? Object.fromEntries(Object.entries(channel.headers).filter(([key]) => key.trim() !== ''))
    : {}

  return {
    ...channel,
    groupIds: Array.isArray(channel.groupIds) ? [...channel.groupIds] : [],
    groupNames: Array.isArray(channel.groupNames) ? [...channel.groupNames] : [],
    models: normalizeChannelModels(channel.models),
    headers,
    translator: { ...DEFAULT_TRANSLATOR, ...(channel.translator || {}) },
  }
}

export async function listChannels(): Promise<Channel[]> {
  const response = await authFetch(API_BASE)
  const data = await handleResponse<{ channels: Channel[] }>(response)
  return (data.channels || []).map(normalizeChannel)
}

export async function getChannel(id: string): Promise<Channel> {
  const response = await authFetch(`${API_BASE}/${id}`)
  return normalizeChannel(await handleResponse<Channel>(response))
}

export async function createChannel(data: ChannelRequest): Promise<Channel> {
  const response = await authFetch(API_BASE, {
    method: 'POST',
    body: JSON.stringify(data),
  })
  return normalizeChannel(await handleResponse<Channel>(response))
}

export async function updateChannel(id: string, data: ChannelRequest): Promise<Channel> {
  const response = await authFetch(`${API_BASE}/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  })
  return normalizeChannel(await handleResponse<Channel>(response))
}

export async function deleteChannel(id: string): Promise<void> {
  const response = await authFetch(`${API_BASE}/${id}`, {
    method: 'DELETE',
  })
  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: '删除失败' }))
    throw new Error(error.error || '删除失败')
  }
}

export async function setChannelEnabled(id: string, enabled: boolean): Promise<void> {
  const response = await authFetch(`${API_BASE}/${id}/enabled`, {
    method: 'PATCH',
    body: JSON.stringify({ enabled }),
  })
  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: '更新失败' }))
    throw new Error(error.error || '更新失败')
  }
}

export async function testChannel(id: string, data: TestChannelRequest): Promise<TestChannelResult> {
  const response = await authFetch(`${API_BASE}/${id}/test`, {
    method: 'POST',
    body: JSON.stringify(data),
  })
  return handleResponse<TestChannelResult>(response)
}
