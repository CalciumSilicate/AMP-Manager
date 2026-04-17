import { useEffect, useMemo, useState } from 'react'

import { AnimatePresence, motion } from '@/lib/motion'
import { buildAPIKeyUsageContent, getAPIKeyAvailableModels, type CCSwitchApp } from '@/lib/api-key-usage'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import type { APIKeyRevealResponse } from '@/api/amp'

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
  const [availableModels, setAvailableModels] = useState<string[]>([])
  const [loadingModels, setLoadingModels] = useState(false)
  const [loadError, setLoadError] = useState('')

  useEffect(() => {
    if (!open || !revealKey) return

    const controller = new AbortController()
    setLoadingModels(true)
    setLoadError('')

    void getAPIKeyAvailableModels({
      origin: window.location.origin,
      apiBaseUrl,
      apiKey: revealKey.apiKey,
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
  }, [apiBaseUrl, open, revealKey])

  const usageContent = useMemo(() => {
    if (!revealKey || availableModels.length === 0) return null

    return buildAPIKeyUsageContent({
      origin: window.location.origin,
      apiBaseUrl,
      apiKey: revealKey.apiKey,
      keyName: revealKey.name,
      models: availableModels,
    })
  }, [apiBaseUrl, availableModels, revealKey])

  const renderContent = () => {
    if (!usageContent) return null

    switch (activeTab) {
      case 'cc-switch':
        return (
          <div className="space-y-4">
            <div className="grid gap-3 md:grid-cols-3">
              {ccSwitchApps.map(({ app, label }) => (
                <div key={app} className="rounded-lg border px-4 py-4 space-y-3">
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
              label="usageScript"
              value={usageContent.ccSwitchUsageScript}
              copyKey="cc-switch-usage-script"
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
        }
        onOpenChange(nextOpen)
      }}
    >
      <DialogContent className="max-w-5xl max-h-[88vh] overflow-hidden flex flex-col">
        <DialogHeader>
          <DialogTitle>{revealKey ? `使用 API Key · ${revealKey.name}` : '使用 API Key'}</DialogTitle>
        </DialogHeader>

        <div className="flex-1 min-h-0 overflow-y-auto pr-1 space-y-5">
          {revealKey ? (
            <div className="rounded-lg border px-4 py-4 space-y-3">
              <div className="flex items-center justify-between gap-3">
                <div className="text-sm font-medium text-foreground">连接信息</div>
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
            </div>
          ) : null}

          <div className="flex items-center gap-1 border-b pb-0 overflow-x-auto">
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
