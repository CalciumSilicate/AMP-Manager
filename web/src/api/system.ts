import { authFetch } from './client'

const API_BASE = '/api'

export interface DatabaseInfo {
  currentType: 'sqlite' | 'postgres'
  supportsFileBackups: boolean
  sqlitePath: string
  databaseURL: string
  databaseURLMasked: string
  archiveMode: string
}

export interface DatabaseMigrationTask {
  error?: string
  finishedAt?: string
  id: string
  logs: string[]
  message: string
  operation: string
  progress: number
  startedAt: string
  status: 'pending' | 'running' | 'succeeded' | 'failed'
  sourceType: 'sqlite' | 'postgres'
  targetType: 'sqlite' | 'postgres'
}

export interface StartDatabaseMigrationRequest {
  clearTarget: boolean
  targetDatabaseUrl: string
  targetSqlitePath: string
  targetType: 'sqlite' | 'postgres'
  withArchive: boolean
}

export async function getDatabaseInfo(): Promise<DatabaseInfo> {
  const res = await authFetch(`${API_BASE}/admin/system/database-info`)

  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '获取数据库信息失败')
  }

  return res.json()
}

export async function uploadDatabase(file: File): Promise<{ message: string; backupFile?: string }> {
  const formData = new FormData()
  formData.append('database', file)

  const res = await authFetch(`${API_BASE}/admin/system/database/upload`, {
    method: 'POST',
    body: formData,
  })

  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '上传失败')
  }

  return res.json()
}

export async function downloadDatabase(): Promise<void> {
  const res = await authFetch(`${API_BASE}/admin/system/database/download`)

  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '下载失败')
  }

  const blob = await res.blob()
  const url = window.URL.createObjectURL(blob)
  const contentDisposition = res.headers.get('content-disposition') || ''
  const filenameMatch = contentDisposition.match(/filename=([^;]+)/i)
  const filename = filenameMatch?.[1]?.replace(/^"|"$/g, '') || `ampmanager_${new Date().toISOString().slice(0, 10)}.db`
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  window.URL.revokeObjectURL(url)
  document.body.removeChild(a)
}

export async function startDatabaseMigration(payload: StartDatabaseMigrationRequest): Promise<DatabaseMigrationTask> {
  const res = await authFetch(`${API_BASE}/admin/system/database/migrate`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })

  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '启动数据库迁移失败')
  }

  return res.json()
}

export async function getDatabaseMigrationTask(taskID: string): Promise<DatabaseMigrationTask> {
  const res = await authFetch(`${API_BASE}/admin/system/database/migrate/${encodeURIComponent(taskID)}`)

  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '获取数据库迁移进度失败')
  }

  return res.json()
}

export interface Backup {
  filename: string
  size: number
  modTime: string
}

export async function listBackups(): Promise<Backup[]> {
  const res = await authFetch(`${API_BASE}/admin/system/database/backups`)

  if (!res.ok) {
    throw new Error('获取备份列表失败')
  }

  return res.json()
}

export async function restoreBackup(filename: string): Promise<{ message: string }> {
  const res = await authFetch(`${API_BASE}/admin/system/database/restore`, {
    method: 'POST',
    body: JSON.stringify({ filename }),
  })

  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '恢复失败')
  }

  return res.json()
}

export async function deleteBackup(filename: string): Promise<{ message: string }> {
  const res = await authFetch(`${API_BASE}/admin/system/database/backups/${encodeURIComponent(filename)}`, {
    method: 'DELETE',
  })

  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '删除失败')
  }

  return res.json()
}

// 重试配置接口
export interface RetryConfig {
  enabled: boolean
  maxAttempts: number
  gateTimeoutMs: number
  maxBodyBytes: number
  backoffBaseMs: number
  backoffMaxMs: number
  retryOn429: boolean
  retryOn5xx: boolean
  respectRetryAfter: boolean
}

// 获取重试配置
export async function getRetryConfig(): Promise<RetryConfig> {
  const res = await authFetch(`${API_BASE}/admin/system/retry-config`)

  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '获取配置失败')
  }

  return res.json()
}

