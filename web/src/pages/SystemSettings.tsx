import { useState, useEffect, useRef, useCallback } from 'react'
import { motion, AnimatePresence } from '@/lib/motion'
import { formatDateTime } from '@/lib/formatters'
import { SITE_TIME_ZONE_OPTIONS } from '@/lib/site-config'
import { getPurchaseSettings, updatePurchaseSettings } from '@/api/purchase'
import {
  getDatabaseInfo,
  DatabaseInfo,
  getDatabaseMigrationTask,
  DatabaseMigrationTask,
  startDatabaseMigration,
  StartDatabaseMigrationRequest,
  uploadDatabase,
  downloadDatabase,
  listBackups,
  restoreBackup,
  deleteBackup,
  Backup,
  getRetryConfig,
  updateRetryConfig,
  RetryConfig,
  getErrorRules,
  updateErrorRules,
  ErrorRule,
  getRequestPayloadLimit,
  updateRequestPayloadLimit,
  getRequestDetailConfig,
  updateRequestDetailConfig,
  RequestDetailConfig,
  getTimeoutConfig,
  updateTimeoutConfig,
  TimeoutConfig,
  getCacheTTLConfig,
  updateCacheTTLConfig,
  updateSiteConfig,
  getSessionStickyConfig,
  updateSessionStickyConfig,
  getSessionStickyRuntimeStatus,
  getBillingRuntimeConfig,
  getBillingRuntimeStats,
  updateBillingRuntimeConfig,
  BillingRuntimeConfig,
  BillingRuntimeStats,
  getBillingDailyResetConfig,
  updateBillingDailyResetConfig,
  BillingDailyResetConfig,
  SessionStickyConfig,
  SessionStickyRuntimeStatus,
  SiteContactConfig,
  type AmpProxySettingsPolicy,
} from '../api/system'
import { Button } from '@/components/ui/button'
import { AnnouncementAdminSection } from '@/components/announcements/AnnouncementAdminSection'
import { StatusMonitorSettingsPanel } from '@/components/system/StatusMonitorSettingsPanel'
import { SecuritySettingsPanel } from '@/components/system/SecuritySettingsPanel'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Progress } from '@/components/ui/progress'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { RefreshCw } from 'lucide-react'

type SettingsTab = 'site' | 'security' | 'status-monitor' | 'announcements' | 'database' | 'retry' | 'error-rules' | 'monitoring' | 'cache' | 'timeout' | 'billing'

const tabs: { key: SettingsTab; label: string }[] = [
  { key: 'site', label: '网站配置' },
  { key: 'security', label: '安全' },
  { key: 'status-monitor', label: '状态监控' },
  { key: 'announcements', label: '公告管理' },
  { key: 'database', label: '数据库管理' },
  { key: 'retry', label: '重试策略' },
  { key: 'error-rules', label: '错误规则' },
  { key: 'monitoring', label: '请求监控' },
  { key: 'cache', label: '缓存配置' },
  { key: 'timeout', label: '超时配置' },
  { key: 'billing', label: 'Redis计费' },
]

interface Props {
  siteName: string
  siteTimeZone: string
  ampProxySettingsPolicy: AmpProxySettingsPolicy
  ampSettingsPolicy: AmpProxySettingsPolicy
  siteContact: SiteContactConfig
  onSiteNameChange: (siteName: string) => void
  onSiteTimeZoneChange: (timeZone: string) => void
  onAmpProxySettingsPolicyChange: (policy: AmpProxySettingsPolicy) => void
  onAmpSettingsPolicyChange: (policy: AmpProxySettingsPolicy) => void
  onSiteContactChange: (contact: SiteContactConfig) => void
}

