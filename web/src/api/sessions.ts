import { authFetch } from './client'

const ADMIN_API_BASE = '/api/admin'

function getString(source: Record<string, unknown>, ...keys: string[]): string | undefined {
  for (const key of keys) {
    const value = source[key]
    if (typeof value === 'string' && value.trim()) {
      return value
    }
  }
  return undefined
}

function getNumber(source: Record<string, unknown>, ...keys: string[]): number | undefined {
  for (const key of keys) {
    const value = source[key]
    if (typeof value === 'number' && Number.isFinite(value)) {
      return value
    }
  }
  return undefined
}

function getBoolean(source: Record<string, unknown>, ...keys: string[]): boolean | undefined {
  for (const key of keys) {
    const value = source[key]
    if (typeof value === 'boolean') {
      return value
    }
  }
  return undefined
}

function getRecord(source: Record<string, unknown>, ...keys: string[]): Record<string, unknown> | undefined {
  for (const key of keys) {
    const value = source[key]
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      return value as Record<string, unknown>
    }
  }
  return undefined
}

function getArray(source: Record<string, unknown>, ...keys: string[]): Record<string, unknown>[] {
  for (const key of keys) {
    const value = source[key]
    if (Array.isArray(value)) {
      return value.filter((item): item is Record<string, unknown> => Boolean(item && typeof item === 'object' && !Array.isArray(item)))
    }
  }
  return []
}

async function handleResponse<T>(response: Response, fallbackMessage: string, transform?: (data: unknown) => T): Promise<T> {
  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: fallbackMessage }))
    throw new Error(error.error || fallbackMessage)
  }

  const data = await response.json()
  return transform ? transform(data) : data as T
}

export type SessionState = 'active' | 'idle' | 'expired' | 'unknown' | string

export interface SessionListItem {
  id: string
  sessionId: string
  userId?: string
  username?: string
  active: boolean
  state: SessionState
  requestCount: number
  distinctRequestCount?: number
  firstSeenAt?: string
  lastSeenAt?: string
  lastModel?: string
  lastStatusCode?: number
  lastApiKeyName?: string
  lastApiKeyPrefix?: string
}

export interface SessionRequestTimelineItem {
  requestLogId: string
  sessionId: string
  createdAt: string
  statusCode?: number
  model?: string
  channelName?: string
  latencyMs?: number
  inputTokens?: number
  outputTokens?: number
  apiKeyName?: string
  apiKeyPrefix?: string
  method?: string
  path?: string
}

export interface SessionDetail {
  session: SessionListItem
  timeline: SessionRequestTimelineItem[]
  distinctApiKeyCount?: number
  distinctModelCount?: number
  totalInputTokens?: number
  totalOutputTokens?: number
  totalCostUsd?: string
}

export interface SessionLeaderboardItem {
  userId?: string
  username: string
  distinctSessionCount: number
}

export interface SessionListResponse {
  items: SessionListItem[]
  total: number
  page: number
  pageSize: number
}

export interface SessionListParams {
  page?: number
  pageSize?: number
  query?: string
  activeOnly?: boolean
}

function normalizeSessionItem(raw: Record<string, unknown>): SessionListItem {
  const active = getBoolean(raw, 'active', 'isActive') ?? false
  const state = getString(raw, 'state', 'status') ?? (active ? 'active' : 'idle')
  const sessionId = getString(raw, 'sessionId', 'id') || ''

  return {
    id: getString(raw, 'id', 'sessionId') || sessionId,
    sessionId,
    userId: getString(raw, 'userId'),
    username: getString(raw, 'username', 'userName'),
    active,
    state,
    requestCount: getNumber(raw, 'requestCount', 'requests', 'totalRequests') ?? 0,
    distinctRequestCount: getNumber(raw, 'distinctRequestCount'),
    firstSeenAt: getString(raw, 'firstSeenAt', 'createdAt'),
    lastSeenAt: getString(raw, 'lastSeenAt', 'lastRequestAt', 'updatedAt'),
    lastModel: getString(raw, 'lastModel', 'model'),
    lastStatusCode: getNumber(raw, 'lastStatusCode', 'statusCode'),
    lastApiKeyName: getString(raw, 'lastApiKeyName', 'apiKeyName'),
    lastApiKeyPrefix: getString(raw, 'lastApiKeyPrefix', 'apiKeyPrefix'),
  }
}

