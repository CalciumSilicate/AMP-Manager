import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Channel,
  ChannelRequest,
  ChannelType,
  ChannelEndpoint,
  ChannelModel,
} from '@/api/channels'
import { Group } from '@/api/groups'
import ModelRulesEditor from './ModelRulesEditor'

const CHANNEL_TYPES: { value: ChannelType; label: string; defaultUrl: string; defaultEndpoint: ChannelEndpoint }[] = [
  { value: 'gemini', label: 'Gemini', defaultUrl: 'https://generativelanguage.googleapis.com', defaultEndpoint: 'generate_content' },
  { value: 'claude', label: 'Claude', defaultUrl: 'https://api.anthropic.com', defaultEndpoint: 'messages' },
  { value: 'openai', label: 'OpenAI', defaultUrl: 'https://api.openai.com', defaultEndpoint: 'chat_completions' },
]

const OPENAI_ENDPOINTS: { value: ChannelEndpoint; label: string }[] = [
  { value: 'chat_completions', label: '/v1/chat/completions' },
  { value: 'responses', label: '/v1/responses' },
]

interface ChannelFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  editingChannel: Channel | null
  formData: ChannelRequest
  setFormData: React.Dispatch<React.SetStateAction<ChannelRequest>>
  onSubmit: () => void
  saving: boolean
  groups?: Group[]
}

