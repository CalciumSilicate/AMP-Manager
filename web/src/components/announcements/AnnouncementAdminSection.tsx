import { useCallback, useEffect, useMemo, useState } from 'react'

import {
  createAnnouncement,
  deleteAnnouncement,
  listAdminAnnouncements,
  updateAnnouncement,
  type Announcement,
  type AnnouncementAudience,
  type AnnouncementRequest,
} from '@/api/announcements'
import { formatDateTime } from '@/lib/formatters'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'

const audienceOptions: { value: AnnouncementAudience; label: string }[] = [
  { value: 'public', label: '公开' },
  { value: 'authenticated', label: '登录用户' },
  { value: 'new_user', label: '新用户' },
]

const defaultForm: AnnouncementRequest = {
  title: '',
  content: '',
  audience: 'public',
  pinned: false,
  enabled: true,
}

interface Props {
  onMessage: (type: 'success' | 'error', text: string) => void
}

export function AnnouncementAdminSection({ onMessage }: Props) {
  const [announcements, setAnnouncements] = useState<Announcement[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [editing, setEditing] = useState<Announcement | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Announcement | null>(null)
  const [open, setOpen] = useState(false)
  const [formData, setFormData] = useState<AnnouncementRequest>(defaultForm)

  const loadAnnouncements = useCallback(async () => {
    try {
      const data = await listAdminAnnouncements()
      setAnnouncements(data)
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '获取公告失败')
    } finally {
      setLoading(false)
    }
  }, [onMessage])

  useEffect(() => {
    void loadAnnouncements()
  }, [loadAnnouncements])

  const stats = useMemo(() => {
    const enabledCount = announcements.filter((item) => item.enabled).length
    const pinnedCount = announcements.filter((item) => item.pinned).length
    return { total: announcements.length, enabledCount, pinnedCount }
  }, [announcements])

  const openCreate = () => {
    setEditing(null)
    setFormData(defaultForm)
    setOpen(true)
  }

  const openEdit = (announcement: Announcement) => {
    setEditing(announcement)
    setFormData({
      title: announcement.title,
      content: announcement.content,
      audience: announcement.audience,
      pinned: announcement.pinned,
      enabled: announcement.enabled,
    })
    setOpen(true)
  }

  const handleSubmit = async () => {
    if (!formData.title.trim() || !formData.content.trim()) {
      onMessage('error', '请填写标题和正文')
      return
    }

    setSaving(true)
    try {
      if (editing) {
        await updateAnnouncement(editing.id, formData)
        onMessage('success', '公告已更新')
      } else {
        await createAnnouncement(formData)
        onMessage('success', '公告已创建')
      }
      setOpen(false)
      await loadAnnouncements()
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '保存公告失败')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    if (!deleteTarget) return

    try {
      await deleteAnnouncement(deleteTarget.id)
      onMessage('success', '公告已删除')
      setDeleteTarget(null)
      await loadAnnouncements()
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '删除公告失败')
    }
  }

  return (
    <>
      <div className="space-y-4">
        <div className="flex items-center justify-between gap-4">
          <div>
            <h3 className="text-lg font-semibold">公告管理</h3>
            <p className="text-sm text-muted-foreground">管理公开、登录用户和新用户公告。新用户当前按注册后 7 天识别。</p>
          </div>
          <Button onClick={openCreate}>新建公告</Button>
        </div>

        <div className="grid gap-4 md:grid-cols-3">
          <div className="rounded-lg border px-4 py-3">
            <div className="text-sm text-muted-foreground">总公告数</div>
            <div className="mt-1 text-2xl font-semibold">{stats.total}</div>
          </div>
          <div className="rounded-lg border px-4 py-3">
            <div className="text-sm text-muted-foreground">启用中</div>
            <div className="mt-1 text-2xl font-semibold">{stats.enabledCount}</div>
          </div>
          <div className="rounded-lg border px-4 py-3">
            <div className="text-sm text-muted-foreground">置顶公告</div>
            <div className="mt-1 text-2xl font-semibold">{stats.pinnedCount}</div>
          </div>
        </div>

        {loading ? (
          <div className="rounded-lg border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">加载中...</div>
        ) : announcements.length === 0 ? (
          <div className="rounded-lg border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">暂无公告</div>
        ) : (
          <div className="overflow-x-auto rounded-lg border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>标题</TableHead>
                  <TableHead>受众</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead>更新时间</TableHead>
                  <TableHead className="text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {announcements.map((announcement) => (
                  <TableRow key={announcement.id}>
                    <TableCell>
                      <div className="space-y-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-medium">{announcement.title}</span>
                          {announcement.pinned ? <Badge>置顶</Badge> : null}
                        </div>
                        <p className="max-w-[38rem] overflow-hidden text-ellipsis whitespace-nowrap text-xs text-muted-foreground">
                          {announcement.content}
                        </p>
                      </div>
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline">
                        {audienceOptions.find((item) => item.value === announcement.audience)?.label || announcement.audience}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <Badge variant={announcement.enabled ? 'default' : 'secondary'}>
                        {announcement.enabled ? '已启用' : '已停用'}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">{formatDateTime(announcement.updatedAt)}</TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-2">
                        <Button variant="ghost" size="sm" onClick={() => openEdit(announcement)}>编辑</Button>
                        <Button variant="ghost" size="sm" className="text-destructive hover:text-destructive" onClick={() => setDeleteTarget(announcement)}>
                          删除
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}

        <Alert>
          <AlertDescription>
            公开公告会出现在登录/注册页和未登录入口。登录用户会看到适用公告的未读弹窗，并可在顶栏公告中心再次查看。
          </AlertDescription>
        </Alert>
      </div>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editing ? '编辑公告' : '新建公告'}</DialogTitle>
            <DialogDescription>公告正文支持多段文本。建议用简洁句式表达变更、维护或引导信息。</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-2">
              <Label htmlFor="announcement-title">标题</Label>
              <Input
                id="announcement-title"
                maxLength={120}
                value={formData.title}
                onChange={(event) => setFormData((prev) => ({ ...prev, title: event.target.value }))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="announcement-content">正文</Label>
              <Textarea
                id="announcement-content"
                maxLength={10000}
                className="min-h-[180px]"
                value={formData.content}
                onChange={(event) => setFormData((prev) => ({ ...prev, content: event.target.value }))}
              />
            </div>
            <div className="space-y-2">
              <Label>受众</Label>
              <div className="flex flex-wrap gap-2">
                {audienceOptions.map((option) => (
                  <Button
                    key={option.value}
                    type="button"
                    variant={formData.audience === option.value ? 'default' : 'outline'}
                    size="sm"
                    onClick={() => setFormData((prev) => ({ ...prev, audience: option.value }))}
                  >
                    {option.label}
                  </Button>
                ))}
              </div>
            </div>
            <div className="grid gap-4 md:grid-cols-2">
              <div className="flex items-center justify-between rounded-lg border px-4 py-3">
                <div>
                  <div className="text-sm font-medium">置顶</div>
                  <div className="text-xs text-muted-foreground">优先展示在列表顶部</div>
                </div>
                <Switch checked={formData.pinned} onCheckedChange={(checked) => setFormData((prev) => ({ ...prev, pinned: checked }))} />
              </div>
              <div className="flex items-center justify-between rounded-lg border px-4 py-3">
                <div>
                  <div className="text-sm font-medium">启用</div>
                  <div className="text-xs text-muted-foreground">关闭后不会再展示给用户</div>
                </div>
                <Switch checked={formData.enabled} onCheckedChange={(checked) => setFormData((prev) => ({ ...prev, enabled: checked }))} />
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>取消</Button>
            <Button onClick={handleSubmit} disabled={saving}>{saving ? '保存中...' : '保存公告'}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!deleteTarget} onOpenChange={(nextOpen) => !nextOpen && setDeleteTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>删除公告</DialogTitle>
            <DialogDescription>删除后将同时移除该公告的用户已读记录。</DialogDescription>
          </DialogHeader>
          <div className="rounded-lg border px-4 py-3 text-sm text-muted-foreground">
            {deleteTarget?.title}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteTarget(null)}>取消</Button>
            <Button variant="destructive" onClick={() => void handleDelete()}>确认删除</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
