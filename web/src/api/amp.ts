import { authFetch } from './client'

const API_BASE = '/api/me/amp'

async function handleResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: '请求失败' }))
    throw new Error(error.error || '请求失败')
  }
  return response.json()
}

// Types
export type WebSearchMode = 'upstream' | 'builtin_free' | 'local_duckduckgo'

export interface ModelMapping {
  from: string
  to: string
  channelId?: string
  regex: boolean
  thinkingLevel?: 'low' | 'medium' | 'high' | 'xhigh' | ''
  pseudoNonStream?: boolean
  auditKeywords?: string[]
  ampOnly?: boolean
  fastMode?: boolean
  customInstructions?: string
  customInstructionsEnabled?: boolean
}

export interface AmpSettings {
  upstreamUrl: string
  apiKeySet: boolean
  modelMappings: ModelMapping[]
  enabled: boolean
  nativeMode: boolean
  webSearchMode?: WebSearchMode
  showBalanceInAd?: boolean
  socks5ProxySet?: boolean
}

export interface UpdateAmpSettingsRequest {
  upstreamUrl?: string
  upstreamApiKey?: string
  modelMappings?: ModelMapping[]
  enabled?: boolean
  nativeMode?: boolean
  webSearchMode?: WebSearchMode
  showBalanceInAd?: boolean
  socks5Proxy?: string
}

export interface TestResult {
  success: boolean
  message: string
  latencyMs?: number
}

export interface APIKey {
  id: string
  name: string
  prefix: string
  lastUsedAt: string | null
  createdAt: string
}

export interface CreateAPIKeyResponse {
  id: string
  name: string
  prefix: string
  apiKey: string
  createdAt: string
  message: string
}

export interface APIKeyRevealResponse {
  id: string
  name: string
  prefix: string
  apiKey: string
  createdAt: string
}

export interface BootstrapInfo {
  proxyBaseUrl: string
  configExample: string
  hasSettings: boolean
  hasApiKey: boolean
}

// Settings API
export async function getAmpSettings(): Promise<AmpSettings> {
  const response = await authFetch(`${API_BASE}/settings`)
  return handleResponse<AmpSettings>(response)
}

export async function updateAmpSettings(data: UpdateAmpSettingsRequest): Promise<AmpSettings> {
  const response = await authFetch(`${API_BASE}/settings`, {
    method: 'PUT',
    body: JSON.stringify(data),
  })
  return handleResponse<AmpSettings>(response)
}

export async function testAmpConnection(): Promise<TestResult> {
  const response = await authFetch(`${API_BASE}/settings/test`, {
    method: 'POST',
  })
  return handleResponse<TestResult>(response)
}

// API Key
export async function getAPIKeys(): Promise<APIKey[]> {
  const response = await authFetch(`${API_BASE}/api-keys`)
  const data = await handleResponse<{ apiKeys: APIKey[] }>(response)
  return data.apiKeys || []
}

export async function createAPIKey(name: string): Promise<CreateAPIKeyResponse> {
  return createAPIKeyWithOptions({ name })
}

export async function createAPIKeyWithOptions(data: { name: string; customKey?: string }): Promise<CreateAPIKeyResponse> {
  const response = await authFetch(`${API_BASE}/api-keys`, {
    method: 'POST',
    body: JSON.stringify(data),
  })
  return handleResponse<CreateAPIKeyResponse>(response)
}

export async function deleteAPIKey(id: string): Promise<void> {
  const response = await authFetch(`${API_BASE}/api-keys/${id}`, {
    method: 'DELETE',
  })
  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: '删除失败' }))
    throw new Error(error.error || '删除失败')
  }
}

export async function getAPIKey(id: string): Promise<APIKeyRevealResponse> {
  const response = await authFetch(`${API_BASE}/api-keys/${id}`)
  return handleResponse<APIKeyRevealResponse>(response)
}

