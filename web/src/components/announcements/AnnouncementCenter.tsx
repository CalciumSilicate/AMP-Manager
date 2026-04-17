import { useMemo, useState } from 'react'

import type { Announcement } from '@/api/announcements'
import type { SiteContactConfig } from '@/api/system'
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
import { Bell, ExternalLink, MessageCircle, Pin, Sparkles } from 'lucide-react'

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
  compact?: boolean
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
  compact = false,
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
      <Button
        variant={triggerVariant}
        size={compact ? 'icon' : undefined}
        className={cn(compact ? 'relative' : 'gap-2', triggerClassName)}
        aria-label={triggerLabel}
        onClick={() => setOpen(true)}
      >
        <Bell className="h-4 w-4" />
        {!compact ? <span>{triggerLabel}</span> : null}
        {unreadCount > 0 ? (
          <span
            className={cn(
              'bg-primary text-[10px] font-semibold text-primary-foreground',
              compact
                ? 'absolute -right-1 -top-1 flex h-4 min-w-4 items-center justify-center rounded-full px-1'
                : 'rounded-full px-1.5 py-0.5',
            )}
          >
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

interface ContactCenterProps {
  contact: SiteContactConfig
  triggerClassName?: string
  triggerLabel?: string
  triggerVariant?: 'default' | 'outline' | 'ghost' | 'secondary'
  compact?: boolean
}

export function ContactCenter({
  contact,
  triggerClassName,
  triggerLabel = '联系方式',
  triggerVariant = 'outline',
  compact = false,
}: ContactCenterProps) {
  const [open, setOpen] = useState(false)

  if (!contact.enabled) {
    return null
  }

  return (
    <>
      <Button
        variant={triggerVariant}
        size={compact ? 'icon' : undefined}
        className={cn(compact ? '' : 'gap-2', triggerClassName)}
        aria-label={triggerLabel}
        onClick={() => setOpen(true)}
      >
        <MessageCircle className="h-4 w-4" />
        {!compact ? <span>{triggerLabel}</span> : null}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>联系方式</DialogTitle>
            <DialogDescription>{contact.description}</DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-5 py-2 sm:flex-row sm:items-start">
            <div className="flex h-56 w-full items-center justify-center rounded-xl border bg-muted/20 sm:w-56">
              {contact.qrCodeImageDataUrl ? (
                <img src={contact.qrCodeImageDataUrl} alt={`${contact.title} 二维码`} className="h-48 w-48 object-contain" />
              ) : (
                <div className="px-4 text-center text-sm text-muted-foreground">未生成二维码</div>
              )}
            </div>
            <div className="min-w-0 flex-1 space-y-3">
              <a
                href={contact.link}
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-2 text-base font-semibold text-primary hover:underline"
              >
                <span className="break-all">{contact.title}</span>
                <ExternalLink className="h-4 w-4" />
              </a>
              <p className="whitespace-pre-wrap text-sm leading-6 text-muted-foreground">{contact.description}</p>
              <p className="break-all text-xs text-muted-foreground">{contact.link}</p>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>关闭</Button>
          </DialogFooter>
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
