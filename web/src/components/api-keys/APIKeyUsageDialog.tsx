import { useMemo, useState, type ReactNode } from 'react'

import { buildAPIKeyUsageContent, type CCSwitchApp } from '@/lib/api-key-usage'
import { formatDateTime } from '@/lib/formatters'
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
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
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

const ccSwitchApps: { app: CCSwitchApp; title: string; desc: string }[] = [
  { app: 'codex', title: '导入到 Codex', desc: '导入 Codex Provider，并附带用量查询脚本。' },
  { app: 'opencode', title: '导入到 OpenCode', desc: '导入 OpenCode Provider，并附带用量查询脚本。' },
  { app: 'openclaw', title: '导入到 OpenClaw', desc: '导入 OpenClaw Provider，并附带用量查询脚本。' },
]

export function APIKeyUsageDialog({ open, onOpenChange, revealKey, apiBaseUrl, copied, onCopy }: APIKeyUsageDialogProps) {
  const [activeTab, setActiveTab] = useState<UsageTab>('cc-switch')

  const usageContent = useMemo(() => {
    if (!revealKey) return null

    return buildAPIKeyUsageContent({
      origin: window.location.origin,
      apiBaseUrl,
      apiKey: revealKey.apiKey,
      keyName: revealKey.name,
    })
  }, [apiBaseUrl, revealKey])

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) {
          setActiveTab('cc-switch')
        }
        onOpenChange(nextOpen)
      }}
    >
      <DialogContent className="max-w-5xl max-h-[85vh] overflow-hidden flex flex-col">
        <DialogHeader>
          <DialogTitle>使用 API Key</DialogTitle>
          <DialogDescription>按客户端复制配置，或直接一键导入到 CC Switch。</DialogDescription>
        </DialogHeader>

        {revealKey && usageContent ? (
          <>
            <div className="grid gap-3 py-2 md:grid-cols-3">
              <InfoCard
                label="名称"
                value={revealKey.name}
                extra={
                  <Badge variant="secondary">
                    {revealKey.expiresAt ? `到期 ${formatDateTime(revealKey.expiresAt)}` : '永不过期'}
                  </Badge>
                }
              />
              <CopyCard
                label="API Base URL"
                value={apiBaseUrl}
                copyKey="usage-base-url"
                copied={copied}
                onCopy={onCopy}
              />
              <CopyCard
                label="API Key"
                value={revealKey.apiKey}
                copyKey="usage-api-key"
                copied={copied}
                onCopy={onCopy}
              />
            </div>

            <Tabs value={activeTab} onValueChange={(value) => setActiveTab(value as UsageTab)} className="flex-1 min-h-0 flex flex-col">
              <div className="overflow-x-auto pb-2">
                <TabsList className="grid min-w-[860px] grid-cols-5">
                  {tabs.map((tab) => (
                    <TabsTrigger key={tab.value} value={tab.value}>
                      {tab.label}
                    </TabsTrigger>
                  ))}
                </TabsList>
              </div>

              <ScrollArea className="flex-1 pr-4">
                <TabsContent value="cc-switch" className="space-y-4 mt-0">
                  <div className="grid gap-3 md:grid-cols-2">
                    <InfoCard label="默认模型" value={usageContent.defaultModel} />
                    <CopyCard
                      label="用量查询接口"
                      value={usageContent.usageEndpoint}
                      copyKey="cc-switch-usage-endpoint"
                      copied={copied}
                      onCopy={onCopy}
                    />
                  </div>

                  <div className="grid gap-3 md:grid-cols-3">
                    {ccSwitchApps.map(({ app, title, desc }) => (
                      <div key={app} className="rounded-lg border px-4 py-4 space-y-3">
                        <div className="space-y-1">
                          <p className="text-sm font-medium text-foreground">{title}</p>
                          <p className="text-xs leading-5 text-muted-foreground">{desc}</p>
                        </div>
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
                    label="CC Switch usageScript"
                    description="用于导入后在 CC Switch 内查询余额与订阅额度。"
                    value={usageContent.ccSwitchUsageScript}
                    copyKey="cc-switch-usage-script"
                    copied={copied}
                    onCopy={onCopy}
                  />
                </TabsContent>

                <TabsContent value="codex-cli" className="space-y-4 mt-0">
                  <CodeBlock
                    label="~/.codex/config.toml"
                    description="持久化 OpenAI 兼容配置，适合常规 Codex CLI 使用。"
                    value={usageContent.codex.configToml}
                    copyKey="codex-config"
                    copied={copied}
                    onCopy={onCopy}
                    language="toml"
                  />
                  <CodeBlock
                    label="~/.codex/auth.json"
                    description="将当前 API Key 写入 Codex 鉴权文件。"
                    value={usageContent.codex.authJson}
                    copyKey="codex-auth"
                    copied={copied}
                    onCopy={onCopy}
                    language="json"
                  />
                  <CodeBlock
                    label="启动命令"
                    description="配置完成后直接启动 Codex。"
                    value={usageContent.codex.command}
                    copyKey="codex-command"
                    copied={copied}
                    onCopy={onCopy}
                    language="bash"
                  />
                </TabsContent>

                <TabsContent value="codex-websocket" className="space-y-4 mt-0">
                  <CodeBlock
                    label="~/.codex/config.toml"
                    description="显式启用 Responses WebSocket，适合走 /v1/responses 的实时回传。"
                    value={usageContent.codexWebsocket.configToml}
                    copyKey="codex-ws-config"
                    copied={copied}
                    onCopy={onCopy}
                    language="toml"
                  />
                  <CodeBlock
                    label="~/.codex/auth.json"
                    description="WebSocket 模式仍使用同一份 OpenAI 鉴权文件。"
                    value={usageContent.codexWebsocket.authJson}
                    copyKey="codex-ws-auth"
                    copied={copied}
                    onCopy={onCopy}
                    language="json"
                  />
                  <CodeBlock
                    label="启动命令"
                    description="保存配置后直接启动 Codex。"
                    value={usageContent.codexWebsocket.command}
                    copyKey="codex-ws-command"
                    copied={copied}
                    onCopy={onCopy}
                    language="bash"
                  />
                </TabsContent>

                <TabsContent value="opencode" className="space-y-4 mt-0">
                  <CodeBlock
                    label="~/.config/opencode/opencode.json"
                    description="OpenCode 本地 provider 配置，已包含当前 key 和 gpt-5.4 模型。"
                    value={usageContent.opencode.configJson}
                    copyKey="opencode-config"
                    copied={copied}
                    onCopy={onCopy}
                    language="json"
                  />
                  <CodeBlock
                    label="启动命令"
                    description="按 provider/model 指定默认模型运行。"
                    value={usageContent.opencode.command}
                    copyKey="opencode-command"
                    copied={copied}
                    onCopy={onCopy}
                    language="bash"
                  />
                </TabsContent>

                <TabsContent value="openclaw" className="space-y-4 mt-0">
                  <CodeBlock
                    label="~/.openclaw/openclaw.json"
                    description="OpenClaw 自定义 provider 配置，直接走 OpenAI Responses 兼容代理。"
                    value={usageContent.openclaw.configJson}
                    copyKey="openclaw-config"
                    copied={copied}
                    onCopy={onCopy}
                    language="json"
                  />
                  <CodeBlock
                    label="启动命令"
                    description="写入配置后直接启动 OpenClaw。"
                    value={usageContent.openclaw.command}
                    copyKey="openclaw-command"
                    copied={copied}
                    onCopy={onCopy}
                    language="bash"
                  />
                </TabsContent>
              </ScrollArea>
            </Tabs>
          </>
        ) : null}

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)}>关闭</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function InfoCard({ label, value, extra }: { label: string; value: string; extra?: ReactNode }) {
  return (
    <div className="rounded-lg border px-4 py-3 space-y-2">
      <div className="flex items-center justify-between gap-3">
        <Label>{label}</Label>
        {extra}
      </div>
      <p className="break-all font-mono text-xs text-foreground">{value}</p>
    </div>
  )
}

function CopyCard({
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
    <div className="rounded-lg border px-4 py-3 space-y-2">
      <div className="flex items-center justify-between gap-3">
        <Label>{label}</Label>
        <Button size="sm" variant="outline" onClick={() => void onCopy(value, copyKey)}>
          {copied === copyKey ? '已复制' : '复制'}
        </Button>
      </div>
      <p className="break-all font-mono text-xs text-foreground">{value}</p>
    </div>
  )
}

function CodeBlock({
  label,
  description,
  value,
  copyKey,
  copied,
  onCopy,
  language,
}: {
  label: string
  description: string
  value: string
  copyKey: string
  copied: string | null
  onCopy: (text: string, key: string) => Promise<void>
  language?: string
}) {
  return (
    <div className="rounded-lg border px-4 py-4 space-y-3">
      <div className="flex items-start justify-between gap-3">
        <div className="space-y-1">
          <Label>{label}</Label>
          <p className="text-xs leading-5 text-muted-foreground">{description}</p>
        </div>
        <Button size="sm" variant="outline" onClick={() => void onCopy(value, copyKey)}>
          {copied === copyKey ? '已复制' : '复制'}
        </Button>
      </div>
      <div className="rounded-md border bg-muted/30 px-4 py-3 overflow-x-auto">
        <pre className="font-mono text-xs leading-6 whitespace-pre-wrap break-all">
          <code className={language ? `language-${language}` : undefined}>{value}</code>
        </pre>
      </div>
    </div>
  )
}