// Bootstrap API
export async function getAmpBootstrap(): Promise<BootstrapInfo> {
  const response = await authFetch(`${API_BASE}/bootstrap`)
  return handleResponse<BootstrapInfo>(response)
}

// Request Logs Types
export interface RequestLog {
  id: string
  createdAt: string
  userId: string
  username?: string
  apiKeyId: string
  apiKeyName?: string
  apiKeyPrefix?: string
  originalModel?: string
  mappedModel?: string
  provider?: string
  channelName?: string
  channelId?: string
  endpoint?: string
  method: string
  path: string
  statusCode: number
  latencyMs: number
  ttfbMs?: number
  tps?: number
  inputTokens?: number
  outputTokens?: number
  cacheReadInputTokens?: number
  cacheCreationInputTokens?: number
  errorType?: string
  requestId?: string
  costMicros?: number
  costUsd?: string
  pricingModel?: string
  inputCostPerToken?: number
  outputCostPerToken?: number
  cacheReadInputPerToken?: number
  cacheCreationInputPerToken?: number
  rateMultiplier?: number
  thinkingLevel?: string
  downstreamTransport?: string
  upstreamTransport?: string
  transportFallbackReason?: string
}

export interface RequestLogListResponse {
  items: RequestLog[]
  total: number
  page: number
  pageSize: number
}

export interface UsageSummary {
  groupKey: string
  inputTokensSum: number
  outputTokensSum: number
  cacheReadInputTokensSum: number
  cacheCreationInputTokensSum: number
  requestCount: number
  errorCount: number
  costMicrosSum?: number
  costUsdSum?: string
}

export interface UsageSummaryResponse {
  items: UsageSummary[]
}

export interface RequestLogListParams {
  page?: number
  pageSize?: number
  apiKeyId?: string
  model?: string
  statusCodes?: number[]
  isStreaming?: boolean
  from?: string
  to?: string
}

// Request Logs API
export async function getRequestLogs(params: RequestLogListParams = {}, signal?: AbortSignal): Promise<RequestLogListResponse> {
  const searchParams = new URLSearchParams()
  if (params.page) searchParams.set('page', params.page.toString())
  if (params.pageSize) searchParams.set('pageSize', params.pageSize.toString())
  if (params.apiKeyId) searchParams.set('apiKeyId', params.apiKeyId)
  if (params.model) searchParams.set('model', params.model)
  params.statusCodes?.forEach((status) => searchParams.append('status', status.toString()))
  if (params.isStreaming !== undefined) searchParams.set('isStreaming', params.isStreaming.toString())
  if (params.from) searchParams.set('from', params.from)
  if (params.to) searchParams.set('to', params.to)

  const query = searchParams.toString()
  const response = await authFetch(`${API_BASE}/request-logs${query ? `?${query}` : ''}`, {
    signal,
  })
  return handleResponse<RequestLogListResponse>(response)
}

export async function getUsageSummary(params: { from?: string; to?: string; groupBy?: string; model?: string } = {}, signal?: AbortSignal): Promise<UsageSummaryResponse> {
  const searchParams = new URLSearchParams()
  if (params.from) searchParams.set('from', params.from)
  if (params.to) searchParams.set('to', params.to)
  if (params.groupBy) searchParams.set('groupBy', params.groupBy)
  if (params.model) searchParams.set('model', params.model)

  const query = searchParams.toString()
  const response = await authFetch(`${API_BASE}/usage/summary${query ? `?${query}` : ''}`, {
    signal,
  })
  return handleResponse<UsageSummaryResponse>(response)
}

export async function getDistinctModels(signal?: AbortSignal): Promise<{ models: string[] }> {
  const response = await authFetch(`${API_BASE}/request-logs/models`, {
    signal,
  })
  return handleResponse<{ models: string[] }>(response)
}

