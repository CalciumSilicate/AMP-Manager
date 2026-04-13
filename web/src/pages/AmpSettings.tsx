import { useState, useEffect, FormEvent } from 'react'
import { motion } from '@/lib/motion'
import {
    getAmpSettings,
    updateAmpSettings,
    testAmpConnection,
    AmpSettings as AmpSettingsType,
    ModelMapping,
    WebSearchMode,
} from '../api/amp'
import ModelMappingEditor from '../components/ModelMappingEditor'
import { Button } from '@/components/ui/button'
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

const WEB_SEARCH_OPTIONS: { value: WebSearchMode; label: string }[] = [
    { value: 'upstream', label: '上游代理' },
    { value: 'builtin_free', label: '内置免费搜索' },
    { value: 'local_duckduckgo', label: '本地搜索' },
]

export default function AmpSettings() {
    const [settings, setSettings] = useState<AmpSettingsType | null>(null)
    const [enabled, setEnabled] = useState(true)
    const [upstreamUrl, setUpstreamUrl] = useState('')
    const [upstreamApiKey, setUpstreamApiKey] = useState('')
    const [modelMappings, setModelMappings] = useState<ModelMapping[]>([])
    const [nativeMode, setNativeMode] = useState(false)
    const [webSearchMode, setWebSearchMode] = useState<WebSearchMode>('upstream')
    const [showBalanceInAd, setShowBalanceInAd] = useState(false)
    const [socks5Proxy, setSocks5Proxy] = useState('')
    const [loading, setLoading] = useState(true)
    const [saving, setSaving] = useState(false)
    const [testing, setTesting] = useState(false)
    const [error, setError] = useState('')
    const [success, setSuccess] = useState('')
    const [testResult, setTestResult] = useState<{ success: boolean; message: string } | null>(null)

    useEffect(() => {
        loadSettings()
    }, [])

    const loadSettings = async () => {
        try {
            const data = await getAmpSettings()
            setSettings(data)
            setEnabled(data.enabled)
            setUpstreamUrl(data.upstreamUrl)
            setModelMappings(data.modelMappings || [])
            setNativeMode(data.nativeMode)
            setWebSearchMode(data.webSearchMode || 'upstream')
            setShowBalanceInAd(data.showBalanceInAd ?? false)
        } catch (err) {
            setError(err instanceof Error ? err.message : '加载设置失败')
        } finally {
            setLoading(false)
        }
    }

    const handleSubmit = async (e: FormEvent) => {
        e.preventDefault()
        setError('')
        setSuccess('')
        setSaving(true)

        try {
            const data = await updateAmpSettings({
                enabled,
                nativeMode,
                upstreamUrl,
                ...(upstreamApiKey ? { upstreamApiKey } : {}),
                ...(socks5Proxy ? { socks5Proxy } : {}),
                modelMappings,
                webSearchMode,
                showBalanceInAd,
            })
            setSettings(data)
            setUpstreamApiKey('')
            setSocks5Proxy('')
            setSuccess('设置已保存')
        } catch (err) {
            setError(err instanceof Error ? err.message : '保存失败')
        } finally {
            setSaving(false)
        }
    }

    const handleTest = async () => {
        setTestResult(null)
        setTesting(true)

        try {
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

    return (
        <motion.div
            initial={{ opacity: 0, y: 30 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ type: 'spring', bounce: 0.2, duration: 0.7 }}
            className="mx-auto max-w-6xl"
        >
            <form onSubmit={handleSubmit} className="space-y-6">
                <div className="space-y-2">
                    <h1 className="text-2xl font-semibold tracking-tight">Amp 设置</h1>
                    <p className="text-sm text-muted-foreground">连接与运行参数。</p>
                </div>

                {error && (
                    <Alert variant="destructive">
                        <AlertDescription>{error}</AlertDescription>
                    </Alert>
                )}
                {success && (
                    <Alert className="border-green-200 bg-green-50 text-green-800">
                        <AlertDescription>{success}</AlertDescription>
                    </Alert>
                )}

                <motion.div
                    initial={{ opacity: 0, x: -20 }}
                    animate={{ opacity: 1, x: 0 }}
                    transition={{ delay: 0.1, type: 'spring', bounce: 0.2, duration: 0.5 }}
                >
                    <Card>
                        <CardHeader className="space-y-1">
                            <CardTitle>基础设置</CardTitle>
                            <CardDescription>连接与模式</CardDescription>
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
                                            <Switch id="enabled" checked={enabled} onCheckedChange={setEnabled} />
                                        </div>
                                        <div className="flex items-start justify-between gap-4">
                                            <div className="space-y-1">
                                                <Label htmlFor="nativeMode">原生模式</Label>
                                                <p className="text-sm text-muted-foreground">开启后仅直连上游。</p>
                                            </div>
                                            <Switch id="nativeMode" checked={nativeMode} onCheckedChange={setNativeMode} />
                                        </div>
                                        {nativeMode && (
                                            <Alert className="border-amber-200 bg-amber-50 text-amber-800">
                                                <AlertDescription>原生模式已开启，以下设置将不生效。</AlertDescription>
                                            </Alert>
                                        )}
                                    </div>

                                    <div className={nativeMode ? 'flex h-full flex-col justify-between gap-3 opacity-50 pointer-events-none' : 'flex h-full flex-col justify-between gap-3'}>
                                        <div className="space-y-2">
                                            <Label htmlFor="webSearchMode">网页搜索模式</Label>
                                            <Select value={webSearchMode} onValueChange={(value) => setWebSearchMode(value as WebSearchMode)}>
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

                                <div className={nativeMode ? 'grid gap-4 opacity-50 pointer-events-none xl:grid-cols-[minmax(0,1fr)_18rem]' : 'grid gap-4 xl:grid-cols-[minmax(0,1fr)_18rem]'}>
                                    <div className="space-y-2">
                                        <Label htmlFor="upstreamUrl">Upstream URL</Label>
                                        <Input
                                            id="upstreamUrl"
                                            type="url"
                                            value={upstreamUrl}
                                            onChange={(e) => setUpstreamUrl(e.target.value)}
                                            placeholder="https://ampcode.com"
                                            className="h-10"
                                        />
                                    </div>

                                    <div className="space-y-2">
                                        <Label htmlFor="showBalanceInAd">显示余额提醒</Label>
                                        <div className="flex h-10 items-center justify-between rounded-md border border-input bg-background px-3">
                                            <span className="text-sm text-muted-foreground">在广告位显示余额</span>
                                            <Switch id="showBalanceInAd" checked={showBalanceInAd} onCheckedChange={setShowBalanceInAd} />
                                        </div>
                                    </div>

                                    <div className="space-y-2">
                                        <Label htmlFor="upstreamApiKey">Upstream API Key</Label>
                                        <div className="flex items-center gap-2">
                                            <Input
                                                id="upstreamApiKey"
                                                type="password"
                                                value={upstreamApiKey}
                                                onChange={(e) => setUpstreamApiKey(e.target.value)}
                                                placeholder={settings?.apiKeySet ? '••••••••（已设置，留空保持不变）' : '请输入 API Key'}
                                                className="h-10"
                                            />
                                            <Badge variant={settings?.apiKeySet ? 'default' : 'secondary'} className="shrink-0">
                                                {settings?.apiKeySet ? '已设置' : '未设置'}
                                            </Badge>
                                        </div>
                                    </div>

                                    <div className="space-y-2">
                                        <Label htmlFor="socks5Proxy">SOCKS5 代理</Label>
                                        <div className="flex items-center gap-2">
                                            <Input
                                                id="socks5Proxy"
                                                type="password"
                                                value={socks5Proxy}
                                                onChange={(e) => setSocks5Proxy(e.target.value)}
                                                placeholder={settings?.socks5ProxySet ? '••••••••（已设置，留空保持不变）' : 'socks5://user:pass@host:port'}
                                                className="h-10"
                                            />
                                            <Badge variant={settings?.socks5ProxySet ? 'default' : 'secondary'} className="shrink-0">
                                                {settings?.socks5ProxySet ? '已设置' : '未设置'}
                                            </Badge>
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </CardContent>
                    </Card>
                </motion.div>

                <motion.div
                    initial={{ opacity: 0, y: 20 }}
                    animate={{ opacity: 1, y: 0 }}
                    transition={{ delay: 0.2, type: 'spring', bounce: 0.2, duration: 0.6 }}
                >
                    <Card>
                        <CardHeader>
                            <CardTitle>映射规则</CardTitle>
                            <CardDescription>模型映射与路由。</CardDescription>
                        </CardHeader>
                        <CardContent>
                            <div className={nativeMode ? 'opacity-50 pointer-events-none' : ''}>
                                <ModelMappingEditor mappings={modelMappings} onChange={setModelMappings} />
                            </div>
                        </CardContent>
                    </Card>
                </motion.div>

                {testResult && (
                    <Alert
                        variant={testResult.success ? 'default' : 'destructive'}
                        className={testResult.success ? 'border-green-200 bg-green-50 text-green-800' : ''}
                    >
                        <AlertDescription>{testResult.message}</AlertDescription>
                    </Alert>
                )}

                <Card>
                    <CardFooter className="flex flex-col gap-3 pt-6 sm:flex-row sm:items-center sm:justify-between">
                        <Button type="button" variant="outline" onClick={handleTest} disabled={testing}>
                            {testing ? '测试中...' : '测试连接'}
                        </Button>
                        <Button type="submit" disabled={saving} onClick={handleSubmit}>
                            {saving ? '保存中...' : '保存设置'}
                        </Button>
                    </CardFooter>
                </Card>
            </form>
        </motion.div>
    )
}
