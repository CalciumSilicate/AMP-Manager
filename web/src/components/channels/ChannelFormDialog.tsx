import type { Dispatch, SetStateAction } from 'react'

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
  ChannelTranslator,
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

const DEFAULT_TRANSLATOR: ChannelTranslator = {
  compatible: false,
  responses: false,
  messages: false,
  gemini: false,
}

const TRANSLATOR_OPTIONS: { key: keyof ChannelTranslator; label: string; hint: string }[] = [
  { key: 'compatible', label: 'Compatible', hint: '允许 OpenAI Compatible 请求通过翻译器进入此渠道。' },
  { key: 'responses', label: 'Responses', hint: '允许 /v1/responses 请求通过翻译器进入此渠道。' },
  { key: 'messages', label: 'Messages', hint: '允许 Claude /v1/messages 请求通过翻译器进入此渠道。' },
  { key: 'gemini', label: 'Gemini', hint: '允许 Gemini generateContent 请求通过翻译器进入此渠道。' },
]

function getNativeTranslatorKey(type: ChannelType, endpoint?: ChannelEndpoint): keyof ChannelTranslator {
  if (type === 'openai') {
    return endpoint === 'responses' ? 'responses' : 'compatible'
  }
  if (type === 'claude') {
    return 'messages'
  }
  return 'gemini'
}

function normalizeTranslatorConfig(type: ChannelType, endpoint: ChannelEndpoint | undefined, translator?: ChannelTranslator): ChannelTranslator {
  const next = { ...DEFAULT_TRANSLATOR, ...(translator || {}) }
  next[getNativeTranslatorKey(type, endpoint)] = false
  return next
}

interface ChannelFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  editingChannel: Channel | null
  formData: ChannelRequest
  setFormData: Dispatch<SetStateAction<ChannelRequest>>
  onSubmit: () => void
  saving: boolean
  groups?: Group[]
}

interface SwitchRowProps {
  label: string
  hint: string
  checked: boolean
  onCheckedChange: (checked: boolean) => void
  disabled?: boolean
}