// 更新重试配置
export async function updateRetryConfig(config: RetryConfig): Promise<{ message: string; config: RetryConfig }> {
  const res = await authFetch(`${API_BASE}/admin/system/retry-config`, {
    method: 'PUT',
    body: JSON.stringify(config),
  })

  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '更新配置失败')
  }

  return res.json()
}

// 获取请求详情监控状态
export async function getRequestDetailEnabled(): Promise<{ enabled: boolean }> {
  const res = await authFetch(`${API_BASE}/admin/system/request-detail-enabled`)

  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '获取配置失败')
  }

  return res.json()
}

// 更新请求详情监控状态
export async function updateRequestDetailEnabled(enabled: boolean): Promise<{ message: string; enabled: boolean }> {
  const res = await authFetch(`${API_BASE}/admin/system/request-detail-enabled`, {
    method: 'PUT',
    body: JSON.stringify({ enabled }),
  })

  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '更新配置失败')
  }

  return res.json()
}

// 超时配置接口
export interface TimeoutConfig {
  idleConnTimeoutSec: number
  readIdleTimeoutSec: number
  keepAliveIntervalSec: number
  dialTimeoutSec: number
  tlsHandshakeTimeoutSec: number
}

// 获取超时配置
export async function getTimeoutConfig(): Promise<TimeoutConfig> {
  const res = await authFetch(`${API_BASE}/admin/system/timeout-config`)

  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '获取配置失败')
  }

  return res.json()
}

// 更新超时配置
export async function updateTimeoutConfig(config: TimeoutConfig): Promise<{ message: string; config: TimeoutConfig }> {
  const res = await authFetch(`${API_BASE}/admin/system/timeout-config`, {
    method: 'PUT',
    body: JSON.stringify(config),
  })

  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '更新配置失败')
  }

  return res.json()
}

// 缓存 TTL 配置
export async function getCacheTTLConfig(): Promise<{ cacheTTL: string }> {
  const res = await authFetch(`${API_BASE}/admin/system/cache-ttl`)

  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '获取配置失败')
  }

  return res.json()
}

export async function updateCacheTTLConfig(cacheTTL: string): Promise<{ message: string; cacheTTL: string }> {
  const res = await authFetch(`${API_BASE}/admin/system/cache-ttl`, {
    method: 'PUT',
    body: JSON.stringify({ cacheTTL }),
  })

  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '更新配置失败')
  }

  return res.json()
}

export interface SiteConfig {
  siteName: string
}

export async function getPublicSiteConfig(): Promise<SiteConfig> {
  const res = await fetch(`${API_BASE}/public/site-config`)

  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error((data as { error?: string }).error || '获取站点配置失败')
  }

  return res.json()
}

export async function getSiteConfig(): Promise<SiteConfig> {
  const res = await authFetch(`${API_BASE}/admin/system/site-config`)

  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '获取站点配置失败')
  }

  return res.json()
}

export async function updateSiteConfig(siteName: string): Promise<{ message: string; config: SiteConfig }> {
  const res = await authFetch(`${API_BASE}/admin/system/site-config`, {
    method: 'PUT',
    body: JSON.stringify({ siteName }),
  })

  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '更新站点配置失败')
  }

  return res.json()
}

export interface BillingRuntimeConfig {
  redisUrl: string
  redisUrlMasked: string
  redisPrefix: string
  reservationTtlSec: number
  reconcileIntervalSec: number
  streamBatchSize: number
  runtimeEnabled: boolean
  runtimeHealthy: boolean
}

export interface BillingRuntimeConfigRequest {
  redisUrl: string
  redisPrefix: string
  reservationTtlSec: number
  reconcileIntervalSec: number
  streamBatchSize: number
}

export async function getBillingRuntimeConfig(): Promise<BillingRuntimeConfig> {
  const res = await authFetch(`${API_BASE}/admin/system/billing-runtime-config`)

  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '获取 Redis 计费配置失败')
  }

  return res.json()
}

export async function updateBillingRuntimeConfig(
  config: BillingRuntimeConfigRequest
): Promise<{ message: string; config: BillingRuntimeConfig }> {
  const res = await authFetch(`${API_BASE}/admin/system/billing-runtime-config`, {
    method: 'PUT',
    body: JSON.stringify(config),
  })

  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '更新 Redis 计费配置失败')
  }

  return res.json()
}
