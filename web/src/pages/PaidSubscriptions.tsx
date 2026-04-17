import { useEffect, useMemo, useState } from 'react'
import { formatDateTime } from '@/lib/formatters'
import {
  createBatchPurchaseManualSettlement,
  createPurchaseProduct,
  createPurchaseWebhookTarget,
  createSinglePurchaseManualSettlement,
  deletePurchaseProduct,
  deletePurchaseWebhookTarget,
  getPurchaseSettings,
  listPurchaseOrderPaymentStatusHistory,
  listPurchaseOrdersAdmin,
  listPurchaseProductsAdmin,
  listPurchaseWebhookTargets,
  previewBatchPurchaseManualSettlement,
  refreshPurchaseOrderAdmin,
  setPurchaseProductEnabled,
  setPurchaseWebhookTargetEnabled,
  testPurchaseWebhookTarget,
  updatePurchaseOrderPaymentStatus,
  updatePurchaseProduct,
  updatePurchaseSettings,
  updatePurchaseWebhookTarget,
  type AdminOrderFilters,
  type AlipayEnvironment,
  type PurchaseManualSettlementBatch,
  type PurchaseManualSettlementPreviewResponse,
  type PurchaseOrder,
  type PurchaseOrderPaymentStatusHistory,
  type PurchaseProduct,
  type PurchaseProductRequest,
  type PurchaseSettingsResponse,
  type PurchaseWebhookTarget,
  type PurchaseWebhookTargetRequest,
  type PurchaseWebhookTestResponse,
} from '@/api/purchase'
import { getPlans, type SubscriptionPlanResponse } from '@/api/subscription'
import { TablePagination } from '@/components/TablePagination'
import { TabbedSettingsPage, type TabbedSettingsPageTab } from '@/components/layout/TabbedSettingsPage'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { CreditCard, Pencil, Plus, RefreshCw, Send, Trash2 } from 'lucide-react'

type PaidSubscriptionsTab = 'products' | 'orders' | 'webhooks' | 'settings'

const DEFAULT_ALIPAY_NOTIFY_URL = 'https://amphk.asxs.top/api/public/purchase/alipay/notify'

const tabs: TabbedSettingsPageTab<PaidSubscriptionsTab>[] = [
  { key: 'products', label: '售卖商品' },
  { key: 'orders', label: '订单' },
  { key: 'webhooks', label: '订单通知' },
  { key: 'settings', label: '支付设置' },
]

function formatCNY(cents: number): string {
  return `¥${(cents / 100).toFixed(2)}`
}

function toDatetimeLocal(value?: string | null): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const offset = date.getTimezoneOffset()
  const local = new Date(date.getTime() - offset * 60_000)
  return local.toISOString().slice(0, 16)
}

function normalizeDatetimeLocal(value: string): string | undefined {
  if (!value) return undefined
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return undefined
  return date.toISOString()
}

function defaultBatchSettlementRange() {
  const now = new Date()
  const from = new Date(now)
  from.setHours(0, 0, 0, 0)
  return {
    paidFrom: toDatetimeLocal(from.toISOString()),
    paidTo: toDatetimeLocal(now.toISOString()),
  }
}

