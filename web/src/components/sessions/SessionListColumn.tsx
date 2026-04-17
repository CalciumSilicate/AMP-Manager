import { formatDateTimeWithSeconds } from '@/lib/formatters'
import type { SessionListItem } from '@/api/sessions'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

interface SessionListColumnProps {
  items: SessionListItem[]
  total: number
  page: number
  pageSize: number
  queryInput: string
  loading: boolean
  fetching: boolean
  selectedSessionId: string | null
  onQueryInputChange: (value: string) => void
  onSearch: () => void
  onRefresh: () => void
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
  queryInput,
  loading,
  fetching,
  selectedSessionId,
  onQueryInputChange,
  onSearch,
  onRefresh,
  onSelect,
  onPageChange,
}: SessionListColumnProps) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  return (
    <section className="flex min-h-[720px] flex-col">
      <div className="border-b border-border/70 px-5 py-4">
        <div className="flex items-center justify-between gap-3">
          <div className="space-y-1">
            <h2 className="text-sm font-medium text-foreground">Session 列表</h2>
            <p className="text-xs text-muted-foreground">{total} 条，active 优先</p>
          </div>
          <Button variant="ghost" size="sm" onClick={onRefresh} disabled={fetching}>
            刷新
          </Button>
        </div>
        <div className="mt-3 flex items-center gap-2">
          <Input
            value={queryInput}
            onChange={(event) => onQueryInputChange(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                onSearch()
              }
            }}
            placeholder="搜索 session / 用户"
            className="h-9"
          />
          <Button variant="outline" size="sm" onClick={onSearch}>
            搜索
          </Button>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto">
        {loading ? (
          <div className="px-5 py-8 text-sm text-muted-foreground">加载中...</div>
        ) : items.length === 0 ? (
          <div className="px-5 py-8 text-sm text-muted-foreground">暂无 Session</div>
        ) : (
          <div className="divide-y divide-border/70">
            {items.map((item) => (
              <button
                key={item.id}
                type="button"
                onClick={() => onSelect(item.sessionId)}
                className={cn(
                  'w-full px-5 py-4 text-left transition-colors hover:bg-muted/40',
                  selectedSessionId === item.sessionId ? 'bg-muted/50' : '',
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
                  <span className="text-right">{item.lastModel || '-'}</span>
                  <span className="col-span-2 truncate">{item.lastSeenAt ? formatDateTimeWithSeconds(item.lastSeenAt) : '-'}</span>
                </div>
              </button>
            ))}
          </div>
        )}
      </div>

      <div className="border-t border-border/70 px-5 py-4">
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
      </div>
    </section>
  )
}
