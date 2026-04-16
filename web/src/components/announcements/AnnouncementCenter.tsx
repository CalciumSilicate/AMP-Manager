import { useMemo, useState } from 'react'

import type { Announcement } from '@/api/announcements'
import { formatDateTime } from '@/lib/formatters'
import { cn } from '@/lib/utils'
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
import { ScrollArea } from '@/components/ui/scroll-area'
import { Bell, Pin, Sparkles } from 'lucide-react'

const audienceLabelMap: Record<Announcement['audience'], string> = {
  authenticated: '登录用户',
  new_user: '新用户',
  public: '公开',
}

interface AnnouncementListProps {
  announcements: Announcement[]
  emptyText: string
  onMarkRead?: (announcement: Announcement) => void | Promise<void>
  busyId?: string | null
}

function AnnouncementList({ announcements, emptyText, onMarkRead, busyId }: AnnouncementListProps) {
  if (announcements.length === 0) {
    return (
      <div className="rounded-lg border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">
        {emptyText}
      </div>
    )
  }

  return (
    <div className="space-y-3">
      {announcements.map((announcement) => (
        <article key={announcement.id} className="rounded-xl border bg-background/80 px-4 py-4 shadow-sm">
          <div className="flex items-start justify-between gap-3">
            <div className="space-y-2">
              <div className="flex flex-wrap items-center gap-2">
                <h3 className="text-sm font-semibold text-foreground">{announcement.title}</h3>
                {announcement.pinned ? (
                  <Badge variant="default" className="gap-1">
                    <Pin className="h-3 w-3" />
                    置顶
                  </Badge>
                ) : null}
                <Badge variant="outline">{audienceLabelMap[announcement.audience]}</Badge>
                {'isRead' in announcement && !announcement.isRead ? (
                  <Badge variant="secondary">未读</Badge>
                ) : null}
              </div>
              <p className="whitespace-pre-wrap text-sm leading-6 text-muted-foreground">{announcement.content}</p>
            </div>
            {onMarkRead && !announcement.isRead ? (
              <Button
                variant="outline"
                size="sm"
                onClick={() => void onMarkRead(announcement)}
                disabled={busyId === announcement.id}
                className="shrink-0"
              >
                {busyId === announcement.id ? '处理中...' : '标记已读'}
              </Button>
            ) : null}
          </div>
          <div className="mt-3 flex items-center justify-between gap-3 text-xs text-muted-foreground">
            <span>发布时间 {formatDateTime(announcement.createdAt)}</span>
            {'readAt' in announcement && announcement.readAt ? <span>已读于 {formatDateTime(announcement.readAt)}</span> : null}
          </div>
        </article>
      ))}
    </div>
  )
}

interface AnnouncementCenterProps {
  announcements: Announcement[]
  emptyText?: string
  triggerClassName?: string
  triggerLabel?: string
  triggerVariant?: 'default' | 'outline' | 'ghost' | 'secondary'
  unreadCount?: number
  description?: string
  onMarkRead?: (announcement: Announcement) => void | Promise<void>
  onMarkAllRead?: () => void | Promise<void>
  busyId?: string | null
  busyAll?: boolean
}

export function AnnouncementCenter({
  announcements,
  emptyText = '暂无公告',
  triggerClassName,
  triggerLabel = '公告',
  triggerVariant = 'outline',
  unreadCount = 0,
  description = '查看当前适用的系统公告。',
  onMarkRead,
  onMarkAllRead,
  busyId,
  busyAll = false,
}: AnnouncementCenterProps) {
  const [open, setOpen] = useState(false)

  return (
    <>
      <Button variant={triggerVariant} className={cn('gap-2', triggerClassName)} onClick={() => setOpen(true)}>
        <Bell className="h-4 w-4" />
        <span>{triggerLabel}</span>
        {unreadCount > 0 ? (
          <span className="rounded-full bg-primary px-1.5 py-0.5 text-[10px] font-semibold text-primary-foreground">
            {unreadCount}
          </span>
        ) : null}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>公告中心</DialogTitle>
            <DialogDescription>{description}</DialogDescription>
          </DialogHeader>
          <ScrollArea className="max-h-[65vh] pr-4">
            <AnnouncementList announcements={announcements} emptyText={emptyText} onMarkRead={onMarkRead} busyId={busyId} />
          </ScrollArea>
          {onMarkAllRead ? (
            <DialogFooter>
              <Button variant="outline" onClick={() => setOpen(false)}>关闭</Button>
              <Button onClick={() => void onMarkAllRead()} disabled={busyAll || unreadCount === 0}>
                {busyAll ? '处理中...' : unreadCount > 0 ? `全部标记已读 (${unreadCount})` : '全部已读'}
              </Button>
            </DialogFooter>
          ) : null}
        </DialogContent>
      </Dialog>
    </>
  )
}

interface AnnouncementUnreadDialogProps {
  announcements: Announcement[]
  open: boolean
  onOpenChange: (open: boolean) => void
  onMarkAllRead: () => void | Promise<void>
  busy?: boolean
}

export function AnnouncementUnreadDialog({
  announcements,
  open,
  onOpenChange,
  onMarkAllRead,
  busy = false,
}: AnnouncementUnreadDialogProps) {
  const unreadItems = useMemo(() => announcements.filter((announcement) => !announcement.isRead), [announcements])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Sparkles className="h-4 w-4 text-primary" />
            未读公告
          </DialogTitle>
          <DialogDescription>
            你有 {unreadItems.length} 条尚未处理的公告。标记为已读后，将不再自动弹出。
          </DialogDescription>
        </DialogHeader>
        <ScrollArea className="max-h-[65vh] pr-4">
          <AnnouncementList announcements={unreadItems} emptyText="暂无未读公告" />
        </ScrollArea>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>稍后查看</Button>
          <Button onClick={() => void onMarkAllRead()} disabled={busy || unreadItems.length === 0}>
            {busy ? '处理中...' : '全部标记已读'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
