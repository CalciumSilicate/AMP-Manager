import { useEffect, useMemo, useState } from 'react'

import { Check, ChevronsUpDown } from 'lucide-react'

import { type APIKeyRevealResponse } from '@/api/amp'
import { type AvailableModel } from '@/api/models'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { buildAPIKeyUsageContent, getAPIKeyAvailableModels, type CCSwitchApp } from '@/lib/api-key-usage'
import { AnimatePresence, motion } from '@/lib/motion'
import { cn } from '@/lib/utils'

type UsageTab = 'cc-switch' | 'codex-cli' | 'codex-websocket' | 'opencode' | 'openclaw'

interface APIKeyUsageDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  revealKey: APIKeyRevealResponse | null
  apiBaseUrl: string
  copied: string | null
  onCopy: (text: string, key: string) => Promise<void>
}

const tabs: { value: UsageTab; label: string }[] = [
  { value: 'cc-switch', label: 'CC Switch 导入' },
  { value: 'codex-cli', label: 'Codex CLI' },
  { value: 'codex-websocket', label: 'Codex CLI Websocket' },
  { value: 'opencode', label: 'Opencode' },
  { value: 'openclaw', label: 'Openclaw' },
]

const ccSwitchApps: { app: CCSwitchApp; label: string }[] = [
  { app: 'codex', label: 'Codex' },
  { app: 'opencode', label: 'OpenCode' },
  { app: 'openclaw', label: 'OpenClaw' },
]

