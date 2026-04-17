import type { UserSubscriptionResponse } from './subscription'
import { authFetch } from './client'

const API_BASE = '/api'

async function fetchJson<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await authFetch(url, options)
  let data: unknown

  try {
    data = await res.json()
  } catch {
    data = undefined
  }

  if (!res.ok) {
    const payload = data as { error?: string; message?: string } | undefined
    throw new Error(payload?.error || payload?.message || `请求失败 (${res.status})`)
  }

  return data as T
}

export type PurchasePaymentStatus = 'pending' | 'paid' | 'expired' | 'refunded' | 'closed' | 'failed'
export type PurchaseFulfillmentStatus = 'pending' | 'fulfilled' | 'failed'
export type AlipayEnvironment = 'sandbox' | 'production'
export type PurchaseManualSettlementBatchMode = 'single' | 'batch'

export interface PurchaseSettingsResponse {
  purchaseEnabled: boolean
  debugAutoPaid: boolean
  alipayAppId: string
  alipayPid: string
  alipayEnvironment: AlipayEnvironment
  alipayNotifyUrl: string
  alipayPublicKey: string
  privateKeySet: boolean
  paymentConfigured: boolean
  balanceTopupPriceCnyPerUsd: number
}

export interface PurchaseSettingsRequest {
  purchaseEnabled: boolean
  debugAutoPaid: boolean
  alipayAppId: string
  alipayPid: string
  alipayEnvironment: AlipayEnvironment
  alipayNotifyUrl: string
  alipayPublicKey: string
  alipayPrivateKey?: string
  balanceTopupPriceCnyPerUsd?: number
}

export interface PurchaseProduct {
  id: string
  name: string
  summary: string
  subscriptionPlanId: string
  subscriptionPlanName: string
  subscriptionPlanUpgradeRank: number
  durationDays: number
  priceCnyCent: number
  groupName: string
  groupSort: number
  isRecommended: boolean
  sortOrder: number
  enabled: boolean
  createdAt: string
  updatedAt: string
}

export interface PurchaseProductRequest {
  name: string
  summary: string
  subscriptionPlanId: string
  durationDays: number
  priceCnyCent: number
  groupName: string
  groupSort: number
  isRecommended: boolean
  sortOrder: number
  enabled: boolean
}

export interface PurchaseOrder {
  id: string
  orderNo: string
  userId: string
  username?: string
  productId: string
  productName: string
  productSummary: string
  subscriptionPlanId: string
  subscriptionPlanName: string
  durationDays: number
  amountCnyCent: number
  orderKind: 'subscription' | 'balance_topup'
  deliveryMode: 'account' | 'redeem_code'
  balanceTopupMicros: number
  paymentChannel: 'alipay'
  paymentStatus: PurchasePaymentStatus
  fulfillmentStatus: PurchaseFulfillmentStatus
  upgradeSourcePlanId: string
  upgradeSourcePlanName: string
  upgradeSourceExpiresAt: string | null
  upgradeCreditCnyCent: number
  upgradeLockedTargetSeconds: number
  generatedRedeemCodeId: string
  generatedRedeemCode: string
  generatedRedeemCodeMask: string
  generatedRedeemCodeStatus: string
  generatedRedeemedAt: string | null
  alipayTradeNo: string
  manualSettlementDone: boolean
  paymentQrCode: string
  paymentQrUrl: string
  paymentQrImageDataUrl: string
  expiresAt: string | null
  paidAt: string | null
  fulfilledAt: string | null
  failureReason: string
  canRefresh: boolean
  createdAt: string
  updatedAt: string
}

export interface PurchaseCatalogResponse {
  purchaseEnabled: boolean
  debugAutoPaid: boolean
  paymentConfigured: boolean
  renewalRule: string
  currentSubscription: UserSubscriptionResponse | null
  products: PurchaseProduct[]
  balanceTopupEnabled: boolean
  balanceTopupPriceCnyPerUsd: number
}

export interface PurchaseOrderListResponse {
  items: PurchaseOrder[]
  total: number
}

export interface PurchaseWebhookTarget {
  id: string
  name: string
  targetUrl: string
  bodyTemplate: string
  headersTemplate: string
  enabled: boolean
  createdAt: string
  updatedAt: string
}

export interface PurchaseWebhookTargetRequest {
  name: string
  targetUrl: string
  bodyTemplate: string
  headersTemplate: string
  enabled: boolean
}

export interface PurchaseWebhookTestResponse {
  ok: boolean
  responseStatusCode: number
  responseHeaders: Record<string, string>
  responseBody: string
  variables: Record<string, string>
}

