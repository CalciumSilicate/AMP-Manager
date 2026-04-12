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
import { Separator } from '@/components/ui/separator'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'

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
                    <p className="text-sm text-muted-foreground">配置 Amp 代理服务的上游连接、运行模式和模型映射。</p>
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
                            <CardDescription>保留现有设置项，将连接、模式和搜索策略整理为更易读的分区表单。</CardDescription>
                        </CardHeader>
                        <CardContent className="space-y-8">
                            <div className="grid gap-6 lg:grid-cols-[minmax(0,1.2fr)_minmax(18rem,0.8fr)]">
                                <div className="space-y-6">
                                    <div className="rounded-lg border p-4">
                                        <div className="space-y-4">
                                            <div className="flex items-start justify-between gap-4">
                                                <div className="space-y-0.5">
                                                    <Label htmlFor="enabled">启用代理</Label>
                                                    <p className="text-sm text-muted-foreground">开启后启用 AMP 增强功能，关闭则纯透传到上游。</p>
                                                </div>
                                                <Switch id="enabled" checked={enabled} onCheckedChange={setEnabled} />
                                            </div>
                                            <Separator />
                                            <div className="flex items-start justify-between gap-4">
                                                <div className="space-y-0.5">
                                                    <Label htmlFor="nativeMode">原生模式</Label>
                                                    <p className="text-sm text-muted-foreground">开启后所有请求直接转发到上游，不做拦截、映射或渠道路由。</p>
                                                </div>
                                                <Switch id="nativeMode" checked={nativeMode} onCheckedChange={setNativeMode} />
                                            </div>
                                            {nativeMode && (
                                                <Alert className="border-amber-200 bg-amber-50 text-amber-800">
                                                    <AlertDescription>原生模式已开启，以下设置将不生效。所有请求将直接转发到上游服务器。</AlertDescription>
                                                </Alert>
                                            )}
                                        </div>
                                    </div>

                                    <div className={nativeMode ? 'space-y-6 opacity-50 pointer-events-none' : 'space-y-6'}>
                                        <div className="grid gap-6 xl:grid-cols-2">
                                            <div className="space-y-2 xl:col-span-2">
                                                <Label htmlFor="upstreamUrl">Upstream URL</Label>
                                                <Input
                                                    id="upstreamUrl"
                                                    type="url"
                                                    value={upstreamUrl}
                                                    onChange={(e) => setUpstreamUrl(e.target.value)}
                                                    placeholder="https://ampcode.com"
                                                />
                                            </div>

                                            <div className="space-y-2">
                                                <Label htmlFor="upstreamApiKey">Upstream API Key</Label>
                                                <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
                                                    <Input
                                                        id="upstreamApiKey"
                                                        type="password"
                                                        value={upstreamApiKey}
                                                        onChange={(e) => setUpstreamApiKey(e.target.value)}
                                                        placeholder={settings?.apiKeySet ? '••••••••（已设置，留空保持不变）' : '请输入 API Key'}
                                                        className="flex-1"
                                                    />
                                                    <Badge variant={settings?.apiKeySet ? 'default' : 'secondary'} className="w-fit shrink-0">
                                                        {settings?.apiKeySet ? '已设置' : '未设置'}
                                                    </Badge>
                                                </div>
                                            </div>

                                            <div className="space-y-2">
                                                <Label htmlFor="socks5Proxy">SOCKS5 代理</Label>
                                                <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
                                                    <Input
                                                        id="socks5Proxy"
                                                        type="password"
                                                        value={socks5Proxy}
                                                        onChange={(e) => setSocks5Proxy(e.target.value)}
                                                        placeholder={settings?.socks5ProxySet ? '••••••••（已设置，留空保持不变）' : 'socks5://user:pass@host:port'}
                                                        className="flex-1"
                                                    />
                                                    <Badge variant={settings?.socks5ProxySet ? 'default' : 'secondary'} className="w-fit shrink-0">
                                                        {settings?.socks5ProxySet ? '已设置' : '未设置'}
                                                    </Badge>
                                                </div>
                                                <p className="text-sm text-muted-foreground">设置 SOCKS5 代理后，所有到 ampcode.com 的请求将通过此代理转发。格式：socks5://用户名:密码@主机:端口</p>
                                            </div>
                                        </div>
                                    </div>
                                </div>

                                <div className={nativeMode ? 'space-y-6 opacity-50 pointer-events-none' : 'space-y-6'}>
                                    <div className="rounded-lg border p-4">
                                        <div className="space-y-3">
                                            <div className="space-y-0.5">
                                                <Label>网页搜索模式</Label>
                                                <p className="text-sm text-muted-foreground">选择网页搜索和网页内容获取的处理方式。</p>
                                            </div>
                                            <RadioGroup
                                                value={webSearchMode}
                                                onValueChange={(value) => setWebSearchMode(value as WebSearchMode)}
                                                className="space-y-3"
                                            >
                                                <div className="rounded-md border p-3">
                                                    <div className="flex items-start space-x-3">
                                                        <RadioGroupItem value="upstream" id="upstream" className="mt-0.5" />
                                                        <Label htmlFor="upstream" className="cursor-pointer space-y-1 font-normal">
                                                            <span className="block font-medium">上游代理</span>
                                                            <span className="block text-sm text-muted-foreground">直接转发到上游，不做任何修改。</span>
                                                        </Label>
                                                    </div>
                                                </div>
                                                <div className="rounded-md border p-3">
                                                    <div className="flex items-start space-x-3">
                                                        <RadioGroupItem value="builtin_free" id="builtin_free" className="mt-0.5" />
                                                        <Label htmlFor="builtin_free" className="cursor-pointer space-y-1 font-normal">
                                                            <span className="block font-medium">内置免费搜索</span>
                                                            <span className="block text-sm text-muted-foreground">使用 Amp 内置的免费搜索功能。</span>
                                                        </Label>
                                                    </div>
                                                </div>
                                                <div className="rounded-md border p-3">
                                                    <div className="flex items-start space-x-3">
                                                        <RadioGroupItem value="local_duckduckgo" id="local_duckduckgo" className="mt-0.5" />
                                                        <Label htmlFor="local_duckduckgo" className="cursor-pointer space-y-1 font-normal">
                                                            <span className="block font-medium">本地搜索</span>
                                                            <span className="block text-sm text-muted-foreground">使用本地 DuckDuckGo 搜索，完全免费。</span>
                                                        </Label>
                                                    </div>
                                                </div>
                                            </RadioGroup>
                                        </div>
                                    </div>

                                    <div className="rounded-lg border p-4">
                                        <div className="flex items-start justify-between gap-4">
                                            <div className="space-y-0.5">
                                                <Label htmlFor="showBalanceInAd">显示余额提醒</Label>
                                                <p className="text-sm text-muted-foreground">在 Amp CLI 的广告位显示当前账户余额。</p>
                                            </div>
                                            <Switch id="showBalanceInAd" checked={showBalanceInAd} onCheckedChange={setShowBalanceInAd} />
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
                            <CardTitle>Model Mappings</CardTitle>
                            <CardDescription>映射编辑器作为独立工作区展示，优先保证宽表格可读性和批量编辑体验。</CardDescription>
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
