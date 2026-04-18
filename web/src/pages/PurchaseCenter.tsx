import { useEffect, useState } from 'react'

import { navigateDashboard } from '@/lib/dashboard-navigation'
import { formatDateTime } from '@/lib/formatters'
import {
  createPurchaseOrderWithMode,
  getPurchaseCatalog,
  listMyPurchaseOrders,
  quotePurchaseOrder,
  refreshMyPurchaseOrder,
  type PurchaseCatalogResponse,
  type PurchaseOrder,
  type PurchaseProduct,
  type PurchaseQuoteResponse,
  type PurchaseSubscriptionMode,
} from '@/api/purchase'
import { OverflowCopyText } from '@/components/OverflowCopyText'
import { RedeemQuickEntry } from '@/components/redeem/RedeemPanels'
import { MultiSubscriptionSummary } from '@/components/subscriptions/MultiSubscriptionSummary'
import { TablePagination } from '@/components/TablePagination'
import { TabbedSettingsPage, type TabbedSettingsPageTab } from '@/components/layout/TabbedSettingsPage'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { CreditCard, QrCode, RefreshCw, ShoppingCart } from 'lucide-react'

type PurchaseCenterTab = 'subscription' | 'redeem' | 'orders' | 'products'

const tabs: TabbedSettingsPageTab<PurchaseCenterTab>[] = [
  { key: 'products', label: '商店' },
  { key: 'subscription', label: '当前订阅' },
  { key: 'redeem', label: '兑换码' },
  { key: 'orders', label: '我的订单' },
]

function formatCNY(cents: number): string {
  return `¥${(cents / 100).toFixed(2)}`
}

function paymentLabel(status: PurchaseOrder['paymentStatus']): string {
  switch (status) {
    case 'paid':
      return '已支付'
    case 'expired':
      return '已过期'
    case 'closed':
      return '已关闭'
    case 'failed':
      return '失败'
    default:
      return '待支付'
  }
}

function paymentTone(status: PurchaseOrder['paymentStatus']): string {
  switch (status) {
    case 'paid':
      return 'border-emerald-200 bg-emerald-50 text-emerald-700'
    case 'expired':
    case 'closed':
    case 'failed':
      return 'border-rose-200 bg-rose-50 text-rose-700'
    default:
      return 'border-amber-200 bg-amber-50 text-amber-700'
  }
}

function fulfillmentLabel(order: PurchaseOrder): string {
  if (order.orderKind === 'balance_topup') {
    return order.fulfillmentStatus === 'fulfilled' ? '已到账' : order.fulfillmentStatus === 'failed' ? '入账失败' : '待入账'
  }
  const status = order.fulfillmentStatus
  switch (status) {
    case 'fulfilled':
      return '已开通'
    case 'failed':
      return '发放失败'
    default:
      return '待发放'
  }
}

function fulfillmentTone(order: PurchaseOrder): string {
  const status = order.fulfillmentStatus
  switch (status) {
    case 'fulfilled':
      return 'border-sky-200 bg-sky-50 text-sky-700'
    case 'failed':
      return 'border-rose-200 bg-rose-50 text-rose-700'
    default:
      return 'border-slate-200 bg-slate-100 text-slate-600'
  }
}

function buildCountdown(expiresAt?: string | null): string {
  if (!expiresAt) return '00:00'
  const diff = new Date(expiresAt).getTime() - Date.now()
  const seconds = Math.max(0, Math.ceil(diff / 1000))
  const minutes = Math.floor(seconds / 60)
  const remain = seconds % 60
  return `${String(minutes).padStart(2, '0')}:${String(remain).padStart(2, '0')}`
}

function paymentChannelLabel(catalog: PurchaseCatalogResponse | null): string {
  if (!catalog?.purchaseEnabled) return '购买已关闭'
  if (catalog.debugAutoPaid) return 'DEBUG 自动支付'
  if (catalog.paymentConfigured) return '支付宝扫码'
  return '支付待配置'
}

