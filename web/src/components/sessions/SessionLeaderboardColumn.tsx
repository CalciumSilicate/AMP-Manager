import { Badge } from '@/components/ui/badge'
import { PageSurface } from '@/components/layout/PageScaffold'
import type { SessionLeaderboardItem } from '@/api/sessions'

interface SessionLeaderboardColumnProps {
  items: SessionLeaderboardItem[]
  loading: boolean
}

export function SessionLeaderboardColumn({ items, loading }: SessionLeaderboardColumnProps) {
  return (
    <PageSurface title="排行" bodyClassName="px-0 py-0" className="min-h-[640px]">
      <div className="max-h-[calc(100vh-360px)] min-h-[540px] overflow-y-auto">
        {loading ? (
          <div className="px-5 py-8 text-sm text-muted-foreground">加载中...</div>
        ) : items.length === 0 ? (
          <div className="px-5 py-8 text-sm text-muted-foreground">暂无数据</div>
        ) : (
          <div className="space-y-2 px-3 py-3">
            {items.map((item, index) => (
              <div key={`${item.userId || item.username}-${index}`} className="flex items-center justify-between gap-3 rounded-xl border border-border/70 px-4 py-3">
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
    </PageSurface>
  )
}
