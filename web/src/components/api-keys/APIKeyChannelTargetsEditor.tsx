import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import type { APIKeyChannelTarget } from '@/api/amp'

interface APIKeyChannelTargetsEditorProps {
  splitBySource: boolean
  onSplitBySourceChange: (next: boolean) => void
  channelTargets: APIKeyChannelTarget[]
  onChannelTargetsChange: (next: APIKeyChannelTarget[]) => void
  subscriptionChannelTargets: APIKeyChannelTarget[]
  onSubscriptionChannelTargetsChange: (next: APIKeyChannelTarget[]) => void
  usageChannelTargets: APIKeyChannelTarget[]
  onUsageChannelTargetsChange: (next: APIKeyChannelTarget[]) => void
}

function normalizeTargets(targets: APIKeyChannelTarget[]) {
  return targets.map((target) => ({
    channelId: target.channelId,
    priority: target.priority > 0 ? target.priority : 100,
  }))
}

function renderTargetRows(
  label: string,
  targets: APIKeyChannelTarget[],
  onChange: (next: APIKeyChannelTarget[]) => void,
) {
  const normalized = normalizeTargets(targets)
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <Label>{label}</Label>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => onChange([...normalized, { channelId: '', priority: 100 }])}
        >
          添加
        </Button>
      </div>
      {normalized.length === 0 ? (
        <div className="rounded-md border border-dashed px-3 py-4 text-xs text-muted-foreground">
          留空表示不限制
        </div>
      ) : (
        <div className="space-y-2">
          {normalized.map((target, index) => (
            <div key={`${label}-${index}`} className="grid gap-2 md:grid-cols-[minmax(0,1fr)_120px_72px]">
              <Input
                value={target.channelId}
                onChange={(event) => {
                  const next = [...normalized]
                  next[index] = { ...next[index], channelId: event.target.value }
                  onChange(next)
                }}
                placeholder="channelId"
                className="font-mono"
              />
              <Input
                type="number"
                min="1"
                value={target.priority}
                onChange={(event) => {
                  const next = [...normalized]
                  next[index] = { ...next[index], priority: Number.parseInt(event.target.value, 10) || 100 }
                  onChange(next)
                }}
              />
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="text-destructive hover:text-destructive"
                onClick={() => onChange(normalized.filter((_, itemIndex) => itemIndex !== index))}
              >
                删除
              </Button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

export function APIKeyChannelTargetsEditor({
  splitBySource,
  onSplitBySourceChange,
  channelTargets,
  onChannelTargetsChange,
  subscriptionChannelTargets,
  onSubscriptionChannelTargetsChange,
  usageChannelTargets,
  onUsageChannelTargetsChange,
}: APIKeyChannelTargetsEditorProps) {
  return (
    <div className="space-y-4 rounded-md border border-border/70 p-4">
      <div className="flex items-center justify-between gap-4">
        <div className="space-y-0.5">
          <Label>渠道目标</Label>
          <p className="text-xs text-muted-foreground">优先级越小越优先。来源列表留空表示不限制。</p>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-muted-foreground">区分订阅 / 用量</span>
          <Switch checked={splitBySource} onCheckedChange={onSplitBySourceChange} />
        </div>
      </div>

      {splitBySource ? (
        <div className="grid gap-4 md:grid-cols-2">
          {renderTargetRows('订阅渠道目标', subscriptionChannelTargets, onSubscriptionChannelTargetsChange)}
          {renderTargetRows('用量渠道目标', usageChannelTargets, onUsageChannelTargetsChange)}
        </div>
      ) : (
        renderTargetRows('默认渠道目标', channelTargets, onChannelTargetsChange)
      )}
    </div>
  )
}
