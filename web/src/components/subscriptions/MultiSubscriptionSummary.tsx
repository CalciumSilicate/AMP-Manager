import type { UserSubscriptionResponse } from '@/api/subscription'
import { Badge } from '@/components/ui/badge'
import { formatDateTime } from '@/lib/formatters'

interface MultiSubscriptionSummaryProps {
  currentSubscription: UserSubscriptionResponse | null | undefined
  subscriptions: UserSubscriptionResponse[] | null | undefined
  emptyText: string
}

export function MultiSubscriptionSummary({ currentSubscription, subscriptions, emptyText }: MultiSubscriptionSummaryProps) {
  const items = subscriptions || []
  if (items.length === 0 && !currentSubscription) {
    return <p className="text-sm text-muted-foreground">{emptyText}</p>
  }

  const currentId = currentSubscription?.id

  return (
    <div className="space-y-3">
      {currentSubscription ? (
        <div className="rounded-lg border border-border/70 px-4 py-3">
          <div className="flex items-center justify-between gap-3">
            <div>
              <p className="text-sm font-semibold">{currentSubscription.planName}</p>
              <p className="text-xs text-muted-foreground">
                {currentSubscription.expiresAt ? `到期 ${formatDateTime(currentSubscription.expiresAt)}` : '永久有效'}
              </p>
            </div>
            <Badge variant="default">当前计费</Badge>
          </div>
        </div>
      ) : null}

      {items.length > 1 ? (
        <div className="space-y-2 rounded-lg border border-border/70 px-4 py-3">
          <div className="text-xs uppercase tracking-[0.18em] text-muted-foreground">全部订阅</div>
          <div className="space-y-2">
            {items.map((subscription) => (
              <div key={subscription.id} className="flex items-center justify-between gap-3 rounded-md border border-border/60 px-3 py-2">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{subscription.planName}</p>
                  <p className="text-xs text-muted-foreground">
                    {subscription.expiresAt ? `到期 ${formatDateTime(subscription.expiresAt)}` : '永久'}
                  </p>
                </div>
                {subscription.id === currentId ? <Badge variant="secondary">当前</Badge> : null}
              </div>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  )
}