export function ChannelFormDialog({
  open,
  onOpenChange,
  editingChannel,
  formData,
  setFormData,
  onSubmit,
  saving,
  groups = [],
}: ChannelFormDialogProps) {
  const handleTypeChange = (type: ChannelType) => {
    const channelType = CHANNEL_TYPES.find(t => t.value === type)
    if (channelType) {
      setFormData(prev => ({
        ...prev,
        type,
        baseUrl: prev.baseUrl || channelType.defaultUrl,
        endpoint: channelType.defaultEndpoint,
      }))
    }
  }

  const handleAddHeader = () => {
    setFormData(prev => ({
      ...prev,
      headers: { ...(prev.headers || {}), '': '' },
    }))
  }

  const handleRemoveHeader = (key: string) => {
    setFormData(prev => {
      const newHeaders = { ...(prev.headers || {}) }
      delete newHeaders[key]
      return { ...prev, headers: newHeaders }
    })
  }

  const handleHeaderChange = (oldKey: string, newKey: string, value: string) => {
    setFormData(prev => {
      const newHeaders: Record<string, string> = {}
      for (const [k, v] of Object.entries(prev.headers || {})) {
        if (k === oldKey) {
          newHeaders[newKey] = value
        } else {
          newHeaders[k] = v
        }
      }
      return { ...prev, headers: newHeaders }
    })
  }

  const handleAddModel = () => {
    setFormData(prev => ({
      ...prev,
      models: [...(prev.models || []), { name: '', alias: '' }],
    }))
  }

  const handleRemoveModel = (index: number) => {
    setFormData(prev => ({
      ...prev,
      models: (prev.models || []).filter((_, i) => i !== index),
    }))
  }

  const handleModelChange = (index: number, field: keyof ChannelModel, value: string) => {
    setFormData(prev => {
      const newModels = [...(prev.models || [])]
      newModels[index] = { ...newModels[index], [field]: value }
      return { ...prev, models: newModels }
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{editingChannel ? '编辑渠道' : '添加渠道'}</DialogTitle>
          <DialogDescription>
            配置 API 代理渠道的基本信息和模型规则
          </DialogDescription>
        </DialogHeader>

        <div className="grid grid-cols-2 gap-4 py-4">
          {/* 类型选择 */}
          <div className="space-y-2">
            <Label>类型 *</Label>
            <select
              value={formData.type}
              onChange={(e) => handleTypeChange(e.target.value as ChannelType)}
              className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {CHANNEL_TYPES.map(t => (
                <option key={t.value} value={t.value}>{t.label}</option>
              ))}
            </select>
          </div>

          {/* OpenAI Endpoint 选择 */}
          {formData.type === 'openai' && (
            <div className="space-y-2">
              <Label>Endpoint</Label>
              <select
                value={formData.endpoint}
                onChange={(e) => setFormData(prev => ({ ...prev, endpoint: e.target.value as ChannelEndpoint }))}
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                {OPENAI_ENDPOINTS.map(e => (
                  <option key={e.value} value={e.value}>{e.label}</option>
                ))}
              </select>
            </div>
          )}

          {/* 非 OpenAI 类型时的占位 */}
          {formData.type !== 'openai' && <div />}

          {/* 名称 */}
          <div className="space-y-2">
            <Label>名称 *</Label>
            <Input
              value={formData.name}
              onChange={(e) => setFormData(prev => ({ ...prev, name: e.target.value }))}
              placeholder="渠道名称"
            />
          </div>

          {/* Base URL */}
          <div className="space-y-2">
            <Label>Base URL *</Label>
            <Input
              value={formData.baseUrl}
              onChange={(e) => setFormData(prev => ({ ...prev, baseUrl: e.target.value }))}
              placeholder="https://api.example.com"
            />
          </div>

          {/* API Key */}
          <div className="space-y-2 col-span-2">
            <Label>API Key {editingChannel ? '(留空保持不变)' : '*'}</Label>
            <Input
              type="password"
              value={formData.apiKey || ''}
              onChange={(e) => setFormData(prev => ({ ...prev, apiKey: e.target.value }))}
              placeholder={editingChannel ? '留空保持原有密钥' : '输入 API Key'}
            />
          </div>

          {/* 分组 */}
          <div className="space-y-2 col-span-2">
            <Label>分组</Label>
            <div className="flex flex-wrap gap-2 rounded-md border p-3 min-h-[40px]">
              {groups.length === 0 ? (
                <span className="text-sm text-muted-foreground">暂无分组</span>
              ) : (
                groups.map(g => {
                  const selected = (formData.groupIds || []).includes(g.id)
                  return (
                    <label key={g.id} className="flex items-center gap-1.5 cursor-pointer">
                      <input
                        type="checkbox"
                        checked={selected}
                        onChange={(e) => {
                          const current = formData.groupIds || []
                          const newIds = e.target.checked
                            ? [...current, g.id]
                            : current.filter(id => id !== g.id)
                          setFormData(prev => ({ ...prev, groupIds: newIds }))
                        }}
                        className="rounded border-input"
                      />
                      <span className="text-sm">{g.name}</span>
                    </label>
                  )
                })
              )}
            </div>
          </div>

          {/* 权重 */}
          <div className="space-y-2">
            <Label>权重</Label>
            <Input
              type="number"
              min={1}
              value={formData.weight}
              onChange={(e) => setFormData(prev => ({ ...prev, weight: parseInt(e.target.value) || 1 }))}
            />
          </div>

          {/* 优先级 */}
          <div className="space-y-2">
            <Label>优先级</Label>
            <Input
              type="number"
              min={0}
              value={formData.priority}
              onChange={(e) => setFormData(prev => ({ ...prev, priority: parseInt(e.target.value) || 0 }))}
            />
          </div>

          {/* 启用开关 */}
          <div className="col-span-2 flex items-center justify-between rounded-lg border p-4">
            <div className="space-y-0.5">
              <Label>启用渠道</Label>
              <p className="text-sm text-muted-foreground">
                启用后此渠道将参与负载均衡
              </p>
            </div>
            <Switch
              checked={formData.enabled}
              onCheckedChange={(checked) => setFormData(prev => ({ ...prev, enabled: checked }))}
            />
          </div>

          {/* 模拟 UA - 仅 Claude 类型显示 */}
          {formData.type === 'claude' && (
            <div className="col-span-2 flex items-center justify-between rounded-lg border p-4">
              <div className="space-y-0.5">
                <Label>模拟 UA</Label>
                <p className="text-sm text-muted-foreground">
                  启用后将伪装 User-Agent 为 claude-cli 并注入 X-Stainless SDK 指纹，需同时开启「模拟请求体」
                </p>
              </div>
              <Switch
                checked={formData.simulateUa || false}
                disabled={formData.copilotApi || false}
                onCheckedChange={(checked) => setFormData(prev => ({ ...prev, simulateUa: checked }))}
              />
            </div>
          )}

          {/* 模拟请求体 - 仅 Claude 类型显示 */}
          {formData.type === 'claude' && (
            <div className="col-span-2 flex items-center justify-between rounded-lg border p-4">
              <div className="space-y-0.5">
                <Label>模拟请求体</Label>
                <p className="text-sm text-muted-foreground">
                  启用后将过滤请求头（白名单模式）并重构请求体以匹配官方 Claude Code CLI 格式（system 注入、cache_control、thinking 等）
                </p>
              </div>
              <Switch
                checked={formData.simulateCli || false}
                disabled={formData.copilotApi || false}
                onCheckedChange={(checked) => setFormData(prev => ({ ...prev, simulateCli: checked }))}
              />
            </div>
          )}

          {/* 模拟系统提示词 - 仅 Claude 类型显示 */}
          {formData.type === 'claude' && (
            <div className="col-span-2 flex items-center justify-between rounded-lg border p-4">
              <div className="space-y-0.5">
                <Label>模拟系统提示词</Label>
                <p className="text-sm text-muted-foreground">
                  启用后将原始系统提示词移入消息中，替换为官方 Claude Code 系统提示词
                </p>
              </div>
              <Switch
                checked={formData.simulateSystemPrompt || false}
                onCheckedChange={(checked) => setFormData(prev => ({ ...prev, simulateSystemPrompt: checked }))}
              />
            </div>
          )}

          {/* 繁体化 - 仅 Claude 类型显示 */}
          {formData.type === 'claude' && (
            <div className="col-span-2 flex items-center justify-between rounded-lg border p-4">
              <div className="space-y-0.5">
                <Label>繁体化</Label>
                <p className="text-sm text-muted-foreground">
                  启用后将用户输入的简体中文转换为繁体中文发送，并将响应的繁体中文转换为简体中文返回
                </p>
              </div>
              <Switch
                checked={formData.traditionalChinese || false}
                onCheckedChange={(checked) => setFormData(prev => ({ ...prev, traditionalChinese: checked }))}
              />
            </div>
          )}

          {/* Copilot API 支持 - 仅 Claude 类型显示，与模拟 CLI 互斥 */}
          {formData.type === 'claude' && (
            <div className="col-span-2 flex items-center justify-between rounded-lg border p-4">
              <div className="space-y-0.5">
                <Label>Copilot API 模式</Label>
                <p className="text-sm text-muted-foreground">
                  启用后将通过 x-session-id 传递会话追踪信息，用于 copilot-api 代理。与模拟 CLI 互斥
                </p>
              </div>
              <Switch
                checked={formData.copilotApi || false}
                disabled={formData.simulateCli || formData.simulateUa || false}
                onCheckedChange={(checked) => setFormData(prev => ({ ...prev, copilotApi: checked }))}
              />
            </div>
          )}

          {/* 模型规则编辑器 */}
          <ModelRulesEditor
            models={formData.models}
            channelId={editingChannel?.id}
            modelWhitelist={formData.modelWhitelist || false}
            onModelWhitelistChange={(checked) => setFormData(prev => ({ ...prev, modelWhitelist: checked }))}
            onAdd={handleAddModel}
            onRemove={handleRemoveModel}
            onChange={handleModelChange}
            onSetModels={(models) => setFormData(prev => ({ ...prev, models }))}
          />

          {/* 自定义请求头 */}
          <div className="col-span-2 space-y-3">
            <div className="flex items-center justify-between">
              <Label>自定义请求头</Label>
              <Button type="button" variant="outline" size="sm" onClick={handleAddHeader}>
                添加
              </Button>
            </div>
            {Object.entries(formData.headers || {}).length > 0 ? (
              <div className="space-y-2">
                {Object.entries(formData.headers || {}).map(([key, value], index) => (
                  <div key={index} className="flex items-center gap-2">
                    <Input
                      value={key}
                      onChange={(e) => handleHeaderChange(key, e.target.value, value)}
                      placeholder="Header 名称，如 User-Agent"
                      className="flex-1"
                    />
                    <Input
                      value={value}
                      onChange={(e) => handleHeaderChange(key, key, e.target.value)}
                      placeholder="Header 值"
                      className="flex-1"
                    />
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => handleRemoveHeader(key)}
                      className="text-destructive hover:text-destructive shrink-0"
                    >
                      删除
                    </Button>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">
                未配置自定义请求头。可添加如 User-Agent 等自定义头部。
              </p>
            )}
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={onSubmit} disabled={saving}>
            {saving ? '保存中...' : '保存'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
