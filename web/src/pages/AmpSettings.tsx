import { useEffect, useState } from 'react'

import { motion } from '@/lib/motion'
import {
  getAmpSettings,
  testAmpConnection,
  updateAmpSettings,
  type AmpSettings as AmpSettingsType,
  type ModelMapping,
  type TestResult,
  type WebSearchMode,
} from '../api/amp'
import ModelMappingEditor from '../components/ModelMappingEditor'
import { TabbedSettingsPage, type TabbedSettingsPageTab } from '@/components/layout/TabbedSettingsPage'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

type AmpSettingsTab = 'route-mapping' | 'amp-upstream'

interface AmpSettingsDraft {
  enabled: boolean
  nativeMode: boolean
  routeMappingsEnabled: boolean
  upstreamUrl: string
  modelMappings: ModelMapping[]
  webSearchMode: WebSearchMode
  showBalanceInAd: boolean
  upstreamApiKey: string
  socks5Proxy: string
}

const WEB_SEARCH_OPTIONS: { value: WebSearchMode; label: string }[] = [
  { value: 'upstream', label: '上游代理' },
  { value: 'builtin_free', label: '内置免费搜索' },
  { value: 'local_duckduckgo', label: '本地搜索' },
]

const tabs: TabbedSettingsPageTab<AmpSettingsTab>[] = [
  { key: 'route-mapping', label: '路由映射' },
  { key: 'amp-upstream', label: 'Amp上游设置' },
]

const DEFAULT_DRAFT: AmpSettingsDraft = {
  enabled: true,
  nativeMode: false,
  routeMappingsEnabled: true,
  upstreamUrl: '',
  modelMappings: [],
  webSearchMode: 'upstream',
  showBalanceInAd: false,
  upstreamApiKey: '',
  socks5Proxy: '',
}

function buildDraft(settings: AmpSettingsType): AmpSettingsDraft {
  return {
    enabled: settings.enabled,
    nativeMode: settings.nativeMode,
    routeMappingsEnabled: settings.routeMappingsEnabled,
    upstreamUrl: settings.upstreamUrl,
    modelMappings: settings.modelMappings || [],
    webSearchMode: settings.webSearchMode || 'upstream',
    showBalanceInAd: settings.showBalanceInAd ?? false,
    upstreamApiKey: '',
    socks5Proxy: '',
  }
}

function syncUpstreamDraft(current: AmpSettingsDraft, settings: AmpSettingsType): AmpSettingsDraft {
  return {
    ...current,
    enabled: settings.enabled,
    nativeMode: settings.nativeMode,
    upstreamUrl: settings.upstreamUrl,
    webSearchMode: settings.webSearchMode || 'upstream',
    showBalanceInAd: settings.showBalanceInAd ?? false,
    upstreamApiKey: '',
    socks5Proxy: '',
  }
}

function syncRouteMappingDraft(current: AmpSettingsDraft, settings: AmpSettingsType): AmpSettingsDraft {
  return {
    ...current,
    routeMappingsEnabled: settings.routeMappingsEnabled,
    modelMappings: settings.modelMappings || [],
  }
}