export function APIKeyUsageDialog({ open, onOpenChange, revealKey, apiBaseUrl, copied, onCopy }: APIKeyUsageDialogProps) {
  const [activeTab, setActiveTab] = useState<UsageTab>('cc-switch')
  const [availableModels, setAvailableModels] = useState<AvailableModel[]>([])
  const [selectedModelId, setSelectedModelId] = useState('')
  const [loadingModels, setLoadingModels] = useState(false)
  const [loadError, setLoadError] = useState('')

  useEffect(() => {
    if (!open || !revealKey) return

    const controller = new AbortController()
    setLoadingModels(true)
    setLoadError('')

    void getAPIKeyAvailableModels({
      signal: controller.signal,
    })
      .then((models) => {
        setAvailableModels(models)
      })
      .catch((error) => {
        if (error instanceof DOMException && error.name === 'AbortError') {
          return
        }
        setAvailableModels([])
        setLoadError(error instanceof Error ? error.message : '获取模型列表失败')
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setLoadingModels(false)
        }
      })

    return () => controller.abort()
  }, [open, revealKey])

  useEffect(() => {
    if (availableModels.length === 0) {
      setSelectedModelId('')
      return
    }

    if (!selectedModelId || !availableModels.some((item) => item.modelId === selectedModelId)) {
      setSelectedModelId(availableModels[0].modelId)
    }
  }, [availableModels, selectedModelId])

  const usageContent = useMemo(() => {
    if (!revealKey || availableModels.length === 0 || !selectedModelId) return null

    return buildAPIKeyUsageContent({
      origin: window.location.origin,
      apiBaseUrl,
      apiKey: revealKey.apiKey,
      keyName: revealKey.name,
      models: availableModels,
      selectedModelId,
    })
  }, [apiBaseUrl, availableModels, revealKey, selectedModelId])

  const renderContent = () => {
    if (!usageContent) return null

    switch (activeTab) {
      case 'cc-switch':
        return (
          <div className="space-y-4">
            <div className="space-y-3">
              {ccSwitchApps.map(({ app, label }) => (
                <div key={app} className="flex flex-wrap items-center justify-between gap-3 rounded-lg border px-4 py-3">
                  <div className="text-sm font-medium">{label}</div>
                  <div className="flex flex-wrap gap-2">
                    <Button
                      size="sm"
                      onClick={() => {
                        window.location.assign(usageContent.ccSwitchLinks[app])
                      }}
                    >
                      一键导入
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => void onCopy(usageContent.ccSwitchLinks[app], `cc-switch-link-${app}`)}
                    >
                      {copied === `cc-switch-link-${app}` ? '已复制' : '复制链接'}
                    </Button>
                  </div>
                </div>
              ))}
            </div>

            <CodeBlock
              label="testConfig patch"
              value={usageContent.ccSwitchFallbackPatch}
              copyKey="cc-switch-test-config"
              copied={copied}
              onCopy={onCopy}
            />
          </div>
        )
      case 'codex-cli':
        return (
          <div className="space-y-4">
            <CodeBlock
              label="~/.codex/config.toml"
              value={usageContent.codex.configToml}
              copyKey="codex-config"
              copied={copied}
              onCopy={onCopy}
            />
            <CodeBlock
              label="~/.codex/auth.json"
              value={usageContent.codex.authJson}
              copyKey="codex-auth"
              copied={copied}
              onCopy={onCopy}
            />
            <CodeBlock
              label="command"
              value={usageContent.codex.command}
              copyKey="codex-command"
              copied={copied}
              onCopy={onCopy}
            />
          </div>
        )
      case 'codex-websocket':
        return (
          <div className="space-y-4">
            <CodeBlock
              label="~/.codex/config.toml"
              value={usageContent.codexWebsocket.configToml}
              copyKey="codex-ws-config"
              copied={copied}
              onCopy={onCopy}
            />
            <CodeBlock
              label="~/.codex/auth.json"
              value={usageContent.codexWebsocket.authJson}
              copyKey="codex-ws-auth"
              copied={copied}
              onCopy={onCopy}
            />
            <CodeBlock
              label="command"
              value={usageContent.codexWebsocket.command}
              copyKey="codex-ws-command"
              copied={copied}
              onCopy={onCopy}
            />
          </div>
        )
      case 'opencode':
        return (
          <div className="space-y-4">
            <CodeBlock
              label="~/.config/opencode/opencode.json"
              value={usageContent.opencode.configJson}
              copyKey="opencode-config"
              copied={copied}
              onCopy={onCopy}
            />
            <CodeBlock
              label="command"
              value={usageContent.opencode.command}
              copyKey="opencode-command"
              copied={copied}
              onCopy={onCopy}
            />
          </div>
        )
      case 'openclaw':
        return (
          <div className="space-y-4">
            <CodeBlock
              label="~/.openclaw/openclaw.json"
              value={usageContent.openclaw.configJson}
              copyKey="openclaw-config"
              copied={copied}
              onCopy={onCopy}
            />
            <CodeBlock
              label="command"
              value={usageContent.openclaw.command}
              copyKey="openclaw-command"
              copied={copied}
              onCopy={onCopy}
            />
          </div>
        )
      default:
        return null
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) {
          setActiveTab('cc-switch')
          setLoadError('')
          setAvailableModels([])
          setSelectedModelId('')
        }
        onOpenChange(nextOpen)
      }}
    >
      <DialogContent className="max-w-5xl max-h-[88vh] overflow-hidden flex flex-col">
        <DialogHeader>
          <DialogTitle>{revealKey ? `使用 API Key · ${revealKey.name}` : '使用 API Key'}</DialogTitle>
        </DialogHeader>

        <div className="flex-1 min-h-0 overflow-y-auto pr-1">
          {revealKey ? (
            <div className="space-y-3 pb-4">
              <div className="flex items-center justify-between gap-3">
                <div className="text-sm font-medium text-foreground">{revealKey.name}</div>
                <Badge variant="secondary">
                  {loadingModels ? '模型加载中' : usageContent ? `${usageContent.models.length} 个模型` : '模型未就绪'}
                </Badge>
              </div>
              <InlineCopyRow
                label="API Base URL"
                value={apiBaseUrl}
                copyKey="usage-base-url"
                copied={copied}
                onCopy={onCopy}
              />
              <InlineCopyRow
                label="API Key"
                value={revealKey.apiKey}
                copyKey="usage-api-key"
                copied={copied}
                onCopy={onCopy}
              />
              <ModelSelectRow
                value={selectedModelId}
                models={availableModels}
                disabled={loadingModels || availableModels.length === 0}
                onValueChange={setSelectedModelId}
              />
            </div>
          ) : null}

          <div className="sticky top-0 z-20 border-b bg-background/95 pb-0 backdrop-blur supports-[backdrop-filter]:bg-background/80">
            <div className="overflow-x-auto overflow-y-hidden [-ms-overflow-style:none] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
              <div className="flex min-w-max items-center gap-1">
                {tabs.map((tab) => (
                  <button
                    key={tab.value}
                    type="button"
                    onClick={() => setActiveTab(tab.value)}
                    className={`relative rounded-t-md px-4 py-2 text-sm font-medium transition-colors whitespace-nowrap ${
                      activeTab === tab.value ? 'text-foreground' : 'text-muted-foreground hover:text-foreground/80'
                    }`}
                  >
                    {tab.label}
                    {activeTab === tab.value ? (
                      <motion.div
                        layoutId="api-key-usage-tab-indicator"
                        className="absolute inset-x-0 -bottom-px h-0.5 bg-primary"
                        transition={{ type: 'spring', bounce: 0.2, duration: 0.4 }}
                      />
                    ) : null}
                  </button>
                ))}
              </div>
            </div>
          </div>

          <div className="pt-5">
            {loadingModels ? (
              <div className="rounded-lg border px-4 py-10 text-center text-sm text-muted-foreground">模型加载中...</div>
            ) : loadError ? (
              <div className="rounded-lg border border-destructive/40 px-4 py-10 text-center text-sm text-destructive">{loadError}</div>
            ) : usageContent ? (
              <AnimatePresence mode="wait">
                <motion.div
                  key={activeTab}
                  initial={{ opacity: 0, y: 16 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, y: -10 }}
                  transition={{ type: 'spring', bounce: 0.2, duration: 0.35 }}
                  className="space-y-4"
                >
                  {renderContent()}
                </motion.div>
              </AnimatePresence>
            ) : (
              <div className="rounded-lg border px-4 py-10 text-center text-sm text-muted-foreground">暂无模型</div>
            )}
          </div>
        </div>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)}>关闭</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function InlineCopyRow({
  label,
  value,
  copyKey,
  copied,
  onCopy,
}: {
  label: string
  value: string
  copyKey: string
  copied: string | null
  onCopy: (text: string, key: string) => Promise<void>
}) {
  return (
    <div className="grid gap-2 md:grid-cols-[140px_minmax(0,1fr)_auto] md:items-center">
      <Label className="text-sm">{label}</Label>
      <code className="rounded-md border bg-muted/30 px-3 py-2 text-xs font-mono break-all">{value}</code>
      <Button size="sm" variant="outline" onClick={() => void onCopy(value, copyKey)}>
        {copied === copyKey ? '已复制' : '复制'}
      </Button>
    </div>
  )
}