export async function getDistinctKeys(signal?: AbortSignal): Promise<{ keys: DistinctAPIKey[] }> {
  const response = await authFetch(`${API_BASE}/request-logs/keys`, {
    signal,
  })
  return handleResponse<{ keys: DistinctAPIKey[] }>(response)
}

// Admin API for request logs
const ADMIN_API_BASE = '/api/admin'

export interface AdminRequestLogListParams extends RequestLogListParams {
  userId?: string
}

export async function getAdminRequestLogs(params: AdminRequestLogListParams = {}, signal?: AbortSignal): Promise<RequestLogListResponse> {
  const searchParams = new URLSearchParams()
  if (params.page) searchParams.set('page', params.page.toString())
  if (params.pageSize) searchParams.set('pageSize', params.pageSize.toString())
  if (params.userId) searchParams.set('userId', params.userId)
  if (params.apiKeyId) searchParams.set('apiKeyId', params.apiKeyId)
  if (params.model) searchParams.set('model', params.model)
  params.statusCodes?.forEach((status) => searchParams.append('status', status.toString()))
  if (params.isStreaming !== undefined) searchParams.set('isStreaming', params.isStreaming.toString())
  if (params.from) searchParams.set('from', params.from)
  if (params.to) searchParams.set('to', params.to)

  const query = searchParams.toString()
  const response = await authFetch(`${ADMIN_API_BASE}/request-logs${query ? `?${query}` : ''}`, {
    signal,
  })
  return handleResponse<RequestLogListResponse>(response)
}

export async function getAdminDistinctModels(signal?: AbortSignal): Promise<{ models: string[] }> {
  const response = await authFetch(`${ADMIN_API_BASE}/request-logs/models`, {
    signal,
  })
  return handleResponse<{ models: string[] }>(response)
}

export interface DistinctAPIKey {
  id: string
  name: string
  prefix: string
}

export async function getAdminDistinctKeys(userId?: string, signal?: AbortSignal): Promise<{ keys: DistinctAPIKey[] }> {
  const searchParams = new URLSearchParams()
  if (userId) searchParams.set('userId', userId)
  const query = searchParams.toString()
  const response = await authFetch(`${ADMIN_API_BASE}/request-logs/keys${query ? `?${query}` : ''}`, {
    signal,
  })
  return handleResponse<{ keys: DistinctAPIKey[] }>(response)
}

export async function getAdminUsageSummary(params: { from?: string; to?: string; groupBy?: string; userId?: string; model?: string } = {}, signal?: AbortSignal): Promise<UsageSummaryResponse> {
  const searchParams = new URLSearchParams()
  if (params.from) searchParams.set('from', params.from)
  if (params.to) searchParams.set('to', params.to)
  if (params.groupBy) searchParams.set('groupBy', params.groupBy)
  if (params.userId) searchParams.set('userId', params.userId)
  if (params.model) searchParams.set('model', params.model)

  const query = searchParams.toString()
  const response = await authFetch(`${ADMIN_API_BASE}/usage/summary${query ? `?${query}` : ''}`, {
    signal,
  })
  return handleResponse<UsageSummaryResponse>(response)
}

// Request Log Detail API
export interface RequestLogDetail {
  requestId: string
  requestHeaders: Record<string, string>
  requestBody: string
  translatedRequestBody?: string
  responseHeaders: Record<string, string>
  responseBody: string
  translatedResponseBody?: string
  createdAt: string
  billingStatus: 'free' | 'settled' | 'overuse' | 'expired' | 'none'
  chargedSubscriptionMicros: number
  chargedBalanceMicros: number
  billingGapMicros?: number
  costMicros?: number
  costUsd?: string
}

export async function getAdminRequestLogDetail(logId: string, signal?: AbortSignal): Promise<RequestLogDetail> {
  const response = await authFetch(`${ADMIN_API_BASE}/request-logs/${logId}/detail`, {
    signal,
  })
  return handleResponse<RequestLogDetail>(response)
}
