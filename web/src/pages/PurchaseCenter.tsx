import { useEffect, useState } from 'react'

import { motion } from '@/lib/motion'
import { navigateDashboard } from '@/lib/dashboard-navigation'
import { formatDateTime } from '@/lib/formatters'
import {
  createPurchaseOrder,
  getPurchaseCatalog,
  listMyPurchaseOrders,
  refreshMyPurchaseOrder,
  type PurchaseCatalogResponse,
  type PurchaseOrder,
  type PurchaseProduct,
} from '@/api/purchase'
import { RedeemQuickEntry } from '@/components/redeem/RedeemPanels'
import { PageShell, PageStat, PageStatStrip, PageSurface } from '@/components/layout/PageScaffold'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { CheckCircle2, CircleAlert, CreditCard, QrCode, RefreshCw, ShoppingCart } from 'lucide-react'

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

function fulfillmentLabel(status: PurchaseOrder['fulfillmentStatus']): string {
  switch (status) {
    case 'fulfilled':
      return '已开通'
    case 'failed':
      return '发放失败'
    default:
      return '待发放'
  }
}

function fulfillmentTone(status: PurchaseOrder['fulfillmentStatus']): string {
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
  const [catalog, setCatalog] = useState<PurchaseCatalogResponse | null>(null)
  const [orders, setOrders] = useState<PurchaseOrder[]>([])
  const [loading, setLoading] = useState(true)
  const [reloading, setReloading] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [confirmProduct, setConfirmProduct] = useState<PurchaseProduct | null>(null)
  const [creating, setCreating] = useState(false)
  const [pendingOrder, setPendingOrder] = useState<PurchaseOrder | null>(null)
  const [refreshingOrderNo, setRefreshingOrderNo] = useState<string | null>(null)
  const [countdown, setCountdown] = useState('00:00')

  const pendingOrders = orders.filter((item) => item.paymentStatus === 'pending')

  useEffect(() => {
    void loadAll()
  }, [])

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

  const showMessage = (type: 'success' | 'error', text: string) => {
    setMessage({ type, text })
    window.setTimeout(() => setMessage(null), 3200)
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
      const order = await createPurchaseOrder(confirmProduct.id)
      setConfirmProduct(null)
      await loadAll(true)

      if (order.paymentStatus === 'paid') {
        showMessage('success', '订阅已开通')
      } else {
        setPendingOrder(order)
        showMessage('success', '订单已创建')
      }
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '创建订单失败')
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

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
      <PageShell
        title="购买订阅"
        description="把当前订阅、兑换入口、商品清单和订单状态放进同一条自助购买路径。"
        width="7xl"
        actions={(
          <Button variant="outline" size="sm" onClick={() => void loadAll(true)} disabled={reloading}>
            <RefreshCw className={`mr-2 h-4 w-4 ${reloading ? 'animate-spin' : ''}`} />
            刷新
          </Button>
        )}
      >
        <PageStatStrip>
          <PageStat label="支付方式" value={paymentChannelLabel(catalog)} note={catalog?.paymentConfigured ? '扫码即可下单' : '等待配置'} />
          <PageStat label="当前套餐" value={catalog?.currentSubscription?.planName || '未订阅'} note={catalog?.currentSubscription?.expiresAt ? `到期 ${formatDateTime(catalog.currentSubscription.expiresAt)}` : catalog?.renewalRule || '-'} />
          <PageStat label="可购商品" value={catalog?.products.length || 0} note={catalog?.purchaseEnabled ? '可立即下单' : '购买已关闭'} />
          <PageStat label="待支付订单" value={pendingOrders.length} note={`${orders.length} 条最近订单`} />
        </PageStatStrip>

        {message && (
          <Alert variant={message.type === 'error' ? 'destructive' : 'default'}>
            {message.type === 'error' ? <CircleAlert className="h-4 w-4" /> : <CheckCircle2 className="h-4 w-4" />}
            <AlertDescription>{message.text}</AlertDescription>
          </Alert>
        )}

        <div className="grid gap-5 xl:grid-cols-[1.1fr_0.9fr]">
          <PageSurface
            title="当前订阅"
            description="续费、开通和支付方式都沿用一条支付链路。"
            actions={(
              <Badge variant="outline" className="self-start sm:self-center">
                {paymentChannelLabel(catalog)}
              </Badge>
            )}
          >
            <div className="grid gap-4 lg:grid-cols-[1.3fr_0.7fr]">
            <div className="space-y-3">
              <div>
                <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">套餐</p>
                <p className="mt-2 text-2xl font-semibold tracking-tight">
                  {catalog?.currentSubscription?.planName || '未订阅'}
                </p>
                <p className="mt-1 text-sm text-muted-foreground">
                  {catalog?.currentSubscription?.expiresAt
                    ? `到期 ${formatDateTime(catalog.currentSubscription.expiresAt)}`
                    : catalog?.currentSubscription
                      ? '永久有效'
                      : catalog?.renewalRule || '-'}
                </p>
              </div>
              <div className="flex flex-wrap gap-2">
                <Badge variant={catalog?.purchaseEnabled ? 'default' : 'secondary'}>
                  {catalog?.purchaseEnabled ? '可下单' : '已关闭'}
                </Badge>
                {pendingOrders.length > 0 ? (
                  <Badge variant="outline">{pendingOrders.length} 个待支付订单</Badge>
                ) : null}
              </div>
            </div>
            <div className="grid gap-3 rounded-xl border border-border/70 bg-background/70 px-4 py-4 text-sm">
              <div className="flex items-center justify-between gap-3">
                <span className="text-muted-foreground">支付方式</span>
                <span className="font-medium">{paymentChannelLabel(catalog)}</span>
              </div>
              <div className="flex items-center justify-between gap-3">
                <span className="text-muted-foreground">商品数</span>
                <span className="font-medium">{catalog?.products.length || 0}</span>
              </div>
              <div className="flex items-center justify-between gap-3">
                <span className="text-muted-foreground">续费规则</span>
                <span className="text-right font-medium">{catalog?.renewalRule || '-'}</span>
              </div>
            </div>
            </div>
          </PageSurface>

          <PageSurface
            title="兑换码"
            description="把兑换入口收成一个独立操作主体，避免和购买动作混淆。"
          >
            <RedeemQuickEntry onSuccess={() => loadAll(true)} />
          </PageSurface>
        </div>

        <PageSurface
          title="可购商品"
          description="展示当前可下单的商品结构，卡片负责选择，页头动作负责跳回额度页。"
          actions={catalog?.currentSubscription ? (
            <Button variant="ghost" size="sm" onClick={() => navigateDashboard('overview')}>
              <CreditCard className="mr-2 h-4 w-4" />
              查看当前额度
            </Button>
          ) : null}
        >
          {catalog?.products.length ? (
          <div className="grid gap-4 xl:grid-cols-3">
            {catalog.products.map((product) => (
              <button
                key={product.id}
                type="button"
                onClick={() => setConfirmProduct(product)}
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
          ) : (
            <div className="rounded-xl border border-dashed border-border/70 px-5 py-8 text-sm text-muted-foreground">
              当前没有可购买的商品。
            </div>
          )}
        </PageSurface>

        <PageSurface
          title="我的订单"
          description="保留最近订单和支付动作，操作区只放当前仍然有效的下一步。"
          actions={<div className="text-xs text-muted-foreground">共 {orders.length} 条</div>}
          bodyClassName="px-0 py-0"
        >
          <div className="ops-table-shell rounded-none border-x-0 border-b-0">
          {orders.length === 0 ? (
            <div className="px-5 py-10 text-sm text-muted-foreground">暂无订单。</div>
          ) : (
            <Table>
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
                {orders.map((order) => (
                  <TableRow key={order.id}>
                    <TableCell>
                      <div>
                        <p className="font-mono text-xs">{order.orderNo}</p>
                        {order.alipayTradeNo ? (
                          <p className="mt-1 text-xs text-muted-foreground">{order.alipayTradeNo}</p>
                        ) : null}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div>
                        <p className="font-medium">{order.productName}</p>
                        <p className="text-xs text-muted-foreground">{order.subscriptionPlanName}</p>
                      </div>
                    </TableCell>
                    <TableCell>{formatCNY(order.amountCnyCent)}</TableCell>
                    <TableCell>
                      <Badge variant="outline" className={paymentTone(order.paymentStatus)}>
                        {paymentLabel(order.paymentStatus)}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline" className={fulfillmentTone(order.fulfillmentStatus)}>
                        {fulfillmentLabel(order.fulfillmentStatus)}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">{formatDateTime(order.createdAt)}</TableCell>
                    <TableCell className="text-right">
                      <div className="flex flex-wrap justify-end gap-2">
                        {order.paymentStatus === 'pending' ? (
                          <Button variant="outline" size="sm" onClick={() => setPendingOrder(order)}>
                            <QrCode className="mr-2 h-4 w-4" />
                            支付
                          </Button>
                        ) : null}
                        {order.canRefresh ? (
                          <Button
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
                        <p className="mt-2 text-xs text-rose-600">{order.failureReason}</p>
                      ) : null}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
          </div>
        </PageSurface>

        <Dialog open={!!confirmProduct} onOpenChange={(open) => !open && setConfirmProduct(null)}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle className="text-xl">确认下单</DialogTitle>
            <DialogDescription>支付后自动续到当前账号。</DialogDescription>
          </DialogHeader>
          {confirmProduct ? (
            <div className="space-y-4 py-2">
              <div className="rounded-xl border border-border/70 px-4 py-4">
                <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">商品</p>
                <p className="mt-3 text-lg font-semibold">{confirmProduct.name}</p>
                <p className="mt-1 text-sm text-muted-foreground">{confirmProduct.summary || confirmProduct.subscriptionPlanName}</p>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div className="rounded-xl border border-border/70 px-4 py-4">
                  <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">时长</p>
                  <p className="mt-3 text-lg font-semibold">{confirmProduct.durationDays} 天</p>
                </div>
                <div className="rounded-xl border border-border/70 px-4 py-4">
                  <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">金额</p>
                  <p className="mt-3 text-2xl font-semibold">{formatCNY(confirmProduct.priceCnyCent)}</p>
                </div>
              </div>
            </div>
          ) : null}
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmProduct(null)}>取消</Button>
            <Button onClick={handleCreateOrder} disabled={creating || !catalog?.purchaseEnabled || !catalog?.paymentConfigured}>
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
      </PageShell>
    </motion.div>
  )
}
