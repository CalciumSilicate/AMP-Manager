import { useEffect, useMemo, useState } from 'react'

import {
  createManagementAPIKey,
  getManagementAPIKeyStatus,
  getUserPanelRateLimitConfig,
  revealManagementAPIKey,
  rotateManagementAPIKey,
  updateManagementAPIKeyEnabled,
  updateUserPanelRateLimitConfig,
  type ManagementAPIKeyRevealResponse,
  type ManagementAPIKeyStatus,
  type UserPanelRateLimitConfig,
  type UserPanelRateLimitSections,
} from '@/api/system'
import { formatDateTime } from '@/lib/formatters'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { KeyRound, RefreshCw } from 'lucide-react'

type MessageFn = (type: 'success' | 'error', text: string) => void
type PasswordAction = 'reveal' | 'rotate' | null

interface Props {
  onMessage: MessageFn
}

const sectionMeta: Array<{ key: keyof UserPanelRateLimitSections; label: string; description: string }> = [
  { key: 'overviewStatus', label: '概览 + 状态监控', description: '概览页、状态监控和公告读取。' },
  { key: 'ampSettings', label: '路由设置', description: 'Amp 设置读取、保存和连接测试。' },
  { key: 'apiKeys', label: 'API Key 管理', description: '普通用户 API Key 列表、查看、创建、修改和删除。' },
  { key: 'requestLogsUsage', label: '请求日志 + 使用量统计', description: '日志列表、详情和使用量统计。' },
  { key: 'models', label: '可用模型', description: '用户可见模型列表。' },
  { key: 'accountPurchase', label: '账户设置 + 购买订阅', description: '余额、计费、密码、用户名、兑换和购买相关接口。' },
]

function emptyRateLimitConfig(): UserPanelRateLimitConfig {
  return {
    enabled: true,
    backend: 'redis',
    failOpen: true,
    sections: {
      overviewStatus: { rps: 1, burst: 3, maxWaitMs: 4500 },
      ampSettings: { rps: 0.5, burst: 2, maxWaitMs: 9000 },
      apiKeys: { rps: 0.5, burst: 2, maxWaitMs: 9000 },
      requestLogsUsage: { rps: 0.2, burst: 1, maxWaitMs: 15000 },
      models: { rps: 1, burst: 2, maxWaitMs: 3000 },
      accountPurchase: { rps: 0.5, burst: 2, maxWaitMs: 12000 },
    },
  }
}

function formatAuthMethod(value?: string) {
  switch (value) {
    case 'x-api-key':
      return 'X-API-Key'
    case 'bearer':
      return 'Bearer'
    case 'jwt':
      return 'JWT'
    case 'jwt_query':
      return 'JWT Query'
    default:
      return '-'
  }
}

