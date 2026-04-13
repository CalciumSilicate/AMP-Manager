import { useState, useEffect } from 'react'
import { motion, AnimatePresence } from '@/lib/motion'
import { changePassword, changeUsername, getMyBalance, BalanceInfo } from '../api/users'
import { getBillingState, updateBillingPriority, BillingStateResponse } from '@/api/billing'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { CircularProgress } from '@/components/CircularProgress'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Badge } from '@/components/ui/badge'
import { formatDecimal } from '@/lib/formatters'
import { CheckCircle2, XCircle, Wallet, RefreshCw, CreditCard, ArrowRightLeft, Shield } from 'lucide-react'

type AccountTab = 'security' | 'balance' | 'billing'

const tabs: { key: AccountTab; label: string }[] = [
  { key: 'balance', label: '余额' },
  { key: 'billing', label: '计费设置' },
  { key: 'security', label: '账户安全' },
]

interface Props {
  username: string
  onUsernameChange: (newUsername: string) => void
}

export default function AccountSettings({ username, onUsernameChange }: Props) {
  const [activeTab, setActiveTab] = useState<AccountTab>('balance')
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [changingPassword, setChangingPassword] = useState(false)

  const [newUsername, setNewUsername] = useState('')
  const [changingUsername, setChangingUsername] = useState(false)

  const [balance, setBalance] = useState<BalanceInfo | null>(null)
  const [balanceLoading, setBalanceLoading] = useState(false)

  const [billingState, setBillingState] = useState<BillingStateResponse | null>(null)
  const [billingLoading, setBillingLoading] = useState(false)
  const [prioritySaving, setPrioritySaving] = useState(false)

  useEffect(() => {
    fetchBalance()
    fetchBillingState()
  }, [])

  const fetchBalance = async () => {
    setBalanceLoading(true)
    try {
      const data = await getMyBalance()
      setBalance(data)
    } catch (err) {
      console.error('获取余额失败:', err)
    } finally {
      setBalanceLoading(false)
    }
  }

  const fetchBillingState = async () => {
    setBillingLoading(true)
    try {
      const data = await getBillingState()
      setBillingState(data)
    } catch (err) {
      console.error('获取计费状态失败:', err)
    } finally {
      setBillingLoading(false)
    }
  }

  const showMessage = (type: 'success' | 'error', text: string) => {
    setMessage({ type, text })
    setTimeout(() => setMessage(null), 3000)
  }

  const handleChangePassword = async (e: React.FormEvent) => {
    e.preventDefault()

    if (newPassword !== confirmPassword) {
      showMessage('error', '两次输入的密码不一致')
      return
    }

    if (newPassword.length < 6) {
      showMessage('error', '新密码至少需要6个字符')
      return
    }

    setChangingPassword(true)
    try {
      await changePassword(oldPassword, newPassword)
      showMessage('success', '密码修改成功')
      setOldPassword('')
      setNewPassword('')
      setConfirmPassword('')
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '修改失败')
    } finally {
      setChangingPassword(false)
    }
  }

  const handleChangeUsername = async (e: React.FormEvent) => {
    e.preventDefault()

    if (newUsername.length < 3) {
      showMessage('error', '用户名至少需要3个字符')
      return
    }

    setChangingUsername(true)
    try {
      await changeUsername(newUsername)
      showMessage('success', '用户名修改成功，请重新登录')
      onUsernameChange(newUsername)
      localStorage.setItem('username', newUsername)
      setNewUsername('')
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '修改失败')
    } finally {
      setChangingUsername(false)
    }
  }

  const formatBalance = (micros: number) => {
    return `$${formatDecimal(micros / 1e6, 6)}`
  }

  const handlePriorityChange = async (value: string) => {
    setPrioritySaving(true)
    try {
      await updateBillingPriority(value as 'subscription' | 'balance')
      showMessage('success', '计费优先级已更新')
      fetchBillingState()
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '更新失败')
    } finally {
      setPrioritySaving(false)
    }
  }

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="space-y-6">
      <motion.div
        initial={{ opacity: 0, y: -20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ type: 'spring', bounce: 0.2, duration: 0.6 }}
      >
        <h2 className="text-2xl font-bold tracking-tight">账户设置</h2>
        <p className="text-muted-foreground">管理您的账户安全和余额信息</p>
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
                layoutId="account-tab-indicator"
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
            <Alert variant={message.type === 'success' ? 'default' : 'destructive'}>
              {message.type === 'success' ? (
                <CheckCircle2 className="h-4 w-4" />
              ) : (
                <XCircle className="h-4 w-4" />
              )}
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
          {activeTab === 'security' && (
            <>
              <Card>
                <CardHeader>
                  <CardTitle>修改密码</CardTitle>
                  <CardDescription>更新您的账户密码</CardDescription>
                </CardHeader>
                <CardContent>
                  <form onSubmit={handleChangePassword} className="space-y-4">
                    <div className="space-y-2">
                      <Label htmlFor="oldPassword">当前密码</Label>
                      <Input
                        id="oldPassword"
                        type="password"
                        value={oldPassword}
                        onChange={(e) => setOldPassword(e.target.value)}
                        required
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="newPassword">新密码</Label>
                      <Input
                        id="newPassword"
                        type="password"
                        value={newPassword}
                        onChange={(e) => setNewPassword(e.target.value)}
                        required
                        minLength={6}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="confirmPassword">确认新密码</Label>
                      <Input
                        id="confirmPassword"
                        type="password"
                        value={confirmPassword}
                        onChange={(e) => setConfirmPassword(e.target.value)}
                        required
                      />
                    </div>
                    <Button type="submit" disabled={changingPassword}>
                      {changingPassword ? '修改中...' : '修改密码'}
                    </Button>
                  </form>
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle>修改用户名</CardTitle>
                  <CardDescription>当前用户名: {username}</CardDescription>
                </CardHeader>
                <CardContent>
                  <form onSubmit={handleChangeUsername} className="space-y-4">
                    <div className="space-y-2">
                      <Label htmlFor="newUsername">新用户名</Label>
                      <Input
                        id="newUsername"
                        type="text"
                        value={newUsername}
                        onChange={(e) => setNewUsername(e.target.value)}
                        required
                        minLength={3}
                        maxLength={32}
                      />
                    </div>
                    <Button type="submit" disabled={changingUsername}>
                      {changingUsername ? '修改中...' : '修改用户名'}
                    </Button>
                  </form>
                </CardContent>
              </Card>
            </>
          )}

          {activeTab === 'balance' && (
            <Card>
              <CardHeader>
                <div className="flex items-center justify-between">
                  <div>
                    <CardTitle className="flex items-center gap-2">
                      <Wallet className="h-5 w-5" />
                      账户余额
                    </CardTitle>
                    <CardDescription>查看当前账户余额信息</CardDescription>
                  </div>
                  <Button variant="outline" size="sm" onClick={fetchBalance} disabled={balanceLoading}>
                    <RefreshCw className={`h-4 w-4 mr-1 ${balanceLoading ? 'animate-spin' : ''}`} />
                    刷新
                  </Button>
                </div>
              </CardHeader>
              <CardContent>
                {balance ? (
                  <div className="space-y-6">
                    <div className="rounded-lg border bg-card p-6">
                      <div className="text-sm text-muted-foreground mb-1">当前余额</div>
                      <div className="text-3xl font-bold tracking-tight">
                        {formatBalance(balance.balanceMicros)}
                      </div>

                    </div>
                    <p className="text-sm text-muted-foreground">
                      余额由管理员充值，每次 API 请求将根据模型定价和分组倍率自动扣费。
                    </p>
                  </div>
                ) : (
                  <div className="text-center text-muted-foreground py-8">
                    {balanceLoading ? '加载中...' : '无法获取余额信息'}
                  </div>
                )}
              </CardContent>
            </Card>
          )}

          {activeTab === 'billing' && (
            <div className="space-y-6">
              <Card>
                <CardHeader>
                  <div className="flex items-center justify-between">
                    <div>
                      <CardTitle className="flex items-center gap-2">
                        <ArrowRightLeft className="h-5 w-5" />
                        扣费优先级
                      </CardTitle>
                      <CardDescription>设置 API 请求的扣费来源优先顺序</CardDescription>
                    </div>
                    <Button variant="outline" size="sm" onClick={fetchBillingState} disabled={billingLoading}>
                      <RefreshCw className={`h-4 w-4 mr-1 ${billingLoading ? 'animate-spin' : ''}`} />
                      刷新
                    </Button>
                  </div>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="space-y-2">
                    <Label>扣费优先级</Label>
                    <Select
                      value={billingState?.primarySource || 'subscription'}
                      onValueChange={handlePriorityChange}
                      disabled={prioritySaving}
                    >
                      <SelectTrigger className="w-64">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="subscription">订阅优先 — 先用订阅额度，不足再扣余额</SelectItem>
                        <SelectItem value="balance">余额优先 — 先扣余额，不足再用订阅额度</SelectItem>
                      </SelectContent>
                    </Select>
                    <p className="text-xs text-muted-foreground">
                      当前: {billingState?.primarySource === 'subscription' ? '① 订阅额度 → ② 余额' : '① 余额 → ② 订阅额度'}
                    </p>
                  </div>
                </CardContent>
              </Card>

              {billingState?.subscription && (
                <Card>
                  <CardHeader>
                    <CardTitle className="flex items-center gap-2">
                      <Shield className="h-5 w-5" />
                      订阅状态
                    </CardTitle>
                    <CardDescription>订阅套餐信息</CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    <div className="flex items-center justify-between rounded-lg border p-4">
                      <div>
                        <p className="font-semibold text-lg">{billingState.subscription.planName}</p>
                        <p className="text-sm text-muted-foreground">
                          {billingState.subscription.expiresAt
                            ? `到期时间: ${new Date(billingState.subscription.expiresAt).toLocaleString('zh-CN')}`
                            : '永不过期'}
                        </p>
                      </div>
                      <Badge variant={billingState.subscription.status === 'active' ? 'default' : 'secondary'}>
                        {billingState.subscription.status === 'active' ? '生效中' : billingState.subscription.status}
                      </Badge>
                    </div>

                    {billingState.windows && billingState.windows.length > 0 && (
                      <div className="space-y-3">
                        {billingState.windows.map((w, i) => {
                          const limitTypeLabels: Record<string, string> = {
                            daily: '日限制', weekly: '周限制', monthly: '月限制',
                            rolling_5h: '5小时滚动', total: '总量限制',
                          }
                          const windowModeLabels: Record<string, string> = {
                            fixed: '固定窗口', sliding: '滑动窗口',
                          }
                          const usedPct = w.limitMicros > 0 ? (w.usedMicros / w.limitMicros) * 100 : 0
                          return (
                            <div key={i} className="flex items-center justify-between gap-3 rounded-lg border px-3 py-2">
                              <div className="min-w-0">
                                <p className="text-sm font-medium">
                                  {limitTypeLabels[w.limitType] || w.limitType}
                                  <span className="ml-1 text-muted-foreground text-xs">({windowModeLabels[w.windowMode] || w.windowMode})</span>
                                </p>
                                <p className="font-mono text-xs text-muted-foreground">
                                  已用 ${formatDecimal(w.usedMicros / 1e6, 2)} / 剩余 ${formatDecimal(w.leftMicros / 1e6, 2)} / 限额 ${formatDecimal(w.limitMicros / 1e6, 2)}
                                </p>
                                {w.limitType !== 'total' && w.windowEnd && (
                                  <div className="text-[10px] text-muted-foreground">
                                    {w.windowMode === 'fixed'
                                      ? `重置于 ${new Date(w.windowEnd).toLocaleString('zh-CN')}`
                                      : `统计窗口: ${new Date(w.windowStart).toLocaleString('zh-CN')} ~ 现在`}
                                  </div>
                                )}
                              </div>
                              <CircularProgress
                                value={usedPct}
                                size={42}
                                strokeWidth={4}
                                label={`${Math.round(Math.min(usedPct, 100))}%`}
                                className="shrink-0"
                                indicatorClassName="stroke-purple-500"
                                trackClassName="stroke-border"
                              />
                            </div>
                          )
                        })}
                      </div>
                    )}
                  </CardContent>
                </Card>
              )}

              {!billingState?.subscription && !billingLoading && (
                <Card>
                  <CardContent className="py-8">
                    <div className="text-center text-muted-foreground">
                      <CreditCard className="h-8 w-8 mx-auto mb-2 opacity-50" />
                      <p>暂未订阅任何套餐</p>
                      <p className="text-xs mt-1">联系管理员分配订阅套餐</p>
                    </div>
                  </CardContent>
                </Card>
              )}
            </div>
          )}
        </motion.div>
      </AnimatePresence>
    </motion.div>
  )
}