function SwitchRow({ label, hint, checked, onCheckedChange, disabled }: SwitchRowProps) {
  return (
    <div className={`col-span-2 flex items-center justify-between gap-4 border-t border-border/70 py-3 ${disabled ? 'opacity-60' : ''}`}>
      <div className="space-y-0.5">
        <Label>{label}</Label>
        <p className="text-xs text-muted-foreground">{hint}</p>
      </div>
      <Switch checked={checked} disabled={disabled} onCheckedChange={onCheckedChange} />
    </div>
  )
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
  const showCodexWebsocketToggle = formData.type === 'openai' && formData.endpoint === 'responses'
  const translatorState = normalizeTranslatorConfig(formData.type, formData.endpoint, formData.translator)
  const nativeTranslatorKey = getNativeTranslatorKey(formData.type, formData.endpoint)

  const handleTypeChange = (type: ChannelType) => {
    const channelType = CHANNEL_TYPES.find(t => t.value === type)
    if (channelType) {
      setFormData(prev => ({
        ...prev,
        type,
        baseUrl: prev.baseUrl || channelType.defaultUrl,
        endpoint: channelType.defaultEndpoint,
        codexWebsocketEnabled: type === 'openai' && channelType.defaultEndpoint === 'responses'
          ? prev.codexWebsocketEnabled
          : false,
        translator: normalizeTranslatorConfig(type, channelType.defaultEndpoint, prev.translator),
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
                onChange={(e) => {
                  const endpoint = e.target.value as ChannelEndpoint
                  setFormData(prev => ({
                    ...prev,
                    endpoint,
                    codexWebsocketEnabled: endpoint === 'responses' ? prev.codexWebsocketEnabled : false,
                    translator: normalizeTranslatorConfig(prev.type, endpoint, prev.translator),
                  }))
                }}
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

          {/* 启用开关 */}
          <SwitchRow
            label="启用渠道"
            hint="启用后此渠道参与路由。"
            checked={formData.enabled}
            onCheckedChange={(checked) => setFormData(prev => ({ ...prev, enabled: checked }))}
          />

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

          <div className="col-span-2 grid gap-4 md:grid-cols-3">
            <div className="space-y-2">
              <Label>权重</Label>
              <Input
                type="number"
                min={1}
                value={formData.weight}
                onChange={(e) => setFormData(prev => ({ ...prev, weight: parseInt(e.target.value) || 1 }))}
              />
            </div>
            <div className="space-y-2">
              <Label>优先级</Label>
              <Input
                type="number"
                min={0}
                value={formData.priority}
                onChange={(e) => setFormData(prev => ({ ...prev, priority: parseInt(e.target.value) || 0 }))}
              />
            </div>
            <div className="space-y-2">
              <Label>倍率</Label>
              <Input
                type="number"
                min={0}
                step="0.01"
                value={formData.rateMultiplier ?? 1}
                onChange={(e) => setFormData(prev => ({ ...prev, rateMultiplier: Number.parseFloat(e.target.value) || 0 }))}
              />
            </div>
          </div>

          <div className="col-span-2 border-t border-border/70 pt-4">
            <div className="space-y-1">
              <Label>翻译器</Label>
              <p className="text-xs text-muted-foreground">
                控制哪些入站协议可以通过 CLIProxyAPI translator SDK 转换后请求到这个渠道。
              </p>
            </div>
            <div className="mt-3 space-y-2">
              {TRANSLATOR_OPTIONS.map(option => (
                <div key={option.key} className="flex items-center justify-between gap-4 rounded-md border border-border/60 px-3 py-2">
                  <div className="space-y-0.5">
                    <Label>{option.label}</Label>
                    <p className="text-xs text-muted-foreground">
                      {option.key === nativeTranslatorKey ? '当前渠道原生支持该协议，无需启用翻译器。' : option.hint}
                    </p>
                  </div>
                  <Switch
                    checked={translatorState[option.key]}
                    disabled={option.key === nativeTranslatorKey}
                    onCheckedChange={(checked) => setFormData(prev => ({
                      ...prev,
                      translator: normalizeTranslatorConfig(
                        prev.type,
                        prev.endpoint,
                        { ...DEFAULT_TRANSLATOR, ...(prev.translator || {}), [option.key]: checked },
                      ),
                    }))}
                  />
                </div>
              ))}
            </div>
          </div>

          {showCodexWebsocketToggle && (
            <div className="col-span-2 flex items-center justify-between border-y py-3">
              <div className="space-y-0.5">
                <Label>Codex WebSocket</Label>
                <p className="text-sm text-muted-foreground">优先使用 /v1/responses WebSocket</p>
              </div>
              <Switch
                checked={formData.codexWebsocketEnabled || false}
                onCheckedChange={(checked) => setFormData(prev => ({ ...prev, codexWebsocketEnabled: checked }))}
              />
            </div>
          )}

          {/* 模拟 UA - 仅 Claude 类型显示 */}
          {formData.type === 'claude' && (
            <SwitchRow
              label="模拟 UA"
              hint="伪装为 claude-cli 的请求头。"
              checked={formData.simulateUa || false}
              disabled={formData.copilotApi || false}
              onCheckedChange={(checked) => setFormData(prev => ({ ...prev, simulateUa: checked }))}
            />
          )}

          {/* 模拟请求体 - 仅 Claude 类型显示 */}
          {formData.type === 'claude' && (
            <SwitchRow
              label="模拟请求体"
              hint="重构为 Claude CLI 兼容请求体。"
              checked={formData.simulateCli || false}
              disabled={formData.copilotApi || false}
              onCheckedChange={(checked) => setFormData(prev => ({ ...prev, simulateCli: checked }))}
            />
          )}

          {/* 模拟系统提示词 - 仅 Claude 类型显示 */}
          {formData.type === 'claude' && (
            <SwitchRow
              label="模拟系统提示词"
              hint="替换为 Claude Code 系统提示词。"
              checked={formData.simulateSystemPrompt || false}
              onCheckedChange={(checked) => setFormData(prev => ({ ...prev, simulateSystemPrompt: checked }))}
            />
          )}

          {/* 繁体化 - 仅 Claude 类型显示 */}
          {formData.type === 'claude' && (
            <SwitchRow
              label="繁体化"
              hint="请求转繁体，响应转回简体。"
              checked={formData.traditionalChinese || false}
              onCheckedChange={(checked) => setFormData(prev => ({ ...prev, traditionalChinese: checked }))}
            />
          )}

          {/* Copilot API 支持 - 仅 Claude 类型显示，与模拟 CLI 互斥 */}
          {formData.type === 'claude' && (
            <SwitchRow
              label="Copilot API 模式"
              hint="通过 x-session-id 传递会话信息。"
              checked={formData.copilotApi || false}
              disabled={formData.simulateCli || formData.simulateUa || false}
              onCheckedChange={(checked) => setFormData(prev => ({ ...prev, copilotApi: checked }))}
            />
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
