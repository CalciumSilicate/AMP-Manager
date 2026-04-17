import { useEffect, useState } from 'react'
import { formatDateTime } from '@/lib/formatters'
import {
  createPurchaseProduct,
  deletePurchaseProduct,
  getPurchaseSettings,
  listPurchaseOrdersAdmin,
  listPurchaseProductsAdmin,
  refreshPurchaseOrderAdmin,
  setPurchaseProductEnabled,
  updatePurchaseProduct,
  updatePurchaseSettings,
  type AdminOrderFilters,
  type PurchaseOrder,
  type PurchaseProduct,
  type PurchaseProductRequest,
  type PurchaseSettingsResponse,
  type AlipayEnvironment,
} from '@/api/purchase'
import { getPlans, type SubscriptionPlanResponse } from '@/api/subscription'
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
import { TablePagination } from '@/components/TablePagination'
import { TabbedSettingsPage, type TabbedSettingsPageTab } from '@/components/layout/TabbedSettingsPage'
import { CreditCard, Pencil, Plus, RefreshCw, Trash2 } from 'lucide-react'

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

function fulfillmentLabel(order: PurchaseOrder): string {
  if (order.orderKind === 'balance_topup') {
    return order.fulfillmentStatus === 'fulfilled' ? '已到账' : order.fulfillmentStatus === 'failed' ? '入账失败' : '待入账'
  }
  const status = order.fulfillmentStatus
  switch (status) {
    case 'fulfilled':
      return '已开通'
    case 'failed':
      return '失败'
    default:
      return '待发放'
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

function initialProductForm(): PurchaseProductRequest {
  return {
    name: '',
    summary: '',
    subscriptionPlanId: '',
    durationDays: 30,
    priceCnyCent: 0,
    isRecommended: false,
    sortOrder: 0,
    enabled: true,
  }
}

type PaidSubscriptionsTab = 'products' | 'orders' | 'settings'

const tabs: TabbedSettingsPageTab<PaidSubscriptionsTab>[] = [
  { key: 'products', label: '售卖商品' },
  { key: 'orders', label: '订单' },
  { key: 'settings', label: '支付设置' },
]

export default function PaidSubscriptions() {
  const [activeTab, setActiveTab] = useState<PaidSubscriptionsTab>('products')
  const [settings, setSettings] = useState<PurchaseSettingsResponse | null>(null)
  const [plans, setPlans] = useState<SubscriptionPlanResponse[]>([])
  const [products, setProducts] = useState<PurchaseProduct[]>([])
  const [orders, setOrders] = useState<PurchaseOrder[]>([])
  const [loading, setLoading] = useState(true)
  const [ordersLoading, setOrdersLoading] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const [settingsDraft, setSettingsDraft] = useState<{
    purchaseEnabled: boolean
    debugAutoPaid: boolean
    alipayAppId: string
    alipayPid: string
    alipayEnvironment: AlipayEnvironment
    alipayNotifyUrl: string
    alipayPublicKey: string
  }>({
    purchaseEnabled: false,
    debugAutoPaid: false,
    alipayAppId: '',
    alipayPid: '',
    alipayEnvironment: 'sandbox',
    alipayNotifyUrl: '',
    alipayPublicKey: '',
  })
  const [alipayPrivateKeyInput, setAlipayPrivateKeyInput] = useState('')
  const [savingSettings, setSavingSettings] = useState(false)

  const [productDialogOpen, setProductDialogOpen] = useState(false)
  const [editingProduct, setEditingProduct] = useState<PurchaseProduct | null>(null)
  const [productForm, setProductForm] = useState<PurchaseProductRequest>(initialProductForm())
  const [productPriceInput, setProductPriceInput] = useState('0.00')
  const [savingProduct, setSavingProduct] = useState(false)
  const [productsPage, setProductsPage] = useState(1)
  const [productsPageSize, setProductsPageSize] = useState(10)

  const [orderFilters, setOrderFilters] = useState({
    paymentStatus: 'all',
    fulfillmentStatus: 'all',
    username: '',
    productId: 'all',
  })
  const [refreshingOrderNo, setRefreshingOrderNo] = useState<string | null>(null)
  const [ordersPage, setOrdersPage] = useState(1)
  const [ordersPageSize, setOrdersPageSize] = useState(20)

  useEffect(() => {
    void loadBase()
  }, [])

  const pendingOrders = orders.filter((item) => item.paymentStatus === 'pending').length

  const loadBase = async () => {
    setLoading(true)
    try {
      const [settingsData, productData, planData] = await Promise.all([
        getPurchaseSettings(),
        listPurchaseProductsAdmin(),
        getPlans(),
      ])
      setSettings(settingsData)
      setProducts(productData)
      setPlans(planData)
      setSettingsDraft({
        purchaseEnabled: settingsData.purchaseEnabled,
        debugAutoPaid: settingsData.debugAutoPaid,
        alipayAppId: settingsData.alipayAppId,
        alipayPid: settingsData.alipayPid,
        alipayEnvironment: settingsData.alipayEnvironment,
        alipayNotifyUrl: settingsData.alipayNotifyUrl,
        alipayPublicKey: settingsData.alipayPublicKey,
      })
      await loadOrders({
        paymentStatus: '',
        fulfillmentStatus: '',
        username: '',
        productId: '',
        limit: 100,
      })
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }

  const loadOrders = async (filters?: AdminOrderFilters) => {
    setOrdersLoading(true)
    try {
      const response = await listPurchaseOrdersAdmin(filters)
      setOrders(response.items || [])
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '加载订单失败')
    } finally {
      setOrdersLoading(false)
    }
  }

  const showMessage = (type: 'success' | 'error', text: string) => {
    setMessage({ type, text })
    window.setTimeout(() => setMessage(null), 5000)
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
        alipayNotifyUrl: response.settings.alipayNotifyUrl,
        alipayPublicKey: response.settings.alipayPublicKey,
      })
      setAlipayPrivateKeyInput('')
      showMessage('success', '支付配置已保存')
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '保存失败')
    } finally {
      setSavingSettings(false)
    }
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
      isRecommended: product.isRecommended,
      sortOrder: product.sortOrder,
      enabled: product.enabled,
    })
    setProductPriceInput((product.priceCnyCent / 100).toFixed(2))
    setProductDialogOpen(true)
  }

  const handleSaveProduct = async () => {
    if (!productForm.subscriptionPlanId) {
      showMessage('error', '请选择套餐')
      return
    }
    const priceYuan = Number.parseFloat(productPriceInput)
    if (Number.isNaN(priceYuan) || priceYuan <= 0) {
      showMessage('error', '售价必须大于 0')
      return
    }

    const payload: PurchaseProductRequest = {
      ...productForm,
      priceCnyCent: Math.round(priceYuan * 100),
    }

    setSavingProduct(true)
    try {
      if (editingProduct) {
        await updatePurchaseProduct(editingProduct.id, payload)
        showMessage('success', '商品已更新')
      } else {
        await createPurchaseProduct(payload)
        showMessage('success', '商品已创建')
      }
      setProductDialogOpen(false)
      const refreshed = await listPurchaseProductsAdmin()
      setProducts(refreshed)
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '保存失败')
    } finally {
      setSavingProduct(false)
    }
  }

  const handleDeleteProduct = async (product: PurchaseProduct) => {
    if (!window.confirm(`确定删除商品 ${product.name} 吗？`)) {
      return
    }
    try {
      await deletePurchaseProduct(product.id)
      setProducts((current) => current.filter((item) => item.id !== product.id))
      showMessage('success', '商品已删除')
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '删除失败')
    }
  }

  const handleToggleProduct = async (product: PurchaseProduct) => {
    try {
      await setPurchaseProductEnabled(product.id, !product.enabled)
      setProducts((current) => current.map((item) => (item.id === product.id ? { ...item, enabled: !item.enabled } : item)))
      showMessage('success', product.enabled ? '商品已下架' : '商品已上架')
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '操作失败')
    }
  }

  const handleSearchOrders = async () => {
    await loadOrders({
      paymentStatus: orderFilters.paymentStatus === 'all' ? '' : orderFilters.paymentStatus as PurchaseOrder['paymentStatus'],
      fulfillmentStatus: orderFilters.fulfillmentStatus === 'all' ? '' : orderFilters.fulfillmentStatus as PurchaseOrder['fulfillmentStatus'],
      username: orderFilters.username.trim(),
      productId: orderFilters.productId === 'all' ? '' : orderFilters.productId,
      limit: 100,
    })
    setOrdersPage(1)
  }

  const handleRefreshOrder = async (orderNo: string) => {
    setRefreshingOrderNo(orderNo)
    try {
      await refreshPurchaseOrderAdmin(orderNo)
      await handleSearchOrders()
      showMessage('success', '订单状态已刷新')
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

  const ordersTotalPages = Math.max(1, Math.ceil(orders.length / ordersPageSize))
  const currentOrdersPage = Math.min(ordersPage, ordersTotalPages)
  const visibleOrders = orders.slice((currentOrdersPage - 1) * ordersPageSize, currentOrdersPage * ordersPageSize)
  const productsTotalPages = Math.max(1, Math.ceil(products.length / productsPageSize))
  const currentProductsPage = Math.min(productsPage, productsTotalPages)
  const visibleProducts = products.slice((currentProductsPage - 1) * productsPageSize, currentProductsPage * productsPageSize)

  return (
    <>
      <TabbedSettingsPage
        title="付费订阅"
        description="管理售卖商品、订单履约和支付设置"
        tabs={tabs}
        activeTab={activeTab}
        onTabChange={setActiveTab}
        indicatorId="paid-subscriptions-tab-indicator"
        message={message}
      >
        {activeTab === 'products' && (
          <Card>
            <CardHeader>
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="space-y-1">
                  <CardTitle>售卖商品</CardTitle>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button type="button" variant="outline" size="sm" className="min-w-24" onClick={() => void loadBase()}>
                    <RefreshCw className="mr-2 h-4 w-4" />
                    刷新
                  </Button>
                  <Button type="button" size="sm" className="min-w-24" onClick={openCreateProduct}>
                    <Plus className="mr-2 h-4 w-4" />
                    新建商品
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="rounded-lg border">
                {products.length === 0 ? (
                  <div className="px-5 py-10 text-sm text-muted-foreground">暂无商品。</div>
                ) : (
                  <>
                    <div className="overflow-x-auto">
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead>商品</TableHead>
                            <TableHead>套餐</TableHead>
                            <TableHead>时长</TableHead>
                            <TableHead>售价</TableHead>
                            <TableHead>排序</TableHead>
                            <TableHead>状态</TableHead>
                            <TableHead className="text-right">操作</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {visibleProducts.map((product) => (
                            <TableRow key={product.id}>
                              <TableCell>
                                <div>
                                  <p className="font-medium">{product.name}</p>
                                  <p className="text-xs text-muted-foreground">{product.summary || '-'}</p>
                                </div>
                              </TableCell>
                              <TableCell>{product.subscriptionPlanName || '-'}</TableCell>
                              <TableCell>{product.durationDays} 天</TableCell>
                              <TableCell>{formatCNY(product.priceCnyCent)}</TableCell>
                              <TableCell>{product.sortOrder}</TableCell>
                              <TableCell>
                                <div className="flex flex-wrap gap-2">
                                  <Badge variant="outline" className={`${product.enabled ? 'border-emerald-200 bg-emerald-50 text-emerald-700' : 'border-slate-200 bg-slate-100 text-slate-600'}`}>
                                    {product.enabled ? '上架' : '下架'}
                                  </Badge>
                                  {product.isRecommended && (
                                    <Badge variant="outline" className="border-sky-200 bg-sky-50 text-sky-700">
                                      推荐
                                    </Badge>
                                  )}
                                </div>
                              </TableCell>
                              <TableCell className="text-right">
                                <div className="flex justify-end gap-2">
                                  <Button type="button" variant="outline" size="sm" onClick={() => openEditProduct(product)}>
                                    <Pencil className="mr-2 h-4 w-4" />
                                    编辑
                                  </Button>
                                  <Button type="button" variant="outline" size="sm" onClick={() => void handleToggleProduct(product)}>
                                    {product.enabled ? '下架' : '上架'}
                                  </Button>
                                  <Button type="button" variant="destructive" size="sm" onClick={() => void handleDeleteProduct(product)}>
                                    <Trash2 className="mr-2 h-4 w-4" />
                                    删除
                                  </Button>
                                </div>
                              </TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    </div>
                    <div className="px-6 pb-6">
                      <TablePagination
                        page={currentProductsPage}
                        pageSize={productsPageSize}
                        total={products.length}
                        onPageChange={setProductsPage}
                        onPageSizeChange={(nextPageSize) => {
                          setProductsPageSize(nextPageSize)
                          setProductsPage(1)
                        }}
                      />
                    </div>
                  </>
                )}
              </div>
            </CardContent>
          </Card>
        )}

        {activeTab === 'orders' && (
          <Card>
            <CardHeader>
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="space-y-1">
                  <CardTitle>订单</CardTitle>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Badge variant="outline">待支付 {pendingOrders}</Badge>
                  <Badge variant="outline">共 {orders.length} 条</Badge>
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-4 px-0 pb-0">
              <div className="grid gap-3 px-6 md:grid-cols-[1fr_180px_180px_220px_auto]">
                <Input
                  value={orderFilters.username}
                  onChange={(event) => setOrderFilters((current) => ({ ...current, username: event.target.value }))}
                  placeholder="搜索用户名"
                />
                <Select value={orderFilters.paymentStatus} onValueChange={(value) => setOrderFilters((current) => ({ ...current, paymentStatus: value }))}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">全部支付状态</SelectItem>
                    <SelectItem value="pending">待支付</SelectItem>
                    <SelectItem value="paid">已支付</SelectItem>
                    <SelectItem value="expired">已过期</SelectItem>
                    <SelectItem value="closed">已关闭</SelectItem>
                    <SelectItem value="failed">失败</SelectItem>
                  </SelectContent>
                </Select>
                <Select value={orderFilters.fulfillmentStatus} onValueChange={(value) => setOrderFilters((current) => ({ ...current, fulfillmentStatus: value }))}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">全部发放状态</SelectItem>
                    <SelectItem value="pending">待发放</SelectItem>
                    <SelectItem value="fulfilled">已开通</SelectItem>
                    <SelectItem value="failed">失败</SelectItem>
                  </SelectContent>
                </Select>
                <Select value={orderFilters.productId} onValueChange={(value) => setOrderFilters((current) => ({ ...current, productId: value }))}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">全部商品</SelectItem>
                    {products.map((product) => (
                      <SelectItem key={product.id} value={product.id}>{product.name}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Button type="button" onClick={() => void handleSearchOrders()} disabled={ordersLoading}>
                  <RefreshCw className={`mr-2 h-4 w-4 ${ordersLoading ? 'animate-spin' : ''}`} />
                  查询
                </Button>
              </div>

              <div className="overflow-x-auto border-t">
                {orders.length === 0 ? (
                  <div className="px-5 py-10 text-sm text-muted-foreground">暂无订单。</div>
                ) : (
                  <div className="px-6 pb-6">
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>订单</TableHead>
                          <TableHead>用户</TableHead>
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
                            <TableCell>
                              <div>
                                <p className="font-mono text-xs">{order.orderNo}</p>
                                {order.alipayTradeNo && <p className="mt-1 text-xs text-muted-foreground">{order.alipayTradeNo}</p>}
                              </div>
                            </TableCell>
                            <TableCell>{order.username || '-'}</TableCell>
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
                              <Badge variant="outline" className={fulfillmentTone(order)}>
                                {fulfillmentLabel(order)}
                              </Badge>
                            </TableCell>
                            <TableCell className="text-xs text-muted-foreground">{formatDateTime(order.createdAt)}</TableCell>
                            <TableCell className="text-right">
                              <div className="flex justify-end gap-2">
                                {order.canRefresh && (
                                  <Button
                                    type="button"
                                    size="sm"
                                    variant="outline"
                                    onClick={() => void handleRefreshOrder(order.orderNo)}
                                    disabled={refreshingOrderNo === order.orderNo}
                                  >
                                    <RefreshCw className={`mr-2 h-4 w-4 ${refreshingOrderNo === order.orderNo ? 'animate-spin' : ''}`} />
                                    刷新
                                  </Button>
                                )}
                              </div>
                              {order.failureReason && (
                                <p className="mt-2 text-xs text-rose-600">{order.failureReason}</p>
                              )}
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

        {activeTab === 'settings' && (
          <Card>
            <CardHeader>
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="space-y-1">
                  <CardTitle>支付设置</CardTitle>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Badge variant="outline" className={settings?.paymentConfigured ? 'border-emerald-200 bg-emerald-50 text-emerald-700' : 'border-slate-200 bg-slate-100 text-slate-600'}>
                    {settings?.paymentConfigured ? '支付配置完整' : '支付配置待补齐'}
                  </Badge>
                  <Button type="button" variant="outline" size="sm" onClick={() => void loadBase()}>
                    <RefreshCw className="mr-2 h-4 w-4" />
                    刷新
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="grid gap-5 lg:grid-cols-2">
                <div className="space-y-4">
                  <div className="flex items-center justify-between border-b border-border/70 pb-3">
                    <div>
                      <p className="text-sm font-medium">支付功能</p>
                    </div>
                    <Switch
                      checked={settingsDraft.purchaseEnabled}
                      onCheckedChange={(checked) => setSettingsDraft((current) => ({ ...current, purchaseEnabled: checked }))}
                    />
                  </div>
                  <div className="flex items-center justify-between border-b border-border/70 pb-3">
                    <div>
                      <p className="text-sm font-medium">DEBUG 自动支付</p>
                    </div>
                    <Switch
                      checked={settingsDraft.debugAutoPaid}
                      onCheckedChange={(checked) => setSettingsDraft((current) => ({ ...current, debugAutoPaid: checked }))}
                    />
                  </div>
                  <div className="grid gap-4 sm:grid-cols-2">
                    <div className="space-y-2">
                      <Label htmlFor="alipayAppId">App ID</Label>
                      <Input
                        id="alipayAppId"
                        value={settingsDraft.alipayAppId}
                        onChange={(event) => setSettingsDraft((current) => ({ ...current, alipayAppId: event.target.value }))}
                        placeholder="支付宝 APPID"
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="alipayPid">PID</Label>
                      <Input
                        id="alipayPid"
                        value={settingsDraft.alipayPid}
                        onChange={(event) => setSettingsDraft((current) => ({ ...current, alipayPid: event.target.value }))}
                        placeholder="可选"
                      />
                    </div>
                    <div className="space-y-2">
                      <Label>环境</Label>
                      <Select
                        value={settingsDraft.alipayEnvironment}
                        onValueChange={(value: 'sandbox' | 'production') => setSettingsDraft((current) => ({ ...current, alipayEnvironment: value }))}
                      >
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="sandbox">沙箱</SelectItem>
                          <SelectItem value="production">生产</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="alipayNotifyUrl">回调地址</Label>
                      <Input
                        id="alipayNotifyUrl"
                        value={settingsDraft.alipayNotifyUrl}
                        onChange={(event) => setSettingsDraft((current) => ({ ...current, alipayNotifyUrl: event.target.value }))}
                        placeholder="https://..."
                      />
                    </div>
                  </div>
                </div>

                <div className="space-y-4">
                  <div className="space-y-2">
                    <Label htmlFor="alipayPublicKey">支付宝公钥</Label>
                    <Textarea
                      id="alipayPublicKey"
                      value={settingsDraft.alipayPublicKey}
                      onChange={(event) => setSettingsDraft((current) => ({ ...current, alipayPublicKey: event.target.value }))}
                      placeholder="-----BEGIN PUBLIC KEY-----"
                      className="min-h-[148px] rounded-xl"
                    />
                  </div>
                  <div className="space-y-2">
                    <div className="flex items-center justify-between">
                      <Label htmlFor="alipayPrivateKey">应用私钥</Label>
                      {settings?.privateKeySet && (
                        <Badge variant="outline" className="border-sky-200 bg-sky-50 text-sky-700">
                          已设置
                        </Badge>
                      )}
                    </div>
                    <Textarea
                      id="alipayPrivateKey"
                      value={alipayPrivateKeyInput}
                      onChange={(event) => setAlipayPrivateKeyInput(event.target.value)}
                      placeholder={settings?.privateKeySet ? '留空表示保持现有私钥' : '-----BEGIN PRIVATE KEY-----'}
                      className="min-h-[148px] rounded-xl"
                    />
                  </div>
                </div>
              </div>

              <div className="flex justify-end border-t border-border/70 pt-4">
                <Button type="button" onClick={handleSaveSettings} disabled={savingSettings}>
                  {savingSettings ? <RefreshCw className="mr-2 h-4 w-4 animate-spin" /> : <CreditCard className="mr-2 h-4 w-4" />}
                  保存设置
                </Button>
              </div>
            </CardContent>
          </Card>
        )}
      </TabbedSettingsPage>

      <Dialog open={productDialogOpen} onOpenChange={setProductDialogOpen}>
      <DialogContent className="max-w-2xl border-border/80">
        <DialogHeader>
          <DialogTitle className="text-xl">{editingProduct ? '编辑商品' : '新建商品'}</DialogTitle>
        </DialogHeader>

          <div className="grid gap-4 py-2 lg:grid-cols-2">
            <div className="space-y-2 lg:col-span-2">
              <Label htmlFor="productName">名称</Label>
              <Input
                id="productName"
                value={productForm.name}
                onChange={(event) => setProductForm((current) => ({ ...current, name: event.target.value }))}
                placeholder="月付标准版"
              />
            </div>
            <div className="space-y-2 lg:col-span-2">
              <Label htmlFor="productSummary">摘要</Label>
              <Input
                id="productSummary"
                value={productForm.summary}
                onChange={(event) => setProductForm((current) => ({ ...current, summary: event.target.value }))}
                placeholder="一句说明"
              />
            </div>
            <div className="space-y-2">
              <Label>绑定套餐</Label>
              <Select
                value={productForm.subscriptionPlanId}
                onValueChange={(value) => setProductForm((current) => ({ ...current, subscriptionPlanId: value }))}
              >
                <SelectTrigger>
                  <SelectValue placeholder="选择套餐" />
                </SelectTrigger>
                <SelectContent>
                  {plans.map((plan) => (
                    <SelectItem key={plan.id} value={plan.id}>{plan.name}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="durationDays">时长</Label>
              <Input
                id="durationDays"
                type="number"
                min="1"
                value={productForm.durationDays}
                onChange={(event) => setProductForm((current) => ({ ...current, durationDays: Number(event.target.value || 0) }))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="priceCnyCent">售价</Label>
              <Input
                id="priceCnyCent"
                type="number"
                min="0.01"
                step="0.01"
                value={productPriceInput}
                onChange={(event) => setProductPriceInput(event.target.value)}
              />
              <p className="text-xs text-muted-foreground">单位：元，支持两位小数</p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="sortOrder">排序</Label>
              <Input
                id="sortOrder"
                type="number"
                value={productForm.sortOrder}
                onChange={(event) => setProductForm((current) => ({ ...current, sortOrder: Number(event.target.value || 0) }))}
              />
            </div>
            <div className="flex items-center justify-between rounded-xl border border-border/70 px-4 py-3">
              <div>
                <p className="text-sm font-medium">推荐</p>
                <p className="text-xs text-muted-foreground">前排展示</p>
              </div>
              <Switch
                checked={productForm.isRecommended}
                onCheckedChange={(checked) => setProductForm((current) => ({ ...current, isRecommended: checked }))}
              />
            </div>
            <div className="flex items-center justify-between rounded-xl border border-border/70 px-4 py-3">
              <div>
                <p className="text-sm font-medium">上架</p>
                <p className="text-xs text-muted-foreground">用户可见</p>
              </div>
              <Switch
                checked={productForm.enabled}
                onCheckedChange={(checked) => setProductForm((current) => ({ ...current, enabled: checked }))}
              />
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setProductDialogOpen(false)}>取消</Button>
            <Button onClick={handleSaveProduct} disabled={savingProduct}>
              {savingProduct ? <RefreshCw className="mr-2 h-4 w-4 animate-spin" /> : null}
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