export default function AmpSettings() {
  const [activeTab, setActiveTab] = useState<AmpSettingsTab>('route-mapping')
  const [settings, setSettings] = useState<AmpSettingsType | null>(null)
  const [draft, setDraft] = useState<AmpSettingsDraft>(DEFAULT_DRAFT)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [routeSaving, setRouteSaving] = useState(false)
  const [upstreamSaving, setUpstreamSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [testResult, setTestResult] = useState<TestResult | null>(null)

  useEffect(() => {
    void loadSettings()
  }, [])

  const showMessage = (type: 'success' | 'error', text: string) => {
    setMessage({ type, text })
    window.setTimeout(() => setMessage(null), 5000)
  }

  const loadSettings = async () => {
    try {
      setLoading(true)
      setLoadError('')
      const data = await getAmpSettings()
      setSettings(data)
      setDraft(buildDraft(data))
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : '加载设置失败')
    } finally {
      setLoading(false)
    }
  }

  const handleRouteMappingChange = (modelMappings: ModelMapping[]) => {
    setDraft((current) => ({
      ...current,
      modelMappings,
    }))
  }

  const updateUpstreamDraft = (patch: Partial<Omit<AmpSettingsDraft, 'modelMappings'>>) => {
    setDraft((current) => ({
      ...current,
      ...patch,
    }))
    setTestResult(null)
  }

  const handleSaveRouteMappings = async () => {
    try {
      setRouteSaving(true)
      setMessage(null)
      const data = await updateAmpSettings({
        modelMappings: draft.modelMappings,
        routeMappingsEnabled: draft.routeMappingsEnabled,
      })
      setSettings(data)
      setDraft((current) => syncRouteMappingDraft(current, data))
      showMessage('success', '路由设置已保存')
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '保存路由映射失败')
    } finally {
      setRouteSaving(false)
    }
  }

  const saveUpstreamSettings = async (notifySuccess: boolean) => {
    try {
      setUpstreamSaving(true)
      setMessage(null)
      const data = await updateAmpSettings({
        enabled: draft.enabled,
        nativeMode: draft.nativeMode,
        upstreamUrl: draft.upstreamUrl,
        ...(draft.upstreamApiKey ? { upstreamApiKey: draft.upstreamApiKey } : {}),
        ...(draft.socks5Proxy ? { socks5Proxy: draft.socks5Proxy } : {}),
        webSearchMode: draft.webSearchMode,
        showBalanceInAd: draft.showBalanceInAd,
      })
      setSettings(data)
      setDraft((current) => syncUpstreamDraft(current, data))
      if (notifySuccess) {
        showMessage('success', 'Amp上游设置已保存')
      }
      return true
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '保存 Amp 上游设置失败')
      return false
    } finally {
      setUpstreamSaving(false)
    }
  }

  const handleTestConnection = async () => {
    setTestResult(null)
    const saved = await saveUpstreamSettings(false)
    if (!saved) return

    try {
      setTesting(true)
      const result = await testAmpConnection()
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

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <div className="text-muted-foreground">加载中...</div>
      </div>
    )
  }

  if (!settings) {
    return (
      <motion.div initial={{ opacity: 0, y: 20 }} animate={{ opacity: 1, y: 0 }} className="mx-auto max-w-6xl space-y-4">
        <Alert variant="destructive">
          <AlertDescription>{loadError || '加载设置失败'}</AlertDescription>
        </Alert>
        <Button type="button" variant="outline" onClick={() => void loadSettings()}>
          重试
        </Button>
      </motion.div>
    )
  }

  return (
    <div className="mx-auto max-w-6xl">
      <TabbedSettingsPage
        title="路由设置"
        description="管理模型路由映射和 Amp 上游配置。"
        tabs={tabs}
        activeTab={activeTab}
        onTabChange={setActiveTab}
        indicatorId="route-settings-tab-indicator"
        message={message}
      >
        {activeTab === 'route-mapping' ? (
          <div className="space-y-4">
            <div className="flex items-center justify-between gap-4 rounded-md border border-input bg-background px-4 py-3">
              <Label htmlFor="routeMappingsEnabled">启用路由映射</Label>
              <Switch
                id="routeMappingsEnabled"
                checked={draft.routeMappingsEnabled}
                onCheckedChange={(routeMappingsEnabled) => setDraft((current) => ({ ...current, routeMappingsEnabled }))}
              />
            </div>

            <Card>
              <CardHeader>
                <CardTitle>路由映射</CardTitle>
                <CardDescription>配置模型映射、目标渠道和规则扩展。</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                {draft.nativeMode ? (
                  <Alert className="border-amber-200 bg-amber-50 text-amber-800">
                    <AlertDescription>原生模式开启时路由映射不生效。</AlertDescription>
                  </Alert>
                ) : null}

                <div className={draft.nativeMode ? 'opacity-50 pointer-events-none' : ''}>
                  <ModelMappingEditor mappings={draft.modelMappings} onChange={handleRouteMappingChange} />
                </div>
              </CardContent>
              <CardFooter className="justify-end">
                <Button type="button" onClick={() => void handleSaveRouteMappings()} disabled={routeSaving}>
                  {routeSaving ? '保存中...' : '保存路由设置'}
                </Button>
              </CardFooter>
            </Card>
          </div>
        ) : (
          <div className="space-y-6">
            <Card>
              <CardHeader className="space-y-1">
                <CardTitle>Amp上游设置</CardTitle>
                <CardDescription>连接与运行参数。</CardDescription>
              </CardHeader>
              <CardContent className="space-y-8">
                <div className="grid gap-8">
                  <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_18rem] lg:items-stretch">
                    <div className="flex h-full flex-col justify-between gap-4 border-b pb-5 lg:border-b-0 lg:pb-0">
                      <div className="flex items-start justify-between gap-4">
                        <div className="space-y-1">
                          <Label htmlFor="enabled">启用代理</Label>
                          <p className="text-sm text-muted-foreground">开启后启用增强功能。</p>
                        </div>
                        <Switch id="enabled" checked={draft.enabled} onCheckedChange={(enabled) => updateUpstreamDraft({ enabled })} />
                      </div>
                      <div className="flex items-start justify-between gap-4">
                        <div className="space-y-1">
                          <Label htmlFor="nativeMode">原生模式</Label>
                          <p className="text-sm text-muted-foreground">开启后仅直连上游。</p>
                        </div>
                        <Switch id="nativeMode" checked={draft.nativeMode} onCheckedChange={(nativeMode) => updateUpstreamDraft({ nativeMode })} />
                      </div>
                      {draft.nativeMode ? (
                        <Alert className="border-amber-200 bg-amber-50 text-amber-800">
                          <AlertDescription>原生模式已开启，以下设置将不生效。</AlertDescription>
                        </Alert>
                      ) : null}
                    </div>

                    <div className={draft.nativeMode ? 'flex h-full flex-col justify-between gap-3 opacity-50 pointer-events-none' : 'flex h-full flex-col justify-between gap-3'}>
                      <div className="space-y-2">
                        <Label htmlFor="webSearchMode">网页搜索模式</Label>
                        <Select value={draft.webSearchMode} onValueChange={(value) => updateUpstreamDraft({ webSearchMode: value as WebSearchMode })}>
                          <SelectTrigger id="webSearchMode" className="h-10 w-full">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {WEB_SEARCH_OPTIONS.map((option) => (
                              <SelectItem key={option.value} value={option.value}>
                                {option.label}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>
                    </div>
                  </div>

                  <div className={draft.nativeMode ? 'grid gap-4 opacity-50 pointer-events-none xl:grid-cols-[minmax(0,1fr)_18rem]' : 'grid gap-4 xl:grid-cols-[minmax(0,1fr)_18rem]'}>
                    <div className="space-y-2">
                      <Label htmlFor="upstreamUrl">Upstream URL</Label>
                      <Input
                        id="upstreamUrl"
                        type="url"
                        value={draft.upstreamUrl}
                        onChange={(event) => updateUpstreamDraft({ upstreamUrl: event.target.value })}
                        placeholder="https://ampcode.com"
                        className="h-10"
                      />
                    </div>

                    <div className="space-y-2">
                      <Label htmlFor="showBalanceInAd">显示余额提醒</Label>
                      <div className="flex h-10 items-center justify-between rounded-md border border-input bg-background px-3">
                        <span className="text-sm text-muted-foreground">在广告位显示余额</span>
                        <Switch id="showBalanceInAd" checked={draft.showBalanceInAd} onCheckedChange={(showBalanceInAd) => updateUpstreamDraft({ showBalanceInAd })} />
                      </div>
                    </div>

                    <div className="space-y-2">
                      <Label htmlFor="upstreamApiKey">Upstream API Key</Label>
                      <div className="flex items-center gap-2">
                        <Input
                          id="upstreamApiKey"
                          type="password"
                          value={draft.upstreamApiKey}
                          onChange={(event) => updateUpstreamDraft({ upstreamApiKey: event.target.value })}
                          placeholder={settings.apiKeySet ? '••••••••（已设置，留空保持不变）' : '请输入 API Key'}
                          className="h-10"
                        />
                        <Badge variant={settings.apiKeySet ? 'default' : 'secondary'} className="shrink-0">
                          {settings.apiKeySet ? '已设置' : '未设置'}
                        </Badge>
                      </div>
                    </div>

                    <div className="space-y-2">
                      <Label htmlFor="socks5Proxy">SOCKS5 代理</Label>
                      <div className="flex items-center gap-2">
                        <Input
                          id="socks5Proxy"
                          type="password"
                          value={draft.socks5Proxy}
                          onChange={(event) => updateUpstreamDraft({ socks5Proxy: event.target.value })}
                          placeholder={settings.socks5ProxySet ? '••••••••（已设置，留空保持不变）' : 'socks5://user:pass@host:port'}
                          className="h-10"
                        />
                        <Badge variant={settings.socks5ProxySet ? 'default' : 'secondary'} className="shrink-0">
                          {settings.socks5ProxySet ? '已设置' : '未设置'}
                        </Badge>
                      </div>
                    </div>
                  </div>
                </div>
              </CardContent>
              <CardFooter className="flex flex-col gap-3 pt-6 sm:flex-row sm:items-center sm:justify-between">
                <Button type="button" variant="outline" onClick={() => void handleTestConnection()} disabled={upstreamSaving || testing}>
                  {testing ? '测试中...' : '测试连接'}
                </Button>
                <Button type="button" onClick={() => void saveUpstreamSettings(true)} disabled={upstreamSaving || testing}>
                  {upstreamSaving ? '保存中...' : '保存 Amp 上游设置'}
                </Button>
              </CardFooter>
            </Card>

            {testResult ? (
              <Alert
                variant={testResult.success ? 'default' : 'destructive'}
                className={testResult.success ? 'border-green-200 bg-green-50 text-green-800' : ''}
              >
                <AlertDescription>{testResult.message}</AlertDescription>
              </Alert>
            ) : null}
          </div>
        )}
      </TabbedSettingsPage>
    </div>
  )
}
