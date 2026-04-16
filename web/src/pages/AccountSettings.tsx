import { useEffect, useState } from 'react'

import { changePassword, changeUsername, getMyBalance, type BalanceInfo } from '../api/users'
import { getBillingState, updateBillingPriority, type BillingStateResponse } from '@/api/billing'
import {
  createBalanceTopupOrder,
  getPurchaseCatalog,
  refreshMyPurchaseOrder,
  type PurchaseCatalogResponse,
  type PurchaseOrder,
} from '@/api/purchase'
import { RedeemQuickEntry, RedeemRecordTable } from '@/components/redeem/RedeemPanels'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { navigateDashboard } from '@/lib/dashboard-navigation'
import { formatDateTime, formatDecimal } from '@/lib/formatters'
import { AnimatePresence, motion } from '@/lib/motion'
import { ArrowRightLeft, CreditCard, QrCode, RefreshCw, Shield } from 'lucide-react'

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

function formatPrimarySource(source: 'subscription' | 'balance'): string {
  return source === 'subscription' ? '订阅优先' : '余额优先'
}

function formatSubscriptionStatus(status: string): string {
  return status === 'active' ? '生效中' : status
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
  const [purchaseCatalog, setPurchaseCatalog] = useState<PurchaseCatalogResponse | null>(null)
  const [topupDialogOpen, setTopupDialogOpen] = useState(false)
  const [topupAmountUsd, setTopupAmountUsd] = useState('10')
  const [creatingTopupOrder, setCreatingTopupOrder] = useState(false)
  const [pendingTopupOrder, setPendingTopupOrder] = useState<PurchaseOrder | null>(null)
  const [refreshingTopupOrderNo, setRefreshingTopupOrderNo] = useState<string | null>(null)

  const [billingState, setBillingState] = useState<BillingStateResponse | null>(null)
  const [billingLoading, setBillingLoading] = useState(false)
  const [prioritySaving, setPrioritySaving] = useState(false)

  useEffect(() => {
    void fetchBalance()
    void fetchBillingState()
    void fetchPurchaseCatalog()
  }, [])

  const showMessage = (type: 'success' | 'error', text: string) => {
    setMessage({ type, text })
    window.setTimeout(() => setMessage(null), 5000)
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

  const fetchPurchaseCatalog = async () => {
    try {
      const data = await getPurchaseCatalog()
      setPurchaseCatalog(data)
    } catch {
      setPurchaseCatalog(null)
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

  const handleCreateTopupOrder = async () => {
    const amountUsd = Number.parseFloat(topupAmountUsd)
    if (Number.isNaN(amountUsd) || amountUsd <= 0) {
      showMessage('error', '充值金额无效')
      return
    }

    setCreatingTopupOrder(true)
    try {
      const order = await createBalanceTopupOrder(amountUsd)
      setTopupDialogOpen(false)
      setTopupAmountUsd(String(amountUsd))
      if (order.paymentStatus === 'paid') {
        await Promise.all([fetchBalance(), fetchBillingState(), fetchPurchaseCatalog()])
        showMessage('success', '余额已到账')
      } else {
        setPendingTopupOrder(order)
        showMessage('success', '充值订单已创建')
      }
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '创建充值订单失败')
    } finally {
      setCreatingTopupOrder(false)
    }
  }

  const handleRefreshTopupOrder = async () => {
    if (!pendingTopupOrder) return
    setRefreshingTopupOrderNo(pendingTopupOrder.orderNo)
    try {
      const order = await refreshMyPurchaseOrder(pendingTopupOrder.orderNo)
      if (order.paymentStatus === 'paid') {
        setPendingTopupOrder(null)
        await Promise.all([fetchBalance(), fetchBillingState(), fetchPurchaseCatalog()])
        showMessage('success', '余额已到账')
      } else {
        setPendingTopupOrder(order)
      }
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '刷新失败')
    } finally {
      setRefreshingTopupOrderNo(null)
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
        <p className="text-muted-foreground">管理账户余额、订阅顺序、兑换记录和安全信息</p>
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
            type="button"
            onClick={() => setActiveTab(tab.key)}
            className={`relative rounded-t-md px-4 py-2 text-sm font-medium transition-colors ${
              activeTab === tab.key
                ? 'text-foreground'
                : 'text-muted-foreground hover:text-foreground/80'
            }`}
          >
            {tab.label}
            {activeTab === tab.key && (
              <motion.div
                layoutId="account-settings-tab-indicator"
                className="absolute inset-x-0 -bottom-px h-0.5 bg-primary"
                transition={{ type: 'spring', bounce: 0.2, duration: 0.4 }}
              />
            )}
          </button>
        ))}
      </motion.div>

      <AnimatePresence>
        {message && (
          <motion.div
            initial={{ opacity: 0, y: -20, scale: 0.95 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: -20, scale: 0.95 }}
            transition={{ type: 'spring', bounce: 0.3, duration: 0.5 }}
          >
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
          {activeTab === 'balance' && (
            <Card>
              <CardHeader>
                <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div className="space-y-1">
                    <CardTitle>账户余额</CardTitle>
                    <CardDescription>查看余额和充值入口。</CardDescription>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Button type="button" variant="outline" size="sm" onClick={() => void fetchBalance()} disabled={balanceLoading}>
                      <RefreshCw className={`mr-2 h-4 w-4 ${balanceLoading ? 'animate-spin' : ''}`} />
                      刷新
                    </Button>
                  </div>
                </div>
              </CardHeader>
              <CardContent className="space-y-4">
                {balanceLoading ? (
                  <div className="py-4 text-center text-muted-foreground">加载中...</div>
                ) : (
                  <div className="grid gap-4 md:grid-cols-2">
                    <div className="space-y-2 rounded-lg border p-4">
                      <div className="flex items-center justify-between">
                        <span className="text-sm text-muted-foreground">当前余额</span>
                        <Badge variant="outline">USD</Badge>
                      </div>
                      <p className="text-3xl font-semibold tracking-tight">
                        {balance ? formatBalance(balance.balanceMicros) : '-'}
                      </p>
                      <p className="text-sm text-muted-foreground">
                        {purchaseCatalog?.balanceTopupEnabled
                          ? `单价 ¥${(purchaseCatalog.balanceTopupPriceCnyPerUsd || 0).toFixed(2)}/$1`
                          : '充值未开启'}
                      </p>
                      <div className="pt-2">
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={() => setTopupDialogOpen(true)}
                          disabled={!purchaseCatalog?.balanceTopupEnabled}
                        >
                          <CreditCard className="mr-2 h-4 w-4" />
                          充值余额
                        </Button>
                      </div>
                    </div>
                    <div className="space-y-3 rounded-lg border p-4">
                      <div className="flex items-center justify-between">
                        <span className="text-sm text-muted-foreground">后续操作</span>
                        <Badge variant="outline">快捷入口</Badge>
                      </div>
                      <p className="text-sm text-muted-foreground">需要购买或续费时，直接跳转到购买订阅页面。</p>
                      <div className="flex flex-wrap gap-2">
                        <Button type="button" variant="outline" size="sm" onClick={() => navigateDashboard('purchase-center')}>
                          <CreditCard className="mr-2 h-4 w-4" />
                          购买订阅
                        </Button>
                      </div>
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>
          )}

          {activeTab === 'billing' && (
            <>
              <Card>
                <CardHeader>
                  <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                    <div className="space-y-1">
                      <CardTitle>扣费顺序</CardTitle>
                      <CardDescription>控制订阅额度与余额的优先级，修改后立即生效。</CardDescription>
                    </div>
                    <Button type="button" variant="outline" size="sm" onClick={() => void fetchBillingState()} disabled={billingLoading}>
                      <RefreshCw className={`mr-2 h-4 w-4 ${billingLoading ? 'animate-spin' : ''}`} />
                      刷新
                    </Button>
                  </div>
                </CardHeader>
                <CardContent className="space-y-4">
                  {billingLoading && !billingState ? (
                    <div className="py-4 text-center text-muted-foreground">加载中...</div>
                  ) : billingState ? (
                    <>
                      <div className="grid gap-4 md:grid-cols-2">
                        <div className="space-y-2 rounded-lg border p-4">
                          <div className="flex items-center justify-between">
                            <span className="text-sm text-muted-foreground">当前优先级</span>
                            <Badge>{formatPrimarySource(billingState.primarySource)}</Badge>
                          </div>
                          <p className="text-sm text-muted-foreground">
                            {billingState.primarySource === 'subscription'
                              ? '先使用订阅额度，不足时自动回退到余额。'
                              : '先使用余额，不足时自动回退到订阅额度。'}
                          </p>
                        </div>
                        <div className="space-y-2 rounded-lg border p-4">
                          <div className="flex items-center justify-between">
                            <span className="text-sm text-muted-foreground">次级来源</span>
                            <Badge variant="outline">{formatPrimarySource(billingState.secondarySource)}</Badge>
                          </div>
                          <p className="text-sm text-muted-foreground">当主来源不足时，系统会自动切换到次级来源继续扣费。</p>
                        </div>
                      </div>

                      <div className="space-y-3">
                        <Label>优先级策略</Label>
                        <RadioGroup
                          value={billingState.primarySource || 'subscription'}
                          onValueChange={(value) => void handlePriorityChange(value)}
                          className={`grid gap-3 md:grid-cols-2 ${prioritySaving ? 'pointer-events-none opacity-60' : ''}`}
                        >
                          {[
                            { value: 'subscription', title: '订阅优先', desc: '先用订阅额度，不足再扣余额' },
                            { value: 'balance', title: '余额优先', desc: '先扣余额，不足再用订阅额度' },
                          ].map((option) => (
                            <label
                              key={option.value}
                              className={`flex items-start gap-3 rounded-lg border px-4 py-4 transition-colors ${
                                billingState.primarySource === option.value
                                  ? 'border-primary bg-primary/5'
                                  : 'border-border hover:bg-accent/40'
                              }`}
                            >
                              <RadioGroupItem value={option.value} className="mt-0.5" />
                              <div className="space-y-1">
                                <div className="font-medium">{option.title}</div>
                                <div className="text-xs text-muted-foreground">{option.desc}</div>
                              </div>
                            </label>
                          ))}
                        </RadioGroup>
                      </div>
                    </>
                  ) : (
                    <div className="py-4 text-center text-muted-foreground">计费状态加载失败</div>
                  )}
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                    <div className="space-y-1">
                      <CardTitle>当前订阅</CardTitle>
                      <CardDescription>查看套餐状态、窗口额度和当前的使用区间。</CardDescription>
                    </div>
                    <Button type="button" variant="outline" size="sm" onClick={() => navigateDashboard('purchase-center')}>
                      <CreditCard className="mr-2 h-4 w-4" />
                      购买续费
                    </Button>
                  </div>
                </CardHeader>
                <CardContent className="space-y-4">
                  {billingLoading && !billingState ? (
                    <div className="py-4 text-center text-muted-foreground">加载中...</div>
                  ) : billingState?.subscription ? (
                    <>
                      <div className="grid gap-4 md:grid-cols-2">
                        <div className="space-y-2 rounded-lg border p-4">
                          <div className="flex items-center justify-between">
                            <span className="text-sm text-muted-foreground">套餐</span>
                            <Badge variant={billingState.subscription.status === 'active' ? 'default' : 'secondary'}>
                              {formatSubscriptionStatus(billingState.subscription.status)}
                            </Badge>
                          </div>
                          <p className="text-lg font-semibold">{billingState.subscription.planName}</p>
                          <p className="text-sm text-muted-foreground">
                            {billingState.subscription.expiresAt
                              ? `到期 ${formatDateTime(billingState.subscription.expiresAt)}`
                              : '永久有效'}
                          </p>
                        </div>
                        <div className="space-y-2 rounded-lg border p-4">
                          <div className="flex items-center justify-between">
                            <span className="text-sm text-muted-foreground">窗口数量</span>
                            <Badge variant="outline">{billingState.windows?.length || 0}</Badge>
                          </div>
                          <p className="text-sm text-muted-foreground">
                            生效时间 {formatDateTime(billingState.subscription.startsAt)}
                          </p>
                          <p className="text-sm text-muted-foreground">窗口会按照订阅计划限制自动刷新或滚动结算。</p>
                        </div>
                      </div>

                      {billingState.windows?.length ? (
                        <div className="overflow-hidden rounded-lg border">
                          <div className="divide-y">
                            {billingState.windows.map((item) => (
                              <div
                                key={`${item.limitType}-${item.windowMode}`}
                                className="grid gap-2 px-4 py-3 md:grid-cols-[0.8fr_1.4fr_0.8fr] md:items-center"
                              >
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
                      ) : (
                        <div className="rounded-lg border border-dashed px-5 py-8 text-center text-sm text-muted-foreground">
                          当前订阅暂未返回额度窗口信息。
                        </div>
                      )}
                    </>
                  ) : (
                    <div className="rounded-lg border border-dashed px-5 py-8 text-center text-sm text-muted-foreground">
                      暂无生效中的订阅。
                    </div>
                  )}
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle>兑换码</CardTitle>
                  <CardDescription>保持单一输入路径，成功后立即刷新余额和订阅状态。</CardDescription>
                </CardHeader>
                <CardContent>
                  <RedeemQuickEntry
                    onSuccess={async () => {
                      setRedeemRefreshKey((current) => current + 1)
                      await Promise.all([fetchBalance(), fetchBillingState()])
                    }}
                  />
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle>最近兑换记录</CardTitle>
                  <CardDescription>把历史记录作为结果面板，和上方兑换操作分层。</CardDescription>
                </CardHeader>
                <CardContent>
                  <RedeemRecordTable compact refreshKey={redeemRefreshKey} />
                </CardContent>
              </Card>
            </>
          )}

          {activeTab === 'security' && (
            <>
              <Card>
                <CardHeader>
                  <CardTitle>修改密码</CardTitle>
                  <CardDescription>安全操作单独成面板，避免和用户名修改互相干扰。</CardDescription>
                </CardHeader>
                <CardContent>
                  <form onSubmit={(event) => void handleChangePassword(event)} className="space-y-4">
                    <div className="space-y-2">
                      <Label htmlFor="oldPassword">当前密码</Label>
                      <Input
                        id="oldPassword"
                        type="password"
                        autoComplete="current-password"
                        value={oldPassword}
                        onChange={(event) => setOldPassword(event.target.value)}
                        required
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="newPassword">新密码</Label>
                      <Input
                        id="newPassword"
                        type="password"
                        autoComplete="new-password"
                        value={newPassword}
                        onChange={(event) => setNewPassword(event.target.value)}
                        required
                        minLength={6}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="confirmPassword">确认新密码</Label>
                      <Input
                        id="confirmPassword"
                        type="password"
                        autoComplete="new-password"
                        value={confirmPassword}
                        onChange={(event) => setConfirmPassword(event.target.value)}
                        required
                      />
                    </div>
                    <div className="flex justify-end">
                      <Button type="submit" disabled={changingPassword}>
                        {changingPassword ? <RefreshCw className="mr-2 h-4 w-4 animate-spin" /> : <Shield className="mr-2 h-4 w-4" />}
                        修改密码
                      </Button>
                    </div>
                  </form>
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle>修改用户名</CardTitle>
                  <CardDescription>{`当前用户名：${username}`}</CardDescription>
                </CardHeader>
                <CardContent>
                  <form onSubmit={(event) => void handleChangeUsername(event)} className="space-y-4">
                    <div className="space-y-2">
                      <Label htmlFor="newUsername">新用户名</Label>
                      <Input
                        id="newUsername"
                        autoComplete="username"
                        value={newUsername}
                        onChange={(event) => setNewUsername(event.target.value)}
                        required
                        minLength={3}
                        maxLength={32}
                      />
                    </div>
                    <div className="flex justify-end">
                      <Button type="submit" disabled={changingUsername}>
                        {changingUsername ? <RefreshCw className="mr-2 h-4 w-4 animate-spin" /> : <ArrowRightLeft className="mr-2 h-4 w-4" />}
                        更新用户名
                      </Button>
                    </div>
                  </form>
                </CardContent>
              </Card>
            </>
          )}
        </motion.div>
      </AnimatePresence>

      <Dialog open={topupDialogOpen} onOpenChange={setTopupDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>充值余额</DialogTitle>
            <DialogDescription>
              {purchaseCatalog?.balanceTopupEnabled
                ? `单价 ¥${(purchaseCatalog.balanceTopupPriceCnyPerUsd || 0).toFixed(2)}/$1`
                : '充值未开启'}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-2">
              <Label htmlFor="topupAmountUsd">金额 (USD)</Label>
              <Input
                id="topupAmountUsd"
                type="number"
                min="0.01"
                step="0.01"
                value={topupAmountUsd}
                onChange={(event) => setTopupAmountUsd(event.target.value)}
              />
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setTopupDialogOpen(false)}>取消</Button>
            <Button type="button" onClick={() => void handleCreateTopupOrder()} disabled={creatingTopupOrder || !purchaseCatalog?.balanceTopupEnabled}>
              {creatingTopupOrder ? <RefreshCw className="mr-2 h-4 w-4 animate-spin" /> : null}
              创建订单
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!pendingTopupOrder} onOpenChange={(open) => !open && setPendingTopupOrder(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>等待支付</DialogTitle>
            <DialogDescription>
              {pendingTopupOrder ? `$${(pendingTopupOrder.balanceTopupMicros / 1_000_000).toFixed(2)}` : '-'}
            </DialogDescription>
          </DialogHeader>
          {pendingTopupOrder ? (
            <div className="space-y-5 py-2">
              <div className="grid gap-2 rounded-xl border border-border/70 px-4 py-4 text-sm">
                <div className="flex items-center justify-between gap-4">
                  <span className="text-muted-foreground">订单号</span>
                  <span className="font-mono text-xs">{pendingTopupOrder.orderNo}</span>
                </div>
                <div className="flex items-center justify-between gap-4">
                  <span className="text-muted-foreground">金额</span>
                  <span className="font-medium">¥{(pendingTopupOrder.amountCnyCent / 100).toFixed(2)}</span>
                </div>
              </div>
              <div className="flex justify-center rounded-xl border border-border/70 bg-background px-4 py-5">
                {pendingTopupOrder.paymentQrImageDataUrl ? (
                  <img src={pendingTopupOrder.paymentQrImageDataUrl} alt="支付宝付款码" className="h-56 w-56 object-contain" />
                ) : (
                  <div className="flex h-56 w-56 items-center justify-center text-sm text-muted-foreground">未返回二维码</div>
                )}
              </div>
              <div className="flex flex-wrap justify-end gap-2">
                <Button type="button" variant="outline" onClick={() => navigator.clipboard.writeText(pendingTopupOrder.paymentQrUrl).catch(() => undefined)}>
                  <QrCode className="mr-2 h-4 w-4" />
                  复制链接
                </Button>
                <Button type="button" onClick={() => void handleRefreshTopupOrder()} disabled={refreshingTopupOrderNo === pendingTopupOrder.orderNo}>
                  <RefreshCw className={`mr-2 h-4 w-4 ${refreshingTopupOrderNo === pendingTopupOrder.orderNo ? 'animate-spin' : ''}`} />
                  刷新状态
                </Button>
              </div>
            </div>
          ) : null}
        </DialogContent>
      </Dialog>
    </motion.div>
  )
}
