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

export type PurchasePaymentStatus = 'pending' | 'paid' | 'expired' | 'closed' | 'failed'
export type PurchaseFulfillmentStatus = 'pending' | 'fulfilled' | 'failed'
export type AlipayEnvironment = 'sandbox' | 'production'

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
  limit?: number
}

export async function listPurchaseOrdersAdmin(filters: AdminOrderFilters = {}): Promise<PurchaseOrderListResponse> {
  const params = new URLSearchParams()
  if (filters.paymentStatus) params.set('paymentStatus', filters.paymentStatus)
  if (filters.fulfillmentStatus) params.set('fulfillmentStatus', filters.fulfillmentStatus)
  if (filters.username) params.set('username', filters.username)
  if (filters.productId) params.set('productId', filters.productId)
  if (filters.limit) params.set('limit', String(filters.limit))
  const query = params.toString()
  return fetchJson<PurchaseOrderListResponse>(`${API_BASE}/admin/purchase/orders${query ? `?${query}` : ''}`)
}

export async function refreshPurchaseOrderAdmin(orderNo: string): Promise<PurchaseOrder> {
  return fetchJson<PurchaseOrder>(`${API_BASE}/admin/purchase/orders/${encodeURIComponent(orderNo)}/refresh`, {
    method: 'POST',
  })
}