function paymentLabel(status: PurchaseOrder['paymentStatus']): string {
  switch (status) {
    case 'paid':
      return '已支付'
    case 'expired':
      return '已过期'
    case 'refunded':
      return '已退款'
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
    case 'refunded':
      return 'border-sky-200 bg-sky-50 text-sky-700'
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
  return order.fulfillmentStatus === 'fulfilled' ? '已开通' : order.fulfillmentStatus === 'failed' ? '失败' : '待发放'
}

function initialProductForm(): PurchaseProductRequest {
  return {
    name: '',
    summary: '',
    subscriptionPlanId: '',
    durationDays: 30,
    priceCnyCent: 0,
    groupName: '',
    groupSort: 0,
    isRecommended: false,
    sortOrder: 0,
    enabled: true,
  }
}

function initialWebhookForm(): PurchaseWebhookTargetRequest {
  return {
    name: '',
    targetUrl: '',
    bodyTemplate: '{"orderNo":"{{ orderNo }}","subscriptionName":"{{ subscriptionName }}","amountCny":"{{ amountCny }}"}',
    headersTemplate: '{"X-Order-No":"{{ orderNo }}"}',
    enabled: true,
  }
}

export default function PaidSubscriptions() {
  const [activeTab, setActiveTab] = useState<PaidSubscriptionsTab>('products')
  const [settings, setSettings] = useState<PurchaseSettingsResponse | null>(null)
  const [products, setProducts] = useState<PurchaseProduct[]>([])
  const [orders, setOrders] = useState<PurchaseOrder[]>([])
  const [ordersTotal, setOrdersTotal] = useState(0)
  const [webhooks, setWebhooks] = useState<PurchaseWebhookTarget[]>([])
  const [plans, setPlans] = useState<SubscriptionPlanResponse[]>([])
  const [loading, setLoading] = useState(true)
  const [ordersLoading, setOrdersLoading] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const [settingsDraft, setSettingsDraft] = useState({
    purchaseEnabled: false,
    debugAutoPaid: false,
    alipayAppId: '',
    alipayPid: '',
    alipayEnvironment: 'sandbox' as AlipayEnvironment,
    alipayNotifyUrl: DEFAULT_ALIPAY_NOTIFY_URL,
    alipayPublicKey: '',
  })
  const [alipayPrivateKeyInput, setAlipayPrivateKeyInput] = useState('')
  const [savingSettings, setSavingSettings] = useState(false)

  const [productDialogOpen, setProductDialogOpen] = useState(false)
  const [editingProduct, setEditingProduct] = useState<PurchaseProduct | null>(null)
  const [productForm, setProductForm] = useState<PurchaseProductRequest>(initialProductForm())
  const [productPriceInput, setProductPriceInput] = useState('0.00')
  const [savingProduct, setSavingProduct] = useState(false)

  const [webhookDialogOpen, setWebhookDialogOpen] = useState(false)
  const [editingWebhook, setEditingWebhook] = useState<PurchaseWebhookTarget | null>(null)
  const [webhookForm, setWebhookForm] = useState<PurchaseWebhookTargetRequest>(initialWebhookForm())
  const [savingWebhook, setSavingWebhook] = useState(false)
  const [testingWebhookID, setTestingWebhookID] = useState<string | null>(null)
  const [testResult, setTestResult] = useState<PurchaseWebhookTestResponse | null>(null)

  const [orderFilters, setOrderFilters] = useState({
    paymentStatus: 'all',
    fulfillmentStatus: 'all',
    username: '',
    productId: 'all',
  })
  const [refreshingOrderNo, setRefreshingOrderNo] = useState<string | null>(null)
  const [statusDialogOrder, setStatusDialogOrder] = useState<PurchaseOrder | null>(null)
  const [statusDraft, setStatusDraft] = useState<'paid' | 'expired' | 'refunded'>('paid')
  const [statusNote, setStatusNote] = useState('')
  const [statusHistory, setStatusHistory] = useState<PurchaseOrderPaymentStatusHistory[]>([])
  const [statusSaving, setStatusSaving] = useState(false)
  const [historyLoading, setHistoryLoading] = useState(false)
  const [settlementDialogMode, setSettlementDialogMode] = useState<'single' | 'batch' | null>(null)
  const [settlementOrder, setSettlementOrder] = useState<PurchaseOrder | null>(null)
  const [settlementNote, setSettlementNote] = useState('')
  const [batchSettlementRange, setBatchSettlementRange] = useState(defaultBatchSettlementRange)
  const [batchSettlementPreview, setBatchSettlementPreview] = useState<PurchaseManualSettlementPreviewResponse | null>(null)
  const [batchPreviewLoading, setBatchPreviewLoading] = useState(false)
  const [batchDebugSettlement, setBatchDebugSettlement] = useState(false)
  const [settlementSaving, setSettlementSaving] = useState(false)

  const [productsPage, setProductsPage] = useState(1)
  const [productsPageSize, setProductsPageSize] = useState(10)
  const [ordersPage, setOrdersPage] = useState(1)
  const [ordersPageSize, setOrdersPageSize] = useState(20)

  useEffect(() => {
    void loadBase()
  }, [])

  const loadBase = async () => {
    setLoading(true)
    try {
      const [settingsData, productData, planData, webhookData] = await Promise.all([
        getPurchaseSettings(),
        listPurchaseProductsAdmin(),
        getPlans(),
        listPurchaseWebhookTargets(),
      ])
      setSettings(settingsData)
      setProducts(productData)
      setPlans(planData)
      setWebhooks(webhookData)
      setSettingsDraft({
        purchaseEnabled: settingsData.purchaseEnabled,
        debugAutoPaid: settingsData.debugAutoPaid,
        alipayAppId: settingsData.alipayAppId,
        alipayPid: settingsData.alipayPid,
        alipayEnvironment: settingsData.alipayEnvironment,
        alipayNotifyUrl: settingsData.alipayNotifyUrl || DEFAULT_ALIPAY_NOTIFY_URL,
        alipayPublicKey: settingsData.alipayPublicKey,
      })
      await loadOrders({
        paymentStatus: '',
        fulfillmentStatus: '',
        username: '',
        productId: '',
        page: 1,
        pageSize: ordersPageSize,
      })
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }

  const loadOrders = async (filters?: AdminOrderFilters) => {
    setOrdersLoading(true)
    try {
      const response = await listPurchaseOrdersAdmin(filters)
      setOrders(response.items || [])
      setOrdersTotal(response.total || 0)
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '加载订单失败')
    } finally {
      setOrdersLoading(false)
    }
  }

  const buildOrderFilters = (page: number, pageSize: number): AdminOrderFilters => ({
    paymentStatus: orderFilters.paymentStatus === 'all' ? '' : orderFilters.paymentStatus as PurchaseOrder['paymentStatus'],
    fulfillmentStatus: orderFilters.fulfillmentStatus === 'all' ? '' : orderFilters.fulfillmentStatus as PurchaseOrder['fulfillmentStatus'],
    username: orderFilters.username.trim(),
    productId: orderFilters.productId === 'all' ? '' : orderFilters.productId,
    page,
    pageSize,
  })

  const refreshWebhooks = async () => {
    try {
      setWebhooks(await listPurchaseWebhookTargets())
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '加载通知目标失败')
    }
  }

  const showMessage = (type: 'success' | 'error', text: string) => {
    setMessage({ type, text })
    window.setTimeout(() => setMessage(null), 4000)
  }

  const openCreateProduct = () => {
    setEditingProduct(null)
    setProductForm({
      ...initialProductForm(),
      subscriptionPlanId: plans.find((plan) => plan.enabled)?.id || plans[0]?.id || '',
    })
    setProductPriceInput('0.00')
    setProductDialogOpen(true)
  }

  const openEditProduct = (product: PurchaseProduct) => {
    setEditingProduct(product)
    setProductForm({
      name: product.name,
      summary: product.summary,
      subscriptionPlanId: product.subscriptionPlanId,
      durationDays: product.durationDays,
      priceCnyCent: product.priceCnyCent,
      groupName: product.groupName,
      groupSort: product.groupSort,
      isRecommended: product.isRecommended,
      sortOrder: product.sortOrder,
      enabled: product.enabled,
    })
    setProductPriceInput((product.priceCnyCent / 100).toFixed(2))
    setProductDialogOpen(true)
  }

  const handleSaveProduct = async () => {
    const priceYuan = Number.parseFloat(productPriceInput)
    if (!productForm.subscriptionPlanId || Number.isNaN(priceYuan) || priceYuan <= 0) {
      showMessage('error', '商品参数无效')
      return
    }
    setSavingProduct(true)
    try {
      const payload = { ...productForm, priceCnyCent: Math.round(priceYuan * 100) }
      if (editingProduct) {
        await updatePurchaseProduct(editingProduct.id, payload)
      } else {
        await createPurchaseProduct(payload)
      }
      setProducts(await listPurchaseProductsAdmin())
      setProductDialogOpen(false)
      showMessage('success', editingProduct ? '商品已更新' : '商品已创建')
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '保存失败')
    } finally {
      setSavingProduct(false)
    }
  }

  const handleDeleteProduct = async (product: PurchaseProduct) => {
    if (!window.confirm(`确定删除 ${product.name}？`)) return
    try {
      await deletePurchaseProduct(product.id)
      setProducts((current) => current.filter((item) => item.id !== product.id))
      showMessage('success', '商品已删除')
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '删除失败')
    }
  }

  const handleToggleProduct = async (product: PurchaseProduct) => {
    try {
      await setPurchaseProductEnabled(product.id, !product.enabled)
      setProducts((current) => current.map((item) => item.id === product.id ? { ...item, enabled: !item.enabled } : item))
      showMessage('success', product.enabled ? '商品已下架' : '商品已上架')
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '操作失败')
    }
  }

  const openCreateWebhook = () => {
    setEditingWebhook(null)
    setWebhookForm(initialWebhookForm())
    setTestResult(null)
    setWebhookDialogOpen(true)
  }

  const openEditWebhook = (item: PurchaseWebhookTarget) => {
    setEditingWebhook(item)
    setWebhookForm({
      name: item.name,
      targetUrl: item.targetUrl,
      bodyTemplate: item.bodyTemplate,
      headersTemplate: item.headersTemplate,
      enabled: item.enabled,
    })
    setTestResult(null)
    setWebhookDialogOpen(true)
  }

  const handleSaveWebhook = async () => {
    setSavingWebhook(true)
    try {
      if (editingWebhook) {
        await updatePurchaseWebhookTarget(editingWebhook.id, webhookForm)
      } else {
        await createPurchaseWebhookTarget(webhookForm)
      }
      setWebhooks(await listPurchaseWebhookTargets())
      setWebhookDialogOpen(false)
      showMessage('success', editingWebhook ? '通知目标已更新' : '通知目标已创建')
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '保存失败')
    } finally {
      setSavingWebhook(false)
    }
  }

  const handleDeleteWebhook = async (item: PurchaseWebhookTarget) => {
    if (!window.confirm(`确定删除 ${item.name}？`)) return
    try {
      await deletePurchaseWebhookTarget(item.id)
      setWebhooks((current) => current.filter((target) => target.id !== item.id))
      showMessage('success', '通知目标已删除')
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '删除失败')
    }
  }

  const handleToggleWebhook = async (item: PurchaseWebhookTarget) => {
    try {
      await setPurchaseWebhookTargetEnabled(item.id, !item.enabled)
      setWebhooks((current) => current.map((target) => target.id === item.id ? { ...target, enabled: !target.enabled } : target))
      showMessage('success', item.enabled ? '已停用' : '已启用')
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '操作失败')
    }
  }

  const handleTestWebhook = async (item: PurchaseWebhookTarget) => {
    setTestingWebhookID(item.id)
    setTestResult(null)
    try {
      setTestResult(await testPurchaseWebhookTarget(item.id))
      showMessage('success', '测试完成')
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '测试失败')
    } finally {
      setTestingWebhookID(null)
    }
  }

  const handleSaveSettings = async () => {
    setSavingSettings(true)
    try {
      const response = await updatePurchaseSettings({
        ...settingsDraft,
        alipayPrivateKey: alipayPrivateKeyInput.trim() || undefined,
      })
      setSettings(response.settings)
      setSettingsDraft({
        purchaseEnabled: response.settings.purchaseEnabled,
        debugAutoPaid: response.settings.debugAutoPaid,
        alipayAppId: response.settings.alipayAppId,
        alipayPid: response.settings.alipayPid,
        alipayEnvironment: response.settings.alipayEnvironment,
        alipayNotifyUrl: response.settings.alipayNotifyUrl || DEFAULT_ALIPAY_NOTIFY_URL,
        alipayPublicKey: response.settings.alipayPublicKey,
      })
      setAlipayPrivateKeyInput('')
      showMessage('success', '支付设置已保存')
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '保存失败')
    } finally {
      setSavingSettings(false)
    }
  }

  const handleSearchOrders = async () => {
    setOrdersPage(1)
    await loadOrders(buildOrderFilters(1, ordersPageSize))
  }

  const handleRefreshOrder = async (orderNo: string) => {
    setRefreshingOrderNo(orderNo)
    try {
      await refreshPurchaseOrderAdmin(orderNo)
      await loadOrders(buildOrderFilters(ordersPage, ordersPageSize))
      showMessage('success', '订单状态已刷新')
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '刷新失败')
    } finally {
      setRefreshingOrderNo(null)
    }
  }

  const openStatusDialog = async (order: PurchaseOrder) => {
    setStatusDialogOrder(order)
    setStatusDraft((order.paymentStatus === 'paid' || order.paymentStatus === 'expired' || order.paymentStatus === 'refunded') ? order.paymentStatus : 'paid')
    setStatusNote('')
    setHistoryLoading(true)
    try {
      setStatusHistory(await listPurchaseOrderPaymentStatusHistory(order.orderNo))
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '加载历史失败')
      setStatusHistory([])
    } finally {
      setHistoryLoading(false)
    }
  }

  const handleSaveStatus = async () => {
    if (!statusDialogOrder) return
    setStatusSaving(true)
    try {
      await updatePurchaseOrderPaymentStatus(statusDialogOrder.orderNo, statusDraft, statusNote)
      setStatusDialogOrder(null)
      await loadOrders(buildOrderFilters(ordersPage, ordersPageSize))
      showMessage('success', '订单状态已更新')
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '保存失败')
    } finally {
      setStatusSaving(false)
    }
  }

  const openSingleSettlementDialog = (order: PurchaseOrder) => {
    setSettlementDialogMode('single')
    setSettlementOrder(order)
    setSettlementNote('')
  }

  const openBatchSettlementDialog = () => {
    setSettlementDialogMode('batch')
    setSettlementOrder(null)
    setSettlementNote('')
    setBatchSettlementRange(defaultBatchSettlementRange())
    setBatchSettlementPreview(null)
    setBatchDebugSettlement(false)
  }

  const handlePreviewBatchSettlement = async () => {
    const paidFrom = normalizeDatetimeLocal(batchSettlementRange.paidFrom)
    const paidTo = normalizeDatetimeLocal(batchSettlementRange.paidTo)
    if (!paidFrom || !paidTo) {
      showMessage('error', '请选择完整时间区间')
      return
    }

    setBatchPreviewLoading(true)
    try {
      const preview = await previewBatchPurchaseManualSettlement({
        paidFrom,
        paidTo,
      })
      setBatchSettlementPreview(preview)
      if (preview.total === 0) {
        showMessage('success', '当前时间区间内没有待分账订单')
      }
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '预览失败')
    } finally {
      setBatchPreviewLoading(false)
    }
  }

  const handleConfirmSettlement = async () => {
    setSettlementSaving(true)
    try {
      let batch: PurchaseManualSettlementBatch
      if (settlementDialogMode === 'single' && settlementOrder) {
        batch = await createSinglePurchaseManualSettlement(settlementOrder.orderNo, settlementNote)
      } else {
        const paidFrom = normalizeDatetimeLocal(batchSettlementRange.paidFrom)
        const paidTo = normalizeDatetimeLocal(batchSettlementRange.paidTo)
        if (!paidFrom || !paidTo) {
          throw new Error('请选择完整时间区间')
        }
        if (!batchSettlementPreview || batchSettlementPreview.total === 0) {
          throw new Error('请先生成分账预览')
        }
        batch = await createBatchPurchaseManualSettlement({
          paidFrom,
          paidTo,
          note: settlementNote,
          debugSettlement: batchDebugSettlement,
        })
      }
      setSettlementDialogMode(null)
      setSettlementOrder(null)
      setBatchSettlementPreview(null)
      await loadOrders(buildOrderFilters(ordersPage, ordersPageSize))
      showMessage('success', `${batch.debugSettlement ? '已记入 DEBUG 批次' : '已记入批次'} ${batch.batchNo}`)
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '分账失败')
    } finally {
      setSettlementSaving(false)
    }
  }

  const visibleProducts = useMemo(() => {
    const start = (productsPage - 1) * productsPageSize
    return products.slice(start, start + productsPageSize)
  }, [products, productsPage, productsPageSize])

  if (loading) {
    return <div className="flex h-64 items-center justify-center"><RefreshCw className="h-6 w-6 animate-spin text-muted-foreground" /></div>
  }

  return (
    <>
      <TabbedSettingsPage
        title="付费订阅"
        description="商品、订单、通知"
        tabs={tabs}
        activeTab={activeTab}
        onTabChange={setActiveTab}
        indicatorId="paid-subscriptions-tab-indicator"
        message={message}
      >
        {activeTab === 'products' && (
          <Card>
            <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <CardTitle>售卖商品</CardTitle>
              <div className="flex flex-wrap gap-2">
                <Button type="button" variant="outline" size="sm" onClick={() => void loadBase()}><RefreshCw className="mr-2 h-4 w-4" />刷新</Button>
                <Button type="button" size="sm" onClick={openCreateProduct}><Plus className="mr-2 h-4 w-4" />新建</Button>
              </div>
            </CardHeader>
            <CardContent className="px-0">
              <div className="space-y-3 px-4 md:hidden">
                {visibleProducts.map((product) => (
                  <div key={product.id} className="rounded-xl border border-border/70 px-4 py-4">
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <div className="font-medium">{product.name}</div>
                        <div className="text-xs text-muted-foreground">{product.subscriptionPlanName}</div>
                        <div className="mt-1 text-xs text-muted-foreground">{product.summary || '-'}</div>
                      </div>
                      <Badge variant="outline" className={product.enabled ? 'border-emerald-200 bg-emerald-50 text-emerald-700' : 'border-slate-200 bg-slate-100 text-slate-600'}>
                        {product.enabled ? '上架' : '下架'}
                      </Badge>
                    </div>
                    <div className="mt-3 flex items-center justify-between text-sm">
                      <span className="text-muted-foreground">{product.durationDays} 天</span>
                      <span className="font-semibold">{formatCNY(product.priceCnyCent)}</span>
                    </div>
                    <div className="mt-3 flex flex-wrap gap-2">
                      <Button type="button" variant="outline" size="sm" onClick={() => openEditProduct(product)}>编辑</Button>
                      <Button type="button" variant="outline" size="sm" onClick={() => void handleToggleProduct(product)}>{product.enabled ? '下架' : '上架'}</Button>
                      <Button type="button" variant="destructive" size="sm" onClick={() => void handleDeleteProduct(product)}>删除</Button>
                    </div>
                  </div>
                ))}
              </div>
              <Table className="hidden md:table">
                <TableHeader>
                  <TableRow>
                    <TableHead>商品</TableHead>
                    <TableHead>套餐</TableHead>
                    <TableHead>时长</TableHead>
                    <TableHead>售价</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead className="text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {visibleProducts.map((product) => (
                    <TableRow key={product.id}>
                      <TableCell>
                        <div>
                          <div className="font-medium">{product.name}</div>
                          <div className="text-xs text-muted-foreground">{product.summary || '-'}</div>
                        </div>
                      </TableCell>
                      <TableCell>{product.subscriptionPlanName}</TableCell>
                      <TableCell>{product.durationDays} 天</TableCell>
                      <TableCell>{formatCNY(product.priceCnyCent)}</TableCell>
                      <TableCell><Badge variant="outline" className={product.enabled ? 'border-emerald-200 bg-emerald-50 text-emerald-700' : 'border-slate-200 bg-slate-100 text-slate-600'}>{product.enabled ? '上架' : '下架'}</Badge></TableCell>
                      <TableCell className="text-right">
                        <div className="flex justify-end gap-2">
                          <Button type="button" variant="outline" size="sm" onClick={() => openEditProduct(product)}><Pencil className="mr-2 h-4 w-4" />编辑</Button>
                          <Button type="button" variant="outline" size="sm" onClick={() => void handleToggleProduct(product)}>{product.enabled ? '下架' : '上架'}</Button>
                          <Button type="button" variant="destructive" size="sm" onClick={() => void handleDeleteProduct(product)}><Trash2 className="mr-2 h-4 w-4" />删除</Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
              <div className="px-6 pb-6">
                <TablePagination
                  page={productsPage}
                  pageSize={productsPageSize}
                  total={products.length}
                  onPageChange={setProductsPage}
                  onPageSizeChange={(value) => {
                    setProductsPageSize(value)
                    setProductsPage(1)
                  }}
                />
              </div>
            </CardContent>
          </Card>
        )}

        {activeTab === 'orders' && (
          <Card>
            <CardHeader className="space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <CardTitle>订单</CardTitle>
                <Button type="button" variant="outline" size="sm" onClick={openBatchSettlementDialog}>批量分账</Button>
              </div>
              <div className="grid gap-3 md:grid-cols-[1fr_180px_180px_220px_auto]">
                <Input value={orderFilters.username} onChange={(event) => setOrderFilters((current) => ({ ...current, username: event.target.value }))} placeholder="搜索用户" />
                <Select value={orderFilters.paymentStatus} onValueChange={(value) => setOrderFilters((current) => ({ ...current, paymentStatus: value }))}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">全部支付状态</SelectItem>
                    <SelectItem value="pending">待支付</SelectItem>
                    <SelectItem value="paid">已支付</SelectItem>
                    <SelectItem value="refunded">已退款</SelectItem>
                    <SelectItem value="expired">已过期</SelectItem>
                    <SelectItem value="closed">已关闭</SelectItem>
                    <SelectItem value="failed">失败</SelectItem>
                  </SelectContent>
                </Select>
                <Select value={orderFilters.fulfillmentStatus} onValueChange={(value) => setOrderFilters((current) => ({ ...current, fulfillmentStatus: value }))}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">全部发放状态</SelectItem>
                    <SelectItem value="pending">待发放</SelectItem>
                    <SelectItem value="fulfilled">已开通</SelectItem>
                    <SelectItem value="failed">失败</SelectItem>
                  </SelectContent>
                </Select>
                <Select value={orderFilters.productId} onValueChange={(value) => setOrderFilters((current) => ({ ...current, productId: value }))}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">全部商品</SelectItem>
                    {products.map((product) => <SelectItem key={product.id} value={product.id}>{product.name}</SelectItem>)}
                  </SelectContent>
                </Select>
                <Button type="button" onClick={() => void handleSearchOrders()} disabled={ordersLoading}><RefreshCw className={`mr-2 h-4 w-4 ${ordersLoading ? 'animate-spin' : ''}`} />查询</Button>
              </div>
            </CardHeader>
            <CardContent className="px-0">
              <div className="space-y-3 px-4 md:hidden">
                {orders.map((order) => (
                  <div key={order.id} className="rounded-xl border border-border/70 px-4 py-4">
                    <div className="min-w-0">
                      <div className="font-mono text-xs text-muted-foreground">{order.orderNo}</div>
                      <div className="mt-1 font-medium">{order.productName}</div>
                      <div className="text-xs text-muted-foreground">{order.username || '-'}</div>
                    </div>

                    <div className="mt-3 flex flex-wrap gap-2">
                      <Badge variant="outline" className={paymentTone(order.paymentStatus)}>{paymentLabel(order.paymentStatus)}</Badge>
                      <Badge variant="outline">{fulfillmentLabel(order)}</Badge>
                      <Badge
                        variant="outline"
                        className={order.manualSettlementDone
                          ? 'border-slate-200 bg-slate-100 text-slate-700'
                          : 'border-dashed text-muted-foreground'}
                      >
                        {order.manualSettlementDone ? '已分账' : '未分账'}
                      </Badge>
                    </div>

                    <div className="mt-3 grid gap-2 text-sm text-muted-foreground">
                      <div className="flex items-center justify-between gap-3">
                        <span>金额</span>
                        <span className="font-medium text-foreground">{formatCNY(order.amountCnyCent)}</span>
                      </div>
                      <div className="flex items-center justify-between gap-3">
                        <span>时间</span>
                        <span>{formatDateTime(order.createdAt)}</span>
                      </div>
                    </div>

                    <div className="mt-3 flex flex-wrap gap-2">
                      {order.canRefresh && <Button type="button" size="sm" variant="outline" disabled={refreshingOrderNo === order.orderNo} onClick={() => void handleRefreshOrder(order.orderNo)}><RefreshCw className={`mr-2 h-4 w-4 ${refreshingOrderNo === order.orderNo ? 'animate-spin' : ''}`} />刷新</Button>}
                      <Button type="button" size="sm" variant="outline" onClick={() => void openStatusDialog(order)}>修改订单信息</Button>
                      <Button type="button" size="sm" variant="outline" disabled={order.manualSettlementDone || !['paid', 'refunded'].includes(order.paymentStatus)} onClick={() => openSingleSettlementDialog(order)}>单笔分账</Button>
                    </div>
                    {order.failureReason && <div className="mt-2 text-xs text-rose-600">{order.failureReason}</div>}
                  </div>
                ))}
              </div>
              <Table className="hidden md:table">
                <TableHeader>
                  <TableRow>
                    <TableHead>订单</TableHead>
                    <TableHead>用户</TableHead>
                    <TableHead>商品</TableHead>
                    <TableHead>金额</TableHead>
                    <TableHead>支付</TableHead>
                    <TableHead>发放</TableHead>
                    <TableHead>分账</TableHead>
                    <TableHead className="text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {orders.map((order) => (
                    <TableRow key={order.id}>
                      <TableCell>
                        <div className="font-mono text-xs">{order.orderNo}</div>
                        <div className="text-xs text-muted-foreground">{formatDateTime(order.createdAt)}</div>
                      </TableCell>
                      <TableCell>{order.username || '-'}</TableCell>
                      <TableCell>
                        <div>
                          <div className="font-medium">{order.productName}</div>
                          <div className="text-xs text-muted-foreground">{order.subscriptionPlanName}</div>
                        </div>
                      </TableCell>
                      <TableCell>{formatCNY(order.amountCnyCent)}</TableCell>
                      <TableCell><Badge variant="outline" className={paymentTone(order.paymentStatus)}>{paymentLabel(order.paymentStatus)}</Badge></TableCell>
                      <TableCell>
                        <Badge variant="outline">{fulfillmentLabel(order)}</Badge>
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant="outline"
                          className={order.manualSettlementDone
                            ? 'border-slate-200 bg-slate-100 text-slate-700'
                            : 'border-dashed text-muted-foreground'}
                        >
                          {order.manualSettlementDone ? '已分账' : '未分账'}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-right">
                        <div className="flex justify-end gap-2">
                          {order.canRefresh && <Button type="button" size="sm" variant="outline" disabled={refreshingOrderNo === order.orderNo} onClick={() => void handleRefreshOrder(order.orderNo)}><RefreshCw className={`mr-2 h-4 w-4 ${refreshingOrderNo === order.orderNo ? 'animate-spin' : ''}`} />刷新</Button>}
                          <Button type="button" size="sm" variant="outline" onClick={() => void openStatusDialog(order)}>修改订单信息</Button>
                          <Button type="button" size="sm" variant="outline" disabled={order.manualSettlementDone || !['paid', 'refunded'].includes(order.paymentStatus)} onClick={() => openSingleSettlementDialog(order)}>单笔分账</Button>
                        </div>
                        {order.failureReason && <div className="mt-2 text-xs text-rose-600">{order.failureReason}</div>}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
              <div className="px-6 pb-6">
                  <TablePagination
                    page={ordersPage}
                    pageSize={ordersPageSize}
                    total={ordersTotal}
                    onPageChange={(page) => {
                      setOrdersPage(page)
                      void loadOrders(buildOrderFilters(page, ordersPageSize))
                    }}
                    onPageSizeChange={(value) => {
                      setOrdersPageSize(value)
                      setOrdersPage(1)
                      void loadOrders(buildOrderFilters(1, value))
                    }}
                  />
              </div>
            </CardContent>
          </Card>
        )}

        {activeTab === 'webhooks' && (
          <div className="space-y-4">
            <div className="flex flex-col gap-3 border-b pb-3 lg:flex-row lg:items-center lg:justify-between">
              <div className="text-sm text-muted-foreground break-all">{'{{ orderNo }} {{ amountCny }} {{ amountToday }} {{ amountTotal }} {{ billsToday }} {{ billsTotal }} {{ subscriptionName }} {{ quantity }} {{ deliveryCdk }} {{ alipayTradeNo }} {{ createdAt }}'}</div>
              <div className="flex flex-wrap gap-2">
                <Button type="button" variant="outline" size="sm" onClick={() => void refreshWebhooks()}><RefreshCw className="mr-2 h-4 w-4" />刷新</Button>
                <Button type="button" size="sm" onClick={openCreateWebhook}><Plus className="mr-2 h-4 w-4" />新建</Button>
              </div>
            </div>
            <div className="divide-y rounded-lg border">
              {webhooks.length === 0 && <div className="px-4 py-8 text-sm text-muted-foreground">暂无通知目标</div>}
              {webhooks.map((item) => (
                <div key={item.id} className="flex flex-col gap-3 px-4 py-4 lg:flex-row lg:items-start lg:justify-between">
                  <div className="min-w-0 space-y-2">
                    <div className="flex items-center gap-2">
                      <div className="font-medium">{item.name}</div>
                      <Badge variant="outline" className={item.enabled ? 'border-emerald-200 bg-emerald-50 text-emerald-700' : 'border-slate-200 bg-slate-100 text-slate-600'}>{item.enabled ? '启用' : '停用'}</Badge>
                    </div>
                    <div className="truncate text-xs text-muted-foreground">{item.targetUrl}</div>
                    <div className="grid gap-2 text-xs text-muted-foreground md:grid-cols-2">
                      <pre className="overflow-auto rounded border bg-muted/40 p-3">{item.headersTemplate || '{}'}</pre>
                      <pre className="overflow-auto rounded border bg-muted/40 p-3">{item.bodyTemplate || ''}</pre>
                    </div>
                  </div>
                  <div className="flex shrink-0 flex-wrap gap-2">
                    <Button type="button" variant="outline" size="sm" disabled={testingWebhookID === item.id} onClick={() => void handleTestWebhook(item)}><Send className="mr-2 h-4 w-4" />测试发送</Button>
                    <Button type="button" variant="outline" size="sm" onClick={() => openEditWebhook(item)}><Pencil className="mr-2 h-4 w-4" />编辑</Button>
                    <Button type="button" variant="outline" size="sm" onClick={() => void handleToggleWebhook(item)}>{item.enabled ? '停用' : '启用'}</Button>
                    <Button type="button" variant="destructive" size="sm" onClick={() => void handleDeleteWebhook(item)}><Trash2 className="mr-2 h-4 w-4" />删除</Button>
                  </div>
                </div>
              ))}
            </div>
            {testResult && (
              <div className="grid gap-3 rounded-lg border p-4 md:grid-cols-2">
                <div className="space-y-2">
                  <div className="flex items-center gap-2">
                    <Badge variant="outline" className={testResult.ok ? 'border-emerald-200 bg-emerald-50 text-emerald-700' : 'border-rose-200 bg-rose-50 text-rose-700'}>{testResult.responseStatusCode}</Badge>
                  </div>
                  <pre className="max-h-64 overflow-auto rounded border bg-muted/40 p-3 text-xs">{testResult.responseBody || '(empty)'}</pre>
                </div>
                <div className="space-y-2">
                  <pre className="max-h-64 overflow-auto rounded border bg-muted/40 p-3 text-xs">{JSON.stringify(testResult.variables, null, 2)}</pre>
                </div>
              </div>
            )}
          </div>
        )}

        {activeTab === 'settings' && (
          <Card>
            <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <CardTitle>支付设置</CardTitle>
              <Badge variant="outline" className={settings?.paymentConfigured ? 'border-emerald-200 bg-emerald-50 text-emerald-700' : 'border-slate-200 bg-slate-100 text-slate-600'}>
                {settings?.paymentConfigured ? '配置完整' : '待补齐'}
              </Badge>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="grid gap-6 lg:grid-cols-2">
                <div className="space-y-4">
                  <div className="flex items-center justify-between border-b pb-3"><Label>支付功能</Label><Switch checked={settingsDraft.purchaseEnabled} onCheckedChange={(checked) => setSettingsDraft((current) => ({ ...current, purchaseEnabled: checked }))} /></div>
                  <div className="flex items-center justify-between border-b pb-3"><Label>DEBUG 自动支付</Label><Switch checked={settingsDraft.debugAutoPaid} onCheckedChange={(checked) => setSettingsDraft((current) => ({ ...current, debugAutoPaid: checked }))} /></div>
                  <div className="grid gap-4 sm:grid-cols-2">
                    <div className="space-y-2"><Label htmlFor="alipayAppId">App ID</Label><Input id="alipayAppId" value={settingsDraft.alipayAppId} onChange={(event) => setSettingsDraft((current) => ({ ...current, alipayAppId: event.target.value }))} /></div>
                    <div className="space-y-2"><Label htmlFor="alipayPid">PID</Label><Input id="alipayPid" value={settingsDraft.alipayPid} onChange={(event) => setSettingsDraft((current) => ({ ...current, alipayPid: event.target.value }))} /></div>
                    <div className="space-y-2">
                      <Label>环境</Label>
                      <Select value={settingsDraft.alipayEnvironment} onValueChange={(value: AlipayEnvironment) => setSettingsDraft((current) => ({ ...current, alipayEnvironment: value }))}>
                        <SelectTrigger><SelectValue /></SelectTrigger>
                        <SelectContent>
                          <SelectItem value="sandbox">沙箱</SelectItem>
                          <SelectItem value="production">生产</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="space-y-2"><Label htmlFor="alipayNotifyUrl">回调地址</Label><Input id="alipayNotifyUrl" value={settingsDraft.alipayNotifyUrl} onChange={(event) => setSettingsDraft((current) => ({ ...current, alipayNotifyUrl: event.target.value }))} /></div>
                  </div>
                </div>
                <div className="space-y-4">
                  <div className="space-y-2"><Label htmlFor="alipayPublicKey">支付宝公钥</Label><Textarea id="alipayPublicKey" value={settingsDraft.alipayPublicKey} onChange={(event) => setSettingsDraft((current) => ({ ...current, alipayPublicKey: event.target.value }))} className="min-h-[148px]" /></div>
                  <div className="space-y-2">
                    <div className="flex items-center justify-between">
                      <Label htmlFor="alipayPrivateKey">应用私钥</Label>
                      {settings?.privateKeySet && <Badge variant="outline">已设置</Badge>}
                    </div>
                    <Textarea id="alipayPrivateKey" value={alipayPrivateKeyInput} onChange={(event) => setAlipayPrivateKeyInput(event.target.value)} placeholder={settings?.privateKeySet ? '留空保持现有私钥' : '-----BEGIN PRIVATE KEY-----'} className="min-h-[148px]" />
                  </div>
                </div>
              </div>
              <div className="flex justify-end">
                <Button type="button" disabled={savingSettings} onClick={() => void handleSaveSettings()}>
                  {savingSettings ? <RefreshCw className="mr-2 h-4 w-4 animate-spin" /> : <CreditCard className="mr-2 h-4 w-4" />}
                  保存设置
                </Button>
              </div>
            </CardContent>
          </Card>
        )}
      </TabbedSettingsPage>

      <Dialog open={productDialogOpen} onOpenChange={setProductDialogOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader><DialogTitle>{editingProduct ? '编辑商品' : '新建商品'}</DialogTitle></DialogHeader>
          <div className="grid gap-4 py-2 lg:grid-cols-2">
            <div className="space-y-2 lg:col-span-2"><Label>名称</Label><Input value={productForm.name} onChange={(event) => setProductForm((current) => ({ ...current, name: event.target.value }))} /></div>
            <div className="space-y-2 lg:col-span-2"><Label>摘要</Label><Input value={productForm.summary} onChange={(event) => setProductForm((current) => ({ ...current, summary: event.target.value }))} /></div>
            <div className="space-y-2">
              <Label>套餐</Label>
              <Select value={productForm.subscriptionPlanId} onValueChange={(value) => setProductForm((current) => ({ ...current, subscriptionPlanId: value }))}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>{plans.map((plan) => <SelectItem key={plan.id} value={plan.id}>{plan.name}</SelectItem>)}</SelectContent>
              </Select>
            </div>
            <div className="space-y-2"><Label>时长</Label><Input type="number" min="1" value={productForm.durationDays} onChange={(event) => setProductForm((current) => ({ ...current, durationDays: Number(event.target.value || 0) }))} /></div>
            <div className="space-y-2"><Label>售价</Label><Input type="number" min="0.01" step="0.01" value={productPriceInput} onChange={(event) => setProductPriceInput(event.target.value)} /></div>
            <div className="space-y-2"><Label>分组</Label><Input value={productForm.groupName} onChange={(event) => setProductForm((current) => ({ ...current, groupName: event.target.value }))} /></div>
            <div className="space-y-2"><Label>组序</Label><Input type="number" value={productForm.groupSort} onChange={(event) => setProductForm((current) => ({ ...current, groupSort: Number(event.target.value || 0) }))} /></div>
            <div className="space-y-2"><Label>排序</Label><Input type="number" value={productForm.sortOrder} onChange={(event) => setProductForm((current) => ({ ...current, sortOrder: Number(event.target.value || 0) }))} /></div>
            <div className="flex items-center justify-between rounded border px-4 py-3"><Label>推荐</Label><Switch checked={productForm.isRecommended} onCheckedChange={(checked) => setProductForm((current) => ({ ...current, isRecommended: checked }))} /></div>
            <div className="flex items-center justify-between rounded border px-4 py-3"><Label>上架</Label><Switch checked={productForm.enabled} onCheckedChange={(checked) => setProductForm((current) => ({ ...current, enabled: checked }))} /></div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setProductDialogOpen(false)}>取消</Button>
            <Button onClick={() => void handleSaveProduct()} disabled={savingProduct}>{savingProduct && <RefreshCw className="mr-2 h-4 w-4 animate-spin" />}保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={webhookDialogOpen} onOpenChange={setWebhookDialogOpen}>
        <DialogContent className="max-w-3xl">
          <DialogHeader><DialogTitle>{editingWebhook ? '编辑订单通知' : '新建订单通知'}</DialogTitle></DialogHeader>
          <div className="grid gap-4 py-2">
            <div className="space-y-2"><Label>名称</Label><Input value={webhookForm.name} onChange={(event) => setWebhookForm((current) => ({ ...current, name: event.target.value }))} /></div>
            <div className="space-y-2"><Label>目标地址</Label><Input value={webhookForm.targetUrl} onChange={(event) => setWebhookForm((current) => ({ ...current, targetUrl: event.target.value }))} placeholder="https://example.com/webhook" /></div>
            <div className="grid gap-4 lg:grid-cols-2">
              <div className="space-y-2"><Label>Headers JSON</Label><Textarea value={webhookForm.headersTemplate} onChange={(event) => setWebhookForm((current) => ({ ...current, headersTemplate: event.target.value }))} className="min-h-[180px]" /></div>
              <div className="space-y-2"><Label>Body 模板</Label><Textarea value={webhookForm.bodyTemplate} onChange={(event) => setWebhookForm((current) => ({ ...current, bodyTemplate: event.target.value }))} className="min-h-[180px]" /></div>
            </div>
            <div className="flex items-center justify-between rounded border px-4 py-3"><Label>启用</Label><Switch checked={webhookForm.enabled} onCheckedChange={(checked) => setWebhookForm((current) => ({ ...current, enabled: checked }))} /></div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setWebhookDialogOpen(false)}>取消</Button>
            <Button onClick={() => void handleSaveWebhook()} disabled={savingWebhook}>{savingWebhook && <RefreshCw className="mr-2 h-4 w-4 animate-spin" />}保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={statusDialogOrder !== null} onOpenChange={(open) => !open && setStatusDialogOrder(null)}>
        <DialogContent className="max-w-2xl">
          <DialogHeader><DialogTitle>修改订单信息</DialogTitle></DialogHeader>
          <div className="grid gap-4 py-2">
            <div className="space-y-2">
              <Label>付款状态</Label>
              <Select value={statusDraft} onValueChange={(value: 'paid' | 'expired' | 'refunded') => setStatusDraft(value)}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="paid">paid</SelectItem>
                  <SelectItem value="expired">expired</SelectItem>
                  <SelectItem value="refunded">refunded</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2"><Label>备注</Label><Textarea value={statusNote} onChange={(event) => setStatusNote(event.target.value)} className="min-h-[100px]" /></div>
            <div className="rounded border">
              <div className="border-b px-4 py-3 text-sm font-medium">备注历史</div>
              <div className="max-h-64 overflow-auto px-4 py-3 text-sm">
                {historyLoading && <div className="text-muted-foreground">加载中...</div>}
                {!historyLoading && statusHistory.length === 0 && <div className="text-muted-foreground">暂无历史</div>}
                {!historyLoading && statusHistory.map((item) => (
                  <div key={item.id} className="border-b py-3 last:border-b-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge variant="outline">{item.fromStatus} → {item.toStatus}</Badge>
                      <span className="text-xs text-muted-foreground">{item.createdBy || '-'}</span>
                      <span className="text-xs text-muted-foreground">{formatDateTime(item.createdAt)}</span>
                    </div>
                    <div className="mt-2 whitespace-pre-wrap text-sm">{item.note || '-'}</div>
                  </div>
                ))}
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setStatusDialogOrder(null)}>取消</Button>
            <Button onClick={() => void handleSaveStatus()} disabled={statusSaving}>{statusSaving && <RefreshCw className="mr-2 h-4 w-4 animate-spin" />}保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={settlementDialogMode !== null} onOpenChange={(open) => !open && setSettlementDialogMode(null)}>
        <DialogContent className="max-h-[calc(100vh-2rem)] overflow-y-auto max-w-3xl">
          <DialogHeader><DialogTitle>{settlementDialogMode === 'single' ? '单笔分账' : '批量分账'}</DialogTitle></DialogHeader>
          {settlementDialogMode === 'single' ? (
            <div className="space-y-4 py-2">
              <div className="text-sm text-muted-foreground">{settlementOrder?.orderNo}</div>
              <div className="space-y-2"><Label>备注</Label><Textarea value={settlementNote} onChange={(event) => setSettlementNote(event.target.value)} className="min-h-[120px]" /></div>
            </div>
          ) : (
            <div className="space-y-4 py-2">
              <div className="grid gap-4 md:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="batchSettlementPaidFrom">支付开始</Label>
                  <Input
                    id="batchSettlementPaidFrom"
                    type="datetime-local"
                    value={batchSettlementRange.paidFrom}
                    onChange={(event) => {
                      setBatchSettlementRange((current) => ({ ...current, paidFrom: event.target.value }))
                      setBatchSettlementPreview(null)
                    }}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="batchSettlementPaidTo">支付结束</Label>
                  <Input
                    id="batchSettlementPaidTo"
                    type="datetime-local"
                    value={batchSettlementRange.paidTo}
                    onChange={(event) => {
                      setBatchSettlementRange((current) => ({ ...current, paidTo: event.target.value }))
                      setBatchSettlementPreview(null)
                    }}
                  />
                </div>
              </div>

              <div className="flex items-center justify-between rounded-lg border px-4 py-3">
                <div>
                  <p className="text-sm font-medium">DEBUG 分账</p>
                  <p className="text-xs text-muted-foreground">用于测试批次标记，不改变订单筛选条件。</p>
                </div>
                <Switch checked={batchDebugSettlement} onCheckedChange={setBatchDebugSettlement} />
              </div>

              <div className="space-y-2">
                <div className="flex items-center justify-between gap-3">
                  <Label>分账预览</Label>
                  <Button type="button" variant="outline" size="sm" onClick={() => void handlePreviewBatchSettlement()} disabled={batchPreviewLoading}>
                    {batchPreviewLoading ? <RefreshCw className="mr-2 h-4 w-4 animate-spin" /> : null}
                    生成预览
                  </Button>
                </div>
                <div className="rounded-lg border">
                  {batchSettlementPreview ? (
                    <div className="space-y-4 p-4">
                      <div className="flex flex-wrap gap-2">
                        <Badge variant="outline">订单 {batchSettlementPreview.total}</Badge>
                        <Badge variant="outline">金额 {formatCNY(batchSettlementPreview.totalAmountCnyCent)}</Badge>
                        {batchSettlementPreview.total > batchSettlementPreview.items.length ? (
                          <Badge variant="secondary">仅展示前 {batchSettlementPreview.items.length} 笔</Badge>
                        ) : null}
                      </div>
                      {batchSettlementPreview.items.length === 0 ? (
                        <div className="text-sm text-muted-foreground">当前时间区间内没有待分账订单。</div>
                      ) : (
                        <div className="space-y-3">
                          {batchSettlementPreview.items.map((order) => (
                            <div key={order.id} className="rounded-lg border border-border/70 px-4 py-3">
                              <div className="flex flex-wrap items-start justify-between gap-3">
                                <div className="min-w-0">
                                  <div className="font-mono text-xs text-muted-foreground">{order.orderNo}</div>
                                  <div className="mt-1 font-medium">{order.productName}</div>
                                  <div className="text-xs text-muted-foreground">{order.username || '-'}</div>
                                </div>
                                <div className="text-right">
                                  <div className="font-medium">{formatCNY(order.amountCnyCent)}</div>
                                  <div className="mt-1 text-xs text-muted-foreground">{formatDateTime(order.paidAt || order.createdAt)}</div>
                                </div>
                              </div>
                              <div className="mt-3 flex flex-wrap gap-2">
                                <Badge variant="outline" className={paymentTone(order.paymentStatus)}>{paymentLabel(order.paymentStatus)}</Badge>
                                <Badge variant="outline">{fulfillmentLabel(order)}</Badge>
                              </div>
                            </div>
                          ))}
                        </div>
                      )}
                    </div>
                  ) : (
                    <div className="p-4 text-sm text-muted-foreground">选择时间区间后生成预览。</div>
                  )}
                </div>
              </div>

              <div className="space-y-2">
                <Label>备注</Label>
                <Textarea value={settlementNote} onChange={(event) => setSettlementNote(event.target.value)} className="min-h-[120px]" />
              </div>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setSettlementDialogMode(null)}>取消</Button>
            <Button
              onClick={() => void handleConfirmSettlement()}
              disabled={settlementSaving || (settlementDialogMode === 'batch' && (!batchSettlementPreview || batchSettlementPreview.total === 0))}
            >
              {settlementSaving && <RefreshCw className="mr-2 h-4 w-4 animate-spin" />}
              确认
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
