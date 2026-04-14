import { useEffect, useState } from 'react'

import { changePassword, changeUsername, getMyBalance, type BalanceInfo } from '../api/users'
import { getBillingState, updateBillingPriority, type BillingStateResponse } from '@/api/billing'
import { navigateDashboard } from '@/lib/dashboard-navigation'
import { formatDateTime, formatDecimal } from '@/lib/formatters'
import { RedeemQuickEntry, RedeemRecordTable } from '@/components/redeem/RedeemPanels'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { motion } from '@/lib/motion'
import { ArrowRightLeft, CheckCircle2, CreditCard, RefreshCw, Shield, XCircle } from 'lucide-react'

type AccountTab = 'balance' | 'billing' | 'security'

const tabs: { key: AccountTab; label: string }[] = [
  { key: 'balance', label: '余额' },
  { key: 'billing', label: '计费' },
  { key: 'security', label: '安全' },
]

interface Props {
  username: string
  onUsernameChange: (newUsername: string) => void
}

const LIMIT_TYPE_LABELS: Record<string, string> = {
  daily: '日限制',
  weekly: '周限制',
  monthly: '月限制',
  rolling_5h: '5小时滚动',
  total: '总量限制',
}

const WINDOW_MODE_LABELS: Record<string, string> = {
  fixed: '固定窗口',
  sliding: '滑动窗口',
}

function formatBalance(micros: number): string {
  return `$${formatDecimal(micros / 1e6, 6)}`
}

