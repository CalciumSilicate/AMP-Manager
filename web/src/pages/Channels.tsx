import { useEffect, useMemo, useState } from 'react'

import { motion } from '@/lib/motion'
import {
  Channel,
  ChannelEndpoint,
  ChannelRequest,
  TestChannelRequest,
  TestChannelResult,
  createChannel,
  deleteChannel,
  listChannels,
  setChannelEnabled,
  testChannel,
  updateChannel,
} from '../api/channels'
import { Group, listGroups } from '../api/groups'
import { fetchChannelModels } from '../api/models'
import { AdminPageShell, AdminSurface } from '@/components/admin/AdminPageShell'
import { ChannelFormDialog } from '@/components/channels/ChannelFormDialog'
import { ChannelTable } from '@/components/channels/ChannelTable'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'

const TEST_FORMAT_OPTIONS: { value: ChannelEndpoint; label: string }[] = [
  { value: 'chat_completions', label: 'Chat Completions' },
  { value: 'responses', label: 'Responses' },
  { value: 'messages', label: 'Messages' },
  { value: 'generate_content', label: 'Generate Content' },
]

function createDefaultTestRequest(channel: Channel | null): TestChannelRequest {
  const firstModel = channel?.models?.[0]?.name || ''
  return {
    format: channel?.endpoint || 'chat_completions',
    model: firstModel,
    prompt: '输出 ok。',
    instructions: '请用一句话完成响应。',
    thinkingEffort: '',
  }
}

