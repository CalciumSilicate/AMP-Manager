import { formatDateTimeWithSeconds } from '@/lib/formatters'
import type { SessionListItem } from '@/api/sessions'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { PageSurface } from '@/components/layout/PageScaffold'
import { cn } from '@/lib/utils'

interface SessionListColumnProps {
  items: SessionListItem[]
  total: number
  page: number
  pageSize: number
  loading: boolean
  fetching: boolean
  selectedSessionId: string | null
  onSelect: (sessionId: string) => void
  onPageChange: (page: number) => void
}

function getStateBadgeVariant(item: SessionListItem): 'default' | 'secondary' | 'outline' {
  if (item.active) return 'default'
  if (item.state === 'expired') return 'outline'
  return 'secondary'
}

export function SessionListColumn({
  items,
  total,
  page,
  pageSize,
  loading,
  fetching,
  selectedSessionId,
  onSelect,
  onPageChange,
}: SessionListColumnProps) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  return (
    <PageSurface
      title="列表"
      actions={fetching ? <span className="text-xs text-muted-foreground">刷新中</span> : <span className="text-xs text-muted-foreground">{total}</span>}
      bodyClassName="px-0 py-0"
      footer={(
        <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
          <span>第 {page} / {totalPages} 页</span>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => onPageChange(page - 1)}>
              上一页
            </Button>
            <Button variant="outline" size="sm" disabled={page >= totalPages} onClick={() => onPageChange(page + 1)}>
              下一页
            </Button>
          </div>
        </div>
      )}
      className="min-h-[640px]"
    >
      <div className="max-h-[calc(100vh-360px)] min-h-[540px] overflow-y-auto">
        {loading ? (
          <div className="px-5 py-8 text-sm text-muted-foreground">加载中...</div>
        ) : items.length === 0 ? (
          <div className="px-5 py-8 text-sm text-muted-foreground">暂无 Session</div>
        ) : (
          <div className="space-y-2 px-3 py-3">
            {items.map((item) => (
              <button
                key={item.id}
                type="button"
                onClick={() => onSelect(item.sessionId)}
                className={cn(
                  'w-full rounded-xl border border-border/70 px-4 py-3 text-left transition-colors hover:bg-muted/30',
                  selectedSessionId === item.sessionId ? 'border-primary/35 bg-primary/[0.06] shadow-sm' : 'bg-background',
                )}
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0 space-y-1">
                    <p className="truncate font-mono text-xs text-foreground">{item.sessionId}</p>
                    <p className="truncate text-sm text-foreground">{item.username || item.userId || '-'}</p>
                  </div>
                  <Badge variant={getStateBadgeVariant(item)} className="shrink-0">
                    {item.active ? 'Active' : item.state}
                  </Badge>
                </div>
                <div className="mt-3 grid grid-cols-2 gap-2 text-xs text-muted-foreground">
                  <span>请求 {item.requestCount}</span>
                  <span className="truncate text-right">{item.lastModel || '-'}</span>
                  <span className="col-span-2 truncate">{item.lastSeenAt ? formatDateTimeWithSeconds(item.lastSeenAt) : '-'}</span>
                </div>
              </button>
            ))}
          </div>
        )}
      </div>
    </PageSurface>
  )
}