function ModelSelectRow({
  value,
  models,
  disabled,
  onValueChange,
}: {
  value: string
  models: AvailableModel[]
  disabled: boolean
  onValueChange: (value: string) => void
}) {
  return (
    <div className="grid gap-2 md:grid-cols-[140px_minmax(0,1fr)] md:items-center">
      <Label className="text-sm">主模型</Label>
      <ModelSelect
        value={value}
        models={models}
        disabled={disabled}
        onValueChange={onValueChange}
      />
    </div>
  )
}

function ModelSelect({
  value,
  models,
  disabled,
  onValueChange,
}: {
  value: string
  models: AvailableModel[]
  disabled: boolean
  onValueChange: (value: string) => void
}) {
  const [open, setOpen] = useState(false)
  const selected = models.find((item) => item.modelId === value)

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          disabled={disabled}
          className="justify-between font-normal h-9"
        >
          <span className="truncate">{selected ? formatModelLabel(selected) : '选择模型'}</span>
          <ChevronsUpDown className="ml-1 h-3.5 w-3.5 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[--radix-popover-trigger-width] p-0" align="start">
        <Command>
          <CommandInput placeholder="搜索模型..." />
          <CommandList>
            <CommandEmpty>无匹配项</CommandEmpty>
            <CommandGroup>
              {models.map((modelItem) => (
                <CommandItem
                  key={modelItem.modelId}
                  value={modelItem.modelId}
                  keywords={[modelItem.displayName, modelItem.channelType, modelItem.channelName]}
                  onSelect={() => {
                    onValueChange(modelItem.modelId)
                    setOpen(false)
                  }}
                >
                  <Check className={cn('mr-2 h-4 w-4', value === modelItem.modelId ? 'opacity-100' : 'opacity-0')} />
                  <div className="min-w-0 flex-1">
                    <div className="truncate">{formatModelLabel(modelItem)}</div>
                  </div>
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}

function formatModelLabel(modelItem: AvailableModel) {
  return modelItem.displayName && modelItem.displayName !== modelItem.modelId
    ? `${modelItem.displayName} (${modelItem.modelId})`
    : modelItem.modelId
}

function CodeBlock({
  label,
  value,
  copyKey,
  copied,
  onCopy,
}: {
  label: string
  value: string
  copyKey: string
  copied: string | null
  onCopy: (text: string, key: string) => Promise<void>
}) {
  return (
    <div className="rounded-lg border px-4 py-4 space-y-3">
      <div className="flex items-center justify-between gap-3">
        <Label>{label}</Label>
        <Button size="sm" variant="outline" onClick={() => void onCopy(value, copyKey)}>
          {copied === copyKey ? '已复制' : '复制'}
        </Button>
      </div>
      <div className="rounded-md border bg-muted/30 px-4 py-3 overflow-x-auto">
        <pre className="font-mono text-xs leading-6 whitespace-pre-wrap break-all">
          <code>{value}</code>
        </pre>
      </div>
    </div>
  )
}
