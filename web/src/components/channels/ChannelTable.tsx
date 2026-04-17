import { type ReactNode, useEffect, useState } from 'react'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { motion, tableStaggerContainer, tableRowVariants } from '@/lib/motion'
import { Channel, ChannelType, TestChannelResult } from '@/api/channels'

export interface ChannelTableProps {
  channels: Channel[]
  testResults: Record<string, TestChannelResult>
  fetchingModels: Record<string, boolean>
  modelCounts: Record<string, number>
  onToggleEnabled: (id: string, enabled: boolean) => void
  onQuickUpdate: (channel: Channel, field: 'priority' | 'weight' | 'rateMultiplier', value: number) => Promise<void>
  onTest: (channel: Channel) => void
  onFetchModels: (id: string) => void
  onEdit: (channel: Channel) => void
  onDelete: (id: string, name: string) => void
}

function getEndpointLabel(endpoint: Channel['endpoint']): string {
  switch (endpoint) {
    case 'responses':
      return 'Responses'
    case 'messages':
      return 'Messages'
    case 'generate_content':
      return 'Generate'
    default:
      return 'Chat'
  }
}

function getTypeBadgeVariant(type: ChannelType): 'default' | 'secondary' | 'outline' {
  switch (type) {
    case 'gemini':
      return 'default'
    case 'claude':
      return 'secondary'
    case 'openai':
      return 'outline'
    default:
      return 'default'
  }
}

function EditableNumericCell({
  value,
  step = '1',
  suffix = '',
  onSave,
}: {
  value: number
  step?: string
  suffix?: string
  onSave: (value: number) => Promise<void>
}) {
  const [editing, setEditing] = useState(false)
  const [saving, setSaving] = useState(false)
  const [draft, setDraft] = useState(String(value))

  useEffect(() => {
    if (!editing) {
      setDraft(String(value))
    }
  }, [editing, value])

  const commit = async () => {
    const nextValue = Number.parseFloat(draft)
    if (Number.isNaN(nextValue)) {
      setDraft(String(value))
      setEditing(false)
      return
    }
    if (nextValue === value) {
      setEditing(false)
      return
    }

    setSaving(true)
    try {
      await onSave(nextValue)
    } finally {
      setSaving(false)
      setEditing(false)
    }
  }

  if (editing) {
    return (
      <Input
        autoFocus
        type="number"
        step={step}
        value={draft}
        onChange={(event) => setDraft(event.target.value)}
        onBlur={() => void commit()}
        onKeyDown={(event) => {
          if (event.key === 'Enter') {
            event.preventDefault()
            void commit()
          }
          if (event.key === 'Escape') {
            setDraft(String(value))
            setEditing(false)
          }
        }}
        className="h-8 w-24 text-right font-mono"
      />
    )
  }

  return (
    <button
      type="button"
      className="font-mono text-sm text-muted-foreground transition-colors hover:text-foreground"
      onClick={() => setEditing(true)}
      disabled={saving}
    >
      {saving ? '保存中' : `${value}${suffix}`}
    </button>
  )
}

function MobileInfoRow({
  label,
  value,
}: {
  label: string
  value: ReactNode
}) {
  return (
    <div className="flex items-start justify-between gap-3">
      <span className="text-muted-foreground">{label}</span>
      <div className="min-w-0 text-right text-foreground">{value}</div>
    </div>
  )
}

