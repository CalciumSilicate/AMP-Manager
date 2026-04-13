import { authFetch } from './client'

const API_BASE = '/api'

export interface UserInfo {
  id: string
  username: string
  isAdmin: boolean
  balanceMicros: number
  balanceUsd: string
  groupIds: string[]
  groupNames: string[]
  createdAt: string
  updatedAt: string
}

export interface UserListPage {
  items: UserInfo[]
  total: number
  page: number
  pageSize: number
}

export async function listUsers(): Promise<UserInfo[]> {
  const res = await authFetch(`${API_BASE}/admin/users`)
  if (!res.ok) throw new Error('获取用户列表失败')
  return res.json()
}

export async function listUsersPaged(page = 1, pageSize = 20): Promise<UserListPage> {
  const params = new URLSearchParams({
    page: String(page),
    pageSize: String(pageSize),
  })
  const res = await authFetch(`${API_BASE}/admin/users/paged?${params.toString()}`)
  if (!res.ok) throw new Error('获取用户列表失败')
  return res.json()
}

export async function setUserAdmin(userId: string, isAdmin: boolean): Promise<void> {
  const res = await authFetch(`${API_BASE}/admin/users/${userId}/admin`, {
    method: 'PATCH',
    body: JSON.stringify({ isAdmin }),
  })
  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '设置权限失败')
  }
}

export async function deleteUser(userId: string): Promise<void> {
  const res = await authFetch(`${API_BASE}/admin/users/${userId}`, {
    method: 'DELETE',
  })
  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '删除用户失败')
  }
}

export async function resetUserPassword(userId: string, newPassword: string): Promise<void> {
  const res = await authFetch(`${API_BASE}/admin/users/${userId}/reset-password`, {
    method: 'POST',
    body: JSON.stringify({ newPassword }),
  })
  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '重置密码失败')
  }
}

export async function changePassword(oldPassword: string, newPassword: string): Promise<void> {
  const res = await authFetch(`${API_BASE}/me/password`, {
    method: 'PUT',
    body: JSON.stringify({ oldPassword, newPassword }),
  })
  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '修改密码失败')
  }
}

export async function setUserGroups(userId: string, groupIds: string[]): Promise<void> {
  const res = await authFetch(`${API_BASE}/admin/users/${userId}/group`, {
    method: 'PATCH',
    body: JSON.stringify({ groupIds }),
  })
  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '设置分组失败')
  }
}

export async function changeUsername(newUsername: string): Promise<void> {
  const res = await authFetch(`${API_BASE}/me/username`, {
    method: 'PUT',
    body: JSON.stringify({ newUsername }),
  })
  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '修改用户名失败')
  }
}

export interface BalanceInfo {
  balanceMicros: number
  balanceUsd: string
}

export async function getMyBalance(): Promise<BalanceInfo> {
  const res = await authFetch(`${API_BASE}/me/balance`)
  if (!res.ok) throw new Error('获取余额失败')
  return res.json()
}

export async function topUpUser(userId: string, amountUsd: number): Promise<BalanceInfo & { message: string }> {
  const res = await authFetch(`${API_BASE}/admin/users/${userId}/topup`, {
    method: 'POST',
    body: JSON.stringify({ amountUsd }),
  })
  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || '充值失败')
  }
  return res.json()
}
