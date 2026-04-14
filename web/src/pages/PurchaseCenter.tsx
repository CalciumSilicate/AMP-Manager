import { useEffect, useState } from 'react'
import { motion, AnimatePresence, staggerContainer, staggerItem } from '@/lib/motion'
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
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { RefreshCw, CircleAlert, CheckCircle2, QrCode, ShoppingCart, ArrowUpRight } from 'lucide-react'

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

function availabilityLabel(catalog: PurchaseCatalogResponse | null): string {
  if (!catalog?.purchaseEnabled) {
    return '已关闭'
  }
  if (catalog.debugAutoPaid) {
    return 'DEBUG 自动支付'
  }
  if (catalog.paymentConfigured) {
    return '支付宝可用'
  }
  return '未配置'
}

function buildCountdown(expiresAt?: string | null): string {
  if (!expiresAt) return '00:00'
  const diff = new Date(expiresAt).getTime() - Date.now()
  const seconds = Math.max(0, Math.ceil(diff / 1000))
  const minutes = Math.floor(seconds / 60)
  const remain = seconds % 60
  return `${String(minutes).padStart(2, '0')}:${String(remain).padStart(2, '0')}`
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
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '加载失败')
    } finally {
      setLoading(false)
      setReloading(false)
    }
  }

  const showMessage = (type: 'success' | 'error', text: string) => {
    setMessage({ type, text })
    window.setTimeout(() => setMessage(null), 3200)
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
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '创建订单失败')
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
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '刷新失败')
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
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="space-y-8">
      <div className="space-y-3 border-b border-border/70 pb-5">
        <div className="flex items-center justify-between gap-4">
          <div>
            <h2 className="text-2xl font-semibold tracking-tight">购买订阅</h2>
            <p className="text-sm text-muted-foreground">当前账号直购，自动续到同套餐。</p>
          </div>
          <Button variant="outline" size="sm" onClick={() => void loadAll(true)} disabled={reloading}>
            <RefreshCw className={`mr-2 h-4 w-4 ${reloading ? 'animate-spin' : ''}`} />
            刷新
          </Button>
        </div>

        <motion.div
          variants={staggerContainer}
          initial="hidden"
          animate="visible"
          className="grid gap-3 md:grid-cols-3"
        >
          {[
            {
              label: '当前订阅',
              value: catalog?.currentSubscription?.planName || '未订阅',
              note: catalog?.currentSubscription?.expiresAt
                ? `到期 ${formatDateTime(catalog.currentSubscription.expiresAt)}`
                : catalog?.currentSubscription
                  ? '永久'
                  : catalog?.renewalRule || '-',
            },
            {
              label: '支付状态',
              value: availabilityLabel(catalog),
              note: catalog?.purchaseEnabled ? '扫码支付' : '管理员关闭',
            },
            {
              label: '待处理订单',
              value: String(pendingOrders.length),
              note: pendingOrders.length > 0 ? '可继续支付' : '无待支付订单',
            },
          ].map((item) => (
            <motion.div
              key={item.label}
              variants={staggerItem}
              className="border border-border/70 bg-background/70 px-4 py-4"
            >
              <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">{item.label}</p>
              <p className="mt-3 text-xl font-semibold tracking-tight">{item.value}</p>
              <p className="mt-1 text-xs text-muted-foreground">{item.note}</p>
            </motion.div>
          ))}
        </motion.div>
      </div>

      <AnimatePresence>
        {message && (
          <motion.div
            initial={{ opacity: 0, y: -10 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -10 }}
          >
            <Alert variant={message.type === 'error' ? 'destructive' : 'default'}>
              {message.type === 'error' ? <CircleAlert className="h-4 w-4" /> : <CheckCircle2 className="h-4 w-4" />}
              <AlertDescription>{message.text}</AlertDescription>
            </Alert>
          </motion.div>
        )}
      </AnimatePresence>

      <motion.section
        initial={{ opacity: 0, y: 18 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ type: 'spring', bounce: 0.18, duration: 0.45 }}
        className="space-y-4"
      >
        <div className="flex items-center justify-between gap-3">
          <div>
            <h3 className="text-sm font-semibold tracking-[0.08em] text-foreground">可购商品</h3>
            <p className="text-xs text-muted-foreground">同套餐续期，不支持跨套餐切换。</p>
          </div>
          {catalog?.currentSubscription && (
            <Button variant="ghost" size="sm" onClick={() => navigateDashboard('overview')}>
              <ArrowUpRight className="mr-2 h-4 w-4" />
              查看当前额度
            </Button>
          )}
        </div>

        {catalog?.products.length ? (
          <motion.div
            variants={staggerContainer}
            initial="hidden"
            animate="visible"
            className="grid gap-3 lg:grid-cols-3"
          >
            {catalog.products.map((product) => (
              <motion.button
                key={product.id}
                type="button"
                variants={staggerItem}
                whileHover={{ y: -4 }}
                whileTap={{ scale: 0.985 }}
                onClick={() => setConfirmProduct(product)}
                disabled={!catalog.purchaseEnabled || !catalog.paymentConfigured}
                className="flex min-h-[184px] flex-col justify-between border border-border/70 bg-background px-5 py-5 text-left transition-colors hover:border-foreground/25 disabled:cursor-not-allowed disabled:opacity-55"
              >
                <div className="space-y-3">
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <p className="text-lg font-semibold tracking-tight">{product.name}</p>
                      <p className="mt-1 text-xs text-muted-foreground">{product.subscriptionPlanName}</p>
                    </div>
                    {product.isRecommended && (
                      <Badge variant="outline" className="rounded-none border-sky-300 bg-sky-50 text-sky-700">
                        推荐
                      </Badge>
                    )}
                  </div>
                  <p className="min-h-[2.75rem] text-sm leading-6 text-muted-foreground">{product.summary || '标准续费档'}</p>
                </div>

                <div className="flex items-end justify-between gap-4 border-t border-border/70 pt-4">
                  <div>
                    <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">时长</p>
                    <p className="mt-1 text-sm font-medium">{product.durationDays} 天</p>
                  </div>
                  <p className="text-3xl font-semibold tracking-tight">{formatCNY(product.priceCnyCent)}</p>
                </div>
              </motion.button>
            ))}
          </motion.div>
        ) : (
          <div className="border border-dashed border-border/70 px-5 py-8 text-sm text-muted-foreground">
            当前没有可购买的商品。
          </div>
        )}
      </motion.section>

      <motion.section
        initial={{ opacity: 0, y: 18 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ type: 'spring', bounce: 0.18, duration: 0.45, delay: 0.06 }}
        className="space-y-4"
      >
        <div className="flex items-center justify-between gap-3">
          <div>
            <h3 className="text-sm font-semibold tracking-[0.08em] text-foreground">我的订单</h3>
            <p className="text-xs text-muted-foreground">只保留当前账号订单。</p>
          </div>
          <div className="text-xs text-muted-foreground">共 {orders.length} 条</div>
        </div>

        <div className="border border-border/70 bg-background">
          {orders.length === 0 ? (
            <div className="px-5 py-10 text-sm text-muted-foreground">暂无订单。</div>
          ) : (
            <div className="divide-y divide-border/70">
              {orders.map((order) => (
                <div key={order.id} className="px-5 py-4">
                  <div className="grid gap-3 lg:grid-cols-[1.6fr_0.8fr_0.8fr_0.9fr_auto] lg:items-center">
                    <div className="space-y-1">
                      <p className="font-medium">{order.productName}</p>
                      <p className="font-mono text-xs text-muted-foreground">{order.orderNo}</p>
                    </div>
                    <div className="text-sm text-muted-foreground">{formatCNY(order.amountCnyCent)}</div>
                    <div className="flex flex-wrap gap-2">
                      <Badge variant="outline" className={`rounded-none ${paymentTone(order.paymentStatus)}`}>
                        {paymentLabel(order.paymentStatus)}
                      </Badge>
                      <Badge variant="outline" className={`rounded-none ${fulfillmentTone(order.fulfillmentStatus)}`}>
                        {fulfillmentLabel(order.fulfillmentStatus)}
                      </Badge>
                    </div>
                    <div className="text-xs text-muted-foreground">
                      {formatDateTime(order.createdAt)}
                    </div>
                    <div className="flex items-center gap-2 justify-self-start lg:justify-self-end">
                      {order.paymentStatus === 'pending' && (
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => setPendingOrder(order)}
                        >
                          <QrCode className="mr-2 h-4 w-4" />
                          支付
                        </Button>
                      )}
                      {order.canRefresh && (
                        <Button
                          size="sm"
                          onClick={() => void handleRefreshOrder(order.orderNo, false)}
                          disabled={refreshingOrderNo === order.orderNo}
                        >
                          <RefreshCw className={`mr-2 h-4 w-4 ${refreshingOrderNo === order.orderNo ? 'animate-spin' : ''}`} />
                          刷新
                        </Button>
                      )}
                    </div>
                  </div>
                  {order.failureReason && (
                    <p className="mt-3 text-xs text-rose-600">{order.failureReason}</p>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      </motion.section>

      <Dialog open={!!confirmProduct} onOpenChange={(open) => !open && setConfirmProduct(null)}>
        <DialogContent className="max-w-md rounded-none border-border/80">
          <DialogHeader>
            <DialogTitle className="text-xl">确认下单</DialogTitle>
            <DialogDescription>支付后自动续到当前账号。</DialogDescription>
          </DialogHeader>
          {confirmProduct && (
            <div className="space-y-4 py-2">
              <div className="border border-border/70 px-4 py-4">
                <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">商品</p>
                <p className="mt-3 text-lg font-semibold">{confirmProduct.name}</p>
                <p className="mt-1 text-sm text-muted-foreground">{confirmProduct.summary || confirmProduct.subscriptionPlanName}</p>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div className="border border-border/70 px-4 py-4">
                  <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">时长</p>
                  <p className="mt-3 text-lg font-semibold">{confirmProduct.durationDays} 天</p>
                </div>
                <div className="border border-border/70 px-4 py-4">
                  <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">金额</p>
                  <p className="mt-3 text-2xl font-semibold">{formatCNY(confirmProduct.priceCnyCent)}</p>
                </div>
              </div>
              <div className="border border-border/70 px-4 py-4 text-sm">
                <div className="flex items-center justify-between">
                  <span className="text-muted-foreground">支付方式</span>
                  <span>{catalog?.debugAutoPaid ? 'DEBUG 自动支付' : '支付宝扫码'}</span>
                </div>
              </div>
            </div>
          )}
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
        <DialogContent className="max-w-md rounded-none border-border/80">
          <DialogHeader>
            <DialogTitle className="text-xl">等待支付</DialogTitle>
            <DialogDescription>{pendingOrder?.productName || '订单'} / {pendingOrder ? formatCNY(pendingOrder.amountCnyCent) : '-'}</DialogDescription>
          </DialogHeader>
          {pendingOrder && (
            <div className="space-y-5 py-2">
              <div className="grid gap-2 border border-border/70 px-4 py-4 text-sm">
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

              <div className="flex justify-center border border-border/70 bg-background px-4 py-5">
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
          )}
        </DialogContent>
      </Dialog>
    </motion.div>
  )
}
