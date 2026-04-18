import { useCallback, useEffect, useState } from 'react'
import { RefreshCw } from 'lucide-react'

import {
  getInviteConfig,
  getInviteStats,
  listInviteRelations,
  listInviteRewardEvents,
  updateInviteConfig,
  type InviteConfig,
  type InviteRelation,
  type InviteRewardEvent,
  type InviteStats,
} from '@/api/invite'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { formatDateTime, formatGroupedNumericString } from '@/lib/formatters'

type MessageFn = (type: 'success' | 'error', text: string) => void

interface Props {
  onMessage: MessageFn
}

function formatUsdExact(value: number, fractionDigits = 2): string {
  return `$${formatGroupedNumericString(value.toFixed(fractionDigits))}`
}

export function InviteSettingsPanel({ onMessage }: Props) {
  const [config, setConfig] = useState<InviteConfig | null>(null)
  const [stats, setStats] = useState<InviteStats | null>(null)
  const [relations, setRelations] = useState<InviteRelation[]>([])
  const [rewardEvents, setRewardEvents] = useState<InviteRewardEvent[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [saving, setSaving] = useState(false)

  const loadData = useCallback(async (silent = false) => {
    if (silent) {
      setRefreshing(true)
    } else {
      setLoading(true)
    }

    try {
      const [nextConfig, nextStats, nextRelations, nextRewards] = await Promise.all([
        getInviteConfig(),
        getInviteStats(),
        listInviteRelations(),
        listInviteRewardEvents(),
      ])

      setConfig(nextConfig)
      setStats(nextStats)
      setRelations(nextRelations)
      setRewardEvents(nextRewards)
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '加载邀请系统配置失败')
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }, [onMessage])

  useEffect(() => {
    void loadData()
  }, [loadData])

  const handleSave = async () => {
    if (!config) return

    setSaving(true)
    try {
      const nextConfig = await updateInviteConfig({
        enabled: config.enabled,
        inviterRewardMicros: config.inviterRewardMicros,
        inviteeRewardMicros: config.inviteeRewardMicros,
        minFirstPaidCnyCent: config.minFirstPaidCnyCent,
      })
      setConfig(nextConfig)
      await loadData(true)
      onMessage('success', '邀请系统配置已更新')
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '保存邀请系统配置失败')
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return <div className="py-10 text-center text-sm text-muted-foreground">加载中...</div>
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
            <div className="space-y-1">
              <CardTitle>邀请系统</CardTitle>
              <CardDescription>控制邀请奖励、首单门槛与全局关系统计。关闭后注册页不会再展示邀请码输入。</CardDescription>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button type="button" variant="outline" size="sm" onClick={() => void loadData(true)} disabled={refreshing}>
                <RefreshCw className={`mr-2 h-4 w-4 ${refreshing ? 'animate-spin' : ''}`} />
                刷新
              </Button>
              <Button type="button" size="sm" onClick={() => void handleSave()} disabled={!config || saving}>
                {saving ? '保存中...' : '保存'}
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-6">
          {config ? (
            <>
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant={config.enabled ? 'default' : 'secondary'}>
                  {config.enabled ? '已启用' : '已关闭'}
                </Badge>
                <Badge variant={config.configComplete ? 'default' : 'outline'}>
                  {config.configComplete ? '奖励配置完整' : '奖励配置未完成'}
                </Badge>
              </div>

              <div className="grid gap-4 md:grid-cols-4">
                <div className="space-y-2">
                  <Label>启用邀请</Label>
                  <div className="flex h-10 items-center">
                    <Switch
                      checked={config.enabled}
                      onCheckedChange={(checked) => setConfig((current) => current ? { ...current, enabled: checked } : current)}
                    />
                  </div>
                </div>
                <div className="space-y-2">
                  <Label>邀请人奖励 (USD)</Label>
                  <Input
                    type="number"
                    min="0"
                    step="0.01"
                    value={String((config.inviterRewardMicros || 0) / 1_000_000)}
                    onChange={(event) => setConfig((current) => current ? {
                      ...current,
                      inviterRewardMicros: Math.round(Number.parseFloat(event.target.value || '0') * 1_000_000),
                    } : current)}
                  />
                </div>
                <div className="space-y-2">
                  <Label>被邀请人奖励 (USD)</Label>
                  <Input
                    type="number"
                    min="0"
                    step="0.01"
                    value={String((config.inviteeRewardMicros || 0) / 1_000_000)}
                    onChange={(event) => setConfig((current) => current ? {
                      ...current,
                      inviteeRewardMicros: Math.round(Number.parseFloat(event.target.value || '0') * 1_000_000),
                    } : current)}
                  />
                </div>
                <div className="space-y-2">
                  <Label>首单最低实付 (CNY)</Label>
                  <Input
                    type="number"
                    min="0"
                    step="0.01"
                    value={String((config.minFirstPaidCnyCent || 0) / 100)}
                    onChange={(event) => setConfig((current) => current ? {
                      ...current,
                      minFirstPaidCnyCent: Math.round(Number.parseFloat(event.target.value || '0') * 100),
                    } : current)}
                  />
                </div>
              </div>
            </>
          ) : (
            <div className="text-sm text-muted-foreground">邀请配置加载失败</div>
          )}

          {stats ? (
            <div className="grid gap-4 md:grid-cols-4">
              <div className="rounded-lg border px-4 py-4">
                <div className="text-xs text-muted-foreground">关系总数</div>
                <div className="mt-2 text-2xl font-semibold">{stats.relationsTotal}</div>
              </div>
              <div className="rounded-lg border px-4 py-4">
                <div className="text-xs text-muted-foreground">待返奖励</div>
                <div className="mt-2 text-2xl font-semibold">{stats.pendingTotal}</div>
              </div>
              <div className="rounded-lg border px-4 py-4">
                <div className="text-xs text-muted-foreground">已返奖励</div>
                <div className="mt-2 text-2xl font-semibold">{stats.rewardedTotal}</div>
              </div>
              <div className="rounded-lg border px-4 py-4">
                <div className="text-xs text-muted-foreground">已发奖励</div>
                <div className="mt-2 text-2xl font-semibold">{formatUsdExact(stats.grantedRewardMicros / 1_000_000)}</div>
              </div>
            </div>
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>最近邀请关系</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {relations.length > 0 ? relations.slice(0, 10).map((item) => (
            <div key={item.id} className="flex flex-col gap-2 rounded-lg border px-4 py-3 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <div className="font-medium">{item.inviterUsername} → {item.inviteeUsername}</div>
                <div className="text-xs text-muted-foreground">
                  邀请码 {item.inviterCode} · {item.firstPaidOrderNo || '未触发首单'}
                </div>
              </div>
              <div className="flex items-center gap-3">
                <Badge variant={item.status === 'rewarded' ? 'default' : 'secondary'}>
                  {item.status === 'rewarded' ? '已返奖' : '待首单'}
                </Badge>
                <span className="text-xs text-muted-foreground">{formatDateTime(item.createdAt)}</span>
              </div>
            </div>
          )) : (
            <div className="rounded-lg border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">
              暂无邀请关系
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>最近奖励事件</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {rewardEvents.length > 0 ? rewardEvents.slice(0, 10).map((item) => (
            <div key={item.id} className="flex flex-col gap-2 rounded-lg border px-4 py-3 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <div className="font-medium">{item.beneficiaryUsername}</div>
                <div className="text-xs text-muted-foreground">
                  订单 {item.orderNo || '-'} · {formatDateTime(item.createdAt)}
                </div>
              </div>
              <div className="flex items-center gap-3">
                <Badge variant={item.status === 'granted' ? 'default' : 'secondary'}>
                  {item.status === 'granted' ? '发放' : '回滚'}
                </Badge>
                <span className="font-medium">{formatUsdExact(item.amountMicros / 1_000_000)}</span>
              </div>
            </div>
          )) : (
            <div className="rounded-lg border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">
              暂无奖励事件
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
