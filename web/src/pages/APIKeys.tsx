import { type ReactNode, useEffect, useState } from 'react'

import { AnimatePresence, motion, tableRowVariants, tableStaggerContainer } from '@/lib/motion'
import {
  APIKey,
  APIKeyRevealResponse,
  CreateAPIKeyResponse,
  createAPIKeyWithOptions,
  deleteAPIKey,
  getAPIKey,
  getAPIKeys,
  setAPIKeyDisabled,
  updateAPIKey,
} from '../api/amp'
import { AdminPageShell, AdminSurface } from '@/components/admin/AdminPageShell'
import { OverflowCopyText } from '@/components/OverflowCopyText'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DateTimePicker } from '@/components/ui/datetime-picker'
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
import { Table, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { APIKeyUsageDialog } from '@/components/api-keys/APIKeyUsageDialog'
import { APIKeyChannelTargetsEditor } from '@/components/api-keys/APIKeyChannelTargetsEditor'
import { formatDateTime } from '@/lib/formatters'
import { Info } from 'lucide-react'

function buildRandomSkKey() {
  const bytes = new Uint8Array(16)
  crypto.getRandomValues(bytes)
  const suffix = Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')
  return `sk-${suffix}`
}

interface Props {
  siteName: string
}

function MobileInfoRow({
  label,
  value,
}: {
  label: string
  value: ReactNode
}) {
  return (
    <div className="flex items-start justify-between gap-3">
      <span className="text-muted-foreground">{label}</span>
      <div className="min-w-0 text-right text-foreground">{value}</div>
    </div>
  )
}

function CircuitBreakerFieldLabel({ label, description }: { label: string; description: ReactNode }) {
  return (
    <div className="flex items-center gap-1.5">
      <span>{label}</span>
      <Tooltip>
        <TooltipTrigger asChild>
          <button type="button" className="text-muted-foreground transition-colors hover:text-foreground" aria-label={`${label}说明`}>
            <Info className="h-3.5 w-3.5" />
          </button>
        </TooltipTrigger>
        <TooltipContent side="top" className="max-w-72 border bg-popover px-3 py-2 text-xs text-popover-foreground shadow-md">
          {description}
        </TooltipContent>
      </Tooltip>
    </div>
  )
}

function parsePositiveIntegerOrFallback(value: string, fallback: number) {
  const parsed = Number.parseInt(value, 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback
}

export default function APIKeys({ siteName }: Props) {
  const [keys, setKeys] = useState<APIKey[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [createName, setCreateName] = useState('')
  const [customKey, setCustomKey] = useState('')
  const [createExpiresAt, setCreateExpiresAt] = useState('')
  const [createCircuitBreakerThreshold, setCreateCircuitBreakerThreshold] = useState('50')
  const [createCircuitBreakerOpenMinutes, setCreateCircuitBreakerOpenMinutes] = useState('10')
  const [createCircuitBreakerHalfOpenMinutes, setCreateCircuitBreakerHalfOpenMinutes] = useState('2')
  const [createSplitChannelTargetsBySource, setCreateSplitChannelTargetsBySource] = useState(false)
  const [createChannelTargets, setCreateChannelTargets] = useState<{ channelId: string; priority: number }[]>([])
  const [createSubscriptionChannelTargets, setCreateSubscriptionChannelTargets] = useState<{ channelId: string; priority: number }[]>([])
  const [createUsageChannelTargets, setCreateUsageChannelTargets] = useState<{ channelId: string; priority: number }[]>([])
  const [creating, setCreating] = useState(false)
  const [newKey, setNewKey] = useState<CreateAPIKeyResponse | null>(null)
  const [revealKey, setRevealKey] = useState<APIKeyRevealResponse | null>(null)
  const [usageKey, setUsageKey] = useState<APIKeyRevealResponse | null>(null)
  const [editingKey, setEditingKey] = useState<APIKey | null>(null)
  const [editName, setEditName] = useState('')
  const [editExpiresAt, setEditExpiresAt] = useState('')
  const [editCircuitBreakerThreshold, setEditCircuitBreakerThreshold] = useState('50')
  const [editCircuitBreakerOpenMinutes, setEditCircuitBreakerOpenMinutes] = useState('10')
  const [editCircuitBreakerHalfOpenMinutes, setEditCircuitBreakerHalfOpenMinutes] = useState('2')
  const [editSplitChannelTargetsBySource, setEditSplitChannelTargetsBySource] = useState(false)
  const [editChannelTargets, setEditChannelTargets] = useState<{ channelId: string; priority: number }[]>([])
  const [editSubscriptionChannelTargets, setEditSubscriptionChannelTargets] = useState<{ channelId: string; priority: number }[]>([])
  const [editUsageChannelTargets, setEditUsageChannelTargets] = useState<{ channelId: string; priority: number }[]>([])
  const [savingEdit, setSavingEdit] = useState(false)
  const [copied, setCopied] = useState<string | null>(null)
  const [revealingId, setRevealingId] = useState<string | null>(null)
  const [usingId, setUsingId] = useState<string | null>(null)
  const [deletingId, setDeletingId] = useState<string | null>(null)
  const [togglingId, setTogglingId] = useState<string | null>(null)

  const apiBaseUrl = `${window.location.origin}/v1`
  const apiKeyPattern = /^(?:sk-)?[A-Za-z0-9]{16,}$/

  useEffect(() => {
    void loadData()
  }, [])

  const loadData = async () => {
    try {
      const keysData = await getAPIKeys()
      setKeys(keysData)
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }

  const handleCreate = async () => {
    if (!createName.trim()) return
    if (customKey && !apiKeyPattern.test(customKey)) {
      setError('自定义 API Key 只能包含字母和数字，且长度至少为 16')
      return
    }

    setCreating(true)
    setError('')

    try {
      const result = await createAPIKeyWithOptions({
        name: createName.trim(),
        ...(customKey ? { customKey } : {}),
        ...(createExpiresAt ? { expiresAt: createExpiresAt } : {}),
        circuitBreakerThreshold: parsePositiveIntegerOrFallback(createCircuitBreakerThreshold, 50),
        circuitBreakerOpenMinutes: parsePositiveIntegerOrFallback(createCircuitBreakerOpenMinutes, 10),
        circuitBreakerHalfOpenMinutes: parsePositiveIntegerOrFallback(createCircuitBreakerHalfOpenMinutes, 2),
        splitChannelTargetsBySource: createSplitChannelTargetsBySource,
        channelTargets: createChannelTargets,
        subscriptionChannelTargets: createSubscriptionChannelTargets,
        usageChannelTargets: createUsageChannelTargets,
      })
      setNewKey(result)
      setCreateName('')
      setCustomKey('')
      setCreateExpiresAt('')
      setCreateCircuitBreakerThreshold('50')
      setCreateCircuitBreakerOpenMinutes('10')
      setCreateCircuitBreakerHalfOpenMinutes('2')
      setCreateSplitChannelTargetsBySource(false)
      setCreateChannelTargets([])
      setCreateSubscriptionChannelTargets([])
      setCreateUsageChannelTargets([])
      setShowCreate(false)
      await loadData()
    } catch (err) {
      setError(err instanceof Error ? err.message : '创建失败')
    } finally {
      setCreating(false)
    }
  }

  const handleToggleDisabled = async (key: APIKey) => {
    setTogglingId(key.id)
    setError('')

    try {
      await setAPIKeyDisabled(key.id, key.status !== 'disabled')
      await loadData()
    } catch (err) {
      setError(err instanceof Error ? err.message : '更新状态失败')
    } finally {
      setTogglingId(null)
    }
  }

  const handleDelete = async (id: string, name: string) => {
    if (!confirm(`确定要删除 API Key "${name}" 吗？此操作不可恢复。`)) return

    setDeletingId(id)
    setError('')

    try {
      await deleteAPIKey(id)
      await loadData()
    } catch (err) {
      setError(err instanceof Error ? err.message : '删除失败')
    } finally {
      setDeletingId(null)
    }
  }

  const handleReveal = async (id: string) => {
    setRevealingId(id)
    setError('')

    try {
      const result = await getAPIKey(id)
      setRevealKey(result)
    } catch (err) {
      setError(err instanceof Error ? err.message : '获取失败')
    } finally {
      setRevealingId(null)
    }
  }

  const handleOpenUsage = async (id: string) => {
    setUsingId(id)
    setError('')

    try {
      const result = await getAPIKey(id)
      setUsageKey(result)
    } catch (err) {
      setError(err instanceof Error ? err.message : '获取失败')
    } finally {
      setUsingId(null)
    }
  }

  const handleOpenEdit = (key: APIKey) => {
    setEditingKey(key)
    setEditName(key.name)
    setEditExpiresAt(key.expiresAt || '')
    setEditCircuitBreakerThreshold(String(key.circuitBreakerThreshold))
    setEditCircuitBreakerOpenMinutes(String(key.circuitBreakerOpenMinutes))
    setEditCircuitBreakerHalfOpenMinutes(String(key.circuitBreakerHalfOpenMinutes))
    setEditSplitChannelTargetsBySource(key.splitChannelTargetsBySource)
    setEditChannelTargets(key.channelTargets || [])
    setEditSubscriptionChannelTargets(key.subscriptionChannelTargets || [])
    setEditUsageChannelTargets(key.usageChannelTargets || [])
  }

  const handleSaveEdit = async () => {
    if (!editingKey || !editName.trim()) return

    setSavingEdit(true)
    setError('')

    try {
      await updateAPIKey(editingKey.id, {
        name: editName.trim(),
        ...(editExpiresAt ? { expiresAt: editExpiresAt } : { clearExpiry: true }),
        circuitBreakerThreshold: parsePositiveIntegerOrFallback(editCircuitBreakerThreshold, editingKey.circuitBreakerThreshold),
        circuitBreakerOpenMinutes: parsePositiveIntegerOrFallback(editCircuitBreakerOpenMinutes, editingKey.circuitBreakerOpenMinutes),
        circuitBreakerHalfOpenMinutes: parsePositiveIntegerOrFallback(editCircuitBreakerHalfOpenMinutes, editingKey.circuitBreakerHalfOpenMinutes),
        splitChannelTargetsBySource: editSplitChannelTargetsBySource,
        channelTargets: editChannelTargets,
        subscriptionChannelTargets: editSubscriptionChannelTargets,
        usageChannelTargets: editUsageChannelTargets,
      })
      setEditingKey(null)
      setEditName('')
      setEditExpiresAt('')
      setEditCircuitBreakerThreshold('50')
      setEditCircuitBreakerOpenMinutes('10')
      setEditCircuitBreakerHalfOpenMinutes('2')
      setEditSplitChannelTargetsBySource(false)
      setEditChannelTargets([])
      setEditSubscriptionChannelTargets([])
      setEditUsageChannelTargets([])
      await loadData()
    } catch (err) {
      setError(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSavingEdit(false)
    }
  }

  const copyToClipboard = async (text: string, type: string) => {
    await navigator.clipboard.writeText(text)
    setCopied(type)
    setTimeout(() => setCopied(null), 2000)
  }

  const formatDate = (dateStr?: string | null) => (dateStr ? formatDateTime(dateStr) : '永不过期')
  const formatUsedAt = (dateStr?: string | null) => (dateStr ? formatDateTime(dateStr) : '-')
  const getCircuitBreakerLabel = (state: APIKey['circuitBreakerState']) => {
    switch (state) {
      case 'open':
        return '已熔断'
      case 'half_open':
        return '半开'
      default:
        return '关闭'
    }
  }
  const getCircuitBreakerVariant = (state: APIKey['circuitBreakerState']): 'default' | 'secondary' | 'destructive' | 'outline' => {
    switch (state) {
      case 'open':
        return 'destructive'
      case 'half_open':
        return 'outline'
      default:
        return 'secondary'
    }
  }
  const formatCircuitBreakerWindow = (key: Pick<APIKey, 'circuitBreakerOpenedAt' | 'circuitBreakerHalfOpenStartedAt' | 'circuitBreakerState'>) => {
    if (key.circuitBreakerState === 'open' && key.circuitBreakerOpenedAt) {
      return `熔断于 ${formatDateTime(key.circuitBreakerOpenedAt)}`
    }
    if (key.circuitBreakerState === 'half_open' && key.circuitBreakerHalfOpenStartedAt) {
      return `半开开始 ${formatDateTime(key.circuitBreakerHalfOpenStartedAt)}`
    }
    return '未触发'
  }
  const getStatusLabel = (status: APIKey['status']) => {
    switch (status) {
      case 'disabled':
        return '已禁用'
      case 'expired':
        return '已过期'
      default:
        return '生效中'
    }
  }

  const getStatusVariant = (status: APIKey['status']): 'default' | 'secondary' => {
    switch (status) {
      case 'active':
        return 'default'
      default:
        return 'secondary'
    }
  }

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <div className="text-muted-foreground">加载中...</div>
      </div>
    )
  }

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
      <TooltipProvider delayDuration={0}>
        <AdminPageShell
          title="API Key 管理"
          description="管理用于认证的 API Key"
          width="5xl"
          actions={<Button onClick={() => setShowCreate(true)}>创建 API Key</Button>}
        >
        <AnimatePresence>
          {newKey ? (
            <motion.div
              initial={{ opacity: 0, y: -20, scale: 0.98 }}
              animate={{ opacity: 1, y: 0, scale: 1 }}
              exit={{ opacity: 0, y: -20, scale: 0.98 }}
              transition={{ type: 'spring', bounce: 0.2, duration: 0.45 }}
              className="space-y-3 rounded-lg border border-emerald-500/30 bg-emerald-500/5 px-5 py-4"
            >
              <div className="flex items-center justify-between gap-3">
                <div className="space-y-1">
                  <p className="text-sm font-semibold text-emerald-700 dark:text-emerald-300">API Key 创建成功</p>
                  <p className="text-xs text-muted-foreground">明文仍可在列表中查看；建议立即复制并配置到你的客户端。</p>
                </div>
                <Button variant="ghost" size="sm" onClick={() => setNewKey(null)}>
                  关闭
                </Button>
              </div>
                <div className="space-y-3">
                  <div className="space-y-2">
                    <Label>API Base URL</Label>
                    <div className="flex flex-col gap-2 sm:flex-row">
                      <code className="min-w-0 flex-1 rounded-md border bg-background px-3 py-2 text-sm font-mono break-all">
                        {apiBaseUrl}
                      </code>
                      <Button size="sm" variant="outline" onClick={() => void copyToClipboard(apiBaseUrl, 'apiBaseUrl')}>
                        {copied === 'apiBaseUrl' ? '已复制' : '复制'}
                      </Button>
                    </div>
                  </div>
                  <div className="space-y-2">
                    <Label>API Key</Label>
                  <div className="flex flex-col gap-2 sm:flex-row">
                    <code className="min-w-0 flex-1 rounded-md border bg-background px-3 py-2 text-sm font-mono break-all">
                      {newKey.apiKey}
                    </code>
                    <Button size="sm" onClick={() => void copyToClipboard(newKey.apiKey, 'apiKey')}>
                      {copied === 'apiKey' ? '已复制' : '复制'}
                    </Button>
                  </div>
                </div>
                <div className="rounded-lg border px-4 py-3 text-sm">
                  <p className="text-muted-foreground">连接地址</p>
                  <p className="mt-1 break-all font-mono text-xs text-foreground">{apiBaseUrl}</p>
                  <p className="mt-3 text-muted-foreground">认证方式</p>
                  <p className="mt-1 text-xs">在客户端中使用以上 Base URL，并将当前 API Key 作为 Bearer Token 或 `X-Api-Key` 传入。</p>
                </div>
              </div>
            </motion.div>
          ) : null}
        </AnimatePresence>

        <motion.div initial={{ opacity: 0, y: 24 }} animate={{ opacity: 1, y: 0 }} transition={{ type: 'spring', bounce: 0.2, duration: 0.55 }}>
          <AdminSurface>
            {error ? (
              <div className="admin-surface-body pb-0">
                <Alert variant="destructive">
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              </div>
            ) : null}
            {keys.length === 0 ? (
              <div className="admin-surface-body">
                <div className="rounded-md border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">
                  暂无 API Key
                </div>
              </div>
            ) : (
              <motion.div initial={{ opacity: 0, y: 18 }} animate={{ opacity: 1, y: 0 }} transition={{ type: 'spring', bounce: 0.2, duration: 0.45 }}>
                <div className="admin-surface-header">
                  <div className="space-y-1">
                    <p className="text-sm font-medium text-foreground">已创建 {keys.length} 个 Key</p>
                    <p className="admin-inline-note">支持禁用/恢复、过期时间管理、明文查看、熔断状态查看和删除操作。</p>
                  </div>
                </div>
                <div className="overflow-hidden rounded-b-lg">
                  <div className="space-y-3 p-4 md:hidden">
                    {keys.map((key) => (
                      <div key={key.id} className="rounded-xl border border-border/70 px-4 py-4">
                        <div className="flex items-start justify-between gap-3">
                          <div className="min-w-0">
                            <p className="font-medium">{key.name}</p>
                            <p className="mt-1 font-mono text-xs text-muted-foreground">{key.prefix}...</p>
                          </div>
                          <Badge variant={getStatusVariant(key.status)}>
                            {getStatusLabel(key.status)}
                          </Badge>
                        </div>

                        <div className="mt-3 grid gap-2 text-sm">
                          <MobileInfoRow label="熔断状态" value={<Badge variant={getCircuitBreakerVariant(key.circuitBreakerState)}>{getCircuitBreakerLabel(key.circuitBreakerState)}</Badge>} />
                          <MobileInfoRow label="熔断窗口" value={formatCircuitBreakerWindow(key)} />
                          <MobileInfoRow label="到期时间" value={formatDate(key.expiresAt)} />
                          <MobileInfoRow label="最后使用" value={formatUsedAt(key.lastUsedAt)} />
                        </div>

                        <div className="mt-3 flex flex-wrap gap-2">
                          <Button variant="outline" size="sm" onClick={() => handleOpenEdit(key)}>
                            编辑
                          </Button>
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => void handleToggleDisabled(key)}
                            disabled={togglingId === key.id}
                          >
                            {togglingId === key.id ? '处理中...' : key.status === 'disabled' ? '恢复' : '禁用'}
                          </Button>
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => void handleOpenUsage(key.id)}
                            disabled={key.status !== 'active' || usingId === key.id}
                          >
                            {usingId === key.id ? '加载中...' : '使用'}
                          </Button>
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => void handleReveal(key.id)}
                            disabled={revealingId === key.id}
                          >
                            {revealingId === key.id ? '加载中...' : '查看'}
                          </Button>
                          <Button
                            variant="outline"
                            size="sm"
                            className="text-destructive hover:text-destructive"
                            onClick={() => void handleDelete(key.id, key.name)}
                            disabled={deletingId === key.id}
                          >
                            {deletingId === key.id ? '删除中...' : '删除'}
                          </Button>
                        </div>
                      </div>
                    ))}
                  </div>

                  <Table className="hidden md:table">
                    <TableHeader>
                        <TableRow>
                          <TableHead>名称</TableHead>
                          <TableHead>Prefix</TableHead>
                          <TableHead>状态</TableHead>
                          <TableHead>熔断</TableHead>
                          <TableHead>到期时间</TableHead>
                          <TableHead>最后使用</TableHead>
                          <TableHead className="text-right">操作</TableHead>
                      </TableRow>
                    </TableHeader>
                    <motion.tbody variants={tableStaggerContainer} initial="hidden" animate="visible" key={keys.length}>
                      {keys.map((key) => (
                        <motion.tr key={key.id} variants={tableRowVariants}>
                          <TableCell className="max-w-[220px]">
                            <OverflowCopyText text={key.name} className="max-w-[220px] font-medium" />
                          </TableCell>
                          <TableCell className="font-mono text-muted-foreground">{key.prefix}...</TableCell>
                          <TableCell>
                            <Badge variant={getStatusVariant(key.status)}>
                              {getStatusLabel(key.status)}
                            </Badge>
                          </TableCell>
                          <TableCell>
                            <div className="space-y-1">
                              <Badge variant={getCircuitBreakerVariant(key.circuitBreakerState)}>
                                {getCircuitBreakerLabel(key.circuitBreakerState)}
                              </Badge>
                              <div className="text-xs text-muted-foreground">{formatCircuitBreakerWindow(key)}</div>
                            </div>
                          </TableCell>
                          <TableCell>{formatDate(key.expiresAt)}</TableCell>
                          <TableCell>{formatUsedAt(key.lastUsedAt)}</TableCell>
                          <TableCell className="min-w-[320px] text-right">
                            <div className="flex items-center justify-end gap-2">
                              <Button variant="ghost" size="sm" onClick={() => handleOpenEdit(key)}>
                                编辑
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => void handleToggleDisabled(key)}
                                disabled={togglingId === key.id}
                              >
                                {togglingId === key.id ? '处理中...' : key.status === 'disabled' ? '恢复' : '禁用'}
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => void handleOpenUsage(key.id)}
                                disabled={key.status !== 'active' || usingId === key.id}
                              >
                                {usingId === key.id ? '加载中...' : '使用'}
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => void handleReveal(key.id)}
                                disabled={revealingId === key.id}
                              >
                                {revealingId === key.id ? '加载中...' : '查看'}
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                className="text-destructive hover:text-destructive"
                                onClick={() => void handleDelete(key.id, key.name)}
                                disabled={deletingId === key.id}
                              >
                                {deletingId === key.id ? '删除中...' : '删除'}
                              </Button>
                            </div>
                          </TableCell>
                        </motion.tr>
                      ))}
                    </motion.tbody>
                  </Table>
                </div>
              </motion.div>
            )}
          </AdminSurface>
        </motion.div>

        <Dialog open={showCreate} onOpenChange={setShowCreate}>
          <DialogContent className="max-h-[calc(100vh-2rem)] overflow-y-auto">
            <DialogHeader>
              <DialogTitle>创建新 API Key</DialogTitle>
              <DialogDescription>为新设备或应用创建一个 API Key。</DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="keyName">API Key 名称</Label>
                <Input
                  id="keyName"
                  value={createName}
                  onChange={(event) => setCreateName(event.target.value)}
                  placeholder="例如：开发机 / 本地脚本 / CI"
                />
              </div>
              <div className="space-y-2">
                <div className="flex items-center justify-between gap-3">
                  <Label htmlFor="customKey">自定义 API Key</Label>
                  <Button type="button" variant="outline" size="sm" onClick={() => setCustomKey(buildRandomSkKey())}>
                    生成随机 sk-Key
                  </Button>
                </div>
                <Input
                  id="customKey"
                  value={customKey}
                  onChange={(event) => setCustomKey(event.target.value.trim())}
                  placeholder="留空自动生成 32 位 key core；也可手动填写 sk- 前缀或纯字母数字"
                  autoComplete="off"
                />
                <p className="text-xs text-muted-foreground">支持字母数字，可选 `sk-` 前缀；最少 16 位，推荐使用 32 位以上。</p>
              </div>
              <div className="space-y-2">
                <Label>到期时间</Label>
                <DateTimePicker value={createExpiresAt} onChange={setCreateExpiresAt} placeholder="留空表示永不过期" className="w-full justify-between text-right" />
              </div>
              <div className="grid gap-4 md:grid-cols-3">
                <div className="space-y-2">
                  <Label>
                    <CircuitBreakerFieldLabel
                      label="错误阈值"
                      description="统计上游非 2xx 和网络错误，429 不计数。连续达到阈值后立即熔断。"
                    />
                  </Label>
                  <Input type="number" min="1" max="1000" value={createCircuitBreakerThreshold} onChange={(event) => setCreateCircuitBreakerThreshold(event.target.value)} />
                </div>
                <div className="space-y-2">
                  <Label>
                    <CircuitBreakerFieldLabel
                      label="熔断时长"
                      description="进入 open 后拒绝请求的时长，结束后自动进入半开窗口。单位：分钟。"
                    />
                  </Label>
                  <Input type="number" min="1" max="1440" value={createCircuitBreakerOpenMinutes} onChange={(event) => setCreateCircuitBreakerOpenMinutes(event.target.value)} />
                </div>
                <div className="space-y-2">
                  <Label>
                    <CircuitBreakerFieldLabel
                      label="半开时长"
                      description="半开窗口整段放流；窗口内任一失败立即重新熔断；窗口结束且无失败才关闭熔断。"
                    />
                  </Label>
                  <Input type="number" min="1" max="240" value={createCircuitBreakerHalfOpenMinutes} onChange={(event) => setCreateCircuitBreakerHalfOpenMinutes(event.target.value)} />
                </div>
              </div>
              <APIKeyChannelTargetsEditor
                splitBySource={createSplitChannelTargetsBySource}
                onSplitBySourceChange={setCreateSplitChannelTargetsBySource}
                channelTargets={createChannelTargets}
                onChannelTargetsChange={setCreateChannelTargets}
                subscriptionChannelTargets={createSubscriptionChannelTargets}
                onSubscriptionChannelTargetsChange={setCreateSubscriptionChannelTargets}
                usageChannelTargets={createUsageChannelTargets}
                onUsageChannelTargetsChange={setCreateUsageChannelTargets}
              />
            </div>
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => {
                  setShowCreate(false)
                  setCreateName('')
                  setCustomKey('')
                  setCreateExpiresAt('')
                  setCreateCircuitBreakerThreshold('50')
                  setCreateCircuitBreakerOpenMinutes('10')
                  setCreateCircuitBreakerHalfOpenMinutes('2')
                  setCreateSplitChannelTargetsBySource(false)
                  setCreateChannelTargets([])
                  setCreateSubscriptionChannelTargets([])
                  setCreateUsageChannelTargets([])
                }}
              >
                取消
              </Button>
              <Button
                onClick={() => void handleCreate()}
                disabled={creating || !createName.trim() || !!(customKey && !apiKeyPattern.test(customKey))}
              >
                {creating ? '创建中...' : '创建'}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={!!editingKey} onOpenChange={(open) => !open && setEditingKey(null)}>
          <DialogContent className="max-h-[calc(100vh-2rem)] overflow-y-auto">
            <DialogHeader>
              <DialogTitle>编辑 API Key</DialogTitle>
              <DialogDescription>可修改名称、到期时间和熔断参数。</DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="editKeyName">名称</Label>
                <Input id="editKeyName" value={editName} onChange={(event) => setEditName(event.target.value)} />
              </div>
              <div className="space-y-2">
                <Label>到期时间</Label>
                <DateTimePicker value={editExpiresAt} onChange={setEditExpiresAt} placeholder="留空表示永不过期" className="w-full justify-between text-right" />
              </div>
              <div className="grid gap-4 md:grid-cols-3">
                <div className="space-y-2">
                  <Label>
                    <CircuitBreakerFieldLabel
                      label="错误阈值"
                      description="连续统计非 2xx 与网络错误次数，429 始终忽略。一次成功会清零。"
                    />
                  </Label>
                  <Input type="number" min="1" max="1000" value={editCircuitBreakerThreshold} onChange={(event) => setEditCircuitBreakerThreshold(event.target.value)} />
                </div>
                <div className="space-y-2">
                  <Label>
                    <CircuitBreakerFieldLabel
                      label="熔断时长"
                      description="open 阶段持续时间。到点后不是直接关闭，而是进入半开。"
                    />
                  </Label>
                  <Input type="number" min="1" max="1440" value={editCircuitBreakerOpenMinutes} onChange={(event) => setEditCircuitBreakerOpenMinutes(event.target.value)} />
                </div>
                <div className="space-y-2">
                  <Label>
                    <CircuitBreakerFieldLabel
                      label="半开时长"
                      description="半开窗口内整段允许请求通过；只要有一次失败，就立刻重新打开熔断。"
                    />
                  </Label>
                  <Input type="number" min="1" max="240" value={editCircuitBreakerHalfOpenMinutes} onChange={(event) => setEditCircuitBreakerHalfOpenMinutes(event.target.value)} />
                </div>
              </div>
              <APIKeyChannelTargetsEditor
                splitBySource={editSplitChannelTargetsBySource}
                onSplitBySourceChange={setEditSplitChannelTargetsBySource}
                channelTargets={editChannelTargets}
                onChannelTargetsChange={setEditChannelTargets}
                subscriptionChannelTargets={editSubscriptionChannelTargets}
                onSubscriptionChannelTargetsChange={setEditSubscriptionChannelTargets}
                usageChannelTargets={editUsageChannelTargets}
                onUsageChannelTargetsChange={setEditUsageChannelTargets}
              />
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setEditingKey(null)}>
                取消
              </Button>
              <Button onClick={() => void handleSaveEdit()} disabled={savingEdit || !editName.trim()}>
                {savingEdit ? '保存中...' : '保存'}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={!!revealKey} onOpenChange={(open) => !open && setRevealKey(null)}>
          <DialogContent className="max-h-[calc(100vh-2rem)] overflow-y-auto sm:max-w-xl">
            <DialogHeader>
              <DialogTitle>查看 API Key</DialogTitle>
              <DialogDescription>API Key 明文会显示在此处，请妥善保管。</DialogDescription>
            </DialogHeader>
            {revealKey ? (
              <div className="space-y-4 py-4">
                <div className="space-y-2">
                  <Label>API Base URL</Label>
                  <div className="flex flex-col gap-2 sm:flex-row">
                    <code className="min-w-0 flex-1 rounded-md border bg-background px-3 py-2 text-sm font-mono break-all">
                      {apiBaseUrl}
                    </code>
                    <Button size="sm" variant="outline" onClick={() => void copyToClipboard(apiBaseUrl, 'revealBaseUrl')}>
                      {copied === 'revealBaseUrl' ? '已复制' : '复制'}
                    </Button>
                  </div>
                </div>
                <div className="space-y-2">
                  <Label>API Key</Label>
                  <div className="flex flex-col gap-2 sm:flex-row">
                    <code className="min-w-0 flex-1 rounded-md border bg-background px-3 py-2 text-sm font-mono break-all">
                      {revealKey.apiKey}
                    </code>
                    <Button size="sm" onClick={() => void copyToClipboard(revealKey.apiKey, 'revealApiKey')}>
                      {copied === 'revealApiKey' ? '已复制' : '复制'}
                    </Button>
                  </div>
                </div>
                <div className="grid gap-4 text-sm md:grid-cols-2">
                  <div className="rounded-lg border px-4 py-3">
                    <p className="text-muted-foreground">名称</p>
                    <p className="font-medium">{revealKey.name}</p>
                  </div>
                  <div className="rounded-lg border px-4 py-3">
                    <p className="text-muted-foreground">到期时间</p>
                    <p className="font-medium">{formatDate(revealKey.expiresAt)}</p>
                  </div>
                  <div className="rounded-lg border px-4 py-3">
                    <p className="text-muted-foreground">熔断状态</p>
                    <div className="mt-1">
                      <Badge variant={getCircuitBreakerVariant(revealKey.circuitBreakerState)}>{getCircuitBreakerLabel(revealKey.circuitBreakerState)}</Badge>
                    </div>
                  </div>
                  <div className="rounded-lg border px-4 py-3">
                    <p className="text-muted-foreground">熔断参数</p>
                    <p className="font-medium">{`${revealKey.circuitBreakerThreshold} / ${revealKey.circuitBreakerOpenMinutes} 分钟 / ${revealKey.circuitBreakerHalfOpenMinutes} 分钟`}</p>
                  </div>
                </div>
                <div className="rounded-lg border px-4 py-3 text-sm">
                  <p className="text-muted-foreground">连接说明</p>
                  <p className="mt-1 text-xs leading-5">
                    客户端 Base URL 使用 <span className="font-mono">{apiBaseUrl}</span>，认证时填入当前 API Key 即可。
                  </p>
                </div>
              </div>
            ) : null}
            <DialogFooter>
              <Button onClick={() => setRevealKey(null)}>关闭</Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <APIKeyUsageDialog
          open={!!usageKey}
          onOpenChange={(open) => !open && setUsageKey(null)}
          revealKey={usageKey}
          siteName={siteName}
          apiBaseUrl={apiBaseUrl}
          copied={copied}
          onCopy={copyToClipboard}
        />
        </AdminPageShell>
      </TooltipProvider>
    </motion.div>
  )
}