function normalizeTimelineItem(raw: Record<string, unknown>): SessionRequestTimelineItem {
  return {
    requestLogId: getString(raw, 'requestLogId', 'id') || '',
    sessionId: getString(raw, 'sessionId') || '',
    createdAt: getString(raw, 'createdAt', 'timestamp') || '',
    statusCode: getNumber(raw, 'statusCode'),
    model: getString(raw, 'mappedModel', 'model', 'originalModel'),
    channelName: getString(raw, 'channelName', 'provider'),
    latencyMs: getNumber(raw, 'latencyMs'),
    inputTokens: getNumber(raw, 'inputTokens'),
    outputTokens: getNumber(raw, 'outputTokens'),
    apiKeyName: getString(raw, 'apiKeyName'),
    apiKeyPrefix: getString(raw, 'apiKeyPrefix'),
    method: getString(raw, 'method'),
    path: getString(raw, 'path'),
  }
}

function normalizeSessionDetail(raw: unknown): SessionDetail {
  const source = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  const sessionRecord = getRecord(source, 'session', 'item') || source
  const summaryRecord = getRecord(source, 'summary')

  return {
    session: normalizeSessionItem(sessionRecord),
    timeline: getArray(source, 'timeline', 'requests', 'items').map(normalizeTimelineItem),
    distinctApiKeyCount: getNumber(source, 'distinctApiKeyCount') ?? getNumber(summaryRecord || {}, 'distinctApiKeyCount'),
    distinctModelCount: getNumber(source, 'distinctModelCount') ?? getNumber(summaryRecord || {}, 'distinctModelCount'),
    totalInputTokens: getNumber(source, 'totalInputTokens') ?? getNumber(summaryRecord || {}, 'totalInputTokens'),
    totalOutputTokens: getNumber(source, 'totalOutputTokens') ?? getNumber(summaryRecord || {}, 'totalOutputTokens'),
    totalCostUsd: getString(source, 'totalCostUsd') ?? getString(summaryRecord || {}, 'totalCostUsd'),
  }
}

function sortSessions(items: SessionListItem[]): SessionListItem[] {
  return [...items].sort((left, right) => {
    if (left.active !== right.active) {
      return Number(right.active) - Number(left.active)
    }

    const leftLast = left.lastSeenAt ? new Date(left.lastSeenAt).getTime() : 0
    const rightLast = right.lastSeenAt ? new Date(right.lastSeenAt).getTime() : 0
    return rightLast - leftLast
  })
}

export async function getAdminSessions(params: SessionListParams = {}, signal?: AbortSignal): Promise<SessionListResponse> {
  const searchParams = new URLSearchParams()
  if (params.page) searchParams.set('page', String(params.page))
  if (params.pageSize) searchParams.set('pageSize', String(params.pageSize))
  if (params.query) searchParams.set('query', params.query)
  if (params.activeOnly) searchParams.set('activeOnly', 'true')

  const query = searchParams.toString()
  const response = await authFetch(`${ADMIN_API_BASE}/sessions${query ? `?${query}` : ''}`, { signal })

  return handleResponse<SessionListResponse>(response, '获取 Session 列表失败', (data) => {
    const payload = (data && typeof data === 'object' ? data : {}) as Record<string, unknown>
    const items = sortSessions(getArray(payload, 'items', 'sessions').map(normalizeSessionItem))
    return {
      items,
      total: getNumber(payload, 'total') ?? items.length,
      page: getNumber(payload, 'page') ?? params.page ?? 1,
      pageSize: getNumber(payload, 'pageSize') ?? params.pageSize ?? items.length,
    }
  })
}

export async function getAdminSessionDetail(sessionId: string, signal?: AbortSignal): Promise<SessionDetail> {
  const response = await authFetch(`${ADMIN_API_BASE}/sessions/${encodeURIComponent(sessionId)}`, { signal })
  return handleResponse<SessionDetail>(response, '获取 Session 详情失败', normalizeSessionDetail)
}

export async function getAdminSessionLeaderboard(params: { windowMinutes?: number; limit?: number } = {}, signal?: AbortSignal): Promise<SessionLeaderboardItem[]> {
  const searchParams = new URLSearchParams()
  if (params.windowMinutes) searchParams.set('windowMinutes', String(params.windowMinutes))
  if (params.limit) searchParams.set('limit', String(params.limit))

  const query = searchParams.toString()
  const response = await authFetch(`${ADMIN_API_BASE}/sessions/leaderboard${query ? `?${query}` : ''}`, { signal })

  return handleResponse<SessionLeaderboardItem[]>(response, '获取 Session 排行失败', (data) => {
    const payload = (data && typeof data === 'object' ? data : {}) as Record<string, unknown>
    return getArray(payload, 'items', 'users', 'rankings').map((item) => ({
      userId: getString(item, 'userId'),
      username: getString(item, 'username', 'userName') || '-',
      distinctSessionCount: getNumber(item, 'distinctSessionCount', 'sessionCount', 'count') ?? 0,
    }))
  })
}