export interface PurchaseOrderPaymentStatusHistory {
  id: string
  orderId: string
  orderNo: string
  fromStatus: PurchasePaymentStatus
  toStatus: PurchasePaymentStatus
  note: string
  createdBy: string
  createdAt: string
}

export interface PurchaseManualSettlementBatch {
  id: string
  batchNo: string
  mode: PurchaseManualSettlementBatchMode
  debugSettlement: boolean
  createdBy: string
  note: string
  orderCount: number
  totalAmountCnyCent: number
  createdAt: string
}

export interface PurchaseManualSettlementPreviewResponse {
  items: PurchaseOrder[]
  total: number
  totalAmountCnyCent: number
}

export async function getPurchaseCatalog(): Promise<PurchaseCatalogResponse> {
  return fetchJson<PurchaseCatalogResponse>(`${API_BASE}/me/purchase/products`)
}

export async function createPurchaseOrder(productId: string): Promise<PurchaseOrder> {
  return fetchJson<PurchaseOrder>(`${API_BASE}/me/purchase/orders`, {
    method: 'POST',
    body: JSON.stringify({ productId, deliveryMode: 'account' }),
  })
}

export async function createPurchaseOrderWithMode(productId: string, deliveryMode: 'account' | 'redeem_code'): Promise<PurchaseOrder> {
  return fetchJson<PurchaseOrder>(`${API_BASE}/me/purchase/orders`, {
    method: 'POST',
    body: JSON.stringify({ productId, deliveryMode }),
  })
}

export async function createBalanceTopupOrder(amountUsd: number): Promise<PurchaseOrder> {
  return fetchJson<PurchaseOrder>(`${API_BASE}/me/purchase/balance-topup/orders`, {
    method: 'POST',
    body: JSON.stringify({ amountUsd }),
  })
}

export async function listMyPurchaseOrders(): Promise<PurchaseOrderListResponse> {
  return fetchJson<PurchaseOrderListResponse>(`${API_BASE}/me/purchase/orders`)
}

export async function getMyPurchaseOrder(orderNo: string): Promise<PurchaseOrder> {
  return fetchJson<PurchaseOrder>(`${API_BASE}/me/purchase/orders/${encodeURIComponent(orderNo)}`)
}

export async function refreshMyPurchaseOrder(orderNo: string): Promise<PurchaseOrder> {
  return fetchJson<PurchaseOrder>(`${API_BASE}/me/purchase/orders/${encodeURIComponent(orderNo)}/refresh`, {
    method: 'POST',
  })
}

export async function getPurchaseSettings(): Promise<PurchaseSettingsResponse> {
  return fetchJson<PurchaseSettingsResponse>(`${API_BASE}/admin/purchase/settings`)
}