export default function AccountSettings({ username, onUsernameChange }: Props) {
  const [activeTab, setActiveTab] = useState<AccountTab>('balance')
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [redeemRefreshKey, setRedeemRefreshKey] = useState(0)

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
    void fetchBalance()
    void fetchBillingState()
  }, [])

  const showMessage = (type: 'success' | 'error', text: string) => {
    setMessage({ type, text })
    window.setTimeout(() => setMessage(null), 3000)
  }

  const fetchBalance = async () => {
    setBalanceLoading(true)
    try {
      const data = await getMyBalance()
      setBalance(data)
    } catch {
      setBalance(null)
    } finally {
      setBalanceLoading(false)
    }
  }

  const fetchBillingState = async () => {
    setBillingLoading(true)
    try {
      const data = await getBillingState()
      setBillingState(data)
    } catch {
      setBillingState(null)
    } finally {
      setBillingLoading(false)
    }
  }

  const handleChangePassword = async (event: React.FormEvent) => {
    event.preventDefault()
    if (newPassword !== confirmPassword) {
      showMessage('error', '两次输入的密码不一致')
      return
    }
    if (newPassword.length < 6) {
      showMessage('error', '新密码至少需要 6 个字符')
      return
    }

    setChangingPassword(true)
    try {
      await changePassword(oldPassword, newPassword)
      setOldPassword('')
      setNewPassword('')
      setConfirmPassword('')
      showMessage('success', '密码修改成功')
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '修改失败')
    } finally {
      setChangingPassword(false)
    }
  }

  const handleChangeUsername = async (event: React.FormEvent) => {
    event.preventDefault()
    if (newUsername.trim().length < 3) {
      showMessage('error', '用户名至少需要 3 个字符')
      return
    }

    setChangingUsername(true)
    try {
      await changeUsername(newUsername.trim())
      localStorage.setItem('username', newUsername.trim())
      onUsernameChange(newUsername.trim())
      setNewUsername('')
      showMessage('success', '用户名修改成功')
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '修改失败')
    } finally {
      setChangingUsername(false)
    }
  }

  const handlePriorityChange = async (value: string) => {
    setPrioritySaving(true)
    try {
      await updateBillingPriority(value as 'subscription' | 'balance')
      showMessage('success', '扣费顺序已更新')
      await fetchBillingState()
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '更新失败')
    } finally {
      setPrioritySaving(false)
    }
  }

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="space-y-6">
      <div className="space-y-1 border-b border-border/70 pb-5">
        <h2 className="text-2xl font-semibold tracking-tight">账户设置</h2>
        <p className="text-sm text-muted-foreground">余额、扣费顺序、订阅与安全。</p>
      </div>

      {message && (
        <Alert variant={message.type === 'error' ? 'destructive' : 'default'}>
          {message.type === 'success' ? <CheckCircle2 className="h-4 w-4" /> : <XCircle className="h-4 w-4" />}
          <AlertDescription>{message.text}</AlertDescription>
        </Alert>
      )}

      <div className="flex items-center gap-2 border-b border-border/70 pb-2">
        {tabs.map((tab) => (
          <button
            key={tab.key}
            onClick={() => setActiveTab(tab.key)}
            className={`rounded-full px-3 py-1.5 text-sm transition-colors ${
              activeTab === tab.key
                ? 'bg-primary text-primary-foreground'
                : 'text-muted-foreground hover:bg-muted hover:text-foreground'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {activeTab === 'balance' && (
        <section className="rounded-xl border border-border/80 bg-card/95 shadow-sm">
          <div className="flex flex-col gap-3 border-b border-border/70 px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h3 className="text-base font-semibold">账户余额</h3>
              <p className="text-sm text-muted-foreground">余额由管理员充值，按请求自动扣费。</p>
            </div>
            <Button variant="outline" size="sm" onClick={() => void fetchBalance()} disabled={balanceLoading}>
              <RefreshCw className={`mr-2 h-4 w-4 ${balanceLoading ? 'animate-spin' : ''}`} />
              刷新
            </Button>
          </div>
          <div className="grid gap-4 px-5 py-5 lg:grid-cols-[1fr_0.8fr]">
            <div className="rounded-xl border border-border/70 bg-background/70 px-5 py-5">
              <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">当前余额</p>
              <p className="mt-3 text-4xl font-semibold tracking-tight">
                {balance ? formatBalance(balance.balanceMicros) : '-'}
              </p>
            </div>
            <div className="rounded-xl border border-border/70 bg-background/70 px-5 py-5">
              <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">快捷入口</p>
              <div className="mt-3 flex flex-wrap gap-2">
                <Button variant="outline" size="sm" onClick={() => navigateDashboard('purchase-center')}>
                  <CreditCard className="mr-2 h-4 w-4" />
                  购买订阅
                </Button>
              </div>
            </div>
          </div>
        </section>
      )}

      {activeTab === 'billing' && (
        <div className="space-y-5">
          <section className="rounded-xl border border-border/80 bg-card/95 shadow-sm">
            <div className="flex flex-col gap-3 border-b border-border/70 px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <h3 className="text-base font-semibold">扣费顺序</h3>
                <p className="text-sm text-muted-foreground">设置请求优先扣订阅还是余额。</p>
              </div>
              <Button variant="outline" size="sm" onClick={() => void fetchBillingState()} disabled={billingLoading}>
                <RefreshCw className={`mr-2 h-4 w-4 ${billingLoading ? 'animate-spin' : ''}`} />
                刷新
              </Button>
            </div>
            <div className="px-5 py-5">
              <RadioGroup
                value={billingState?.primarySource || 'subscription'}
                onValueChange={(value) => void handlePriorityChange(value)}
                className={`grid gap-3 md:grid-cols-2 ${prioritySaving ? 'pointer-events-none opacity-60' : ''}`}
              >
                {[
                  { value: 'subscription', title: '订阅优先', desc: '先用订阅额度，不足再扣余额' },
                  { value: 'balance', title: '余额优先', desc: '先扣余额，不足再用订阅额度' },
                ].map((option) => (
                  <label
                    key={option.value}
                    className={`flex items-start gap-3 rounded-xl border px-4 py-4 transition-colors ${
                      (billingState?.primarySource || 'subscription') === option.value
                        ? 'border-primary bg-primary/5'
                        : 'border-border/70 hover:bg-accent/40'
                    }`}
                  >
                    <RadioGroupItem value={option.value} className="mt-0.5" />
                    <div>
                      <div className="font-medium">{option.title}</div>
                      <div className="text-xs text-muted-foreground">{option.desc}</div>
                    </div>
                  </label>
                ))}
              </RadioGroup>
            </div>
          </section>

          <section className="rounded-xl border border-border/80 bg-card/95 shadow-sm">
            <div className="flex flex-col gap-3 border-b border-border/70 px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <h3 className="text-base font-semibold">当前订阅</h3>
                <p className="text-sm text-muted-foreground">查看套餐状态和额度窗口。</p>
              </div>
              <Button variant="outline" size="sm" onClick={() => navigateDashboard('purchase-center')}>
                <CreditCard className="mr-2 h-4 w-4" />
                购买续费
              </Button>
            </div>
            <div className="px-5 py-5">
              {billingState?.subscription ? (
                <div className="space-y-4">
                  <div className="flex flex-wrap items-start justify-between gap-3 rounded-xl border border-border/70 bg-background/70 px-4 py-4">
                    <div className="space-y-1">
                      <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">套餐</p>
                      <p className="text-lg font-semibold">{billingState.subscription.planName}</p>
                      <p className="text-sm text-muted-foreground">
                        {billingState.subscription.expiresAt
                          ? `到期 ${formatDateTime(billingState.subscription.expiresAt)}`
                          : '永久有效'}
                      </p>
                    </div>
                    <Badge variant={billingState.subscription.status === 'active' ? 'default' : 'secondary'}>
                      {billingState.subscription.status === 'active' ? '生效中' : billingState.subscription.status}
                    </Badge>
                  </div>
                  {billingState.windows?.length ? (
                    <div className="overflow-hidden rounded-xl border border-border/70">
                      <div className="divide-y divide-border/70">
                        {billingState.windows.map((item) => (
                          <div key={`${item.limitType}-${item.windowMode}`} className="grid gap-2 px-4 py-3 md:grid-cols-[0.8fr_1.4fr_0.8fr] md:items-center">
                            <div>
                              <p className="font-medium">{LIMIT_TYPE_LABELS[item.limitType] || item.limitType}</p>
                              <p className="text-xs text-muted-foreground">{WINDOW_MODE_LABELS[item.windowMode] || item.windowMode}</p>
                            </div>
                            <p className="font-mono text-xs text-muted-foreground">
                              已用 ${formatDecimal(item.usedMicros / 1e6, 2)} / 剩余 ${formatDecimal(item.leftMicros / 1e6, 2)} / 限额 ${formatDecimal(item.limitMicros / 1e6, 2)}
                            </p>
                            <p className="text-xs text-muted-foreground md:text-right">
                              {formatDateTime(item.windowStart)} - {formatDateTime(item.windowEnd)}
                            </p>
                          </div>
                        ))}
                      </div>
                    </div>
                  ) : null}
                </div>
              ) : (
                <div className="rounded-xl border border-dashed border-border/70 px-5 py-8 text-center text-sm text-muted-foreground">
                  暂无生效中的订阅。
                </div>
              )}
            </div>
          </section>

          <section className="rounded-xl border border-border/80 bg-card/95 shadow-sm">
            <div className="border-b border-border/70 px-5 py-4">
              <h3 className="text-base font-semibold">兑换码</h3>
              <p className="mt-1 text-sm text-muted-foreground">可直接兑换订阅或余额。</p>
            </div>
            <div className="px-5 py-5">
              <RedeemQuickEntry
                onSuccess={async () => {
                  setRedeemRefreshKey((current) => current + 1)
                  await Promise.all([fetchBalance(), fetchBillingState()])
                }}
              />
            </div>
          </section>

          <section className="rounded-xl border border-border/80 bg-card/95 shadow-sm">
            <div className="border-b border-border/70 px-5 py-4">
              <h3 className="text-base font-semibold">最近兑换记录</h3>
              <p className="mt-1 text-sm text-muted-foreground">保留当前账号最近记录。</p>
            </div>
            <div className="overflow-hidden rounded-b-xl px-5 py-5">
              <RedeemRecordTable compact refreshKey={redeemRefreshKey} />
            </div>
          </section>
        </div>
      )}

      {activeTab === 'security' && (
        <div className="grid gap-5 xl:grid-cols-2">
          <section className="rounded-xl border border-border/80 bg-card/95 shadow-sm">
            <div className="border-b border-border/70 px-5 py-4">
              <h3 className="text-base font-semibold">修改密码</h3>
              <p className="mt-1 text-sm text-muted-foreground">更新当前账户密码。</p>
            </div>
            <form onSubmit={(event) => void handleChangePassword(event)} className="space-y-4 px-5 py-5">
              <div className="space-y-2">
                <Label htmlFor="oldPassword">当前密码</Label>
                <Input id="oldPassword" type="password" value={oldPassword} onChange={(event) => setOldPassword(event.target.value)} required />
              </div>
              <div className="space-y-2">
                <Label htmlFor="newPassword">新密码</Label>
                <Input id="newPassword" type="password" value={newPassword} onChange={(event) => setNewPassword(event.target.value)} required minLength={6} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="confirmPassword">确认新密码</Label>
                <Input id="confirmPassword" type="password" value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} required />
              </div>
              <Button type="submit" disabled={changingPassword}>
                {changingPassword ? <RefreshCw className="mr-2 h-4 w-4 animate-spin" /> : <Shield className="mr-2 h-4 w-4" />}
                修改密码
              </Button>
            </form>
          </section>

          <section className="rounded-xl border border-border/80 bg-card/95 shadow-sm">
            <div className="border-b border-border/70 px-5 py-4">
              <h3 className="text-base font-semibold">修改用户名</h3>
              <p className="mt-1 text-sm text-muted-foreground">当前用户名：{username}</p>
            </div>
            <form onSubmit={(event) => void handleChangeUsername(event)} className="space-y-4 px-5 py-5">
              <div className="space-y-2">
                <Label htmlFor="newUsername">新用户名</Label>
                <Input id="newUsername" value={newUsername} onChange={(event) => setNewUsername(event.target.value)} required minLength={3} maxLength={32} />
              </div>
              <Button type="submit" disabled={changingUsername}>
                {changingUsername ? <RefreshCw className="mr-2 h-4 w-4 animate-spin" /> : <ArrowRightLeft className="mr-2 h-4 w-4" />}
                更新用户名
              </Button>
            </form>
          </section>
        </div>
      )}
    </motion.div>
  )
}
