import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import type { SessionLeaderboardItem } from '@/api/sessions'

interface SessionLeaderboardColumnProps {
  items: SessionLeaderboardItem[]
  loading: boolean
  onRefresh: () => void
}

export function SessionLeaderboardColumn({ items, loading, onRefresh }: SessionLeaderboardColumnProps) {
  return (
    <section className="flex min-h-[640px] flex-col overflow-hidden rounded-[24px] border border-border/70 bg-background/90 shadow-sm">
      <div className="border-b border-border/70 px-5 py-4">
        <div className="flex items-center justify-between gap-3">
          <div className="space-y-1">
            <h2 className="text-sm font-medium text-foreground">最近 5 分钟</h2>
            <p className="text-xs text-muted-foreground">Session 排行</p>
          </div>
          <Button variant="ghost" size="sm" onClick={onRefresh} disabled={loading}>
            刷新
          </Button>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto">
        {loading ? (
          <div className="px-5 py-8 text-sm text-muted-foreground">加载中...</div>
        ) : items.length === 0 ? (
          <div className="px-5 py-8 text-sm text-muted-foreground">暂无数据</div>
        ) : (
          <div className="space-y-2 px-3 py-3">
            {items.map((item, index) => (
              <div key={`${item.userId || item.username}-${index}`} className="flex items-center justify-between gap-3 rounded-2xl border border-border/70 px-4 py-4">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium text-foreground">{item.username}</p>
                  <p className="truncate text-xs text-muted-foreground">{item.userId || '-'}</p>
                </div>
                <Badge variant={index === 0 ? 'default' : 'outline'}>{item.distinctSessionCount}</Badge>
              </div>
            ))}
          </div>
        )}
      </div>
    </section>
  )
}
