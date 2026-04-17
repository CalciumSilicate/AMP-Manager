import { type ReactNode, useCallback, useEffect, useMemo, useState } from 'react'
import { Loader2, Pencil, Play, Plus, RefreshCw, Trash2 } from 'lucide-react'

import { listChannels, type Channel } from '@/api/channels'
import {
  createStatusMonitorItem,
  deleteStatusMonitorItem,
  getStatusMonitorRuntimeConfig,
  listStatusMonitorItems,
  runAllStatusMonitorItems,
  runStatusMonitorItem,
  updateStatusMonitorItem,
  updateStatusMonitorRuntimeConfig,
  type StatusMonitorAdminItem,
  type StatusMonitorItemRequest,
  type StatusMonitorRequestFormat,
  type StatusMonitorRuntimeConfig,
  type StatusMonitorRuntimeConfigUpdate,
  type StatusMonitorTargetType,
} from '@/api/statusMonitor'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
import { ScrollArea } from '@/components/ui/scroll-area'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'

type MessageFn = (type: 'success' | 'error', text: string) => void
type SecretMode = 'replace' | 'keep' | 'clear'

interface Props {
  onMessage: MessageFn
}

interface MonitorDraft {
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
  headersJson: string
  headersMode: SecretMode
  bodyTemplate: string
  bodyTemplateMode: SecretMode
  expectedStatusCodesInput: string
  expectedSubstring: string
}

const TARGET_LABELS: Record<StatusMonitorTargetType, string> = {
  service_proxy: '服务层',
  channel_direct: '上游层',
  custom_http: '自定义',
}

const FORMAT_LABELS: Record<StatusMonitorRequestFormat, string> = {
  chat_completions: 'Chat',
  responses: 'Responses',
  messages: 'Messages',
  generate_content: 'Gemini',
}