export function SecuritySettingsPanel({ onMessage }: Props) {
  const [managementStatus, setManagementStatus] = useState<ManagementAPIKeyStatus | null>(null)
  const [rateLimitConfig, setRateLimitConfig] = useState<UserPanelRateLimitConfig>(emptyRateLimitConfig())
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [busyAction, setBusyAction] = useState<string | null>(null)
  const [passwordAction, setPasswordAction] = useState<PasswordAction>(null)
  const [currentPassword, setCurrentPassword] = useState('')
  const [revealedKey, setRevealedKey] = useState<ManagementAPIKeyRevealResponse | null>(null)
  const [copied, setCopied] = useState(false)

  const graceLabel = useMemo(() => {
    if (!managementStatus?.graceUntil) return '-'
    return formatDateTime(managementStatus.graceUntil)
  }, [managementStatus?.graceUntil])

  useEffect(() => {
    void loadData()
  }, [])

  const loadData = async (silent = false) => {
    if (silent) {
      setRefreshing(true)
    } else {
      setLoading(true)
    }
    try {
      const [status, rateLimit] = await Promise.all([
        getManagementAPIKeyStatus(),
        getUserPanelRateLimitConfig(),
      ])
      setManagementStatus(status)
      setRateLimitConfig(rateLimit)
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '加载安全设置失败')
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }

  const handleCreate = async () => {
    setBusyAction('create')
    try {
      const result = await createManagementAPIKey()
      setRevealedKey(result)
      await loadData(true)
      onMessage('success', '管理 API Key 已创建')
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '创建管理 API Key 失败')
    } finally {
      setBusyAction(null)
    }
  }

  const handleSubmitPasswordAction = async () => {
    if (!passwordAction) return
    setBusyAction(passwordAction)
    try {
      const result = passwordAction === 'reveal'
        ? await revealManagementAPIKey(currentPassword)
        : await rotateManagementAPIKey(currentPassword)
      setCurrentPassword('')
      setPasswordAction(null)
      setRevealedKey(result)
      await loadData(true)
      onMessage('success', passwordAction === 'reveal' ? '管理 API Key 已显示' : '管理 API Key 已轮换')
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '操作失败')
    } finally {
      setBusyAction(null)
    }
  }

  const handleToggleEnabled = async (enabled: boolean) => {
    setBusyAction('toggle')
    try {
      const result = await updateManagementAPIKeyEnabled(enabled)
      setManagementStatus(result.status)
      onMessage('success', enabled ? '管理 API Key 已启用' : '管理 API Key 已停用')
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '更新状态失败')
    } finally {
      setBusyAction(null)
    }
  }

  const handleSectionFieldChange = (
    sectionKey: keyof UserPanelRateLimitSections,
    field: 'rps' | 'burst' | 'maxWaitMs',
    value: number,
  ) => {
    setRateLimitConfig((current) => ({
      ...current,
      sections: {
        ...current.sections,
        [sectionKey]: {
          ...current.sections[sectionKey],
          [field]: value,
        },
      },
    }))
  }

  const handleSaveRateLimit = async () => {
    setBusyAction('save-rate-limit')
    try {
      const result = await updateUserPanelRateLimitConfig({
        enabled: rateLimitConfig.enabled,
        sections: rateLimitConfig.sections,
      })
      setRateLimitConfig(result.config)
      onMessage('success', '普通用户面板限流配置已保存')
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '保存限流配置失败')
    } finally {
      setBusyAction(null)
    }
  }

  const handleCopy = async () => {
    if (!revealedKey?.apiKey) return
    await navigator.clipboard.writeText(revealedKey.apiKey)
    setCopied(true)
    window.setTimeout(() => setCopied(false), 2000)
  }

  if (loading) {
    return <div className="py-10 text-center text-sm text-muted-foreground">加载中...</div>
  }

  return (
    <div className="space-y-6">
      {!managementStatus?.encryptionReady ? (
        <Alert variant="destructive">
          <AlertDescription>未配置 `DATA_ENCRYPTION_KEY`，当前只能查看状态，无法创建、查看或轮换管理 API Key。</AlertDescription>
        </Alert>
      ) : null}

      <Card>
        <CardHeader>
          <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
            <div className="space-y-1">
              <CardTitle>管理 API Key</CardTitle>
              <CardDescription>每个管理员账号一个管理 API Key，可用 Bearer 或 X-API-Key 调用后台管理接口。</CardDescription>
            </div>
            <Button type="button" variant="outline" size="sm" onClick={() => void loadData(true)} disabled={refreshing}>
              <RefreshCw className={`mr-2 h-4 w-4 ${refreshing ? 'animate-spin' : ''}`} />
              刷新
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            <div className="rounded-lg border p-4 space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-sm text-muted-foreground">状态</span>
                <Badge variant={managementStatus?.enabled ? 'default' : 'secondary'}>
                  {managementStatus?.keyExists ? (managementStatus.enabled ? '已启用' : '已停用') : '未创建'}
                </Badge>
              </div>
              <div className="flex items-center gap-2 text-lg font-semibold">
                <KeyRound className="h-4 w-4" />
                <span>{managementStatus?.keyExists ? managementStatus.prefix : '-'}</span>
              </div>
              <p className="text-xs text-muted-foreground">请求头支持 `Authorization: Bearer &lt;key&gt;` 或 `X-API-Key: &lt;key&gt;`。</p>
            </div>
            <div className="rounded-lg border p-4 space-y-2">
              <div className="text-sm text-muted-foreground">最近使用</div>
              <div className="text-sm font-medium">{managementStatus?.lastUsedAt ? formatDateTime(managementStatus.lastUsedAt) : '-'}</div>
              <div className="text-xs text-muted-foreground">最近鉴权方式：{formatAuthMethod(managementStatus?.lastAuthMethod)}</div>
            </div>
            <div className="rounded-lg border p-4 space-y-2">
              <div className="text-sm text-muted-foreground">生命周期</div>
              <div className="text-xs text-muted-foreground">创建：{managementStatus?.createdAt ? formatDateTime(managementStatus.createdAt) : '-'}</div>
              <div className="text-xs text-muted-foreground">轮换：{managementStatus?.rotatedAt ? formatDateTime(managementStatus.rotatedAt) : '-'}</div>
              <div className="text-xs text-muted-foreground">旧 Key 重叠到：{graceLabel}</div>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-3">
            {!managementStatus?.keyExists ? (
              <Button type="button" onClick={() => void handleCreate()} disabled={busyAction === 'create' || !managementStatus?.encryptionReady}>
                {busyAction === 'create' ? '创建中...' : '创建管理 API Key'}
              </Button>
            ) : (
              <>
                <Button type="button" variant="outline" onClick={() => setPasswordAction('reveal')} disabled={busyAction === 'reveal' || !managementStatus.encryptionReady}>
                  查看明文
                </Button>
                <Button type="button" onClick={() => setPasswordAction('rotate')} disabled={busyAction === 'rotate' || !managementStatus.encryptionReady}>
                  轮换 Key
                </Button>
                <div className="flex items-center gap-3 rounded-lg border px-4 py-2">
                  <div className="space-y-0.5">
                    <div className="text-sm font-medium">启用</div>
                    <div className="text-xs text-muted-foreground">停用后当前 Key 和 grace key 会一起失效</div>
                  </div>
                  <Switch
                    checked={managementStatus.enabled}
                    onCheckedChange={(checked) => void handleToggleEnabled(checked)}
                    disabled={busyAction === 'toggle'}
                  />
                </div>
              </>
            )}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>普通用户面板限流</CardTitle>
          <CardDescription>仅作用于普通用户；管理员完全不受限。Redis 异常时自动放行，请求在超出速率后会先静默等待，超时再返回 429。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between rounded-lg border px-4 py-3">
            <div className="space-y-1">
              <Label htmlFor="user-panel-rate-limit-enabled">启用限流</Label>
              <p className="text-xs text-muted-foreground">后端固定使用 {rateLimitConfig.backend || 'redis'}，故障放行固定为开启。</p>
            </div>
            <Switch
              id="user-panel-rate-limit-enabled"
              checked={rateLimitConfig.enabled}
              onCheckedChange={(checked) => setRateLimitConfig((current) => ({ ...current, enabled: checked }))}
            />
          </div>

          <div className="space-y-3">
            {sectionMeta.map((section) => {
              const value = rateLimitConfig.sections[section.key]
              return (
                <div key={section.key} className="rounded-lg border p-4 space-y-3">
                  <div className="space-y-1">
                    <div className="font-medium">{section.label}</div>
                    <div className="text-xs text-muted-foreground">{section.description}</div>
                  </div>
                  <div className="grid gap-4 md:grid-cols-3">
                    <div className="space-y-2">
                      <Label>RPS</Label>
                      <Input
                        type="number"
                        min="0.1"
                        step="0.1"
                        value={value.rps}
                        onChange={(event) => handleSectionFieldChange(section.key, 'rps', Number.parseFloat(event.target.value) || 0)}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label>Burst</Label>
                      <Input
                        type="number"
                        min="1"
                        step="1"
                        value={value.burst}
                        onChange={(event) => handleSectionFieldChange(section.key, 'burst', Number.parseInt(event.target.value || '1', 10))}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label>maxWaitMs</Label>
                      <Input
                        type="number"
                        min="0"
                        step="100"
                        value={value.maxWaitMs}
                        onChange={(event) => handleSectionFieldChange(section.key, 'maxWaitMs', Number.parseInt(event.target.value || '0', 10))}
                      />
                    </div>
                  </div>
                </div>
              )
            })}
          </div>

          <div className="flex justify-end">
            <Button type="button" onClick={() => void handleSaveRateLimit()} disabled={busyAction === 'save-rate-limit'}>
              {busyAction === 'save-rate-limit' ? '保存中...' : '保存限流配置'}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Dialog open={passwordAction !== null} onOpenChange={(open) => {
        if (!open) {
          setPasswordAction(null)
          setCurrentPassword('')
        }
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{passwordAction === 'rotate' ? '轮换管理 API Key' : '查看管理 API Key'}</DialogTitle>
            <DialogDescription>请输入当前管理员密码以继续。</DialogDescription>
          </DialogHeader>
          <div className="space-y-2 py-2">
            <Label htmlFor="management-key-password">当前密码</Label>
            <Input
              id="management-key-password"
              type="password"
              autoComplete="current-password"
              value={currentPassword}
              onChange={(event) => setCurrentPassword(event.target.value)}
            />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => {
              setPasswordAction(null)
              setCurrentPassword('')
            }}>
              取消
            </Button>
            <Button type="button" onClick={() => void handleSubmitPasswordAction()} disabled={!currentPassword || busyAction === passwordAction}>
              {busyAction === passwordAction ? '处理中...' : '继续'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={revealedKey !== null} onOpenChange={(open) => {
        if (!open) {
          setRevealedKey(null)
          setCopied(false)
        }
      }}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>管理 API Key</DialogTitle>
            <DialogDescription>请尽快复制保存。旧 Key 重叠窗口结束后会自动失效。</DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label>明文</Label>
            <Input readOnly value={revealedKey?.apiKey || ''} />
            <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
              <span>前缀：{revealedKey?.prefix || '-'}</span>
              <span>Grace Until：{revealedKey?.graceUntil ? formatDateTime(revealedKey.graceUntil) : '-'}</span>
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => {
              setRevealedKey(null)
              setCopied(false)
            }}>
              关闭
            </Button>
            <Button type="button" onClick={() => void handleCopy()}>
              {copied ? '已复制' : '复制'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