export async function updatePurchaseSettings(payload: PurchaseSettingsRequest): Promise<{ message: string; settings: PurchaseSettingsResponse }> {
  return fetchJson<{ message: string; settings: PurchaseSettingsResponse }>(`${API_BASE}/admin/purchase/settings`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export async function listPurchaseWebhookTargets(): Promise<PurchaseWebhookTarget[]> {
  const data = await fetchJson<{ items: PurchaseWebhookTarget[] }>(`${API_BASE}/admin/purchase/webhooks`)
  return data.items || []
}

export async function createPurchaseWebhookTarget(payload: PurchaseWebhookTargetRequest): Promise<PurchaseWebhookTarget> {
  return fetchJson<PurchaseWebhookTarget>(`${API_BASE}/admin/purchase/webhooks`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function updatePurchaseWebhookTarget(id: string, payload: PurchaseWebhookTargetRequest): Promise<PurchaseWebhookTarget> {
  return fetchJson<PurchaseWebhookTarget>(`${API_BASE}/admin/purchase/webhooks/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export async function setPurchaseWebhookTargetEnabled(id: string, enabled: boolean): Promise<void> {
  await fetchJson<{ message: string }>(`${API_BASE}/admin/purchase/webhooks/${encodeURIComponent(id)}/enabled`, {
    method: 'PATCH',
    body: JSON.stringify({ enabled }),
  })
}

export async function deletePurchaseWebhookTarget(id: string): Promise<void> {
  await fetchJson<{ message: string }>(`${API_BASE}/admin/purchase/webhooks/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export async function testPurchaseWebhookTarget(id: string): Promise<PurchaseWebhookTestResponse> {
  return fetchJson<PurchaseWebhookTestResponse>(`${API_BASE}/admin/purchase/webhooks/${encodeURIComponent(id)}/test`, {
    method: 'POST',
  })
}

export async function listPurchaseProductsAdmin(): Promise<PurchaseProduct[]> {
  const data = await fetchJson<{ products: PurchaseProduct[] }>(`${API_BASE}/admin/purchase/products`)
  return data.products || []
}

export async function createPurchaseProduct(payload: PurchaseProductRequest): Promise<PurchaseProduct> {
  return fetchJson<PurchaseProduct>(`${API_BASE}/admin/purchase/products`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function updatePurchaseProduct(id: string, payload: PurchaseProductRequest): Promise<PurchaseProduct> {
  return fetchJson<PurchaseProduct>(`${API_BASE}/admin/purchase/products/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export async function deletePurchaseProduct(id: string): Promise<void> {
  await fetchJson<{ message: string }>(`${API_BASE}/admin/purchase/products/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export async function setPurchaseProductEnabled(id: string, enabled: boolean): Promise<void> {
  await fetchJson<{ message: string }>(`${API_BASE}/admin/purchase/products/${encodeURIComponent(id)}/enabled`, {
    method: 'PATCH',
    body: JSON.stringify({ enabled }),
  })
}

export interface AdminOrderFilters {
  paymentStatus?: PurchasePaymentStatus | ''
  fulfillmentStatus?: PurchaseFulfillmentStatus | ''
  username?: string
  productId?: string
  page?: number
  pageSize?: number
}

export async function listPurchaseOrdersAdmin(filters: AdminOrderFilters = {}): Promise<PurchaseOrderListResponse> {
  const params = new URLSearchParams()
  if (filters.paymentStatus) params.set('paymentStatus', filters.paymentStatus)
  if (filters.fulfillmentStatus) params.set('fulfillmentStatus', filters.fulfillmentStatus)
  if (filters.username) params.set('username', filters.username)
  if (filters.productId) params.set('productId', filters.productId)
  if (filters.page) params.set('page', String(filters.page))
  if (filters.pageSize) params.set('pageSize', String(filters.pageSize))
  const query = params.toString()
  return fetchJson<PurchaseOrderListResponse>(`${API_BASE}/admin/purchase/orders${query ? `?${query}` : ''}`)
}

export async function refreshPurchaseOrderAdmin(orderNo: string): Promise<PurchaseOrder> {
  return fetchJson<PurchaseOrder>(`${API_BASE}/admin/purchase/orders/${encodeURIComponent(orderNo)}/refresh`, {
    method: 'POST',
  })
}

export async function updatePurchaseOrderPaymentStatus(orderNo: string, paymentStatus: 'paid' | 'expired' | 'refunded', note: string): Promise<PurchaseOrder> {
  return fetchJson<PurchaseOrder>(`${API_BASE}/admin/purchase/orders/${encodeURIComponent(orderNo)}/payment-status`, {
    method: 'POST',
    body: JSON.stringify({ paymentStatus, note }),
  })
}

export async function listPurchaseOrderPaymentStatusHistory(orderNo: string): Promise<PurchaseOrderPaymentStatusHistory[]> {
  const data = await fetchJson<{ items: PurchaseOrderPaymentStatusHistory[] }>(`${API_BASE}/admin/purchase/orders/${encodeURIComponent(orderNo)}/payment-status-history`)
  return data.items || []
}

export async function createSinglePurchaseManualSettlement(orderNo: string, note: string): Promise<PurchaseManualSettlementBatch> {
  const data = await fetchJson<{ batch: PurchaseManualSettlementBatch }>(`${API_BASE}/admin/purchase/orders/${encodeURIComponent(orderNo)}/manual-settlement`, {
    method: 'POST',
    body: JSON.stringify({ note }),
  })
  return data.batch
}

export async function previewBatchPurchaseManualSettlement(payload: { paidFrom: string; paidTo: string; limit?: number }): Promise<PurchaseManualSettlementPreviewResponse> {
  return fetchJson<PurchaseManualSettlementPreviewResponse>(`${API_BASE}/admin/purchase/manual-settlements/preview`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function createBatchPurchaseManualSettlement(payload: { orderNos?: string[]; paidFrom?: string; paidTo?: string; note: string; debugSettlement?: boolean }): Promise<PurchaseManualSettlementBatch> {
  const data = await fetchJson<{ batch: PurchaseManualSettlementBatch }>(`${API_BASE}/admin/purchase/manual-settlements/confirm`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
  return data.batch
}