export default function Channels() {
  const [channels, setChannels] = useState<Channel[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [showForm, setShowForm] = useState(false)
  const [editingChannel, setEditingChannel] = useState<Channel | null>(null)
  const [fetchingModels, setFetchingModels] = useState<Record<string, boolean>>({})
  const [modelCounts, setModelCounts] = useState<Record<string, number>>({})
  const [groups, setGroups] = useState<Group[]>([])
  const [saving, setSaving] = useState(false)

  const [testDialogChannel, setTestDialogChannel] = useState<Channel | null>(null)
  const [testRequest, setTestRequest] = useState<TestChannelRequest>(createDefaultTestRequest(null))
  const [customModel, setCustomModel] = useState('')
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<TestChannelResult | null>(null)

  const [formData, setFormData] = useState<ChannelRequest>({
    type: 'openai',
    endpoint: 'chat_completions',
    name: '',
    baseUrl: 'https://api.openai.com',
    apiKey: '',
    enabled: true,
    weight: 1,
    priority: 100,
    rateMultiplier: 1,
    groupIds: [],
    models: [],
    modelWhitelist: false,
    simulateCli: false,
    simulateUa: false,
    simulateSystemPrompt: false,
    traditionalChinese: false,
    copilotApi: false,
    codexWebsocketEnabled: false,
    headers: {},
    translator: {
      compatible: false,
      responses: false,
      messages: false,
      gemini: false,
    },
  })

  useEffect(() => {
    void loadChannels()
    void loadGroups()
  }, [])

  const loadChannels = async () => {
    try {
      const data = await listChannels()
      setChannels(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }

  const loadGroups = async () => {
    try {
      const data = await listGroups()
      setGroups(data)
    } catch {
      // ignore group loading failures here
    }
  }

  const buildChannelPayload = (channel: Channel, overrides: Partial<ChannelRequest> = {}): ChannelRequest => ({
    type: channel.type,
    endpoint: channel.endpoint,
    name: channel.name,
    baseUrl: channel.baseUrl,
    enabled: channel.enabled,
    weight: channel.weight,
    priority: channel.priority,
    rateMultiplier: channel.rateMultiplier,
    groupIds: channel.groupIds || [],
    models: channel.models,
    modelWhitelist: channel.modelWhitelist || false,
    simulateCli: channel.simulateCli || false,
    simulateUa: channel.simulateUa || false,
    simulateSystemPrompt: channel.simulateSystemPrompt || false,
    traditionalChinese: channel.traditionalChinese || false,
    copilotApi: channel.copilotApi || false,
    codexWebsocketEnabled: channel.codexWebsocketEnabled || false,
    headers: channel.headers,
    translator: channel.translator || {
      compatible: false,
      responses: false,
      messages: false,
      gemini: false,
    },
    ...overrides,
  })

  const handleCreate = () => {
    setEditingChannel(null)
    setFormData({
      type: 'openai',
      endpoint: 'chat_completions',
      name: '',
      baseUrl: 'https://api.openai.com',
      apiKey: '',
      enabled: true,
      weight: 1,
      priority: 100,
      rateMultiplier: 1,
      groupIds: [],
      models: [],
      modelWhitelist: false,
      simulateCli: false,
      simulateUa: false,
      simulateSystemPrompt: false,
      traditionalChinese: false,
      copilotApi: false,
      codexWebsocketEnabled: false,
      headers: {},
      translator: {
        compatible: false,
        responses: false,
        messages: false,
        gemini: false,
      },
    })
    setShowForm(true)
  }

  const handleEdit = (channel: Channel) => {
    setEditingChannel(channel)
    setFormData({
      type: channel.type,
      endpoint: channel.endpoint,
      name: channel.name,
      baseUrl: channel.baseUrl,
      apiKey: '',
      enabled: channel.enabled,
      weight: channel.weight,
      priority: channel.priority,
      rateMultiplier: channel.rateMultiplier,
      groupIds: channel.groupIds || [],
      models: channel.models,
      modelWhitelist: channel.modelWhitelist || false,
      simulateCli: channel.simulateCli || false,
      simulateUa: channel.simulateUa || false,
      simulateSystemPrompt: channel.simulateSystemPrompt || false,
      traditionalChinese: channel.traditionalChinese || false,
      copilotApi: channel.copilotApi || false,
      codexWebsocketEnabled: channel.codexWebsocketEnabled || false,
      headers: channel.headers,
      translator: channel.translator || {
        compatible: false,
        responses: false,
        messages: false,
        gemini: false,
      },
    })
    setShowForm(true)
  }

  const handleSubmit = async () => {
    if (!formData.name.trim() || !formData.baseUrl.trim()) {
      setError('请填写必填字段')
      return
    }

    setSaving(true)
    setError('')

    try {
      if (editingChannel) {
        await updateChannel(editingChannel.id, formData)
      } else {
        if (!formData.apiKey) {
          setError('创建渠道时 API Key 为必填')
          setSaving(false)
          return
        }
        await createChannel(formData)
      }
      setShowForm(false)
      await loadChannels()
    } catch (err) {
      setError(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (id: string, name: string) => {
    if (!confirm(`确定要删除渠道 "${name}" 吗？`)) return

    try {
      await deleteChannel(id)
      await loadChannels()
    } catch (err) {
      setError(err instanceof Error ? err.message : '删除失败')
    }
  }

  const handleToggleEnabled = async (id: string, enabled: boolean) => {
    try {
      await setChannelEnabled(id, enabled)
      await loadChannels()
    } catch (err) {
      setError(err instanceof Error ? err.message : '更新失败')
    }
  }

  const handleQuickUpdate = async (
    channel: Channel,
    field: 'priority' | 'weight' | 'rateMultiplier',
    value: number,
  ) => {
    try {
      await updateChannel(channel.id, buildChannelPayload(channel, { [field]: value }))
      await loadChannels()
    } catch (err) {
      setError(err instanceof Error ? err.message : '更新失败')
    }
  }

  const openTestDialog = (channel: Channel) => {
    setTestDialogChannel(channel)
    setTestRequest(createDefaultTestRequest(channel))
    setCustomModel('')
    setTestResult(null)
  }

  const handleTest = async () => {
    if (!testDialogChannel) return
    const model = customModel.trim() || testRequest.model.trim()
    if (!model || !testRequest.prompt.trim() || !testRequest.instructions.trim()) {
      setTestResult({ success: false, message: '模型、提示词和 instructions 不能为空' })
      return
    }

    setTesting(true)
    try {
      const result = await testChannel(testDialogChannel.id, {
        ...testRequest,
        model,
        prompt: testRequest.prompt.trim(),
        instructions: testRequest.instructions.trim(),
        thinkingEffort: testRequest.thinkingEffort?.trim() || undefined,
      })
      setTestResult(result)
    } catch (err) {
      setTestResult({
        success: false,
        message: err instanceof Error ? err.message : '测试失败',
      })
    } finally {
      setTesting(false)
    }
  }

  const handleFetchModels = async (id: string) => {
    try {
      setFetchingModels((prev) => ({ ...prev, [id]: true }))
      const result = await fetchChannelModels(id)
      setModelCounts((prev) => ({ ...prev, [id]: result.count }))
    } catch (err) {
      setError(err instanceof Error ? err.message : '获取模型失败')
    } finally {
      setFetchingModels((prev) => ({ ...prev, [id]: false }))
    }
  }

  const availableModels = useMemo(() => testDialogChannel?.models || [], [testDialogChannel])

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <div className="text-muted-foreground">加载中...</div>
      </div>
    )
  }

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
      <AdminPageShell
        title="渠道管理"
        description="管理渠道配置、模型抓取与路由优先级"
        actions={<Button onClick={handleCreate}>添加渠道</Button>}
      >
        {error && (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        <motion.div initial={{ opacity: 0, y: 24 }} animate={{ opacity: 1, y: 0 }} transition={{ type: 'spring', bounce: 0.2, duration: 0.55 }}>
          <AdminSurface>
            <div className="admin-surface-header">
              <div className="space-y-1">
                <p className="text-sm font-medium text-foreground">{channels.length} 个渠道</p>
                <p className="admin-inline-note">默认按模型匹配、优先级和权重进行路由</p>
              </div>
            </div>

            <div className="admin-surface-body space-y-4">
              {channels.length === 0 ? (
                <div className="rounded-xl border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">
                  暂无渠道
                </div>
              ) : (
                <ChannelTable
                  channels={channels}
                  testResults={{}}
                  fetchingModels={fetchingModels}
                  modelCounts={modelCounts}
                  onToggleEnabled={handleToggleEnabled}
                  onQuickUpdate={handleQuickUpdate}
                  onTest={openTestDialog}
                  onFetchModels={handleFetchModels}
                  onEdit={handleEdit}
                  onDelete={handleDelete}
                />
              )}
            </div>
          </AdminSurface>
        </motion.div>

        <ChannelFormDialog
          open={showForm}
          onOpenChange={setShowForm}
          editingChannel={editingChannel}
          formData={formData}
          setFormData={setFormData}
          onSubmit={handleSubmit}
          saving={saving}
          groups={groups}
        />

        <Dialog open={!!testDialogChannel} onOpenChange={(open) => !open && setTestDialogChannel(null)}>
          <DialogContent className="max-h-[calc(100vh-2rem)] overflow-y-auto sm:max-w-2xl">
            <DialogHeader>
              <DialogTitle>测试渠道</DialogTitle>
              <DialogDescription>{testDialogChannel?.name || '-'}</DialogDescription>
            </DialogHeader>
            <div className="grid gap-4 py-2 sm:grid-cols-2">
              <div className="space-y-2">
                <Label>接口制式</Label>
                <Select
                  value={testRequest.format}
                  onValueChange={(value: ChannelEndpoint) => setTestRequest((current) => ({ ...current, format: value }))}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {TEST_FORMAT_OPTIONS.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {option.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className="space-y-2">
                <Label>模型</Label>
                <Select
                  value={testRequest.model}
                  onValueChange={(value) => setTestRequest((current) => ({ ...current, model: value }))}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="从列表选择" />
                  </SelectTrigger>
                  <SelectContent>
                    {availableModels.length === 0 ? (
                      <SelectItem value="__empty__" disabled>无可选模型</SelectItem>
                    ) : (
                      availableModels.map((model) => (
                        <SelectItem key={`${model.name}:${model.alias || ''}`} value={model.name}>
                          {model.alias ? `${model.alias} -> ${model.name}` : model.name}
                        </SelectItem>
                      ))
                    )}
                  </SelectContent>
                </Select>
              </div>

              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="customModel">自定义模型</Label>
                <Input
                  id="customModel"
                  value={customModel}
                  onChange={(event) => setCustomModel(event.target.value)}
                  placeholder="留空则使用上方选择"
                />
              </div>

              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="instructions">instructions</Label>
                <Input
                  id="instructions"
                  value={testRequest.instructions}
                  onChange={(event) => setTestRequest((current) => ({ ...current, instructions: event.target.value }))}
                />
              </div>

              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="prompt">提示词</Label>
                <Textarea
                  id="prompt"
                  value={testRequest.prompt}
                  onChange={(event) => setTestRequest((current) => ({ ...current, prompt: event.target.value }))}
                  className="min-h-[120px]"
                />
              </div>

              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="thinkingEffort">思维强度</Label>
                <Input
                  id="thinkingEffort"
                  value={testRequest.thinkingEffort || ''}
                  onChange={(event) => setTestRequest((current) => ({ ...current, thinkingEffort: event.target.value }))}
                  placeholder="可选，例如 low / medium / high"
                />
              </div>
            </div>

            {testResult ? (
              <Alert variant={testResult.success ? 'default' : 'destructive'}>
                <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
                  <span>{testResult.message}</span>
                  <div className="flex items-center gap-2">
                    {typeof testResult.statusCode === 'number' ? <Badge variant="outline">HTTP {testResult.statusCode}</Badge> : null}
                    {typeof testResult.ttfbMs === 'number' ? <Badge variant="outline">TTFB {testResult.ttfbMs}ms</Badge> : null}
                  </div>
                </AlertDescription>
              </Alert>
            ) : null}

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setTestDialogChannel(null)}>关闭</Button>
              <Button type="button" onClick={() => void handleTest()} disabled={testing}>
                {testing ? '测试中...' : '开始测试'}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </AdminPageShell>
    </motion.div>
  )
}