export default function PurchaseCenter() {
  const [activeTab, setActiveTab] = useState<PurchaseCenterTab>('products')
  const [catalog, setCatalog] = useState<PurchaseCatalogResponse | null>(null)
  const [orders, setOrders] = useState<PurchaseOrder[]>([])
  const [loading, setLoading] = useState(true)
  const [reloading, setReloading] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [confirmProduct, setConfirmProduct] = useState<PurchaseProduct | null>(null)
  const [deliveryMode, setDeliveryMode] = useState<'account' | 'redeem_code'>('account')
  const [subscriptionMode, setSubscriptionMode] = useState<PurchaseSubscriptionMode>('new')
  const [targetSubscriptionId, setTargetSubscriptionId] = useState('')
  const [creating, setCreating] = useState(false)
  const [couponCode, setCouponCode] = useState('')
  const [quote, setQuote] = useState<PurchaseQuoteResponse | null>(null)
  const [pendingOrder, setPendingOrder] = useState<PurchaseOrder | null>(null)
  const [refreshingOrderNo, setRefreshingOrderNo] = useState<string | null>(null)
  const [countdown, setCountdown] = useState('00:00')
  const [ordersPage, setOrdersPage] = useState(1)
  const [ordersPageSize, setOrdersPageSize] = useState(10)

  const pendingOrders = orders.filter((item) => item.paymentStatus === 'pending')
  const couponEnabled = Boolean(catalog?.couponEnabled)

  useEffect(() => {
    void loadAll()
  }, [])

  useEffect(() => {
    if (!couponEnabled && couponCode) {
      setCouponCode('')
    }
  }, [couponEnabled, couponCode])

  useEffect(() => {
    if (!pendingOrder?.expiresAt || pendingOrder.paymentStatus !== 'pending') {
      setCountdown('00:00')
      return
    }

    const update = () => setCountdown(buildCountdown(pendingOrder.expiresAt))
    update()
    const timer = window.setInterval(update, 1000)
    return () => window.clearInterval(timer)
  }, [pendingOrder])

  useEffect(() => {
    if (!confirmProduct) {
      setCouponCode('')
      setQuote(null)
      return
    }
    const run = async () => {
      try {
        const nextQuote = await quotePurchaseOrder({
          kind: 'subscription',
          productId: confirmProduct.id,
          deliveryMode,
          couponCode: couponEnabled ? couponCode.trim() : '',
          subscriptionMode,
          targetSubscriptionId,
        })
        setQuote(nextQuote)
      } catch (error) {
        setQuote(null)
        if (couponEnabled && couponCode.trim()) {
          showMessage('error', error instanceof Error ? error.message : '预览失败')
        }
      }
    }
    void run()
  }, [confirmProduct, couponCode, couponEnabled, deliveryMode, subscriptionMode, targetSubscriptionId])

  const showMessage = (type: 'success' | 'error', text: string) => {
    setMessage({ type, text })
    window.setTimeout(() => setMessage(null), 5000)
  }

  const loadAll = async (silent = false) => {
    if (silent) {
      setReloading(true)
    } else {
      setLoading(true)
    }

    try {
      const [catalogData, ordersData] = await Promise.all([
        getPurchaseCatalog(),
        listMyPurchaseOrders(),
      ])
      setCatalog(catalogData)
      setOrders(ordersData.items || [])
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '加载失败')
    } finally {
      setLoading(false)
      setReloading(false)
    }
  }

  const handleCreateOrder = async () => {
    if (!confirmProduct) return
    setCreating(true)
    try {
      const order = await createPurchaseOrderWithMode(confirmProduct.id, deliveryMode, couponEnabled ? couponCode.trim() : '', subscriptionMode, targetSubscriptionId)
      setConfirmProduct(null)
      setDeliveryMode('account')
      setSubscriptionMode('new')
      setTargetSubscriptionId('')
      setCouponCode('')
      setQuote(null)
      await loadAll(true)

      if (order.paymentStatus === 'paid') {
        showMessage('success', order.deliveryMode === 'redeem_code' ? '兑换码已生成' : '订阅已开通')
        setActiveTab(order.deliveryMode === 'redeem_code' ? 'orders' : 'subscription')
      } else {
        setPendingOrder(order)
        setActiveTab('orders')
        showMessage('success', '订单已创建')
      }
    } catch (error) {
      const text = error instanceof Error ? error.message : '创建订单失败'
      showMessage('error', text)
    } finally {
      setCreating(false)
    }
  }

  const handleRefreshOrder = async (orderNo: string, openPending = false) => {
    setRefreshingOrderNo(orderNo)
    try {
      const order = await refreshMyPurchaseOrder(orderNo)
      await loadAll(true)
      if (order.paymentStatus === 'paid') {
        setPendingOrder(null)
        showMessage('success', '支付完成，订阅已更新')
      } else {
        if (openPending || pendingOrder?.orderNo === orderNo) {
          setPendingOrder(order)
        }
        if (order.paymentStatus === 'expired' || order.paymentStatus === 'closed') {
          showMessage('error', '订单已关闭')
        }
      }
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '刷新失败')
    } finally {
      setRefreshingOrderNo(null)
    }
  }

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <RefreshCw className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  const ordersTotalPages = Math.max(1, Math.ceil(orders.length / ordersPageSize))
  const currentOrdersPage = Math.min(ordersPage, ordersTotalPages)
  const visibleOrders = orders.slice((currentOrdersPage - 1) * ordersPageSize, currentOrdersPage * ordersPageSize)
  const groupedProducts = (catalog?.products || []).reduce<Array<{ key: string; groupName: string; items: PurchaseProduct[] }>>((groups, product) => {
    const groupName = product.groupName || '未分组'
    const key = `${product.groupSort}:${groupName}`
    const existing = groups.find((item) => item.key === key)
    if (existing) {
      existing.items.push(product)
      return groups
    }
    groups.push({ key, groupName, items: [product] })
    return groups
  }, [])
  const purchaseHint = (() => {
    if (!confirmProduct) return ''
    return '同套餐可选新建或续期，异套餐总是新增。'
  })()
  const samePlanSubscriptions = (catalog?.subscriptions || []).filter((subscription) => subscription.planId === confirmProduct?.subscriptionPlanId)

  return (
    <>
      <TabbedSettingsPage
        title="购买订阅"
        description="查看订阅、兑换入口、订单和商店"
        tabs={tabs}
        activeTab={activeTab}
        onTabChange={setActiveTab}
        indicatorId="purchase-center-tab-indicator"
        message={message}
      >
        {activeTab === 'subscription' && (
          <Card>
            <CardHeader>
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="space-y-1">
                  <CardTitle>当前订阅</CardTitle>
                  <CardDescription>查看当前套餐和续费规则</CardDescription>
                </div>
                <div className="flex flex-wrap gap-2">
                  {catalog?.currentSubscription ? (
                    <Button type="button" variant="outline" size="sm" onClick={() => navigateDashboard('account-settings')}>
                      <CreditCard className="mr-2 h-4 w-4" />
                      充值额度
                    </Button>
                  ) : null}
                  <Button type="button" variant="outline" size="sm" onClick={() => void loadAll(true)} disabled={reloading}>
                    <RefreshCw className={`mr-2 h-4 w-4 ${reloading ? 'animate-spin' : ''}`} />
                    刷新
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-4 rounded-lg border p-4">
                <div className="flex flex-wrap items-start justify-between gap-3 border-b border-border/70 pb-4">
                  <div className="space-y-1">
                    <div className="text-sm text-muted-foreground">套餐</div>
                    <p className="text-2xl font-semibold tracking-tight">
                      {catalog?.currentSubscription?.planName || '未订阅'}
                    </p>
                    <p className="text-sm text-muted-foreground">
                      {catalog?.currentSubscription?.expiresAt
                        ? `到期 ${formatDateTime(catalog.currentSubscription.expiresAt)}`
                        : catalog?.currentSubscription
                          ? '永久有效'
                          : catalog?.renewalRule || '-'}
                    </p>
                  </div>
                  <Badge variant={catalog?.purchaseEnabled ? 'default' : 'secondary'}>
                    {catalog?.purchaseEnabled ? '可下单' : '已关闭'}
                  </Badge>
                </div>

                <div className="grid gap-3 sm:grid-cols-3">
                  <div className="space-y-1">
                    <div className="text-xs text-muted-foreground">支付</div>
                    <div className="text-sm font-medium">{paymentChannelLabel(catalog)}</div>
                  </div>
                  <div className="space-y-1">
                    <div className="text-xs text-muted-foreground">商品</div>
                    <div className="text-sm font-medium">{catalog?.products.length || 0}</div>
                  </div>
                  <div className="space-y-1">
                    <div className="text-xs text-muted-foreground">订单</div>
                    <div className="text-sm font-medium">{orders.length}</div>
                  </div>
                </div>

                <div className="flex flex-wrap gap-2">
                  {pendingOrders.length > 0 ? (
                    <Badge variant="outline">{pendingOrders.length} 个待支付订单</Badge>
                  ) : null}
                  <Badge variant="outline">{catalog?.renewalRule || '暂无续费规则'}</Badge>
                </div>
              </div>
              <MultiSubscriptionSummary
                currentSubscription={catalog?.currentSubscription}
                subscriptions={catalog?.subscriptions}
                emptyText="当前没有生效中的订阅。"
              />
            </CardContent>
          </Card>
        )}

        {activeTab === 'redeem' && (
          <Card>
            <CardHeader>
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="space-y-1">
                  <CardTitle>兑换码</CardTitle>
                </div>
                <Button type="button" variant="outline" size="sm" onClick={() => void loadAll(true)} disabled={reloading}>
                  <RefreshCw className={`mr-2 h-4 w-4 ${reloading ? 'animate-spin' : ''}`} />
                  刷新
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              <RedeemQuickEntry onSuccess={() => void loadAll(true)} />
            </CardContent>
          </Card>
        )}

        {activeTab === 'orders' && (
          <Card>
            <CardHeader>
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="space-y-1">
                  <CardTitle>我的订单</CardTitle>
                  <CardDescription>保留最近订单和当前仍然有效的支付、刷新动作</CardDescription>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Badge variant="outline">共 {orders.length} 条</Badge>
                  <Button type="button" variant="outline" size="sm" onClick={() => void loadAll(true)} disabled={reloading}>
                    <RefreshCw className={`mr-2 h-4 w-4 ${reloading ? 'animate-spin' : ''}`} />
                    刷新
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent className="px-0 pb-0">
              <div className="overflow-x-auto border-t">
                {orders.length === 0 ? (
                  <div className="px-6 py-10 text-sm text-muted-foreground">暂无订单。</div>
                ) : (
                  <div className="px-6 pb-6">
                    <div className="space-y-3 md:hidden">
                      {visibleOrders.map((order) => (
                        <div key={order.id} className="rounded-xl border border-border/70 px-4 py-4">
                          <div className="flex items-start justify-between gap-3">
                            <div className="min-w-0">
                              <p className="font-mono text-xs text-muted-foreground">{order.orderNo}</p>
                              <p className="mt-1 font-medium">{order.productName}</p>
                              <p className="text-xs text-muted-foreground">{order.subscriptionPlanName}</p>
                            </div>
                            <div className="text-right">
                              <p className="text-lg font-semibold">{formatCNY(order.amountCnyCent)}</p>
                              <p className="text-xs text-muted-foreground">{formatDateTime(order.createdAt)}</p>
                            </div>
                          </div>

                          <div className="mt-3 flex flex-wrap gap-2">
                            <Badge variant="outline" className={paymentTone(order.paymentStatus)}>
                              {paymentLabel(order.paymentStatus)}
                            </Badge>
                            <Badge variant="outline" className={fulfillmentTone(order)}>
                              {fulfillmentLabel(order)}
                            </Badge>
                            <Badge variant="outline">
                              {order.deliveryMode === 'redeem_code' ? '兑换码' : '本账号'}
                            </Badge>
                          </div>

                          <div className="mt-3 flex flex-wrap gap-2">
                            {order.paymentStatus === 'pending' ? (
                              <Button type="button" variant="outline" size="sm" onClick={() => setPendingOrder(order)}>
                                <QrCode className="mr-2 h-4 w-4" />
                                支付
                              </Button>
                            ) : null}
                            {order.canRefresh ? (
                              <Button
                                type="button"
                                size="sm"
                                onClick={() => void handleRefreshOrder(order.orderNo, false)}
                                disabled={refreshingOrderNo === order.orderNo}
                              >
                                <RefreshCw className={`mr-2 h-4 w-4 ${refreshingOrderNo === order.orderNo ? 'animate-spin' : ''}`} />
                                刷新
                              </Button>
                            ) : null}
                          </div>

                          {order.failureReason ? (
                            <p className="mt-3 text-xs text-rose-600">{order.failureReason}</p>
                          ) : null}
                          {order.generatedRedeemCode ? (
                            <div className="mt-3 rounded-lg border border-border/70 px-3 py-2">
                              <p className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">兑换码</p>
                              <p className="mt-1 font-mono text-xs">{order.generatedRedeemCode}</p>
                            </div>
                          ) : null}
                        </div>
                      ))}
                    </div>

                    <Table className="hidden md:table">
                      <TableHeader>
                        <TableRow>
                          <TableHead>订单</TableHead>
                          <TableHead>商品</TableHead>
                          <TableHead>金额</TableHead>
                          <TableHead>支付</TableHead>
                          <TableHead>发放</TableHead>
                          <TableHead>时间</TableHead>
                          <TableHead className="text-right">操作</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {visibleOrders.map((order) => (
                          <TableRow key={order.id}>
                            <TableCell className="max-w-[220px]">
                              <OverflowCopyText
                                text={order.orderNo}
                                copyValue={order.orderNo}
                                tooltipText={order.alipayTradeNo ? `${order.orderNo} · ${order.alipayTradeNo}` : order.orderNo}
                                className="max-w-[220px] font-mono text-xs"
                              />
                            </TableCell>
                            <TableCell className="max-w-[320px]">
                              <OverflowCopyText
                                text={order.productName}
                                copyValue={order.productName}
                                tooltipText={`${order.productName} · ${order.subscriptionPlanName} · ${order.deliveryMode === 'redeem_code' ? '交付：兑换码' : '交付：本账号'}`}
                                className="max-w-[320px] font-medium"
                              />
                            </TableCell>
                            <TableCell>{formatCNY(order.amountCnyCent)}</TableCell>
                            <TableCell>
                              <Badge variant="outline" className={paymentTone(order.paymentStatus)}>
                                {paymentLabel(order.paymentStatus)}
                              </Badge>
                            </TableCell>
                            <TableCell>
                              <Badge variant="outline" className={fulfillmentTone(order)}>
                                {fulfillmentLabel(order)}
                              </Badge>
                            </TableCell>
                            <TableCell className="text-xs text-muted-foreground">{formatDateTime(order.createdAt)}</TableCell>
                            <TableCell className="min-w-[360px] text-right">
                              <div className="flex items-center justify-end gap-2">
                                {order.paymentStatus === 'pending' ? (
                                  <Button type="button" variant="outline" size="sm" onClick={() => setPendingOrder(order)}>
                                    <QrCode className="mr-2 h-4 w-4" />
                                    支付
                                  </Button>
                                ) : null}
                                {order.canRefresh ? (
                                  <Button
                                    type="button"
                                    size="sm"
                                    onClick={() => void handleRefreshOrder(order.orderNo, false)}
                                    disabled={refreshingOrderNo === order.orderNo}
                                  >
                                    <RefreshCw className={`mr-2 h-4 w-4 ${refreshingOrderNo === order.orderNo ? 'animate-spin' : ''}`} />
                                    刷新
                                  </Button>
                                ) : null}
                                {order.failureReason ? (
                                  <OverflowCopyText
                                    text={order.failureReason}
                                    className="max-w-[180px] text-xs text-rose-600"
                                  />
                                ) : null}
                                {order.generatedRedeemCode ? (
                                  <OverflowCopyText
                                    text={order.generatedRedeemCode}
                                    copyValue={order.generatedRedeemCode}
                                    className="max-w-[180px] font-mono text-xs"
                                    tooltipText={`兑换码 · ${order.generatedRedeemCode}`}
                                  />
                                ) : null}
                              </div>
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                    <TablePagination
                      page={currentOrdersPage}
                      pageSize={ordersPageSize}
                      total={orders.length}
                      onPageChange={setOrdersPage}
                      onPageSizeChange={(nextPageSize) => {
                        setOrdersPageSize(nextPageSize)
                        setOrdersPage(1)
                      }}
                    />
                  </div>
                )}
              </div>
            </CardContent>
          </Card>
        )}

        {activeTab === 'products' && (
          <Card>
            <CardHeader>
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="space-y-1">
                  <CardTitle>商店</CardTitle>
                  <CardDescription>选择商品后创建订单</CardDescription>
                </div>
                <div className="flex flex-wrap gap-2">
                  {catalog?.currentSubscription ? (
                    <Button type="button" variant="outline" size="sm" onClick={() => navigateDashboard('account-settings')}>
                      <CreditCard className="mr-2 h-4 w-4" />
                      充值额度
                    </Button>
                  ) : null}
                  <Button type="button" variant="outline" size="sm" onClick={() => void loadAll(true)} disabled={reloading}>
                    <RefreshCw className={`mr-2 h-4 w-4 ${reloading ? 'animate-spin' : ''}`} />
                    刷新
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent>
              {catalog?.products.length ? (
                <div className="space-y-6">
                  {groupedProducts.map((group) => (
                    <section key={group.key} className="space-y-3">
                      <div className="flex flex-col gap-2 border-b border-border/60 pb-2 sm:flex-row sm:items-center sm:justify-between">
                        <h3 className="text-sm font-semibold uppercase tracking-[0.16em] text-muted-foreground">{group.groupName}</h3>
                        <span className="text-xs text-muted-foreground">{group.items.length} 项</span>
                      </div>
                      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                        {group.items.map((product) => (
                          <button
                            key={product.id}
                            type="button"
                            onClick={() => {
                              setConfirmProduct(product)
                              setDeliveryMode('account')
                              setSubscriptionMode('new')
                              setTargetSubscriptionId('')
                              setCouponCode('')
                              setQuote(null)
                            }}
                            disabled={!catalog.purchaseEnabled || !catalog.paymentConfigured}
                            className="group flex min-h-[168px] flex-col justify-between rounded-xl border border-border/80 bg-card/95 px-5 py-5 text-left shadow-sm transition hover:border-foreground/20 disabled:cursor-not-allowed disabled:opacity-55"
                          >
                            <div className="space-y-3">
                              <div className="flex items-start justify-between gap-3">
                                <div>
                                  <p className="text-lg font-semibold tracking-tight">{product.name}</p>
                                  <p className="mt-1 text-sm text-muted-foreground">{product.subscriptionPlanName}</p>
                                </div>
                                {product.isRecommended ? <Badge variant="outline">推荐</Badge> : null}
                              </div>
                              <p className="text-sm text-muted-foreground">{product.summary || '标准续费档'}</p>
                            </div>

                            <div className="flex items-end justify-between gap-4 border-t border-border/70 pt-4">
                              <div>
                                <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">时长</p>
                                <p className="mt-1 text-sm font-medium">{product.durationDays} 天</p>
                              </div>
                              <p className="text-3xl font-semibold tracking-tight">{formatCNY(product.priceCnyCent)}</p>
                            </div>
                          </button>
                        ))}
                      </div>
                    </section>
                  ))}
                </div>
              ) : (
                <div className="rounded-lg border border-dashed px-5 py-8 text-sm text-muted-foreground">
                  当前没有可购买的商品。
                </div>
              )}
            </CardContent>
          </Card>
        )}
      </TabbedSettingsPage>

      <Dialog open={!!confirmProduct} onOpenChange={(open) => !open && setConfirmProduct(null)}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle className="text-xl">确认下单</DialogTitle>
            <DialogDescription>{deliveryMode === 'redeem_code' ? '支付后生成兑换码。' : '支付后直接生效。'}</DialogDescription>
          </DialogHeader>
          {confirmProduct ? (
            <div className="space-y-4 py-2">
              <div className="space-y-1 border-b border-border/70 pb-4">
                <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">商品</p>
                <p className="text-lg font-semibold">{confirmProduct.name}</p>
                <p className="text-sm text-muted-foreground">{confirmProduct.summary || confirmProduct.subscriptionPlanName}</p>
              </div>
              <div className="space-y-3 border-b border-border/70 pb-4">
                <div className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">交付方式</div>
                <RadioGroup value={deliveryMode} onValueChange={(value) => setDeliveryMode(value as 'account' | 'redeem_code')} className="divide-y divide-border/70">
                  <label className={`flex items-start gap-3 py-3 ${deliveryMode === 'account' ? 'text-foreground' : 'text-muted-foreground'}`}>
                    <RadioGroupItem value="account" className="mt-0.5" />
                    <div>
                      <div className="font-medium">直充本账号</div>
                      <div className="text-xs text-muted-foreground">支付成功后直接生效</div>
                    </div>
                  </label>
                  <label className={`flex items-start gap-3 py-3 ${deliveryMode === 'redeem_code' ? 'text-foreground' : 'text-muted-foreground'}`}>
                    <RadioGroupItem value="redeem_code" className="mt-0.5" />
                    <div>
                      <div className="font-medium">拿兑换码</div>
                      <div className="text-xs text-muted-foreground">支付成功后生成单次码</div>
                    </div>
                  </label>
                </RadioGroup>
                {purchaseHint ? <p className="mt-3 text-xs text-muted-foreground">{purchaseHint}</p> : null}
              </div>
              {deliveryMode === 'account' ? (
                <div className="space-y-3 border-b border-border/70 pb-4">
                  <div className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">订阅模式</div>
                  {samePlanSubscriptions.length > 0 ? (
                    <RadioGroup
                      value={subscriptionMode}
                      onValueChange={(value) => {
                        setSubscriptionMode(value as PurchaseSubscriptionMode)
                        if (value !== 'renew') {
                          setTargetSubscriptionId('')
                        }
                      }}
                      className="divide-y divide-border/70"
                    >
                      <label className={`flex items-start gap-3 py-3 ${subscriptionMode === 'new' ? 'text-foreground' : 'text-muted-foreground'}`}>
                        <RadioGroupItem value="new" className="mt-0.5" />
                        <div>
                          <div className="font-medium">新建订阅</div>
                          <div className="text-xs text-muted-foreground">始终新增一条独立订阅</div>
                        </div>
                      </label>
                      <label className={`flex items-start gap-3 py-3 ${subscriptionMode === 'renew' ? 'text-foreground' : 'text-muted-foreground'}`}>
                        <RadioGroupItem value="renew" className="mt-0.5" />
                        <div>
                          <div className="font-medium">续期已有订阅</div>
                          <div className="text-xs text-muted-foreground">仅同套餐有效订阅可选</div>
                        </div>
                      </label>
                    </RadioGroup>
                  ) : (
                    <div className="rounded-lg border border-dashed px-3 py-4 text-xs text-muted-foreground">当前没有可续期的同套餐有效订阅，将按新建处理。</div>
                  )}
                  {subscriptionMode === 'renew' && samePlanSubscriptions.length > 0 ? (
                    <select
                      value={targetSubscriptionId}
                      onChange={(event) => setTargetSubscriptionId(event.target.value)}
                      className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                    >
                      <option value="">选择目标订阅</option>
                      {samePlanSubscriptions.map((subscription) => (
                        <option key={subscription.id} value={subscription.id}>
                          {subscription.planName} / {subscription.expiresAt ? `到期 ${formatDateTime(subscription.expiresAt)}` : '永久'}
                        </option>
                      ))}
                    </select>
                  ) : null}
                </div>
              ) : null}
              {couponEnabled ? (
                <div className="space-y-3 border-b border-border/70 pb-4">
                  <div className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">优惠码</div>
                  <input
                    value={couponCode}
                    onChange={(event) => setCouponCode(event.target.value.toUpperCase())}
                    placeholder="可选填写"
                    className="w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-sm"
                  />
                  {quote ? (
                    <div className="rounded-lg border px-3 py-3 text-sm">
                      <div className="flex items-center justify-between gap-4">
                        <span className="text-muted-foreground">原价</span>
                        <span>{formatCNY(quote.originalAmountCnyCent)}</span>
                      </div>
                      <div className="mt-2 flex items-center justify-between gap-4">
                        <span className="text-muted-foreground">优惠</span>
                        <span>-{formatCNY(quote.discountCnyCent)}</span>
                      </div>
                      <div className="mt-2 flex items-center justify-between gap-4 font-semibold">
                        <span>应付</span>
                        <span>{formatCNY(quote.finalAmountCnyCent)}</span>
                      </div>
                    </div>
                  ) : null}
                </div>
              ) : null}
              <div className="space-y-3 text-sm">
                <div className="flex items-center justify-between gap-4">
                  <span className="text-muted-foreground">时长</span>
                  <span className="font-medium">{confirmProduct.durationDays} 天</span>
                </div>
                <div className="flex items-center justify-between gap-4">
                  <span className="text-muted-foreground">金额</span>
                  <span className="text-lg font-semibold">{formatCNY(quote?.finalAmountCnyCent ?? confirmProduct.priceCnyCent)}</span>
                </div>
              </div>
            </div>
          ) : null}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setConfirmProduct(null)}>取消</Button>
            <Button type="button" onClick={handleCreateOrder} disabled={creating || !catalog?.purchaseEnabled || !catalog?.paymentConfigured || (deliveryMode === 'account' && subscriptionMode === 'renew' && !targetSubscriptionId)}>
              {creating ? <RefreshCw className="mr-2 h-4 w-4 animate-spin" /> : <ShoppingCart className="mr-2 h-4 w-4" />}
              提交
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!pendingOrder} onOpenChange={(open) => !open && setPendingOrder(null)}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle className="text-xl">等待支付</DialogTitle>
            <DialogDescription>{pendingOrder?.productName || '订单'} / {pendingOrder ? formatCNY(pendingOrder.amountCnyCent) : '-'}</DialogDescription>
          </DialogHeader>
          {pendingOrder ? (
            <div className="space-y-5 py-2">
              <div className="grid gap-2 rounded-xl border border-border/70 px-4 py-4 text-sm">
                <div className="flex items-center justify-between gap-4">
                  <span className="text-muted-foreground">订单号</span>
                  <span className="font-mono text-xs">{pendingOrder.orderNo}</span>
                </div>
                <div className="flex items-center justify-between gap-4">
                  <span className="text-muted-foreground">金额</span>
                  <span className="font-medium">{formatCNY(pendingOrder.amountCnyCent)}</span>
                </div>
                <div className="flex items-center justify-between gap-4">
                  <span className="text-muted-foreground">剩余时间</span>
                  <span className="font-medium">{countdown}</span>
                </div>
              </div>

              <div className="flex justify-center rounded-xl border border-border/70 bg-background px-4 py-5">
                {pendingOrder.paymentQrImageDataUrl ? (
                  <img src={pendingOrder.paymentQrImageDataUrl} alt="支付宝付款码" className="h-56 w-56 object-contain" />
                ) : (
                  <div className="flex h-56 w-56 items-center justify-center text-sm text-muted-foreground">
                    未返回二维码
                  </div>
                )}
              </div>

              <div className="flex flex-wrap justify-between gap-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => {
                    if (pendingOrder.paymentQrUrl) {
                      navigator.clipboard.writeText(pendingOrder.paymentQrUrl).catch(() => undefined)
                    }
                  }}
                >
                  复制链接
                </Button>
                <Button
                  type="button"
                  onClick={() => void handleRefreshOrder(pendingOrder.orderNo, true)}
                  disabled={refreshingOrderNo === pendingOrder.orderNo}
                >
                  <RefreshCw className={`mr-2 h-4 w-4 ${refreshingOrderNo === pendingOrder.orderNo ? 'animate-spin' : ''}`} />
                  刷新状态
                </Button>
              </div>
            </div>
          ) : null}
        </DialogContent>
      </Dialog>
    </>
  )
}
