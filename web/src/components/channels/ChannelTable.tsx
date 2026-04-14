import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatDate } from '@/lib/formatters'
import { motion, tableStaggerContainer, tableRowVariants } from '@/lib/motion'
import { Channel, ChannelType, TestChannelResult } from '@/api/channels'

export interface ChannelTableProps {
  channels: Channel[]
  testResults: Record<string, TestChannelResult>
  fetchingModels: Record<string, boolean>
  modelCounts: Record<string, number>
  onToggleEnabled: (id: string, enabled: boolean) => void
  onTest: (id: string) => void
  onFetchModels: (id: string) => void
  onEdit: (channel: Channel) => void
  onDelete: (id: string, name: string) => void
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

export function ChannelTable({
  channels,
  testResults: _testResults,
  fetchingModels,
  modelCounts,
  onToggleEnabled,
  onTest,
  onFetchModels,
  onEdit,
  onDelete,
}: ChannelTableProps) {
  void _testResults
  return (
    <div className="rounded-md border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>名称</TableHead>
            <TableHead>类型</TableHead>
            <TableHead>Base URL</TableHead>
            <TableHead>状态</TableHead>
            <TableHead>优先级/权重</TableHead>
            <TableHead>更新时间</TableHead>
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
              <TableCell>{channel.priority} / {channel.weight}</TableCell>
              <TableCell>{formatDate(channel.updatedAt)}</TableCell>
              <TableCell className="text-right space-x-2">
                <Button variant="ghost" size="sm" onClick={() => onTest(channel.id)}>
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