const DEFAULT_DRAFT: MonitorDraft = {
  name: '',
  groupName: '',
  targetType: 'service_proxy',
  enabled: true,
  sortOrder: 0,
  timeoutMs: 45000,
  degradedThresholdMs: 10000,
  requestFormat: 'responses',
  model: '',
  channelId: '',
  url: '',
  method: 'GET',
  headersJson: '',
  headersMode: 'replace',
  bodyTemplate: '',
  bodyTemplateMode: 'replace',
  expectedStatusCodesInput: '200',
  expectedSubstring: '',
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

export function StatusMonitorSettingsPanel({ onMessage }: Props) {
  const [runtimeConfig, setRuntimeConfig] = useState<StatusMonitorRuntimeConfig | null>(null)
  const [runtimeKeyInput, setRuntimeKeyInput] = useState('')
  const [runtimeSaving, setRuntimeSaving] = useState(false)
  const [items, setItems] = useState<StatusMonitorAdminItem[]>([])
  const [channels, setChannels] = useState<Channel[]>([])
  const [loading, setLoading] = useState(true)
  const [runningAll, setRunningAll] = useState(false)
  const [busyItemId, setBusyItemId] = useState<string | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [savingItem, setSavingItem] = useState(false)
  const [editingItem, setEditingItem] = useState<StatusMonitorAdminItem | null>(null)
  const [draft, setDraft] = useState<MonitorDraft>(DEFAULT_DRAFT)

  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      const [runtime, monitorItems, channelItems] = await Promise.all([
        getStatusMonitorRuntimeConfig(),
        listStatusMonitorItems(),
        listChannels(),
      ])
      setRuntimeConfig(runtime)
      setRuntimeKeyInput('')
      setItems(monitorItems)
      setChannels(channelItems)
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '加载状态监控配置失败')
    } finally {
      setLoading(false)
    }
  }, [onMessage])

  useEffect(() => {
    void loadData()
  }, [loadData])

  const channelOptions = useMemo(
    () => channels.map((channel) => ({ value: channel.id, label: channel.name })),
    [channels],
  )

  const handleRuntimeField = <K extends keyof StatusMonitorRuntimeConfig>(key: K, value: StatusMonitorRuntimeConfig[K]) => {
    if (!runtimeConfig) return
    setRuntimeConfig({ ...runtimeConfig, [key]: value })
  }

  const handleSaveRuntimeConfig = async () => {
    if (!runtimeConfig) return

    const payload: StatusMonitorRuntimeConfigUpdate = {
      enabled: runtimeConfig.enabled,
      serviceBaseUrl: runtimeConfig.serviceBaseUrl,
      serviceMonitorApiKey: runtimeKeyInput,
      retainServiceMonitorApiKey: runtimeConfig.serviceMonitorApiKeySet && runtimeKeyInput.trim() === '',
      clearServiceMonitorApiKey: runtimeKeyInput.trim() === '__CLEAR__',
      pollIntervalSec: runtimeConfig.pollIntervalSec,
      defaultTimeoutMs: runtimeConfig.defaultTimeoutMs,
      retentionDays: runtimeConfig.retentionDays,
    }

    setRuntimeSaving(true)
    try {
      const next = await updateStatusMonitorRuntimeConfig(payload)
      setRuntimeConfig(next)
      setRuntimeKeyInput('')
      onMessage('success', '状态监控运行时配置已保存')
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '保存状态监控配置失败')
    } finally {
      setRuntimeSaving(false)
    }
  }

  const openCreateDialog = () => {
    setEditingItem(null)
    setDraft(DEFAULT_DRAFT)
    setDialogOpen(true)
  }

  const openEditDialog = (item: StatusMonitorAdminItem) => {
    setEditingItem(item)
    setDraft({
      name: item.name,
      groupName: item.groupName,
      targetType: item.targetType,
      enabled: item.enabled,
      sortOrder: item.sortOrder,
      timeoutMs: item.timeoutMs || 45000,
      degradedThresholdMs: item.degradedThresholdMs || 10000,
      requestFormat: item.requestFormat,
      model: item.model,
      channelId: item.channelId,
      url: item.url,
      method: item.method || 'GET',
      headersJson: '',
      headersMode: item.headersSet ? 'keep' : 'replace',
      bodyTemplate: '',
      bodyTemplateMode: item.bodyTemplateSet ? 'keep' : 'replace',
      expectedStatusCodesInput: item.expectedStatusCodes.join(', ') || '200',
      expectedSubstring: item.expectedSubstring,
    })
    setDialogOpen(true)
  }

  const parseStatusCodes = (input: string): number[] => {
    const values = input
      .split(',')
      .map((value) => Number.parseInt(value.trim(), 10))
      .filter((value) => Number.isFinite(value))
    return values.length > 0 ? values : [200]
  }

  const buildPayload = (source: MonitorDraft): StatusMonitorItemRequest => ({
    name: source.name.trim(),
    groupName: source.groupName.trim(),
    targetType: source.targetType,
    enabled: source.enabled,
    sortOrder: source.sortOrder,
    timeoutMs: source.timeoutMs,
    degradedThresholdMs: source.degradedThresholdMs,
    requestFormat: source.requestFormat,
    model: source.model.trim(),
    channelId: source.channelId,
    url: source.url.trim(),
    method: source.method.trim().toUpperCase(),
    headersJson: source.headersMode === 'replace' ? source.headersJson : '',
    retainHeadersJson: source.headersMode === 'keep',
    clearHeadersJson: source.headersMode === 'clear',
    bodyTemplate: source.bodyTemplateMode === 'replace' ? source.bodyTemplate : '',
    retainBodyTemplate: source.bodyTemplateMode === 'keep',
    clearBodyTemplate: source.bodyTemplateMode === 'clear',
    expectedStatusCodes: parseStatusCodes(source.expectedStatusCodesInput),
    expectedSubstring: source.expectedSubstring.trim(),
  })

  const handleSubmitItem = async () => {
    setSavingItem(true)
    try {
      const payload = buildPayload(draft)
      if (editingItem) {
        await updateStatusMonitorItem(editingItem.id, payload)
      } else {
        await createStatusMonitorItem(payload)
      }
      await loadData()
      setDialogOpen(false)
      onMessage('success', editingItem ? '状态监控项已更新' : '状态监控项已创建')
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '保存状态监控项失败')
    } finally {
      setSavingItem(false)
    }
  }

  const handleToggleItem = async (item: StatusMonitorAdminItem, enabled: boolean) => {
    setBusyItemId(item.id)
    try {
      await updateStatusMonitorItem(item.id, {
        name: item.name,
        groupName: item.groupName,
        targetType: item.targetType,
        enabled,
        sortOrder: item.sortOrder,
        timeoutMs: item.timeoutMs,
        degradedThresholdMs: item.degradedThresholdMs,
        requestFormat: item.requestFormat,
        model: item.model,
        channelId: item.channelId,
        url: item.url,
        method: item.method,
        retainHeadersJson: item.headersSet,
        retainBodyTemplate: item.bodyTemplateSet,
        expectedStatusCodes: item.expectedStatusCodes,
        expectedSubstring: item.expectedSubstring,
      })
      setItems((current) => current.map((entry) => (entry.id === item.id ? { ...entry, enabled } : entry)))
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '更新状态失败')
    } finally {
      setBusyItemId(null)
    }
  }

  const handleDeleteItem = async (item: StatusMonitorAdminItem) => {
    if (!window.confirm(`确定删除监控项“${item.name}”吗？`)) return
    setBusyItemId(item.id)
    try {
      await deleteStatusMonitorItem(item.id)
      setItems((current) => current.filter((entry) => entry.id !== item.id))
      onMessage('success', '状态监控项已删除')
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '删除状态监控项失败')
    } finally {
      setBusyItemId(null)
    }
  }

  const handleRunItem = async (item: StatusMonitorAdminItem) => {
    setBusyItemId(item.id)
    try {
      const result = await runStatusMonitorItem(item.id)
      onMessage('success', `${item.name}: ${result.result.message}`)
      await loadData()
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '执行探测失败')
    } finally {
      setBusyItemId(null)
    }
  }

  const handleRunAll = async () => {
    setRunningAll(true)
    try {
      const result = await runAllStatusMonitorItems()
      onMessage('success', result.message)
      await loadData()
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '执行全量探测失败')
    } finally {
      setRunningAll(false)
    }
  }

  if (loading) {
    return <div className="py-8 text-center text-sm text-muted-foreground">加载中...</div>
  }

  return (
    <div className="space-y-6">
      {runtimeConfig ? (
        <div className="space-y-4 rounded-lg border p-5">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <h3 className="text-base font-semibold">运行时配置</h3>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <div className="flex items-center gap-2 rounded-lg border px-3 py-2">
                <Label htmlFor="status-runtime-enabled" className="text-sm">启用</Label>
                <Switch
                  id="status-runtime-enabled"
                  checked={runtimeConfig.enabled}
                  onCheckedChange={(checked) => handleRuntimeField('enabled', checked)}
                />
              </div>
              <Button variant="outline" onClick={() => void loadData()}>
                <RefreshCw className="mr-2 h-4 w-4" />
                刷新
              </Button>
              <Button variant="outline" onClick={handleRunAll} disabled={runningAll}>
                {runningAll ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Play className="mr-2 h-4 w-4" />}
                立即全量探测
              </Button>
              <Button onClick={handleSaveRuntimeConfig} disabled={runtimeSaving}>
                {runtimeSaving ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
                保存配置
              </Button>
            </div>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2 md:col-span-2">
              <Label htmlFor="status-runtime-base-url">服务层探测基地址</Label>
              <Input
                id="status-runtime-base-url"
                value={runtimeConfig.serviceBaseUrl}
                onChange={(event) => handleRuntimeField('serviceBaseUrl', event.target.value)}
                placeholder="https://your-amp-manager.example.com"
              />
            </div>
            <div className="space-y-2 md:col-span-2">
              <Label htmlFor="status-runtime-api-key">服务层探测 API Key</Label>
              <Input
                id="status-runtime-api-key"
                value={runtimeKeyInput}
                onChange={(event) => setRuntimeKeyInput(event.target.value)}
                placeholder={runtimeConfig.serviceMonitorApiKeySet ? `${runtimeConfig.serviceMonitorApiKeyMasked}，留空保持不变；输入 __CLEAR__ 清空` : '输入用于访问 /v1 的 API Key'}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="status-runtime-poll-interval">轮询周期 (秒)</Label>
              <Input
                id="status-runtime-poll-interval"
                type="number"
                min={30}
                value={runtimeConfig.pollIntervalSec}
                onChange={(event) => handleRuntimeField('pollIntervalSec', Number.parseInt(event.target.value || '300', 10))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="status-runtime-timeout">默认超时 (毫秒)</Label>
              <Input
                id="status-runtime-timeout"
                type="number"
                min={1000}
                value={runtimeConfig.defaultTimeoutMs}
                onChange={(event) => handleRuntimeField('defaultTimeoutMs', Number.parseInt(event.target.value || '45000', 10))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="status-runtime-retention">保留天数</Label>
              <Input
                id="status-runtime-retention"
                type="number"
                min={7}
                value={runtimeConfig.retentionDays}
                onChange={(event) => handleRuntimeField('retentionDays', Number.parseInt(event.target.value || '30', 10))}
              />
            </div>
          </div>
        </div>
      ) : null}

      <div className="space-y-4 rounded-lg border p-5">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <h3 className="text-base font-semibold">监控项管理</h3>
          </div>
          <Button onClick={openCreateDialog}>
            <Plus className="mr-2 h-4 w-4" />
            新增监控项
          </Button>
        </div>

        {items.length === 0 ? (
          <div className="rounded-lg border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">
            还没有状态监控项。
          </div>
        ) : (
          <>
            <div className="space-y-3 md:hidden">
              {items.map((item) => (
                <div key={item.id} className="rounded-xl border border-border/70 px-4 py-4">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="font-medium">{item.name}</div>
                      <div className="mt-1 text-xs text-muted-foreground">
                        {item.requestFormat ? FORMAT_LABELS[item.requestFormat] : 'HTTP'} · 超时 {item.timeoutMs || runtimeConfig?.defaultTimeoutMs || 45000} ms
                      </div>
                    </div>
                    <Switch
                      checked={item.enabled}
                      disabled={busyItemId === item.id}
                      onCheckedChange={(checked) => void handleToggleItem(item, checked)}
                    />
                  </div>

                  <div className="mt-3 grid gap-2 text-sm">
                    <MobileInfoRow label="分组" value={item.groupName || TARGET_LABELS[item.targetType]} />
                    <MobileInfoRow label="类型" value={<Badge variant="outline">{TARGET_LABELS[item.targetType]}</Badge>} />
                    <MobileInfoRow
                      label="目标"
                      value={
                        item.targetType === 'custom_http'
                          ? `${item.method} ${item.url}`
                          : item.targetType === 'channel_direct'
                            ? `${item.channelName || '未命名渠道'} · ${item.model}`
                            : item.model
                      }
                    />
                  </div>

                  <div className="mt-3 flex flex-wrap gap-2">
                    <Button variant="outline" size="sm" disabled={busyItemId === item.id} onClick={() => void handleRunItem(item)}>
                      {busyItemId === item.id ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
                    </Button>
                    <Button variant="outline" size="sm" onClick={() => openEditDialog(item)}>
                      <Pencil className="h-4 w-4" />
                    </Button>
                    <Button variant="destructive" size="sm" disabled={busyItemId === item.id} onClick={() => void handleDeleteItem(item)}>
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              ))}
            </div>

            <div className="hidden overflow-x-auto md:block">
              <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>名称</TableHead>
                  <TableHead>分组</TableHead>
                  <TableHead>类型</TableHead>
                  <TableHead>目标</TableHead>
                  <TableHead>启用</TableHead>
                  <TableHead className="text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((item) => (
                  <TableRow key={item.id}>
                    <TableCell className="min-w-[180px]">
                      <div className="space-y-1">
                        <div className="font-medium">{item.name}</div>
                        <div className="text-xs text-muted-foreground">
                          {item.requestFormat ? FORMAT_LABELS[item.requestFormat] : 'HTTP'} · 超时 {item.timeoutMs || runtimeConfig?.defaultTimeoutMs || 45000} ms
                        </div>
                      </div>
                    </TableCell>
                    <TableCell>{item.groupName || TARGET_LABELS[item.targetType]}</TableCell>
                    <TableCell>
                      <Badge variant="outline">{TARGET_LABELS[item.targetType]}</Badge>
                    </TableCell>
                    <TableCell className="max-w-[280px] text-sm text-muted-foreground">
                      {item.targetType === 'custom_http'
                        ? `${item.method} ${item.url}`
                        : item.targetType === 'channel_direct'
                          ? `${item.channelName || '未命名渠道'} · ${item.model}`
                          : item.model}
                    </TableCell>
                    <TableCell>
                      <Switch
                        checked={item.enabled}
                        disabled={busyItemId === item.id}
                        onCheckedChange={(checked) => void handleToggleItem(item, checked)}
                      />
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-2">
                        <Button variant="outline" size="sm" disabled={busyItemId === item.id} onClick={() => void handleRunItem(item)}>
                          {busyItemId === item.id ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
                        </Button>
                        <Button variant="outline" size="sm" onClick={() => openEditDialog(item)}>
                          <Pencil className="h-4 w-4" />
                        </Button>
                        <Button variant="destructive" size="sm" disabled={busyItemId === item.id} onClick={() => void handleDeleteItem(item)}>
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
              </Table>
            </div>
          </>
        )}
      </div>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-w-4xl">
          <DialogHeader>
            <DialogTitle>{editingItem ? '编辑状态监控项' : '新增状态监控项'}</DialogTitle>
            <DialogDescription>根据目标类型填写必要字段；敏感头信息和请求体默认会在更新时保留。</DialogDescription>
          </DialogHeader>

          <ScrollArea className="max-h-[70vh] pr-4">
            <div className="space-y-5">
              <div className="grid gap-4 md:grid-cols-2">
                <div className="space-y-2">
                  <Label>名称</Label>
                  <Input value={draft.name} onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))} />
                </div>
                <div className="space-y-2">
                  <Label>分组名称</Label>
                  <Input value={draft.groupName} onChange={(event) => setDraft((current) => ({ ...current, groupName: event.target.value }))} placeholder="留空时按目标类型自动分组" />
                </div>
                <div className="space-y-2">
                  <Label>目标类型</Label>
                  <Select value={draft.targetType} onValueChange={(value) => setDraft((current) => ({ ...current, targetType: value as StatusMonitorTargetType }))}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="service_proxy">服务层</SelectItem>
                      <SelectItem value="channel_direct">上游层</SelectItem>
                      <SelectItem value="custom_http">自定义 HTTP</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label>请求制式</Label>
                  <Select value={draft.requestFormat} onValueChange={(value) => setDraft((current) => ({ ...current, requestFormat: value as StatusMonitorRequestFormat }))}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="responses">Responses</SelectItem>
                      <SelectItem value="chat_completions">Chat Completions</SelectItem>
                      <SelectItem value="messages">Messages</SelectItem>
                      <SelectItem value="generate_content">Gemini</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label>排序</Label>
                  <Input type="number" value={draft.sortOrder} onChange={(event) => setDraft((current) => ({ ...current, sortOrder: Number.parseInt(event.target.value || '0', 10) }))} />
                </div>
                <div className="space-y-2">
                  <Label>超时 (毫秒)</Label>
                  <Input type="number" value={draft.timeoutMs} onChange={(event) => setDraft((current) => ({ ...current, timeoutMs: Number.parseInt(event.target.value || '45000', 10) }))} />
                </div>
                <div className="space-y-2">
                  <Label>降级阈值 (毫秒)</Label>
                  <Input type="number" value={draft.degradedThresholdMs} onChange={(event) => setDraft((current) => ({ ...current, degradedThresholdMs: Number.parseInt(event.target.value || '10000', 10) }))} />
                </div>
                <div className="flex items-center justify-between rounded-lg border px-4 py-3">
                  <div className="space-y-0.5">
                    <Label>启用</Label>
                    <p className="text-sm text-muted-foreground">只有启用项才会被自动轮询。</p>
                  </div>
                  <Switch checked={draft.enabled} onCheckedChange={(checked) => setDraft((current) => ({ ...current, enabled: checked }))} />
                </div>
              </div>

              {(draft.targetType === 'service_proxy' || draft.targetType === 'channel_direct') ? (
                <div className="grid gap-4 md:grid-cols-2">
                  {draft.targetType === 'channel_direct' ? (
                    <div className="space-y-2">
                      <Label>渠道</Label>
                      <Select value={draft.channelId} onValueChange={(value) => setDraft((current) => ({ ...current, channelId: value }))}>
                        <SelectTrigger>
                          <SelectValue placeholder="选择渠道" />
                        </SelectTrigger>
                        <SelectContent>
                          {channelOptions.map((channel) => (
                            <SelectItem key={channel.value} value={channel.value}>
                              {channel.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  ) : null}
                  <div className="space-y-2">
                    <Label>模型</Label>
                    <Input value={draft.model} onChange={(event) => setDraft((current) => ({ ...current, model: event.target.value }))} placeholder="例如 gpt-5.4" />
                  </div>
                </div>
              ) : null}

              {draft.targetType === 'custom_http' ? (
                <div className="space-y-5">
                  <div className="grid gap-4 md:grid-cols-[140px_minmax(0,1fr)]">
                    <div className="space-y-2">
                      <Label>方法</Label>
                      <Select value={draft.method} onValueChange={(value) => setDraft((current) => ({ ...current, method: value }))}>
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="GET">GET</SelectItem>
                          <SelectItem value="POST">POST</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="space-y-2">
                      <Label>URL</Label>
                      <Input value={draft.url} onChange={(event) => setDraft((current) => ({ ...current, url: event.target.value }))} placeholder="https://example.com/health" />
                    </div>
                  </div>

                  <div className="grid gap-4 md:grid-cols-2">
                    <div className="space-y-2">
                      <Label>预期状态码</Label>
                      <Input value={draft.expectedStatusCodesInput} onChange={(event) => setDraft((current) => ({ ...current, expectedStatusCodesInput: event.target.value }))} placeholder="200, 204" />
                    </div>
                    <div className="space-y-2">
                      <Label>预期文本</Label>
                      <Input value={draft.expectedSubstring} onChange={(event) => setDraft((current) => ({ ...current, expectedSubstring: event.target.value }))} placeholder="留空则仅校验状态码" />
                    </div>
                  </div>

                  <div className="space-y-3 rounded-lg border p-4">
                    <div className="flex items-center justify-between">
                      <div>
                        <Label>敏感头信息</Label>
                        <p className="text-sm text-muted-foreground">{editingItem?.headersSet ? editingItem.headersMasked : 'JSON 对象；例如 {"Authorization":"Bearer ..."}'}</p>
                      </div>
                      {editingItem?.headersSet ? (
                        <div className="flex gap-2">
                          <Button type="button" variant={draft.headersMode === 'keep' ? 'default' : 'outline'} size="sm" onClick={() => setDraft((current) => ({ ...current, headersMode: 'keep', headersJson: '' }))}>
                            保留
                          </Button>
                          <Button type="button" variant={draft.headersMode === 'replace' ? 'default' : 'outline'} size="sm" onClick={() => setDraft((current) => ({ ...current, headersMode: 'replace' }))}>
                            替换
                          </Button>
                          <Button type="button" variant={draft.headersMode === 'clear' ? 'destructive' : 'outline'} size="sm" onClick={() => setDraft((current) => ({ ...current, headersMode: 'clear', headersJson: '' }))}>
                            清空
                          </Button>
                        </div>
                      ) : null}
                    </div>
                    <Textarea
                      value={draft.headersJson}
                      onChange={(event) => setDraft((current) => ({ ...current, headersJson: event.target.value, headersMode: 'replace' }))}
                      placeholder='{"Authorization":"Bearer xxx"}'
                      disabled={draft.headersMode === 'keep' || draft.headersMode === 'clear'}
                      className="min-h-[120px] font-mono text-xs"
                    />
                  </div>

                  <div className="space-y-3 rounded-lg border p-4">
                    <div className="flex items-center justify-between">
                      <div>
                        <Label>请求体模板</Label>
                        <p className="text-sm text-muted-foreground">{editingItem?.bodyTemplateSet ? editingItem.bodyTemplateMasked : 'POST 请求体；GET 可留空'}</p>
                      </div>
                      {editingItem?.bodyTemplateSet ? (
                        <div className="flex gap-2">
                          <Button type="button" variant={draft.bodyTemplateMode === 'keep' ? 'default' : 'outline'} size="sm" onClick={() => setDraft((current) => ({ ...current, bodyTemplateMode: 'keep', bodyTemplate: '' }))}>
                            保留
                          </Button>
                          <Button type="button" variant={draft.bodyTemplateMode === 'replace' ? 'default' : 'outline'} size="sm" onClick={() => setDraft((current) => ({ ...current, bodyTemplateMode: 'replace' }))}>
                            替换
                          </Button>
                          <Button type="button" variant={draft.bodyTemplateMode === 'clear' ? 'destructive' : 'outline'} size="sm" onClick={() => setDraft((current) => ({ ...current, bodyTemplateMode: 'clear', bodyTemplate: '' }))}>
                            清空
                          </Button>
                        </div>
                      ) : null}
                    </div>
                    <Textarea
                      value={draft.bodyTemplate}
                      onChange={(event) => setDraft((current) => ({ ...current, bodyTemplate: event.target.value, bodyTemplateMode: 'replace' }))}
                      placeholder='{"message":"ok"}'
                      disabled={draft.bodyTemplateMode === 'keep' || draft.bodyTemplateMode === 'clear'}
                      className="min-h-[120px] font-mono text-xs"
                    />
                  </div>
                </div>
              ) : null}
            </div>
          </ScrollArea>

          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>
              取消
            </Button>
            <Button onClick={handleSubmitItem} disabled={savingItem}>
              {savingItem ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
              {editingItem ? '保存修改' : '创建监控项'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
