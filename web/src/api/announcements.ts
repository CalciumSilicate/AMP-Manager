import { authFetch } from './client'

const API_BASE = '/api'

export type AnnouncementAudience = 'authenticated' | 'new_user' | 'public'

export interface Announcement {
  id: string
  title: string
  content: string
  audience: AnnouncementAudience
  pinned: boolean
  enabled: boolean
  isRead: boolean
  readAt?: string
  createdAt: string
  updatedAt: string
}

export interface AnnouncementRequest {
  title: string
  content: string
  audience: AnnouncementAudience
  pinned: boolean
  enabled: boolean
}

async function parseAnnouncementListResponse(response: Response, fallback: string): Promise<Announcement[]> {
  if (!response.ok) {
    const data = await response.json()
    throw new Error(data.error || fallback)
  }

  const data = await response.json()
  return Array.isArray(data.announcements) ? data.announcements : []
}

export async function listPublicAnnouncements(): Promise<Announcement[]> {
  const response = await fetch(`${API_BASE}/public/announcements`)
  return parseAnnouncementListResponse(response, '获取公告失败')
}

export async function listMyAnnouncements(): Promise<Announcement[]> {
  const response = await authFetch(`${API_BASE}/me/announcements`)
  return parseAnnouncementListResponse(response, '获取公告失败')
}

export async function markAnnouncementRead(id: string): Promise<void> {
  const response = await authFetch(`${API_BASE}/me/announcements/${encodeURIComponent(id)}/read`, {
    method: 'POST',
  })

  if (!response.ok) {
    const data = await response.json()
    throw new Error(data.error || '标记公告已读失败')
  }
}

export async function listAdminAnnouncements(): Promise<Announcement[]> {
  const response = await authFetch(`${API_BASE}/admin/announcements`)
  return parseAnnouncementListResponse(response, '获取公告失败')
}

export async function createAnnouncement(payload: AnnouncementRequest): Promise<Announcement> {
  const response = await authFetch(`${API_BASE}/admin/announcements`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })

  if (!response.ok) {
    const data = await response.json()
    throw new Error(data.error || '创建公告失败')
  }

  return response.json()
}

export async function updateAnnouncement(id: string, payload: AnnouncementRequest): Promise<Announcement> {
  const response = await authFetch(`${API_BASE}/admin/announcements/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })

  if (!response.ok) {
    const data = await response.json()
    throw new Error(data.error || '更新公告失败')
  }

  return response.json()
}

export async function deleteAnnouncement(id: string): Promise<void> {
  const response = await authFetch(`${API_BASE}/admin/announcements/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })

  if (!response.ok) {
    const data = await response.json()
    throw new Error(data.error || '删除公告失败')
  }
}
