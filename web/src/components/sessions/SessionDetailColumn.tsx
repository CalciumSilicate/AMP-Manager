import { formatDateTimeWithSeconds } from '@/lib/formatters'
import type { SessionDetail, SessionListItem } from '@/api/sessions'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { PageSurface } from '@/components/layout/PageScaffold'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Separator } from '@/components/ui/separator'
import { StatusBadge } from '@/components/StatusBadge'
import { cn } from '@/lib/utils'

interface SessionDetailColumnProps {
  selectedSessionId: string | null
  detail: SessionDetail | null
  loading: boolean
  error: string
  searchValue: string
  searchResults: SessionListItem[]
  searchLoading: boolean
  onSearchValueChange: (value: string) => void
  onSelectSession: (sessionId: string) => void
}

interface SummaryItemProps {
  label: string
  value: string | number
}

function SummaryItem({ label, value }: SummaryItemProps) {
  return (
    <div className="space-y-1">
      <p className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">{label}</p>
      <p className="text-sm font-medium text-foreground">{value}</p>
    </div>
  )
}

export function SessionDetailColumn({
  selectedSessionId,
  detail,
  loading,
  error,
  searchValue,
  searchResults,
  searchLoading,
  onSearchValueChange,
  onSelectSession,
}: SessionDetailColumnProps) {
  const selectedState = detail?.session.active ? 'Active' : (detail?.session.state || 'Idle')

  return (
    <PageSurface
      title="详情"
      actions={detail ? <Badge variant={detail.session.active ? 'default' : 'secondary'}>{selectedState}</Badge> : null}
      bodyClassName="space-y-4"
      className="min-h-[640px]"
    >
      <div className="space-y-3">
        <Input
          value={searchValue}
          onChange={(event) => onSearchValueChange(event.target.value)}
          placeholder="快速切换"
          className="h-10"
        />
        <RadioGroup
          value={selectedSessionId || ''}
          onValueChange={onSelectSession}
          className="max-h-28 overflow-y-auto rounded-xl border border-border/70 bg-background/70 p-2"
        >
          {searchLoading ? (
            <div className="px-2 py-1 text-xs text-muted-foreground">搜索中...</div>
          ) : searchResults.length === 0 ? (
            <div className="px-2 py-1 text-xs text-muted-foreground">无匹配项</div>
          ) : (
            searchResults.map((item) => (
              <label
                key={item.id}
                className={cn(
                  'flex cursor-pointer items-center gap-3 rounded-lg px-2 py-2 text-sm hover:bg-muted/40',
                  selectedSessionId === item.sessionId ? 'bg-muted/50' : '',
                )}
              >
                <RadioGroupItem value={item.sessionId} />
                <div className="min-w-0">
                  <p className="truncate font-mono text-xs text-foreground">{item.sessionId}</p>
                  <p className="truncate text-xs text-muted-foreground">{item.username || item.userId || '-'}</p>
                </div>
              </label>
            ))
          )}
        </RadioGroup>
      </div>

      <div className="max-h-[calc(100vh-420px)] min-h-[430px] overflow-y-auto pr-1">
        {loading ? (
          <div className="text-sm text-muted-foreground">加载中...</div>
        ) : error ? (
          <div className="text-sm text-destructive">{error}</div>
        ) : !detail ? (
          <div className="text-sm text-muted-foreground">选择 Session 查看详情</div>
        ) : (
          <div className="space-y-5">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0 space-y-1">
                <p className="truncate font-mono text-xs text-muted-foreground">{detail.session.sessionId}</p>
                <p className="truncate text-base font-semibold text-foreground">{detail.session.username || detail.session.userId || '-'}</p>
              </div>
            </div>

            <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
              <div className="rounded-xl border border-border/70 bg-muted/20 px-4 py-3"><SummaryItem label="请求" value={detail.session.requestCount} /></div>
              <div className="rounded-xl border border-border/70 bg-muted/20 px-4 py-3"><SummaryItem label="API Keys" value={detail.distinctApiKeyCount ?? '-'} /></div>
              <div className="rounded-xl border border-border/70 bg-muted/20 px-4 py-3"><SummaryItem label="模型" value={detail.distinctModelCount ?? '-'} /></div>
              <div className="rounded-xl border border-border/70 bg-muted/20 px-4 py-3"><SummaryItem label="首次" value={detail.session.firstSeenAt ? formatDateTimeWithSeconds(detail.session.firstSeenAt) : '-'} /></div>
              <div className="rounded-xl border border-border/70 bg-muted/20 px-4 py-3"><SummaryItem label="最近" value={detail.session.lastSeenAt ? formatDateTimeWithSeconds(detail.session.lastSeenAt) : '-'} /></div>
              <div className="rounded-xl border border-border/70 bg-muted/20 px-4 py-3"><SummaryItem label="成本" value={detail.totalCostUsd ? `$${detail.totalCostUsd}` : '-'} /></div>
            </div>

            <Separator />

            <div className="space-y-3">
              <div className="flex items-center justify-between gap-3">
                <h3 className="text-sm font-medium text-foreground">时间线</h3>
                <span className="text-xs text-muted-foreground">{detail.timeline.length}</span>
              </div>

              {detail.timeline.length === 0 ? (
                <div className="text-sm text-muted-foreground">暂无请求</div>
              ) : (
                <div className="divide-y divide-border/70 rounded-xl border border-border/70">
                  {detail.timeline.map((item) => (
                    <div key={item.requestLogId || `${item.sessionId}-${item.createdAt}`} className="space-y-2 px-4 py-3">
                      <div className="flex items-center justify-between gap-3">
                        <span className="text-xs text-muted-foreground">{item.createdAt ? formatDateTimeWithSeconds(item.createdAt) : '-'}</span>
                        {typeof item.statusCode === 'number' ? <StatusBadge status={item.statusCode} /> : <span className="text-xs text-muted-foreground">-</span>}
                      </div>
                      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-foreground">
                        <span>{item.model || '-'}</span>
                        <span className="text-muted-foreground">{item.channelName || '-'}</span>
                        <span className="text-muted-foreground">{typeof item.latencyMs === 'number' ? `${item.latencyMs}ms` : '-'}</span>
                      </div>
                      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                        <span>{item.method || '-'}</span>
                        <span className="truncate">{item.path || '-'}</span>
                        <span>{typeof item.inputTokens === 'number' || typeof item.outputTokens === 'number' ? `${item.inputTokens ?? 0} / ${item.outputTokens ?? 0}` : '- / -'}</span>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </PageSurface>
  )
}