export function ChannelTable({
  channels,
  testResults: _testResults,
  fetchingModels,
  modelCounts,
  onToggleEnabled,
  onQuickUpdate,
  onTest,
  onFetchModels,
  onEdit,
  onDelete,
}: ChannelTableProps) {
  void _testResults

  return (
    <div className="rounded-md border">
      <div className="space-y-3 p-4 md:hidden">
        {channels.map((channel) => (
          <div key={channel.id} className="rounded-xl border border-border/70 px-4 py-4">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium">{channel.name}</span>
                  {channel.type === 'openai' && channel.endpoint === 'responses' && channel.codexWebsocketEnabled ? (
                    <Badge variant="outline" className="text-[10px] uppercase tracking-[0.12em] text-muted-foreground">
                      WS
                    </Badge>
                  ) : null}
                </div>
                <p className="mt-1 break-all text-xs text-muted-foreground">{channel.baseUrl}</p>
              </div>
              <Badge variant={getTypeBadgeVariant(channel.type)}>
                {channel.type.toUpperCase()}
              </Badge>
            </div>

            <div className="mt-3 flex flex-wrap gap-2">
              <Badge variant="outline">{getEndpointLabel(channel.endpoint)}</Badge>
              {channel.groupNames.length > 0 ? (
                <Badge variant="secondary" className="max-w-full truncate">
                  {channel.groupNames.join(' / ')}
                </Badge>
              ) : null}
              <Badge variant="outline">
                模型 {modelCounts[channel.id] ?? channel.models.length}
              </Badge>
            </div>

            <div className="mt-3 grid gap-2 text-sm">
              <MobileInfoRow
                label="状态"
                value={(
                  <div className="flex items-center justify-end gap-2">
                    <span className="text-sm text-muted-foreground">
                      {channel.enabled ? '启用' : '禁用'}
                    </span>
                    <Switch
                      checked={channel.enabled}
                      onCheckedChange={(checked) => onToggleEnabled(channel.id, checked)}
                    />
                  </div>
                )}
              />
              <MobileInfoRow
                label="优先级"
                value={<EditableNumericCell value={channel.priority} onSave={(value) => onQuickUpdate(channel, 'priority', Math.round(value))} />}
              />
              <MobileInfoRow
                label="权重"
                value={<EditableNumericCell value={channel.weight} onSave={(value) => onQuickUpdate(channel, 'weight', Math.round(value))} />}
              />
              <MobileInfoRow
                label="倍率"
                value={<EditableNumericCell value={channel.rateMultiplier} step="0.01" suffix="x" onSave={(value) => onQuickUpdate(channel, 'rateMultiplier', value)} />}
              />
            </div>

            <div className="mt-3 flex flex-wrap gap-2">
              <Button variant="outline" size="sm" onClick={() => onTest(channel)}>
                测试
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => onFetchModels(channel.id)}
                disabled={fetchingModels[channel.id]}
              >
                {fetchingModels[channel.id] ? '获取中...' : '获取模型'}
              </Button>
              <Button variant="outline" size="sm" onClick={() => onEdit(channel)}>
                编辑
              </Button>
              <Button
                variant="outline"
                size="sm"
                className="text-destructive hover:text-destructive"
                onClick={() => onDelete(channel.id, channel.name)}
              >
                删除
              </Button>
            </div>
          </div>
        ))}
      </div>

      <Table className="hidden md:table">
        <TableHeader>
          <TableRow>
            <TableHead>名称</TableHead>
            <TableHead>类型</TableHead>
            <TableHead>Base URL</TableHead>
            <TableHead>状态</TableHead>
            <TableHead>优先级</TableHead>
            <TableHead>权重</TableHead>
            <TableHead>倍率</TableHead>
            <TableHead className="text-right">操作</TableHead>
          </TableRow>
        </TableHeader>
        <motion.tbody variants={tableStaggerContainer} initial="hidden" animate="visible" key={channels.length}>
          {channels.map((channel) => (
            <motion.tr key={channel.id} variants={tableRowVariants} className="border-b transition-colors hover:bg-muted/50 data-[state=selected]:bg-muted">
              <TableCell>
                <div className="flex items-center gap-2">
                  <span className="font-medium">{channel.name}</span>
                  {channel.type === 'openai' && channel.endpoint === 'responses' && channel.codexWebsocketEnabled && (
                    <Badge variant="outline" className="text-[10px] uppercase tracking-[0.12em] text-muted-foreground">
                      WS
                    </Badge>
                  )}
                </div>
              </TableCell>
              <TableCell>
                <Badge variant={getTypeBadgeVariant(channel.type)}>
                  {channel.type.toUpperCase()}
                </Badge>
              </TableCell>
              <TableCell className="max-w-xs truncate" title={channel.baseUrl}>
                {channel.baseUrl}
              </TableCell>
              <TableCell>
                <div className="flex items-center gap-2">
                  <Switch
                    checked={channel.enabled}
                    onCheckedChange={(checked) => onToggleEnabled(channel.id, checked)}
                  />
                  <span className="text-sm text-muted-foreground">
                    {channel.enabled ? '启用' : '禁用'}
                  </span>
                </div>
              </TableCell>
              <TableCell>
                <EditableNumericCell value={channel.priority} onSave={(value) => onQuickUpdate(channel, 'priority', Math.round(value))} />
              </TableCell>
              <TableCell>
                <EditableNumericCell value={channel.weight} onSave={(value) => onQuickUpdate(channel, 'weight', Math.round(value))} />
              </TableCell>
              <TableCell>
                <EditableNumericCell value={channel.rateMultiplier} step="0.01" suffix="x" onSave={(value) => onQuickUpdate(channel, 'rateMultiplier', value)} />
              </TableCell>
              <TableCell className="text-right space-x-2">
                <Button variant="ghost" size="sm" onClick={() => onTest(channel)}>
                  测试
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => onFetchModels(channel.id)}
                  disabled={fetchingModels[channel.id]}
                >
                  {fetchingModels[channel.id] ? '获取中...' : '获取模型'}
                  {modelCounts[channel.id] !== undefined && (
                    <span className="ml-1 text-xs text-muted-foreground">
                      ({modelCounts[channel.id]})
                    </span>
                  )}
                </Button>
                <Button variant="ghost" size="sm" onClick={() => onEdit(channel)}>
                  编辑
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  className="text-destructive hover:text-destructive"
                  onClick={() => onDelete(channel.id, channel.name)}
                >
                  删除
                </Button>
              </TableCell>
            </motion.tr>
          ))}
        </motion.tbody>
      </Table>
    </div>
  )
}
