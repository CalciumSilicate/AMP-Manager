import { authFetch } from './client'

const USER_API_BASE = '/api/me/status'
const ADMIN_API_BASE = '/api/admin/system'

async function parseJSON<T>(response: Response, fallbackMessage: string): Promise<T> {
  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: fallbackMessage }))
    throw new Error(error.error || fallbackMessage)
  }
  return response.json()
}

export type StatusMonitorTargetType = 'service_proxy' | 'channel_direct' | 'custom_http'
export type StatusMonitorState = 'operational' | 'degraded' | 'error' | 'failed' | 'unknown'
export type StatusMonitorRequestFormat = 'chat_completions' | 'responses' | 'messages' | 'generate_content'
export type StatusMonitorPeriod = '7d' | '15d' | '30d'

export interface StatusMonitorRuntimeConfig {
  enabled: boolean
  serviceBaseUrl: string
  serviceMonitorApiKeySet: boolean
  serviceMonitorApiKeyMasked?: string
  pollIntervalSec: number
  defaultTimeoutMs: number
  retentionDays: number
}

export interface StatusMonitorRuntimeConfigUpdate {
  enabled: boolean
  serviceBaseUrl: string
  serviceMonitorApiKey?: string
  retainServiceMonitorApiKey?: boolean
  clearServiceMonitorApiKey?: boolean
  pollIntervalSec: number
  defaultTimeoutMs: number
  retentionDays: number
}

export interface StatusMonitorAdminItem {
  id: string
  name: string
  groupName: string
  targetType: StatusMonitorTargetType
  enabled: boolean
  sortOrder: number
  timeoutMs: number
  degradedThresholdMs: number
  requestFormat: StatusMonitorRequestFormat
  model: string
  channelId: string
  channelName: string
  url: string
  method: string
  headersSet: boolean
  headersMasked?: string
  bodyTemplateSet: boolean
  bodyTemplateMasked?: string
  expectedStatusCodes: number[]
  expectedSubstring: string
  createdAt: string
  updatedAt: string
}

export interface StatusMonitorItemRequest {
  name: string
  groupName: string
  targetType: StatusMonitorTargetType
  enabled: boolean
  sortOrder: number
  timeoutMs: number
  degradedThresholdMs: number
  requestFormat: StatusMonitorRequestFormat
  model: string
  channelId: string
  url: string
  method: string
  headersJson?: string
  retainHeadersJson?: boolean
  clearHeadersJson?: boolean
  bodyTemplate?: string
  retainBodyTemplate?: boolean
  clearBodyTemplate?: boolean
  expectedStatusCodes: number[]
  expectedSubstring: string
}

export interface StatusMonitorLatestResult {
  status: StatusMonitorState
  latencyMs: number
  ttfbMs: number
  httpStatusCode: number
  message: string
  checkedAt?: string
}

export interface StatusMonitorAvailability {
  totalChecks: number
  operationalCount: number
  availabilityPct: number
}

export interface StatusMonitorHistoryPoint {
  status: StatusMonitorState
  ttfbMs: number
  checkedAt: string
}

export interface StatusMonitorDashboardItem {
  id: string
  name: string
  targetType: StatusMonitorTargetType
  requestFormat: StatusMonitorRequestFormat
  model: string
  channelName?: string
  endpointLabel: string
  latest: StatusMonitorLatestResult
  availability: StatusMonitorAvailability
  history: StatusMonitorHistoryPoint[]
}

export interface StatusMonitorDashboardGroup {
  groupName: string
  items: StatusMonitorDashboardItem[]
}

export interface StatusMonitorSummaryCounts {
  total: number
  operational: number
  degraded: number
  error: number
  failed: number
  unknown: number
}

export interface StatusMonitorDashboardResponse {
  period: StatusMonitorPeriod
  generatedAt: string
  lastUpdated?: string
  pollIntervalSec: number
  overallStatus: StatusMonitorState
  summaryCounts: StatusMonitorSummaryCounts
  groups: StatusMonitorDashboardGroup[]
}

export async function getStatusMonitorDashboard(period: StatusMonitorPeriod): Promise<StatusMonitorDashboardResponse> {
  const response = await authFetch(`${USER_API_BASE}/dashboard?period=${encodeURIComponent(period)}`)
  return parseJSON<StatusMonitorDashboardResponse>(response, '获取状态监控面板失败')
}

export async function getStatusMonitorRuntimeConfig(): Promise<StatusMonitorRuntimeConfig> {
  const response = await authFetch(`${ADMIN_API_BASE}/status-monitor-runtime`)
  return parseJSON<StatusMonitorRuntimeConfig>(response, '获取状态监控配置失败')
}

export async function updateStatusMonitorRuntimeConfig(payload: StatusMonitorRuntimeConfigUpdate): Promise<StatusMonitorRuntimeConfig> {
  const response = await authFetch(`${ADMIN_API_BASE}/status-monitor-runtime`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
  const data = await parseJSON<{ config: StatusMonitorRuntimeConfig }>(response, '保存状态监控配置失败')
  return data.config
}

export async function listStatusMonitorItems(): Promise<StatusMonitorAdminItem[]> {
  const response = await authFetch(`${ADMIN_API_BASE}/status-monitors`)
  const data = await parseJSON<{ items: StatusMonitorAdminItem[] }>(response, '获取状态监控项失败')
  return data.items || []
}

export async function createStatusMonitorItem(payload: StatusMonitorItemRequest): Promise<StatusMonitorAdminItem> {
  const response = await authFetch(`${ADMIN_API_BASE}/status-monitors`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
  return parseJSON<StatusMonitorAdminItem>(response, '创建状态监控项失败')
}

export async function updateStatusMonitorItem(id: string, payload: StatusMonitorItemRequest): Promise<StatusMonitorAdminItem> {
  const response = await authFetch(`${ADMIN_API_BASE}/status-monitors/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
  return parseJSON<StatusMonitorAdminItem>(response, '更新状态监控项失败')
}

export async function deleteStatusMonitorItem(id: string): Promise<void> {
  const response = await authFetch(`${ADMIN_API_BASE}/status-monitors/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: '删除状态监控项失败' }))
    throw new Error(error.error || '删除状态监控项失败')
  }
}

export async function runStatusMonitorItem(id: string): Promise<{ message: string; result: StatusMonitorLatestResult & { endpointLabel?: string } }> {
  const response = await authFetch(`${ADMIN_API_BASE}/status-monitors/${encodeURIComponent(id)}/run`, {
    method: 'POST',
  })
  return parseJSON<{ message: string; result: StatusMonitorLatestResult & { endpointLabel?: string } }>(response, '执行状态监控探测失败')
}

export async function runAllStatusMonitorItems(): Promise<{ message: string; count: number }> {
  const response = await authFetch(`${ADMIN_API_BASE}/status-monitors/run-all`, {
    method: 'POST',
  })
  return parseJSON<{ message: string; count: number }>(response, '执行全量状态监控探测失败')
}
