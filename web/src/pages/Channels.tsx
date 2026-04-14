import { useState, useEffect } from 'react'
import { motion } from '@/lib/motion'
import {
  listChannels,
  createChannel,
  updateChannel,
  deleteChannel,
  setChannelEnabled,
  testChannel,
  Channel,
  ChannelRequest,
  TestChannelResult,
} from '../api/channels'
import { listGroups, Group } from '../api/groups'
import { fetchChannelModels } from '../api/models'
import { Button } from '@/components/ui/button'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { ChannelTable } from '@/components/channels/ChannelTable'
import { ChannelFormDialog } from '@/components/channels/ChannelFormDialog'
import { TestResultsDisplay } from '@/components/channels/TestResultsDisplay'

export default function Channels() {
  const [channels, setChannels] = useState<Channel[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [showForm, setShowForm] = useState(false)
  const [editingChannel, setEditingChannel] = useState<Channel | null>(null)
  const [testResults, setTestResults] = useState<Record<string, TestChannelResult>>({})
  const [fetchingModels, setFetchingModels] = useState<Record<string, boolean>>({})
  const [modelCounts, setModelCounts] = useState<Record<string, number>>({})
  const [groups, setGroups] = useState<Group[]>([])

  const [formData, setFormData] = useState<ChannelRequest>({
    type: 'openai',
    endpoint: 'chat_completions',
    name: '',
    baseUrl: 'https://api.openai.com',
    apiKey: '',
    enabled: true,
    weight: 1,
    priority: 100,
    groupIds: [],
    models: [],
    modelWhitelist: false,
    simulateCli: false,
    simulateUa: false,
    simulateSystemPrompt: false,
    traditionalChinese: false,
    codexWebsocketEnabled: false,
    headers: {},
  })
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    loadChannels()
    loadGroups()
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
    } catch {}
  }

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
      loadChannels()
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
      loadChannels()
    } catch (err) {
      setError(err instanceof Error ? err.message : '删除失败')
    }
  }

  const handleToggleEnabled = async (id: string, enabled: boolean) => {
    try {
      await setChannelEnabled(id, enabled)
      loadChannels()
    } catch (err) {
      setError(err instanceof Error ? err.message : '更新失败')
    }
  }

  const handleTest = async (id: string) => {
    try {
      const result = await testChannel(id)
      setTestResults(prev => ({ ...prev, [id]: result }))
    } catch (err) {
      setTestResults(prev => ({
        ...prev,
        [id]: { success: false, message: err instanceof Error ? err.message : '测试失败' },
      }))
    }
  }

  const handleFetchModels = async (id: string) => {
    try {
      setFetchingModels(prev => ({ ...prev, [id]: true }))
      const result = await fetchChannelModels(id)
      setModelCounts(prev => ({ ...prev, [id]: result.count }))
    } catch (err) {
      setError(err instanceof Error ? err.message : '获取模型失败')
    } finally {
      setFetchingModels(prev => ({ ...prev, [id]: false }))
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
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="mx-auto max-w-6xl space-y-6">
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <motion.div initial={{ opacity: 0, y: 30 }} animate={{ opacity: 1, y: 0 }} transition={{ type: 'spring', bounce: 0.2, duration: 0.6, delay: 0.1 }}>
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-4">
          <div>
            <CardTitle>渠道管理</CardTitle>
            <CardDescription>管理 API 代理渠道配置</CardDescription>
          </div>
          <Button onClick={handleCreate}>添加渠道</Button>
        </CardHeader>
        <CardContent>
          {channels.length === 0 ? (
            <div className="rounded-md border border-dashed p-8 text-center text-muted-foreground">
              暂无渠道，点击上方按钮添加
            </div>
          ) : (
            <ChannelTable
              channels={channels}
              testResults={testResults}
              fetchingModels={fetchingModels}
              modelCounts={modelCounts}
              onToggleEnabled={handleToggleEnabled}
              onTest={handleTest}
              onFetchModels={handleFetchModels}
              onEdit={handleEdit}
              onDelete={handleDelete}
            />
          )}

          <TestResultsDisplay testResults={testResults} channels={channels} />
        </CardContent>
      </Card>
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

      <motion.div initial={{ opacity: 0, y: 30 }} animate={{ opacity: 1, y: 0 }} transition={{ type: 'spring', bounce: 0.2, duration: 0.6, delay: 0.2 }}>
      <Card>
        <CardHeader>
          <CardTitle>渠道路由说明</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2 text-sm text-muted-foreground">
          <p>• <strong>模型匹配</strong>：根据请求中的 model 字段自动选择合适的渠道</p>
          <p>• <strong>默认匹配</strong>：如果未配置模型规则，将根据类型自动匹配（gemini-* → Gemini, claude-* → Claude, gpt-* → OpenAI）</p>
          <p>• <strong>负载均衡</strong>：多个相同类型渠道时，按优先级和权重进行轮询分配</p>
          <p>• <strong>回退机制</strong>：如果没有匹配的渠道，请求将转发到用户配置的上游地址</p>
        </CardContent>
      </Card>
      </motion.div>
    </motion.div>
  )
}