export default function SystemSettings({
  siteName,
  siteTimeZone,
  ampProxySettingsPolicy,
  ampSettingsPolicy,
  siteContact,
  onSiteNameChange,
  onSiteTimeZoneChange,
  onAmpProxySettingsPolicyChange,
  onAmpSettingsPolicyChange,
  onSiteContactChange,
}: Props) {
  const [activeTab, setActiveTab] = useState<SettingsTab>('site')

  const [backups, setBackups] = useState<Backup[]>([])
  const [databaseInfo, setDatabaseInfo] = useState<DatabaseInfo | null>(null)
  const [databaseInfoLoading, setDatabaseInfoLoading] = useState(false)
  const [migrationTask, setMigrationTask] = useState<DatabaseMigrationTask | null>(null)
  const [migrationStarting, setMigrationStarting] = useState(false)
  const [migrationTargetType, setMigrationTargetType] = useState<'sqlite' | 'postgres'>('postgres')
  const [migrationTargetSqlitePath, setMigrationTargetSqlitePath] = useState('./data/data.db')
  const [migrationTargetDatabaseUrl, setMigrationTargetDatabaseUrl] = useState('postgres://postgres:mysecretpassword@localhost:5432/ampmanager?sslmode=disable')
  const [migrationClearTarget, setMigrationClearTarget] = useState(true)
  const [migrationWithArchive, setMigrationWithArchive] = useState(true)
  const [loading, setLoading] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [retryConfig, setRetryConfig] = useState<RetryConfig | null>(null)
  const [retryLoading, setRetryLoading] = useState(false)
  const [errorRules, setErrorRules] = useState<ErrorRule[]>([])
  const [errorRulesLoading, setErrorRulesLoading] = useState(false)
  const [requestPayloadLimitMB, setRequestPayloadLimitMB] = useState(128)
  const [requestDetailConfig, setRequestDetailConfig] = useState<RequestDetailConfig | null>(null)
  const [requestDetailLoading, setRequestDetailLoading] = useState(false)
  const [requestDetailLoadError, setRequestDetailLoadError] = useState<string | null>(null)
  const [timeoutConfig, setTimeoutConfig] = useState<TimeoutConfig | null>(null)
  const [timeoutLoading, setTimeoutLoading] = useState(false)
  const [cacheTTL, setCacheTTL] = useState<string>('1h')
  const [cacheTTLLoading, setCacheTTLLoading] = useState(false)
  const [siteNameInput, setSiteNameInput] = useState(siteName)
  const [siteTimeZoneInput, setSiteTimeZoneInput] = useState(siteTimeZone)
  const [ampProxySettingsPolicyInput, setAmpProxySettingsPolicyInput] = useState<AmpProxySettingsPolicy>(ampProxySettingsPolicy)
  const [ampSettingsPolicyInput, setAmpSettingsPolicyInput] = useState<AmpProxySettingsPolicy>(ampSettingsPolicy)
  const [siteContactInput, setSiteContactInput] = useState<SiteContactConfig>(siteContact)
  const [siteConfigSaving, setSiteConfigSaving] = useState(false)
  const [balanceTopupPriceInput, setBalanceTopupPriceInput] = useState('0')
  const [balanceTopupSaving, setBalanceTopupSaving] = useState(false)
  const [billingDailyResetConfig, setBillingDailyResetConfig] = useState<BillingDailyResetConfig | null>(null)
  const [billingDailyResetSaving, setBillingDailyResetSaving] = useState(false)
  const [sessionStickyConfig, setSessionStickyConfig] = useState<SessionStickyConfig | null>(null)
  const [sessionStickyRuntime, setSessionStickyRuntime] = useState<SessionStickyRuntimeStatus | null>(null)
  const [sessionStickyLoading, setSessionStickyLoading] = useState(false)
  const [billingRuntimeConfig, setBillingRuntimeConfig] = useState<BillingRuntimeConfig | null>(null)
  const [billingRuntimeLoading, setBillingRuntimeLoading] = useState(false)
  const [billingRuntimeStats, setBillingRuntimeStats] = useState<BillingRuntimeStats | null>(null)
  const [billingRuntimeStatsLoading, setBillingRuntimeStatsLoading] = useState(false)

  useEffect(() => {
    setSiteNameInput(siteName)
  }, [siteName])

  useEffect(() => {
    setSiteTimeZoneInput(siteTimeZone)
  }, [siteTimeZone])

  useEffect(() => {
    setAmpProxySettingsPolicyInput(ampProxySettingsPolicy)
  }, [ampProxySettingsPolicy])

  useEffect(() => {
    setAmpSettingsPolicyInput(ampSettingsPolicy)
  }, [ampSettingsPolicy])

  useEffect(() => {
    setSiteContactInput(siteContact)
  }, [siteContact])

  const showMessage = useCallback((type: 'success' | 'error', text: string) => {
    setMessage({ type, text })
    setTimeout(() => setMessage(null), 5000)
  }, [])

  const fetchBackups = useCallback(async () => {
    try {
      const data = await listBackups()
      setBackups(data)
    } catch (err) {
      console.error('获取备份列表失败:', err)
    }
  }, [])

  const fetchDatabaseInfo = useCallback(async () => {
    setDatabaseInfoLoading(true)
    try {
      const data = await getDatabaseInfo()
      setDatabaseInfo(data)
      setMigrationTargetType(data.currentType === 'sqlite' ? 'postgres' : 'sqlite')
      setMigrationTargetSqlitePath(data.sqlitePath || './data/data.db')
      if (data.databaseURL) {
        setMigrationTargetDatabaseUrl(data.databaseURL)
      }
      if (data.supportsFileBackups) {
        await fetchBackups()
      } else {
        setBackups([])
      }
    } catch (err) {
      console.error('获取数据库信息失败:', err)
    } finally {
      setDatabaseInfoLoading(false)
    }
  }, [fetchBackups])

  const fetchRetryConfig = useCallback(async () => {
    try {
      const data = await getRetryConfig()
      setRetryConfig(data)
    } catch (err) {
      console.error('获取重试配置失败:', err)
    }
  }, [])

  const fetchRequestPayloadLimit = useCallback(async () => {
    try {
      const data = await getRequestPayloadLimit()
      setRequestPayloadLimitMB(Math.max(1, Math.round(data.maxBytes / 1024 / 1024)))
    } catch (err) {
      console.error('获取请求体上限失败:', err)
    }
  }, [])

  const fetchErrorRules = useCallback(async () => {
    try {
      const data = await getErrorRules()
      setErrorRules(data.rules)
    } catch (err) {
      console.error('获取错误规则失败:', err)
    }
  }, [])

  const fetchRequestDetailConfig = useCallback(async () => {
    setRequestDetailLoading(true)
    setRequestDetailLoadError(null)
    try {
      const data = await getRequestDetailConfig()
      setRequestDetailConfig(data)
    } catch (err) {
      console.error('获取请求详情监控配置失败:', err)
      setRequestDetailLoadError(err instanceof Error ? err.message : '获取请求详情监控配置失败')
    } finally {
      setRequestDetailLoading(false)
    }
  }, [])

  const fetchTimeoutConfig = useCallback(async () => {
    try {
      const data = await getTimeoutConfig()
      setTimeoutConfig(data)
    } catch (err) {
      console.error('获取超时配置失败:', err)
    }
  }, [])

  const fetchCacheTTLConfig = useCallback(async () => {
    try {
      const data = await getCacheTTLConfig()
      setCacheTTL(data.cacheTTL)
    } catch (err) {
      console.error('获取缓存TTL配置失败:', err)
    }
  }, [])

  const fetchBillingRuntimeConfig = useCallback(async () => {
    try {
      const data = await getBillingRuntimeConfig()
      setBillingRuntimeConfig(data)
    } catch (err) {
      console.error('获取 Redis 计费配置失败:', err)
    }
  }, [])

  const fetchBalanceTopupSettings = useCallback(async () => {
    try {
      const data = await getPurchaseSettings()
      setBalanceTopupPriceInput(String(data.balanceTopupPriceCnyPerUsd || 0))
    } catch (err) {
      console.error('获取余额充值单价失败:', err)
    }
  }, [])

  const fetchBillingDailyResetConfig = useCallback(async () => {
    try {
      const data = await getBillingDailyResetConfig()
      setBillingDailyResetConfig(data)
    } catch (err) {
      console.error('获取今日计费重置配置失败:', err)
    }
  }, [])

  const fetchSessionSticky = useCallback(async () => {
    try {
      const [config, runtime] = await Promise.all([
        getSessionStickyConfig(),
        getSessionStickyRuntimeStatus(),
      ])
      setSessionStickyConfig(config)
      setSessionStickyRuntime(runtime)
    } catch (err) {
      console.error('获取 Session 粘滞配置失败:', err)
    }
  }, [])

  const fetchBillingRuntimeStats = useCallback(async (silent = false) => {
    if (!silent) {
      setBillingRuntimeStatsLoading(true)
    }
    try {
      const data = await getBillingRuntimeStats()
      setBillingRuntimeStats(data)
    } catch (err) {
      console.error('获取 Redis 计费运行时统计失败:', err)
      if (!silent) {
        showMessage('error', err instanceof Error ? err.message : '获取运行时统计失败')
      }
    } finally {
      if (!silent) {
        setBillingRuntimeStatsLoading(false)
      }
    }
  }, [showMessage])

  useEffect(() => {
    fetchDatabaseInfo()
    fetchRetryConfig()
    fetchErrorRules()
    fetchRequestPayloadLimit()
    fetchRequestDetailConfig()
    fetchTimeoutConfig()
    fetchCacheTTLConfig()
    fetchBillingRuntimeConfig()
    fetchBillingRuntimeStats(true)
    fetchBalanceTopupSettings()
    fetchBillingDailyResetConfig()
    fetchSessionSticky()
  }, [
    fetchBalanceTopupSettings,
    fetchBillingDailyResetConfig,
    fetchBillingRuntimeConfig,
    fetchBillingRuntimeStats,
    fetchCacheTTLConfig,
    fetchDatabaseInfo,
    fetchErrorRules,
    fetchRequestPayloadLimit,
    fetchRequestDetailConfig,
    fetchRetryConfig,
    fetchSessionSticky,
    fetchTimeoutConfig,
  ])

  const handleCacheTTLChange = async (value: string) => {
    setCacheTTLLoading(true)
    try {
      await updateCacheTTLConfig(value)
      setCacheTTL(value)
      showMessage('success', '缓存 TTL 配置已更新')
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '保存失败')
    } finally {
      setCacheTTLLoading(false)
    }
  }

  const handleSaveSiteConfig = async () => {
    setSiteConfigSaving(true)
    try {
      const nextPayloadLimitMB = Math.max(1, Math.round(requestPayloadLimitMB))
      const [result] = await Promise.all([
        updateSiteConfig({
          siteName: siteNameInput,
          timeZone: siteTimeZoneInput,
          ampProxySettingsPolicy: ampProxySettingsPolicyInput,
          ampSettingsPolicy: ampSettingsPolicyInput,
          contact: {
            enabled: siteContactInput.enabled,
            title: siteContactInput.title,
            description: siteContactInput.description,
            link: siteContactInput.link,
          },
        }),
        updateRequestPayloadLimit({ maxBytes: nextPayloadLimitMB * 1024 * 1024 }),
      ])
      onSiteNameChange(result.config.siteName)
      onSiteTimeZoneChange(result.config.timeZone)
      onAmpProxySettingsPolicyChange(result.config.ampProxySettingsPolicy)
      onAmpSettingsPolicyChange(result.config.ampSettingsPolicy)
      onSiteContactChange(result.config.contact)
      setSiteNameInput(result.config.siteName)
      setSiteTimeZoneInput(result.config.timeZone)
      setAmpProxySettingsPolicyInput(result.config.ampProxySettingsPolicy)
      setAmpSettingsPolicyInput(result.config.ampSettingsPolicy)
      setSiteContactInput(result.config.contact)
      setRequestPayloadLimitMB(nextPayloadLimitMB)
      showMessage('success', '网站配置已保存')
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '保存失败')
    } finally {
      setSiteConfigSaving(false)
    }
  }

  const handleBillingRuntimeConfigChange = (key: keyof BillingRuntimeConfig, value: string | number | boolean) => {
    if (!billingRuntimeConfig) return
    setBillingRuntimeConfig({ ...billingRuntimeConfig, [key]: value } as BillingRuntimeConfig)
  }

  const handleSessionStickyConfigChange = (key: keyof SessionStickyConfig, value: number | boolean) => {
    if (!sessionStickyConfig) return
    setSessionStickyConfig({ ...sessionStickyConfig, [key]: value } as SessionStickyConfig)
  }

  const handleSaveSessionStickyConfig = async () => {
    if (!sessionStickyConfig) return

    setSessionStickyLoading(true)
    try {
      const result = await updateSessionStickyConfig(sessionStickyConfig)
      setSessionStickyConfig(result.config)
      await fetchSessionSticky()
      showMessage('success', 'Session 粘滞配置已保存')
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '保存失败')
    } finally {
      setSessionStickyLoading(false)
    }
  }

  const handleSaveBalanceTopupPrice = async () => {
    const price = Number.parseFloat(balanceTopupPriceInput)
    if (Number.isNaN(price) || price < 0) {
      showMessage('error', '单价格式错误')
      return
    }

    setBalanceTopupSaving(true)
    try {
      const current = await getPurchaseSettings()
      await updatePurchaseSettings({
        purchaseEnabled: current.purchaseEnabled,
        debugAutoPaid: current.debugAutoPaid,
        alipayAppId: current.alipayAppId,
        alipayPid: current.alipayPid,
        alipayEnvironment: current.alipayEnvironment,
        alipayNotifyUrl: current.alipayNotifyUrl,
        alipayPublicKey: current.alipayPublicKey,
        balanceTopupPriceCnyPerUsd: price,
      })
      setBalanceTopupPriceInput(String(price))
      showMessage('success', '充值单价已保存')
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '保存失败')
    } finally {
      setBalanceTopupSaving(false)
    }
  }

  const handleBillingDailyResetConfigChange = (
    key: keyof BillingDailyResetConfig,
    value: number
  ) => {
    if (!billingDailyResetConfig) return
    setBillingDailyResetConfig({ ...billingDailyResetConfig, [key]: value })
  }

  const handleSaveBillingDailyResetConfig = async () => {
    if (!billingDailyResetConfig) return

    setBillingDailyResetSaving(true)
    try {
      const result = await updateBillingDailyResetConfig({
        enabled: billingDailyResetConfig.enabled,
        minRemainingDays: Math.max(2, Math.round(billingDailyResetConfig.minRemainingDays || 2)),
        usageThresholdPercent: Math.min(100, Math.max(0, Math.round(billingDailyResetConfig.usageThresholdPercent || 0))),
        dailyLimit: Math.max(0, Math.round(billingDailyResetConfig.dailyLimit || 0)),
      })
      setBillingDailyResetConfig(result.config)
      showMessage('success', '今日计费重置配置已保存')
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '保存失败')
    } finally {
      setBillingDailyResetSaving(false)
    }
  }

  const handleSaveBillingRuntimeConfig = async () => {
    if (!billingRuntimeConfig) return

    setBillingRuntimeLoading(true)
    try {
      const result = await updateBillingRuntimeConfig({
        redisUrl: billingRuntimeConfig.redisUrl,
        redisPrefix: billingRuntimeConfig.redisPrefix,
        reservationTtlSec: billingRuntimeConfig.reservationTtlSec,
        reconcileIntervalSec: billingRuntimeConfig.reconcileIntervalSec,
        streamBatchSize: billingRuntimeConfig.streamBatchSize,
        reconcileBatchSize: billingRuntimeConfig.reconcileBatchSize,
        expiryBatchSize: billingRuntimeConfig.expiryBatchSize,
        projectorWorkers: billingRuntimeConfig.projectorWorkers,
        projectorClaimIdleSec: billingRuntimeConfig.projectorClaimIdleSec,
      })
      setBillingRuntimeConfig(result.config)
      await fetchBillingRuntimeStats(true)
      showMessage('success', 'Redis 计费配置已保存并已热更新')
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '保存失败')
    } finally {
      setBillingRuntimeLoading(false)
    }
  }

  const handleTimeoutConfigChange = (key: keyof TimeoutConfig, value: number) => {
    if (timeoutConfig) {
      setTimeoutConfig({ ...timeoutConfig, [key]: value })
    }
  }

  const handleSaveTimeoutConfig = async () => {
    if (!timeoutConfig) return
    
    setTimeoutLoading(true)
    try {
      await updateTimeoutConfig(timeoutConfig)
      showMessage('success', '超时配置已保存')
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '保存失败')
    } finally {
      setTimeoutLoading(false)
    }
  }

  const handleRequestDetailConfigChange = (
    key: keyof RequestDetailConfig,
    value: RequestDetailConfig[keyof RequestDetailConfig]
  ) => {
    if (!requestDetailConfig) return
    setRequestDetailConfig({ ...requestDetailConfig, [key]: value })
  }

  const handleSaveRequestDetailConfig = async () => {
    if (!requestDetailConfig) return

    setRequestDetailLoading(true)
    try {
      const result = await updateRequestDetailConfig(requestDetailConfig)
      setRequestDetailConfig(result.config)
      showMessage('success', '请求详情监控配置已保存')
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '保存失败')
    } finally {
      setRequestDetailLoading(false)
    }
  }

  const handleRetryConfigChange = (key: keyof RetryConfig, value: boolean | number) => {
    if (retryConfig) {
      setRetryConfig({ ...retryConfig, [key]: value })
    }
  }

  const handleErrorRuleChange = (index: number, key: keyof ErrorRule, value: string | number | boolean) => {
    setErrorRules((current) =>
      current.map((rule, currentIndex) => (currentIndex === index ? { ...rule, [key]: value } as ErrorRule : rule))
    )
  }

  const handleAddCustomRule = () => {
    setErrorRules((current) => [
      ...current,
      {
        id: `custom-${Date.now()}`,
        name: '自定义规则',
        builtIn: false,
        enabled: true,
        requestType: 'responses',
        upstreamStatus: '200',
        pattern: '',
        matchMode: 'substring',
        overrideStatus: 502,
        overrideMessage: '上游返回了错误内容',
      },
    ])
  }

  const handleRemoveCustomRule = (id: string) => {
    setErrorRules((current) => current.filter((rule) => rule.id !== id))
  }

  const handleSaveErrorRules = async () => {
    setErrorRulesLoading(true)
    try {
      const result = await updateErrorRules(errorRules)
      setErrorRules(result.rules)
      showMessage('success', '错误规则已保存')
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '保存失败')
    } finally {
      setErrorRulesLoading(false)
    }
  }

  const handleSaveRetryConfig = async () => {
    if (!retryConfig) return
    
    setRetryLoading(true)
    try {
      await updateRetryConfig(retryConfig)
      showMessage('success', '重试配置已保存')
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '保存失败')
    } finally {
      setRetryLoading(false)
    }
  }

  const billingRuntimeBadge = !billingRuntimeConfig?.runtimeEnabled
    ? { label: 'Legacy', variant: 'secondary' as const }
    : billingRuntimeConfig.runtimeHealthy
      ? { label: 'Healthy', variant: 'default' as const }
      : { label: 'Fallback', variant: 'destructive' as const }

  const billingRuntimeDescription = !billingRuntimeConfig?.runtimeEnabled
    ? '当前未配置 Redis，系统使用旧的 SQL 计费路径。'
    : billingRuntimeConfig.runtimeHealthy
      ? (billingRuntimeConfig.redisUrlMasked || 'Redis 计费运行时已启用。')
      : '已配置 Redis，但当前运行时不可用，系统已回退到 legacy billing。'

  const sessionStickyBadge = !sessionStickyConfig?.enabled
    ? { label: 'Disabled', variant: 'secondary' as const }
    : sessionStickyRuntime?.runtimeHealthy
      ? { label: 'Healthy', variant: 'default' as const }
      : { label: 'Fallback', variant: 'destructive' as const }

  const sessionStickyRuntimeDescription = !sessionStickyConfig?.enabled
    ? '当前未启用 Session 粘滞。'
    : sessionStickyRuntime?.redisConfigured
      ? (sessionStickyRuntime.redisUrlMasked || 'Redis 已接入 Session 粘滞运行时。')
      : 'Redis 未配置，运行时状态可能退化。'

  const formatLatency = (ms: number) => (ms > 0 ? `${ms} ms` : '-')
  const formatIdleMs = (ms: number) => (ms > 0 ? `${Math.round(ms / 1000)} s` : '-')

  const billingMetricItems = billingRuntimeStats
    ? [
        { key: 'reserve', label: 'Reserve', value: billingRuntimeStats.reserve },
        { key: 'settle', label: 'Settle', value: billingRuntimeStats.settle },
        { key: 'project', label: 'Project', value: billingRuntimeStats.project },
        { key: 'reclaim', label: 'Reclaim', value: billingRuntimeStats.reclaim },
        { key: 'reconcile', label: 'Reconcile', value: billingRuntimeStats.reconcile },
      ]
    : []

  const requestTypeLabelMap: Record<ErrorRule['requestType'], string> = {
    responses: 'Responses',
    chat_completions: 'Chat Completions',
    gemini: 'Gemini',
    messages: 'Anthropic Messages',
  }

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return

    const expectedExtension = databaseInfo?.currentType === 'postgres' ? '.sql' : '.db'
    if (!file.name.toLowerCase().endsWith(expectedExtension)) {
      showMessage('error', `请选择 ${expectedExtension} 数据库文件`)
      return
    }

    if (!confirm('上传新数据库将覆盖现有数据，确定继续吗？')) {
      if (fileInputRef.current) fileInputRef.current.value = ''
      return
    }

    setLoading(true)
    try {
      const result = await uploadDatabase(file)
      showMessage('success', result.message)
      fetchBackups()
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '上传失败')
    } finally {
      setLoading(false)
      if (fileInputRef.current) fileInputRef.current.value = ''
    }
  }

  const handleDownload = async () => {
    setLoading(true)
    try {
      await downloadDatabase()
      showMessage('success', databaseInfo?.currentType === 'postgres' ? 'PostgreSQL dump 下载成功' : '数据库下载成功')
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '下载失败')
    } finally {
      setLoading(false)
    }
  }

  const handleStartMigration = async () => {
    if (!databaseInfo) return

    const payload: StartDatabaseMigrationRequest = {
      clearTarget: migrationClearTarget,
      targetDatabaseUrl: migrationTargetDatabaseUrl,
      targetSqlitePath: migrationTargetSqlitePath,
      targetType: migrationTargetType,
      withArchive: migrationWithArchive,
    }

    if (!confirm(`确定要将当前 ${databaseInfo.currentType === 'sqlite' ? 'SQLite' : 'PostgreSQL'} 数据迁移并切换到 ${migrationTargetType === 'sqlite' ? 'SQLite' : 'PostgreSQL'} 吗？`)) {
      return
    }

    setMigrationStarting(true)
    try {
      const task = await startDatabaseMigration(payload)
      setMigrationTask(task)
      showMessage('success', '数据库迁移任务已启动')
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '启动迁移失败')
    } finally {
      setMigrationStarting(false)
    }
  }

  useEffect(() => {
    if (!migrationTask || migrationTask.status === 'succeeded' || migrationTask.status === 'failed') {
      return
    }

    const timer = window.setInterval(async () => {
      try {
        const latestTask = await getDatabaseMigrationTask(migrationTask.id)
        const previousStatus = migrationTask.status
        setMigrationTask(latestTask)

        if (latestTask.status !== previousStatus && latestTask.status === 'succeeded') {
          showMessage('success', '数据库迁移并切换完成；如果重启服务，请同步环境变量或启动脚本配置')
          fetchDatabaseInfo()
        }
        if (latestTask.status !== previousStatus && latestTask.status === 'failed') {
          showMessage('error', latestTask.error || '数据库迁移失败')
        }
      } catch (err) {
        console.error('获取数据库迁移进度失败:', err)
      }
    }, 1500)

    return () => window.clearInterval(timer)
  }, [fetchDatabaseInfo, migrationTask, showMessage])

  const handleRestore = async (filename: string) => {
    if (!confirm(`确定要恢复备份 ${filename} 吗？当前数据将被备份。`)) {
      return
    }

    setLoading(true)
    try {
      const result = await restoreBackup(filename)
      showMessage('success', result.message)
      fetchBackups()
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '恢复失败')
    } finally {
      setLoading(false)
    }
  }

  const handleDelete = async (filename: string) => {
    if (!confirm(`确定要删除备份 ${filename} 吗？`)) {
      return
    }

    setLoading(true)
    try {
      await deleteBackup(filename)
      showMessage('success', '备份已删除')
      fetchBackups()
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '删除失败')
    } finally {
      setLoading(false)
    }
  }

  const formatSize = (bytes: number) => {
    if (bytes < 1024) return bytes + ' B'
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
    return (bytes / 1024 / 1024).toFixed(1) + ' MB'
  }

  const formatDate = (dateStr: string) => {
    return formatDateTime(dateStr)
  }

  const siteTimeZoneOptions = SITE_TIME_ZONE_OPTIONS.some((option) => option.value === siteTimeZoneInput)
    ? SITE_TIME_ZONE_OPTIONS
    : [{ value: siteTimeZoneInput, label: siteTimeZoneInput }, ...SITE_TIME_ZONE_OPTIONS]

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="space-y-6">
      <motion.div
        initial={{ opacity: 0, y: -20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ type: 'spring', bounce: 0.2, duration: 0.6 }}
      >
        <h2 className="text-2xl font-bold tracking-tight">系统设置</h2>
        <p className="text-muted-foreground">管理系统级配置和数据库</p>
      </motion.div>

      <motion.div
        initial={{ opacity: 0, y: 10 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ type: 'spring', bounce: 0.2, duration: 0.5, delay: 0.05 }}
        className="flex items-center gap-1 border-b pb-0"
      >
        {tabs.map((tab) => (
          <button
            key={tab.key}
            onClick={() => setActiveTab(tab.key)}
            className={`relative px-4 py-2 text-sm font-medium transition-colors rounded-t-md ${
              activeTab === tab.key
                ? 'text-foreground'
                : 'text-muted-foreground hover:text-foreground/80'
            }`}
          >
            {tab.label}
            {activeTab === tab.key && (
              <motion.div
                layoutId="settings-tab-indicator"
                className="absolute inset-x-0 -bottom-px h-0.5 bg-primary"
                transition={{ type: 'spring', bounce: 0.2, duration: 0.4 }}
              />
            )}
          </button>
        ))}
      </motion.div>

      <AnimatePresence>
        {message && (
          <motion.div initial={{ opacity: 0, y: -20, scale: 0.95 }} animate={{ opacity: 1, y: 0, scale: 1 }} exit={{ opacity: 0, y: -20, scale: 0.95 }} transition={{ type: 'spring', bounce: 0.3, duration: 0.5 }}>
            <Alert variant={message.type === 'error' ? 'destructive' : 'default'}>
              <AlertDescription>{message.text}</AlertDescription>
            </Alert>
          </motion.div>
        )}
      </AnimatePresence>

      <AnimatePresence mode="wait">
        <motion.div
          key={activeTab}
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -10 }}
          transition={{ type: 'spring', bounce: 0.2, duration: 0.4 }}
          className="space-y-6"
        >
          {activeTab === 'site' && (
            <>
              <Card>
                <CardHeader>
                  <CardTitle>网站配置</CardTitle>
                  <CardDescription>同步影响登录、注册、左上角、浏览器标题和全站时间显示</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="max-w-xl space-y-2">
                    <Label htmlFor="siteName">网站名称</Label>
                    <Input
                      id="siteName"
                      value={siteNameInput}
                      onChange={(e) => setSiteNameInput(e.target.value)}
                      placeholder="AMP Manager"
                      maxLength={64}
                    />
                  </div>
                  <div className="max-w-xl space-y-2">
                    <Label htmlFor="siteTimeZone">网站时区</Label>
                    <Select value={siteTimeZoneInput} onValueChange={setSiteTimeZoneInput}>
                      <SelectTrigger id="siteTimeZone">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {siteTimeZoneOptions.map((option) => (
                          <SelectItem key={option.value} value={option.value}>
                            {option.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="max-w-xl space-y-2">
                    <Label htmlFor="ampProxySettingsPolicy">路由设置权限</Label>
                    <Select value={ampProxySettingsPolicyInput} onValueChange={(value) => setAmpProxySettingsPolicyInput(value as AmpProxySettingsPolicy)}>
                      <SelectTrigger id="ampProxySettingsPolicy">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="disabled">禁用</SelectItem>
                        <SelectItem value="admin_only">仅管理员</SelectItem>
                        <SelectItem value="all">启用</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="max-w-xl space-y-2">
                    <Label htmlFor="ampSettingsPolicy">Amp设置权限</Label>
                    <Select value={ampSettingsPolicyInput} onValueChange={(value) => setAmpSettingsPolicyInput(value as AmpProxySettingsPolicy)}>
                      <SelectTrigger id="ampSettingsPolicy">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="disabled">禁用</SelectItem>
                        <SelectItem value="admin_only">仅管理员</SelectItem>
                        <SelectItem value="all">启用</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="max-w-xl rounded-lg border px-4 py-4 space-y-4">
                    <div className="flex items-center justify-between gap-4">
                      <div className="space-y-1">
                        <Label htmlFor="siteContactEnabled">联系方式按钮</Label>
                        <p className="text-xs text-muted-foreground">开启后，会在公告按钮旁显示联系方式入口。</p>
                      </div>
                      <Switch
                        id="siteContactEnabled"
                        checked={siteContactInput.enabled}
                        onCheckedChange={(checked) => setSiteContactInput((current) => ({ ...current, enabled: checked }))}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="siteContactTitle">标题</Label>
                      <Input
                        id="siteContactTitle"
                        value={siteContactInput.title}
                        onChange={(e) => setSiteContactInput((current) => ({ ...current, title: e.target.value }))}
                        placeholder="例如 Telegram / 企业微信 / Discord"
                        maxLength={64}
                      />
                      <p className="text-xs text-muted-foreground">用于超链接点击展示。</p>
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="siteContactDescription">说明</Label>
                      <Textarea
                        id="siteContactDescription"
                        value={siteContactInput.description}
                        onChange={(e) => setSiteContactInput((current) => ({ ...current, description: e.target.value }))}
                        placeholder="说明加入方式、服务时间或联系用途。"
                        rows={4}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="siteContactLink">链接</Label>
                      <Input
                        id="siteContactLink"
                        value={siteContactInput.link}
                        onChange={(e) => setSiteContactInput((current) => ({ ...current, link: e.target.value }))}
                        placeholder="https://example.com/contact"
                        maxLength={2048}
                      />
                      <p className="text-xs text-muted-foreground">标题跳转和二维码都基于这个链接生成。</p>
                    </div>
                    <div className="space-y-2">
                      <Label>二维码预览</Label>
                      <div className="flex h-40 w-40 items-center justify-center rounded-xl border bg-muted/20">
                        {siteContactInput.qrCodeImageDataUrl ? (
                          <img src={siteContactInput.qrCodeImageDataUrl} alt="联系方式二维码预览" className="h-32 w-32 object-contain" />
                        ) : (
                          <div className="px-4 text-center text-xs text-muted-foreground">
                            {siteContactInput.link.trim() ? '保存后生成二维码' : '填写链接后可生成二维码'}
                          </div>
                        )}
                      </div>
                    </div>
                  </div>
                  <div className="max-w-xs space-y-2">
                    <Label htmlFor="requestPayloadLimit">请求体上限 (MiB)</Label>
                    <Input
                      id="requestPayloadLimit"
                      type="number"
                      min={1}
                      value={requestPayloadLimitMB}
                      onChange={(e) => setRequestPayloadLimitMB(parseInt(e.target.value) || 1)}
                    />
                  </div>
                  <div className="flex justify-end">
                    <Button onClick={handleSaveSiteConfig} disabled={siteConfigSaving}>
                      {siteConfigSaving ? '保存中...' : '保存设置'}
                    </Button>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle>Session 粘滞</CardTitle>
                  <CardDescription>控制会话窗口、日志检索阈值与运行时状态</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  {sessionStickyConfig ? (
                    <>
                      <div className="grid gap-4 md:grid-cols-2">
                        <div className="rounded-lg border px-4 py-3 space-y-2">
                          <div className="flex items-center justify-between gap-3">
                            <span className="text-sm text-muted-foreground">运行时状态</span>
                            <Badge variant={sessionStickyBadge.variant}>{sessionStickyBadge.label}</Badge>
                          </div>
                          <p className="text-sm text-muted-foreground">{sessionStickyRuntimeDescription}</p>
                        </div>
                        <div className="rounded-lg border px-4 py-3 space-y-2">
                          <div className="flex items-center justify-between gap-3">
                            <span className="text-sm text-muted-foreground">Redis 前缀</span>
                            <Badge variant="outline">{sessionStickyRuntime?.redisPrefix || '-'}</Badge>
                          </div>
                          <p className="text-sm text-muted-foreground">
                            活跃 Session {sessionStickyRuntime?.activeSessionCount ?? '-'}
                          </p>
                        </div>
                      </div>

                      <div className="flex items-center justify-between rounded-lg border px-4 py-3">
                        <div className="space-y-1">
                          <Label htmlFor="sessionStickyEnabled">启用 Session 粘滞</Label>
                          <p className="text-xs text-muted-foreground">只暴露只读排障相关配置</p>
                        </div>
                        <Switch
                          id="sessionStickyEnabled"
                          checked={sessionStickyConfig.enabled}
                          onCheckedChange={(checked) => handleSessionStickyConfigChange('enabled', checked)}
                        />
                      </div>

                      <div className="grid gap-4 md:grid-cols-2">
                        <div className="space-y-2">
                          <Label htmlFor="sessionStickyWindowMinutes">windowMinutes</Label>
                          <Input
                            id="sessionStickyWindowMinutes"
                            type="number"
                            min={1}
                            value={sessionStickyConfig.windowMinutes}
                            onChange={(e) => handleSessionStickyConfigChange('windowMinutes', parseInt(e.target.value, 10) || 1)}
                          />
                        </div>
                        <div className="space-y-2">
                          <Label htmlFor="sessionStickyLogSearchMinChars">logSearchMinChars</Label>
                          <Input
                            id="sessionStickyLogSearchMinChars"
                            type="number"
                            min={1}
                            value={sessionStickyConfig.logSearchMinChars}
                            onChange={(e) => handleSessionStickyConfigChange('logSearchMinChars', parseInt(e.target.value, 10) || 1)}
                          />
                        </div>
                      </div>

                      <div className="flex justify-end gap-2">
                        <Button variant="outline" onClick={() => fetchSessionSticky()} disabled={sessionStickyLoading}>
                          刷新状态
                        </Button>
                        <Button onClick={handleSaveSessionStickyConfig} disabled={sessionStickyLoading}>
                          {sessionStickyLoading ? '保存中...' : '保存配置'}
                        </Button>
                      </div>
                    </>
                  ) : (
                    <div className="text-center text-muted-foreground py-4">加载中...</div>
                  )}
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle>余额充值</CardTitle>
                  <CardDescription>设置充值单价</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="max-w-xs space-y-2">
                    <Label htmlFor="balanceTopupPrice">元/刀</Label>
                    <Input
                      id="balanceTopupPrice"
                      type="number"
                      min="0"
                      step="0.01"
                      value={balanceTopupPriceInput}
                      onChange={(e) => setBalanceTopupPriceInput(e.target.value)}
                    />
                  </div>
                  <div className="flex justify-end">
                    <Button onClick={handleSaveBalanceTopupPrice} disabled={balanceTopupSaving}>
                      {balanceTopupSaving ? '保存中...' : '保存单价'}
                    </Button>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle>计费重置</CardTitle>
                  <CardDescription>控制用户侧“重置今日计费”按钮的准入规则</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  {billingDailyResetConfig ? (
                    <>
                      <div className="flex items-center justify-between rounded-lg border px-4 py-3">
                        <div className="space-y-1">
                          <Label htmlFor="billingDailyResetEnabled">启用计费重置</Label>
                          <p className="text-xs text-muted-foreground">关闭后概览页不显示“重置今日计费”按钮，接口也会拒绝重置。</p>
                        </div>
                        <Switch
                          id="billingDailyResetEnabled"
                          checked={billingDailyResetConfig.enabled}
                          onCheckedChange={(checked) => setBillingDailyResetConfig({ ...billingDailyResetConfig, enabled: checked })}
                        />
                      </div>
                      <div className="grid gap-4 md:grid-cols-3">
                        <div className="space-y-2">
                          <Label htmlFor="billingDailyResetMinRemainingDays">重置前最少时长 (天)</Label>
                          <Input
                            id="billingDailyResetMinRemainingDays"
                            type="number"
                            min={2}
                            step={1}
                            value={billingDailyResetConfig.minRemainingDays}
                            onChange={(e) => handleBillingDailyResetConfigChange('minRemainingDays', parseInt(e.target.value) || 2)}
                          />
                          <p className="text-xs text-muted-foreground">剩余时长需严格大于该值，最低 2 天。</p>
                        </div>
                        <div className="space-y-2">
                          <Label htmlFor="billingDailyResetUsageThresholdPercent">重置消耗阈值 (%)</Label>
                          <Input
                            id="billingDailyResetUsageThresholdPercent"
                            type="number"
                            min={0}
                            max={100}
                            step={1}
                            value={billingDailyResetConfig.usageThresholdPercent}
                            onChange={(e) => handleBillingDailyResetConfigChange('usageThresholdPercent', parseInt(e.target.value) || 0)}
                          />
                          <p className="text-xs text-muted-foreground">今日用量需高于该百分比才允许重置。</p>
                        </div>
                        <div className="space-y-2">
                          <Label htmlFor="billingDailyResetDailyLimit">每日重置次数限制</Label>
                          <Input
                            id="billingDailyResetDailyLimit"
                            type="number"
                            min={0}
                            step={1}
                            value={billingDailyResetConfig.dailyLimit}
                            onChange={(e) => handleBillingDailyResetConfigChange('dailyLimit', parseInt(e.target.value) || 0)}
                          />
                          <p className="text-xs text-muted-foreground">填 0 表示启用状态下也不允许任何重置。</p>
                        </div>
                      </div>
                      <div className="flex justify-end">
                        <Button onClick={handleSaveBillingDailyResetConfig} disabled={billingDailyResetSaving}>
                          {billingDailyResetSaving ? '保存中...' : '保存规则'}
                        </Button>
                      </div>
                    </>
                  ) : (
                    <div className="text-center text-muted-foreground py-4">加载中...</div>
                  )}
                </CardContent>
              </Card>
            </>
          )}

          {activeTab === 'announcements' && (
            <AnnouncementAdminSection onMessage={showMessage} />
          )}

          {activeTab === 'status-monitor' && (
            <StatusMonitorSettingsPanel onMessage={showMessage} />
          )}

          {activeTab === 'security' && (
            <SecuritySettingsPanel onMessage={showMessage} />
          )}

          {activeTab === 'database' && (
            <>
              <Card>
                <CardHeader>
                  <CardTitle>数据库模式</CardTitle>
                  <CardDescription>数据库模式与当前连接信息。</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  {databaseInfoLoading ? (
                    <div className="text-center text-muted-foreground py-4">加载中...</div>
                  ) : databaseInfo ? (
                    <>
                      <div className="grid gap-4 md:grid-cols-2">
                        <div className="rounded-lg border p-4 space-y-2">
                          <div className="flex items-center justify-between">
                            <span className="text-sm text-muted-foreground">当前模式</span>
                            <Badge variant={databaseInfo.currentType === 'postgres' ? 'default' : 'secondary'}>
                              {databaseInfo.currentType === 'postgres' ? 'PostgreSQL' : 'SQLite'}
                            </Badge>
                          </div>
                          <p className="text-sm text-muted-foreground">
                            {databaseInfo.currentType === 'postgres'
                              ? databaseInfo.databaseURLMasked || '未暴露连接串'
                              : databaseInfo.sqlitePath}
                          </p>
                        </div>
                        <div className="rounded-lg border p-4 space-y-2">
                          <div className="flex items-center justify-between">
                            <span className="text-sm text-muted-foreground">请求详情归档</span>
                            <Badge variant="outline">{databaseInfo.archiveMode}</Badge>
                          </div>
                          <p className="text-sm text-muted-foreground">
                            {databaseInfo.supportsFileBackups
                              ? '支持文件级下载、上传、备份和恢复。'
                              : '当前模式使用 PostgreSQL dump 导入导出。'}
                          </p>
                        </div>
                      </div>

                    </>
                  ) : (
                    <div className="text-center text-muted-foreground py-4">数据库信息加载失败</div>
                  )}
                </CardContent>
              </Card>

              {databaseInfo && (
                <Card>
                  <CardHeader>
                    <CardTitle>迁移并切换数据库</CardTitle>
                    <CardDescription>SQLite ↔ PostgreSQL 迁移任务。</CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    <div className="grid gap-4 md:grid-cols-2">
                      <div className="space-y-2">
                        <Label>目标数据库类型</Label>
                        <Select value={migrationTargetType} onValueChange={(value: 'sqlite' | 'postgres') => setMigrationTargetType(value)}>
                          <SelectTrigger>
                            <SelectValue placeholder="选择目标数据库" />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="sqlite">SQLite</SelectItem>
                            <SelectItem value="postgres">PostgreSQL</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                      <div className="space-y-2">
                        <Label>{migrationTargetType === 'postgres' ? '目标 PostgreSQL 连接串' : '目标 SQLite 文件路径'}</Label>
                        {migrationTargetType === 'postgres' ? (
                          <Input
                            value={migrationTargetDatabaseUrl}
                            onChange={(e) => setMigrationTargetDatabaseUrl(e.target.value)}
                            placeholder="postgres://postgres:mysecretpassword@localhost:5432/ampmanager?sslmode=disable"
                          />
                        ) : (
                          <Input
                            value={migrationTargetSqlitePath}
                            onChange={(e) => setMigrationTargetSqlitePath(e.target.value)}
                            placeholder="./data/data.db"
                          />
                        )}
                      </div>
                    </div>

                    <div className="space-y-4 rounded-lg border p-4">
                      <div className="flex items-center justify-between">
                        <div className="space-y-0.5">
                          <Label>迁移请求详情归档</Label>
                          <p className="text-sm text-muted-foreground">同时复制请求详情归档表或归档库中的数据</p>
                        </div>
                        <Switch checked={migrationWithArchive} onCheckedChange={setMigrationWithArchive} />
                      </div>
                      <div className="flex items-center justify-between">
                        <div className="space-y-0.5">
                          <Label>清空目标数据库</Label>
                          <p className="text-sm text-muted-foreground">迁移前先清空目标数据库的业务表，避免重复数据</p>
                        </div>
                        <Switch checked={migrationClearTarget} onCheckedChange={setMigrationClearTarget} />
                      </div>
                    </div>

                    <div className="flex items-center justify-between gap-4">
                      <p className="text-sm text-muted-foreground">
                        当前会从 {databaseInfo.currentType === 'sqlite' ? 'SQLite' : 'PostgreSQL'} 迁移到 {migrationTargetType === 'sqlite' ? 'SQLite' : 'PostgreSQL'}。
                      </p>
                      <Button onClick={handleStartMigration} disabled={migrationStarting || migrationTask?.status === 'running'}>
                        {migrationStarting ? '启动中...' : '开始迁移并切换'}
                      </Button>
                    </div>
                  </CardContent>
                </Card>
              )}

              {migrationTask && (
                <Card>
                  <CardHeader>
                    <CardTitle>迁移任务进度</CardTitle>
                    <CardDescription>
                      {migrationTask.sourceType.toUpperCase()} → {migrationTask.targetType.toUpperCase()} · 状态：{migrationTask.status}
                    </CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    <div className="space-y-2">
                      <div className="flex items-center justify-between text-sm">
                        <span>{migrationTask.message}</span>
                        <span className="text-muted-foreground">{migrationTask.progress}%</span>
                      </div>
                      <Progress value={migrationTask.progress} />
                    </div>

                    {migrationTask.error && (
                      <Alert variant="destructive">
                        <AlertDescription>{migrationTask.error}</AlertDescription>
                      </Alert>
                    )}

                    <div className="space-y-2">
                      <Label>任务日志</Label>
                      <Textarea
                        readOnly
                        value={migrationTask.logs.join('\n')}
                        className="min-h-[180px] font-mono text-xs"
                      />
                    </div>
                  </CardContent>
                </Card>
              )}

              <Card>
                <CardHeader>
                  <CardTitle>{databaseInfo?.currentType === 'postgres' ? 'PostgreSQL dump 导入导出' : '数据库导入导出'}</CardTitle>
                  <CardDescription>
                    {databaseInfo?.currentType === 'postgres'
                      ? '导入导出当前数据库。'
                      : '上传、下载和管理 SQLite 数据库文件'}
                  </CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="flex flex-wrap gap-4">
                    <div>
                      <input
                        ref={fileInputRef}
                        type="file"
                        accept={databaseInfo?.currentType === 'postgres' ? '.sql' : '.db'}
                        onChange={handleUpload}
                        className="hidden"
                        id="db-upload"
                        disabled={loading}
                      />
                      <Button asChild disabled={loading}>
                        <label htmlFor="db-upload" className="cursor-pointer">
                          {databaseInfo?.currentType === 'postgres' ? '导入 dump' : '上传数据库'}
                        </label>
                      </Button>
                    </div>
                    <Button variant="secondary" onClick={handleDownload} disabled={loading}>
                      {databaseInfo?.currentType === 'postgres' ? '导出当前 dump' : '下载当前数据库'}
                    </Button>
                  </div>
                  <p className="text-sm text-muted-foreground">
                    {databaseInfo?.currentType === 'postgres'
                      ? '支持导入和导出当前数据库。'
                      : '上传新数据库将自动备份当前数据库。更改生效需要重启服务。'}
                  </p>
                </CardContent>
              </Card>

              {databaseInfo?.supportsFileBackups && (
                <Card>
                  <CardHeader>
                    <CardTitle>备份列表</CardTitle>
                    <CardDescription>查看和管理 SQLite 数据库备份</CardDescription>
                  </CardHeader>
                  <CardContent>
                    {backups.length === 0 ? (
                      <div className="rounded-md border border-dashed p-8 text-center text-muted-foreground">
                        暂无备份
                      </div>
                    ) : (
                      <div className="overflow-x-auto">
                        <Table>
                          <TableHeader>
                            <TableRow>
                              <TableHead>文件名</TableHead>
                              <TableHead>大小</TableHead>
                              <TableHead>备份时间</TableHead>
                              <TableHead>操作</TableHead>
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {backups.map((backup) => (
                              <TableRow key={backup.filename}>
                                <TableCell className="font-mono text-xs">{backup.filename}</TableCell>
                                <TableCell>
                                  <Badge variant="outline">{formatSize(backup.size)}</Badge>
                                </TableCell>
                                <TableCell className="text-muted-foreground">
                                  {formatDate(backup.modTime)}
                                </TableCell>
                                <TableCell>
                                  <div className="flex gap-2">
                                    <Button
                                      variant="outline"
                                      size="sm"
                                      onClick={() => handleRestore(backup.filename)}
                                      disabled={loading}
                                    >
                                      恢复
                                    </Button>
                                    <Button
                                      variant="destructive"
                                      size="sm"
                                      onClick={() => handleDelete(backup.filename)}
                                      disabled={loading}
                                    >
                                      删除
                                    </Button>
                                  </div>
                                </TableCell>
                              </TableRow>
                            ))}
                          </TableBody>
                        </Table>
                      </div>
                    )}
                  </CardContent>
                </Card>
              )}
            </>
          )}

          {activeTab === 'retry' && (
            <Card>
              <CardHeader>
                <CardTitle>重试配置</CardTitle>
                <CardDescription>配置请求失败时的自动重试策略（首包门控）</CardDescription>
              </CardHeader>
              <CardContent className="space-y-6">
                {retryConfig ? (
                  <>
                    <div className="flex items-center justify-between">
                      <div className="space-y-0.5">
                        <Label>启用重试</Label>
                        <p className="text-sm text-muted-foreground">在首包到达前自动重试失败的请求</p>
                      </div>
                      <Switch
                        checked={retryConfig.enabled}
                        onCheckedChange={(checked) => handleRetryConfigChange('enabled', checked)}
                      />
                    </div>

                    <div className="grid gap-4 md:grid-cols-2">
                      <div className="space-y-2">
                        <Label>最大重试次数</Label>
                        <Input
                          type="number"
                          min={1}
                          value={retryConfig.maxAttempts}
                          onChange={(e) => handleRetryConfigChange('maxAttempts', parseInt(e.target.value) || 1)}
                        />
                      </div>
                      <div className="space-y-2">
                        <Label>首包超时 (毫秒)</Label>
                        <Input
                          type="number"
                          min={1000}
                          value={retryConfig.gateTimeoutMs}
                          onChange={(e) => handleRetryConfigChange('gateTimeoutMs', parseInt(e.target.value) || 10000)}
                        />
                      </div>
                      <div className="space-y-2">
                        <Label>退避基数 (毫秒)</Label>
                        <Input
                          type="number"
                          min={50}
                          value={retryConfig.backoffBaseMs}
                          onChange={(e) => handleRetryConfigChange('backoffBaseMs', parseInt(e.target.value) || 100)}
                        />
                      </div>
                      <div className="space-y-2">
                        <Label>退避上限 (毫秒)</Label>
                        <Input
                          type="number"
                          min={500}
                          value={retryConfig.backoffMaxMs}
                          onChange={(e) => handleRetryConfigChange('backoffMaxMs', parseInt(e.target.value) || 2000)}
                        />
                      </div>
                    </div>

                    <div className="space-y-4">
                      <div className="flex items-center justify-between">
                        <div className="space-y-0.5">
                          <Label>429 时重试</Label>
                          <p className="text-sm text-muted-foreground">请求被限流时自动重试</p>
                        </div>
                        <Switch
                          checked={retryConfig.retryOn429}
                          onCheckedChange={(checked) => handleRetryConfigChange('retryOn429', checked)}
                        />
                      </div>
                      <div className="flex items-center justify-between">
                        <div className="space-y-0.5">
                          <Label>5xx 时重试</Label>
                          <p className="text-sm text-muted-foreground">服务端错误时自动重试</p>
                        </div>
                        <Switch
                          checked={retryConfig.retryOn5xx}
                          onCheckedChange={(checked) => handleRetryConfigChange('retryOn5xx', checked)}
                        />
                      </div>
                      <div className="flex items-center justify-between">
                        <div className="space-y-0.5">
                          <Label>尊重 Retry-After</Label>
                          <p className="text-sm text-muted-foreground">按服务器返回的等待时间退避</p>
                        </div>
                        <Switch
                          checked={retryConfig.respectRetryAfter}
                          onCheckedChange={(checked) => handleRetryConfigChange('respectRetryAfter', checked)}
                        />
                      </div>
                    </div>

                    <Button onClick={handleSaveRetryConfig} disabled={retryLoading}>
                      {retryLoading ? '保存中...' : '保存配置'}
                    </Button>
                  </>
                ) : requestDetailLoading ? (
                  <div className="text-center text-muted-foreground py-4">加载中...</div>
                ) : (
                  <div className="space-y-4">
                    <Alert variant="destructive">
                      <AlertDescription>
                        {requestDetailLoadError || '请求详情监控配置加载失败，请重试。'}
                      </AlertDescription>
                    </Alert>
                    <Button variant="outline" onClick={fetchRequestDetailConfig} disabled={requestDetailLoading}>
                      重新加载
                    </Button>
                  </div>
                )}
              </CardContent>
            </Card>
          )}

          {activeTab === 'error-rules' && (
            <Card>
              <CardHeader>
                <CardTitle>错误规则</CardTitle>
                <CardDescription>匹配上游错误文本与状态码，并改写为面向客户端的最终错误状态与消息。</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <Alert>
                  <AlertDescription>
                    默认内置规则会处理 fake-200 和 OpenAI 风格 “An Error Occurred” 场景。内置规则可编辑但不会被删除。
                  </AlertDescription>
                </Alert>

                <div className="space-y-4">
                  {errorRules.map((rule, index) => (
                    <div key={rule.id} className="rounded-lg border p-4 space-y-4">
                      <div className="flex flex-wrap items-center justify-between gap-3">
                        <div className="space-y-1">
                          <div className="flex items-center gap-2">
                            <Input
                              value={rule.name}
                              onChange={(e) => handleErrorRuleChange(index, 'name', e.target.value)}
                              className="h-8 w-64"
                            />
                            <Badge variant={rule.builtIn ? 'secondary' : 'outline'}>
                              {rule.builtIn ? '内置' : '自定义'}
                            </Badge>
                          </div>
                          <p className="text-xs text-muted-foreground">规则 ID: {rule.id}</p>
                        </div>
                        <div className="flex items-center gap-3">
                          <div className="flex items-center gap-2">
                            <Label>启用</Label>
                            <Switch
                              checked={rule.enabled}
                              onCheckedChange={(checked) => handleErrorRuleChange(index, 'enabled', checked)}
                            />
                          </div>
                          {!rule.builtIn && (
                            <Button type="button" variant="outline" size="sm" onClick={() => handleRemoveCustomRule(rule.id)}>
                              删除
                            </Button>
                          )}
                        </div>
                      </div>

                      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                        <div className="space-y-2">
                          <Label>请求类型</Label>
                          <Select
                            value={rule.requestType}
                            onValueChange={(value) => handleErrorRuleChange(index, 'requestType', value)}
                          >
                            <SelectTrigger>
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              {Object.entries(requestTypeLabelMap).map(([value, label]) => (
                                <SelectItem key={value} value={value}>
                                  {label}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </div>

                        <div className="space-y-2">
                          <Label>上游状态码</Label>
                          <Input
                            value={rule.upstreamStatus}
                            onChange={(e) => handleErrorRuleChange(index, 'upstreamStatus', e.target.value)}
                            placeholder="例如 200 或 500-599"
                          />
                        </div>

                        <div className="space-y-2">
                          <Label>匹配模式</Label>
                          <Select
                            value={rule.matchMode}
                            onValueChange={(value) => handleErrorRuleChange(index, 'matchMode', value)}
                          >
                            <SelectTrigger>
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="substring">包含文本</SelectItem>
                              <SelectItem value="regex">正则表达式</SelectItem>
                            </SelectContent>
                          </Select>
                        </div>

                        <div className="space-y-2 xl:col-span-2">
                          <Label>匹配内容</Label>
                          <Input
                            value={rule.pattern}
                            onChange={(e) => handleErrorRuleChange(index, 'pattern', e.target.value)}
                            placeholder={rule.matchMode === 'regex' ? '输入正则表达式' : '输入需要匹配的文本'}
                          />
                        </div>

                        <div className="space-y-2">
                          <Label>覆盖状态码</Label>
                          <Input
                            type="number"
                            min={100}
                            max={599}
                            value={rule.overrideStatus}
                            onChange={(e) => handleErrorRuleChange(index, 'overrideStatus', Number.parseInt(e.target.value || '0', 10))}
                          />
                        </div>

                        <div className="space-y-2 md:col-span-2 xl:col-span-3">
                          <Label>返回消息</Label>
                          <Input
                            value={rule.overrideMessage}
                            onChange={(e) => handleErrorRuleChange(index, 'overrideMessage', e.target.value)}
                            placeholder="客户端最终看到的错误消息"
                          />
                        </div>
                      </div>
                    </div>
                  ))}
                </div>

                <div className="flex flex-wrap justify-between gap-3">
                  <Button type="button" variant="outline" onClick={handleAddCustomRule}>
                    新增自定义规则
                  </Button>
                  <Button type="button" onClick={handleSaveErrorRules} disabled={errorRulesLoading}>
                    {errorRulesLoading ? '保存中...' : '保存规则'}
                  </Button>
                </div>
              </CardContent>
            </Card>
          )}

          {activeTab === 'monitoring' && (
            <Card>
              <CardHeader>
                <CardTitle>请求详情监控</CardTitle>
                <CardDescription>控制请求详情记录策略，包括高 RPM 降级模式、阈值和采样率</CardDescription>
              </CardHeader>
              <CardContent className="space-y-6">
                {requestDetailConfig ? (
                  <>
                    <div className="grid gap-4 md:grid-cols-2">
                      <div className="rounded-lg border p-4 space-y-2">
                        <div className="flex items-center justify-between">
                          <span className="text-sm text-muted-foreground">当前状态</span>
                          <Badge variant={requestDetailConfig.enabled ? 'default' : 'secondary'}>
                            {requestDetailConfig.enabled ? '已启用' : '已关闭'}
                          </Badge>
                        </div>
                        <p className="text-sm text-muted-foreground">
                          启用后可在日志页面点击状态列查看请求/响应详情；关闭后将停止新的详情采集。
                        </p>
                      </div>
                      <div className="rounded-lg border p-4 space-y-2">
                        <div className="flex items-center justify-between">
                          <span className="text-sm text-muted-foreground">高 RPM 策略</span>
                          <Badge variant="outline">
                            {requestDetailConfig.highRpmMode === 'full'
                              ? '全量保留'
                              : requestDetailConfig.highRpmMode === 'sample'
                                ? `采样 ${requestDetailConfig.highRpmSamplePercent}%`
                                : '停止采集'}
                          </Badge>
                        </div>
                        <p className="text-sm text-muted-foreground">
                          当分钟请求量达到 {requestDetailConfig.highRpmThreshold} RPM 时，自动切换到该降级策略。
                        </p>
                      </div>
                    </div>

                    <div className="space-y-4">
                      <div className="flex items-center justify-between">
                        <div className="space-y-0.5">
                          <Label>启用详情监控</Label>
                          <p className="text-sm text-muted-foreground">
                            控制是否记录请求与响应的头部和正文详情。
                          </p>
                        </div>
                        <Switch
                          checked={requestDetailConfig.enabled}
                          onCheckedChange={(checked) => handleRequestDetailConfigChange('enabled', checked)}
                          disabled={requestDetailLoading}
                        />
                      </div>
                      <div className="flex items-center justify-between">
                        <div className="space-y-0.5">
                          <Label>启用持久化</Label>
                          <p className="text-sm text-muted-foreground">
                            开启后，详情会落库归档，便于跨重启排查问题。
                          </p>
                        </div>
                        <Switch
                          checked={requestDetailConfig.persistEnabled}
                          onCheckedChange={(checked) => handleRequestDetailConfigChange('persistEnabled', checked)}
                          disabled={requestDetailLoading}
                        />
                      </div>
                    </div>

                    <div className="grid gap-4 md:grid-cols-2">
                      <div className="space-y-2">
                        <Label>内存保留时间 (秒)</Label>
                        <Input
                          type="number"
                          min={30}
                          value={requestDetailConfig.ttlSec}
                          onChange={(e) => handleRequestDetailConfigChange('ttlSec', parseInt(e.target.value) || 30)}
                        />
                        <p className="text-xs text-muted-foreground">内存中的请求详情保留时长，最小 30 秒。</p>
                      </div>
                      <div className="space-y-2">
                        <Label>最大记录条数</Label>
                        <Input
                          type="number"
                          min={50}
                          value={requestDetailConfig.maxEntries}
                          onChange={(e) => handleRequestDetailConfigChange('maxEntries', parseInt(e.target.value) || 50)}
                        />
                        <p className="text-xs text-muted-foreground">超过上限后会优先淘汰旧记录，最小 50 条。</p>
                      </div>
                      <div className="space-y-2">
                        <Label>最大内存预算 (MB)</Label>
                        <Input
                          type="number"
                          min={16}
                          value={requestDetailConfig.maxMemoryMB}
                          onChange={(e) => handleRequestDetailConfigChange('maxMemoryMB', parseInt(e.target.value) || 16)}
                        />
                        <p className="text-xs text-muted-foreground">请求详情总内存预算，最小 16 MB。</p>
                      </div>
                      <div className="space-y-2">
                        <Label>单次正文截断上限 (KB)</Label>
                        <Input
                          type="number"
                          min={4}
                          value={requestDetailConfig.bodyCapKB}
                          onChange={(e) => handleRequestDetailConfigChange('bodyCapKB', parseInt(e.target.value) || 4)}
                        />
                        <p className="text-xs text-muted-foreground">单次请求或响应正文最多记录大小，最小 4 KB。</p>
                      </div>
                    </div>

                    <div className="grid gap-4 md:grid-cols-2">
                      <div className="space-y-2">
                        <Label>高 RPM 模式</Label>
                        <Select
                          value={requestDetailConfig.highRpmMode}
                          onValueChange={(value: RequestDetailConfig['highRpmMode']) =>
                            handleRequestDetailConfigChange('highRpmMode', value)
                          }
                        >
                          <SelectTrigger>
                            <SelectValue placeholder="选择高 RPM 策略" />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="full">继续全量记录</SelectItem>
                            <SelectItem value="sample">按采样率记录</SelectItem>
                            <SelectItem value="off">停止记录详情</SelectItem>
                          </SelectContent>
                        </Select>
                        <p className="text-xs text-muted-foreground">高流量时的降级方式。</p>
                      </div>
                      <div className="space-y-2">
                        <Label>高 RPM 阈值</Label>
                        <Input
                          type="number"
                          min={100}
                          value={requestDetailConfig.highRpmThreshold}
                          onChange={(e) =>
                            handleRequestDetailConfigChange('highRpmThreshold', parseInt(e.target.value) || 100)
                          }
                        />
                        <p className="text-xs text-muted-foreground">当分钟请求量达到该值后启用高 RPM 模式，最小 100。</p>
                      </div>
                      <div className="space-y-2 md:col-span-2">
                        <Label>高 RPM 采样率 (%)</Label>
                        <Input
                          type="number"
                          min={1}
                          max={100}
                          value={requestDetailConfig.highRpmSamplePercent}
                          onChange={(e) =>
                            handleRequestDetailConfigChange('highRpmSamplePercent', parseInt(e.target.value) || 1)
                          }
                          disabled={requestDetailConfig.highRpmMode !== 'sample'}
                        />
                        <p className="text-xs text-muted-foreground">
                          仅在“按采样率记录”模式下生效，取值 1-100。
                        </p>
                      </div>
                    </div>

                    <Alert>
                      <AlertDescription>
                        建议在高并发环境下结合阈值与采样率使用，避免请求详情占用过多内存和存储资源。
                      </AlertDescription>
                    </Alert>

                    <Button onClick={handleSaveRequestDetailConfig} disabled={requestDetailLoading}>
                      {requestDetailLoading ? '保存中...' : '保存配置'}
                    </Button>
                  </>
                ) : (
                  <div className="text-center text-muted-foreground py-4">加载中...</div>
                )}
              </CardContent>
            </Card>
          )}

          {activeTab === 'cache' && (
            <Card>
              <CardHeader>
                <CardTitle>缓存 TTL 覆盖</CardTitle>
                <CardDescription>控制发送给 Claude API 的 cache_control TTL 值</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="space-y-3">
                  <Label>TTL 策略</Label>
                  <div className="flex gap-2">
                    <Button
                      variant={cacheTTL === '1h' ? 'default' : 'outline'}
                      size="sm"
                      onClick={() => handleCacheTTLChange('1h')}
                      disabled={cacheTTLLoading}
                    >
                      1 小时
                    </Button>
                    <Button
                      variant={cacheTTL === '5m' ? 'default' : 'outline'}
                      size="sm"
                      onClick={() => handleCacheTTLChange('5m')}
                      disabled={cacheTTLLoading}
                    >
                      5 分钟
                    </Button>
                    <Button
                      variant={cacheTTL === '' ? 'default' : 'outline'}
                      size="sm"
                      onClick={() => handleCacheTTLChange('')}
                      disabled={cacheTTLLoading}
                    >
                      不覆盖
                    </Button>
                  </div>
                  <p className="text-sm text-muted-foreground">
                    选择"1小时"将强制所有 cache_control TTL 为 1h（省钱），"5分钟"为原始值，"不覆盖"保留请求原始 TTL 不做修改。
                  </p>
                </div>
              </CardContent>
            </Card>
          )}

          {activeTab === 'timeout' && (
            <Card>
              <CardHeader>
                <CardTitle>超时配置</CardTitle>
                <CardDescription>配置代理连接和流式响应的超时时间（单位：秒）</CardDescription>
              </CardHeader>
              <CardContent className="space-y-6">
                {timeoutConfig ? (
                  <>
                    <div className="grid gap-4 md:grid-cols-2">
                      <div className="space-y-2">
                        <Label>空闲连接超时</Label>
                        <Input
                          type="number"
                          min={30}
                          value={timeoutConfig.idleConnTimeoutSec}
                          onChange={(e) => handleTimeoutConfigChange('idleConnTimeoutSec', parseInt(e.target.value) || 300)}
                        />
                        <p className="text-xs text-muted-foreground">连接池中空闲连接的最大存活时间（&gt;=30秒）</p>
                      </div>
                      <div className="space-y-2">
                        <Label>读取空闲超时</Label>
                        <Input
                          type="number"
                          min={60}
                          value={timeoutConfig.readIdleTimeoutSec}
                          onChange={(e) => handleTimeoutConfigChange('readIdleTimeoutSec', parseInt(e.target.value) || 300)}
                        />
                        <p className="text-xs text-muted-foreground">AI 思考时无数据的最大等待时间（&gt;=60秒）</p>
                      </div>
                      <div className="space-y-2">
                        <Label>心跳间隔</Label>
                        <Input
                          type="number"
                          min={5}
                          value={timeoutConfig.keepAliveIntervalSec}
                          onChange={(e) => handleTimeoutConfigChange('keepAliveIntervalSec', parseInt(e.target.value) || 15)}
                        />
                        <p className="text-xs text-muted-foreground">SSE 流心跳发送间隔（&gt;=5秒）</p>
                      </div>
                      <div className="space-y-2">
                        <Label>连接超时</Label>
                        <Input
                          type="number"
                          min={5}
                          value={timeoutConfig.dialTimeoutSec}
                          onChange={(e) => handleTimeoutConfigChange('dialTimeoutSec', parseInt(e.target.value) || 30)}
                        />
                        <p className="text-xs text-muted-foreground">建立 TCP 连接的超时时间（&gt;=5秒）</p>
                      </div>
                      <div className="space-y-2">
                        <Label>TLS 握手超时</Label>
                        <Input
                          type="number"
                          min={5}
                          value={timeoutConfig.tlsHandshakeTimeoutSec}
                          onChange={(e) => handleTimeoutConfigChange('tlsHandshakeTimeoutSec', parseInt(e.target.value) || 15)}
                        />
                        <p className="text-xs text-muted-foreground">TLS 握手的超时时间（&gt;=5秒）</p>
                      </div>
                    </div>

                    <Button onClick={handleSaveTimeoutConfig} disabled={timeoutLoading}>
                      {timeoutLoading ? '保存中...' : '保存配置'}
                    </Button>
                  </>
                ) : (
                  <div className="text-center text-muted-foreground py-4">加载中...</div>
                )}
              </CardContent>
            </Card>
          )}

          {activeTab === 'billing' && (
            <Card>
              <CardHeader>
                <CardTitle>Redis 计费运行时</CardTitle>
                <CardDescription>配置多实例共享的 Redis 计费热路径，并在保存后立即重连应用运行时。</CardDescription>
              </CardHeader>
              <CardContent className="space-y-6">
                {billingRuntimeConfig ? (
                  <>
                    <div className="grid gap-4 md:grid-cols-2">
                      <div className="rounded-lg border p-4 space-y-2">
                        <div className="flex items-center justify-between">
                          <span className="text-sm text-muted-foreground">运行时状态</span>
                          <Badge variant={billingRuntimeBadge.variant}>{billingRuntimeBadge.label}</Badge>
                        </div>
                        <p className="text-sm text-muted-foreground">{billingRuntimeDescription}</p>
                      </div>
                      <div className="rounded-lg border p-4 space-y-2">
                        <div className="flex items-center justify-between">
                          <span className="text-sm text-muted-foreground">Redis 前缀</span>
                          <Badge variant="outline">{billingRuntimeConfig.redisPrefix || 'ampmanager'}</Badge>
                        </div>
                        <p className="text-sm text-muted-foreground">
                          多实例部署时请确保所有实例共享同一 Redis 和同一前缀。
                        </p>
                      </div>
                    </div>

                    <div className="rounded-lg border p-4 space-y-4">
                      <div className="flex items-center justify-between gap-3">
                        <div className="space-y-1">
                          <h3 className="text-sm font-medium">运行时摘要</h3>
                          <p className="text-sm text-muted-foreground">
                            页面进入时拉取一次，必要时手动刷新；不进行后台轮询。
                          </p>
                        </div>
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={() => fetchBillingRuntimeStats()}
                          disabled={billingRuntimeStatsLoading}
                        >
                          <RefreshCw className={`mr-2 h-4 w-4 ${billingRuntimeStatsLoading ? 'animate-spin' : ''}`} />
                          刷新统计
                        </Button>
                      </div>

                      {billingRuntimeStats ? (
                        <>
                          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                            <div className="rounded-md bg-muted/40 p-3">
                              <div className="text-xs text-muted-foreground">Reclaim claimed</div>
                              <div className="mt-1 text-2xl font-semibold">{billingRuntimeStats.reclaimClaimed}</div>
                            </div>
                            <div className="rounded-md bg-muted/40 p-3">
                              <div className="text-xs text-muted-foreground">Reconcile repairs</div>
                              <div className="mt-1 text-2xl font-semibold">{billingRuntimeStats.reconcileRepairs}</div>
                            </div>
                            <div className="rounded-md bg-muted/40 p-3">
                              <div className="text-xs text-muted-foreground">Runtime enabled</div>
                              <div className="mt-1 text-2xl font-semibold">{billingRuntimeStats.runtimeEnabled ? 'Yes' : 'No'}</div>
                            </div>
                            <div className="rounded-md bg-muted/40 p-3">
                              <div className="text-xs text-muted-foreground">Runtime healthy</div>
                              <div className="mt-1 text-2xl font-semibold">{billingRuntimeStats.runtimeHealthy ? 'Yes' : 'No'}</div>
                            </div>
                          </div>

                          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-5">
                            <div className="rounded-md bg-muted/40 p-3">
                              <div className="text-xs text-muted-foreground">Projector consumers</div>
                              <div className="mt-1 text-2xl font-semibold">{billingRuntimeStats.consumerCount}</div>
                            </div>
                            <div className="rounded-md bg-muted/40 p-3">
                              <div className="text-xs text-muted-foreground">Active consumers</div>
                              <div className="mt-1 text-2xl font-semibold">{billingRuntimeStats.activeProjectorConsumers}</div>
                            </div>
                            <div className="rounded-md bg-muted/40 p-3">
                              <div className="text-xs text-muted-foreground">Stale consumers</div>
                              <div className="mt-1 text-2xl font-semibold">{billingRuntimeStats.staleProjectorConsumers}</div>
                            </div>
                            <div className="rounded-md bg-muted/40 p-3">
                              <div className="text-xs text-muted-foreground">Pending entries</div>
                              <div className="mt-1 text-2xl font-semibold">{billingRuntimeStats.pendingEntries}</div>
                            </div>
                            <div className="rounded-md bg-muted/40 p-3">
                              <div className="text-xs text-muted-foreground">Oldest pending idle</div>
                              <div className="mt-1 text-2xl font-semibold">{formatIdleMs(billingRuntimeStats.oldestPendingIdleMs)}</div>
                            </div>
                          </div>

                          <div className="overflow-x-auto">
                            <Table>
                              <TableHeader>
                                <TableRow>
                                  <TableHead>阶段</TableHead>
                                  <TableHead>样本数</TableHead>
                                  <TableHead>p95</TableHead>
                                  <TableHead>p99</TableHead>
                                  <TableHead>失败数</TableHead>
                                </TableRow>
                              </TableHeader>
                              <TableBody>
                                {billingMetricItems.map((item) => (
                                  <TableRow key={item.key}>
                                    <TableCell className="font-medium">{item.label}</TableCell>
                                    <TableCell>{item.value.samples}</TableCell>
                                    <TableCell>{formatLatency(item.value.p95Ms)}</TableCell>
                                    <TableCell>{formatLatency(item.value.p99Ms)}</TableCell>
                                    <TableCell>{item.value.failures}</TableCell>
                                  </TableRow>
                                ))}
                              </TableBody>
                            </Table>
                          </div>
                        </>
                      ) : (
                        <div className="text-sm text-muted-foreground">暂无运行时统计。</div>
                      )}
                    </div>

                    <div className="grid gap-4 md:grid-cols-2">
                      <div className="space-y-2 md:col-span-2">
                        <Label htmlFor="redisUrl">Redis URL</Label>
                        <Input
                          id="redisUrl"
                          value={billingRuntimeConfig.redisUrl}
                          onChange={(e) => handleBillingRuntimeConfigChange('redisUrl', e.target.value)}
                          placeholder="redis://localhost:6379/0"
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="redisPrefix">Redis Prefix</Label>
                        <Input
                          id="redisPrefix"
                          value={billingRuntimeConfig.redisPrefix}
                          onChange={(e) => handleBillingRuntimeConfigChange('redisPrefix', e.target.value)}
                          placeholder="ampmanager"
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="streamBatchSize">Projector 批大小</Label>
                        <Input
                          id="streamBatchSize"
                          type="number"
                          min={1}
                          value={billingRuntimeConfig.streamBatchSize}
                          onChange={(e) => handleBillingRuntimeConfigChange('streamBatchSize', parseInt(e.target.value) || 1)}
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="projectorWorkers">Projector Workers</Label>
                        <Input
                          id="projectorWorkers"
                          type="number"
                          min={1}
                          value={billingRuntimeConfig.projectorWorkers}
                          onChange={(e) => handleBillingRuntimeConfigChange('projectorWorkers', parseInt(e.target.value) || 1)}
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="projectorClaimIdleSec">Projector reclaim idle (秒)</Label>
                        <Input
                          id="projectorClaimIdleSec"
                          type="number"
                          min={1}
                          value={billingRuntimeConfig.projectorClaimIdleSec}
                          onChange={(e) => handleBillingRuntimeConfigChange('projectorClaimIdleSec', parseInt(e.target.value) || 1)}
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="reconcileBatchSize">Reconcile 批大小</Label>
                        <Input
                          id="reconcileBatchSize"
                          type="number"
                          min={1}
                          value={billingRuntimeConfig.reconcileBatchSize}
                          onChange={(e) => handleBillingRuntimeConfigChange('reconcileBatchSize', parseInt(e.target.value) || 1)}
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="expiryBatchSize">Expiry 批大小</Label>
                        <Input
                          id="expiryBatchSize"
                          type="number"
                          min={1}
                          value={billingRuntimeConfig.expiryBatchSize}
                          onChange={(e) => handleBillingRuntimeConfigChange('expiryBatchSize', parseInt(e.target.value) || 1)}
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="reservationTtlSec">Reservation TTL (秒)</Label>
                        <Input
                          id="reservationTtlSec"
                          type="number"
                          min={60}
                          value={billingRuntimeConfig.reservationTtlSec}
                          onChange={(e) => handleBillingRuntimeConfigChange('reservationTtlSec', parseInt(e.target.value) || 60)}
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="reconcileIntervalSec">Reconcile 间隔 (秒)</Label>
                        <Input
                          id="reconcileIntervalSec"
                          type="number"
                          min={10}
                          value={billingRuntimeConfig.reconcileIntervalSec}
                          onChange={(e) => handleBillingRuntimeConfigChange('reconcileIntervalSec', parseInt(e.target.value) || 10)}
                        />
                      </div>
                    </div>

                    <Alert>
                      <AlertDescription>
                        保存后会立即重载后端 Redis 计费运行时。要完成真实压测，请让所有应用实例共享同一 PostgreSQL 和同一 Redis。
                      </AlertDescription>
                    </Alert>

                    <div className="flex justify-end">
                      <Button onClick={handleSaveBillingRuntimeConfig} disabled={billingRuntimeLoading}>
                        {billingRuntimeLoading ? '保存中...' : '保存并热更新'}
                      </Button>
                    </div>
                  </>
                ) : (
                  <div className="text-center text-muted-foreground py-4">加载中...</div>
                )}
              </CardContent>
            </Card>
          )}
        </motion.div>
      </AnimatePresence>
    </motion.div>
  )
}
