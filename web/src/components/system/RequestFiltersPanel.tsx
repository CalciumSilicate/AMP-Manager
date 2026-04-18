import { type ReactNode, useCallback, useEffect, useMemo, useState } from 'react'

import {
  createRequestFilter,
  deleteRequestFilter,
  getRequestFilterBindings,
  getRequestFilters,
  refreshRequestFilters,
  updateRequestFilter,
  type RequestFilter,
  type RequestFilterBindingOption,
  type RequestFilterMatchType,
  type RequestFilterUpsertRequest,
} from '@/api/system'
import { OverflowCopyText } from '@/components/OverflowCopyText'
import { SearchableMultiSelect } from '@/components/SearchableMultiSelect'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'

function buildDraft(): RequestFilterUpsertRequest {
  return {
    name: '',
    description: '',
    scope: 'body',
    action: 'json_path',
    matchType: 'contains',
    target: '',
    replacement: '',
    priority: 0,
    isEnabled: true,
    bindingType: 'global',
    channelIds: [],
    groupIds: [],
    ruleMode: 'simple',
    executionPhase: 'guard',
    operations: [],
  }
}

interface Props {
  onMessage: (type: 'success' | 'error', text: string) => void
  requestPayloadLimitMB: number
  requestPayloadSaving: boolean
  onRequestPayloadLimitMBChange: (value: number) => void
  onSaveRequestPayloadLimit: () => void
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

export function RequestFiltersPanel({
  onMessage,
  requestPayloadLimitMB,
  requestPayloadSaving,
  onRequestPayloadLimitMBChange,
  onSaveRequestPayloadLimit,
}: Props) {
  const [filters, setFilters] = useState<RequestFilter[]>([])
  const [channels, setChannels] = useState<RequestFilterBindingOption[]>([])
  const [groups, setGroups] = useState<RequestFilterBindingOption[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingFilter, setEditingFilter] = useState<RequestFilter | null>(null)
  const [draft, setDraft] = useState<RequestFilterUpsertRequest>(buildDraft())
  const [replacementText, setReplacementText] = useState('""')
  const [operationsText, setOperationsText] = useState('[]')

  const channelOptions = useMemo(() => channels.map((item) => ({ value: item.id, label: item.name })), [channels])
  const groupOptions = useMemo(() => groups.map((item) => ({ value: item.id, label: item.name })), [groups])

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [filtersResult, bindingsResult] = await Promise.all([
        getRequestFilters(),
        getRequestFilterBindings(),
      ])
      setFilters(filtersResult.filters)
      setChannels(bindingsResult.channels)
      setGroups(bindingsResult.groups)
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '加载请求过滤失败')
    } finally {
      setLoading(false)
    }
  }, [onMessage])

  useEffect(() => {
    void load()
  }, [load])

  const openCreate = () => {
    setEditingFilter(null)
    setDraft(buildDraft())
    setReplacementText('""')
    setOperationsText('[]')
    setDialogOpen(true)
  }

  const openEdit = (filter: RequestFilter) => {
    setEditingFilter(filter)
    setDraft({
      name: filter.name,
      description: filter.description,
      scope: filter.scope,
      action: filter.action,
      matchType: (filter.matchType as RequestFilterMatchType | null) ?? 'contains',
      target: filter.target,
      replacement: filter.replacement ?? '',
      priority: filter.priority,
      isEnabled: filter.isEnabled,
      bindingType: filter.bindingType,
      channelIds: filter.channelIds,
      groupIds: filter.groupIds,
      ruleMode: filter.ruleMode,
      executionPhase: filter.executionPhase,
      operations: filter.operations,
    })
    setReplacementText(JSON.stringify(filter.replacement ?? '', null, 2))
    setOperationsText(JSON.stringify(filter.operations ?? [], null, 2))
    setDialogOpen(true)
  }

  const handleSave = async () => {
    setSaving(true)
    try {
      const payload: RequestFilterUpsertRequest = {
        ...draft,
        replacement: draft.ruleMode === 'simple' ? JSON.parse(replacementText) : '',
        operations: draft.ruleMode === 'advanced' ? JSON.parse(operationsText) : [],
      }
      if (editingFilter) {
        await updateRequestFilter(editingFilter.id, payload)
        onMessage('success', '请求过滤已更新')
      } else {
        await createRequestFilter(payload)
        onMessage('success', '请求过滤已创建')
      }
      setDialogOpen(false)
      await load()
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (filter: RequestFilter) => {
    if (!window.confirm(`删除 ${filter.name}？`)) return
    try {
      await deleteRequestFilter(filter.id)
      onMessage('success', '请求过滤已删除')
      await load()
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '删除失败')
    }
  }

  const handleRefresh = async () => {
    try {
      const result = await refreshRequestFilters()
      onMessage('success', result.message)
      await load()
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '刷新失败')
    }
  }

  return (
    <div className="space-y-4">
      <div className="rounded-lg border p-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
          <div className="max-w-xs space-y-2">
            <Label htmlFor="requestPayloadLimit">请求体上限 (MiB)</Label>
            <Input
              id="requestPayloadLimit"
              type="number"
              min={1}
              value={requestPayloadLimitMB}
              onChange={(event) => onRequestPayloadLimitMBChange(parseInt(event.target.value || '1', 10) || 1)}
            />
          </div>
          <Button type="button" onClick={onSaveRequestPayloadLimit} disabled={requestPayloadSaving}>
            {requestPayloadSaving ? '保存中...' : '保存上限'}
          </Button>
        </div>
      </div>

      <div className="rounded-lg border">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b p-4">
          <div className="text-sm text-muted-foreground">{filters.length} 条</div>
          <div className="flex gap-2">
            <Button type="button" variant="outline" onClick={() => void handleRefresh()}>刷新缓存</Button>
            <Button type="button" onClick={openCreate}>新增</Button>
          </div>
        </div>
        <div className="space-y-3 p-4 md:hidden">
          {loading ? (
            <div className="rounded-lg border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">加载中...</div>
          ) : filters.length === 0 ? (
            <div className="rounded-lg border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">暂无过滤器</div>
          ) : (
            filters.map((filter) => (
              <div key={filter.id} className="rounded-xl border border-border/70 px-4 py-4">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="font-medium">{filter.name}</div>
                    <div className="mt-1 font-mono text-[11px] text-muted-foreground">优先级 {filter.priority}</div>
                  </div>
                  <Badge variant={filter.isEnabled ? 'default' : 'secondary'}>{filter.isEnabled ? '启用' : '停用'}</Badge>
                </div>

                <div className="mt-3 grid gap-2 text-sm">
                  <MobileInfoRow label="绑定" value={filter.bindingType} />
                  <MobileInfoRow label="阶段" value={filter.executionPhase} />
                  <MobileInfoRow label="模式" value={filter.ruleMode} />
                  <MobileInfoRow label="目标" value={filter.target || filter.action} />
                </div>

                <div className="mt-3 flex flex-wrap gap-2">
                  <Button type="button" variant="outline" size="sm" onClick={() => openEdit(filter)}>编辑</Button>
                  <Button type="button" variant="outline" size="sm" onClick={() => void handleDelete(filter)}>删除</Button>
                </div>
              </div>
            ))
          )}
        </div>

        <div className="hidden overflow-x-auto md:block">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>名称</TableHead>
                <TableHead>绑定</TableHead>
                <TableHead>阶段</TableHead>
                <TableHead>模式</TableHead>
                <TableHead>目标</TableHead>
                <TableHead>状态</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading ? (
                <TableRow>
                  <TableCell colSpan={7} className="text-center text-sm text-muted-foreground">加载中...</TableCell>
                </TableRow>
              ) : filters.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={7} className="text-center text-sm text-muted-foreground">暂无过滤器</TableCell>
                </TableRow>
              ) : (
                filters.map((filter) => (
                  <TableRow key={filter.id}>
                    <TableCell>
                      <div className="flex min-w-0 items-center gap-2">
                        <OverflowCopyText text={filter.name} className="max-w-[180px] font-medium" />
                        <span className="shrink-0 font-mono text-[11px] text-muted-foreground">P{filter.priority}</span>
                      </div>
                    </TableCell>
                    <TableCell>{filter.bindingType}</TableCell>
                    <TableCell>{filter.executionPhase}</TableCell>
                    <TableCell>{filter.ruleMode}</TableCell>
                    <TableCell className="max-w-[260px] truncate">{filter.target || filter.action}</TableCell>
                    <TableCell>
                      <Badge variant={filter.isEnabled ? 'default' : 'secondary'}>{filter.isEnabled ? '启用' : '停用'}</Badge>
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-2">
                        <Button type="button" variant="outline" size="sm" onClick={() => openEdit(filter)}>编辑</Button>
                        <Button type="button" variant="outline" size="sm" onClick={() => void handleDelete(filter)}>删除</Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
      </div>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-h-[calc(100vh-2rem)] overflow-y-auto max-w-3xl">
          <DialogHeader>
            <DialogTitle>{editingFilter ? '编辑请求过滤' : '新建请求过滤'}</DialogTitle>
          </DialogHeader>
          <div className="grid gap-4 py-2 md:grid-cols-2">
            <div className="space-y-2">
              <Label>名称</Label>
              <Input value={draft.name} onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))} />
            </div>
            <div className="space-y-2">
              <Label>绑定</Label>
              <Select value={draft.bindingType} onValueChange={(value: RequestFilter['bindingType']) => setDraft((current) => ({ ...current, bindingType: value }))}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="global">Global</SelectItem>
                  <SelectItem value="channels">Channels</SelectItem>
                  <SelectItem value="groups">Groups</SelectItem>
                </SelectContent>
              </Select>
            </div>
            {draft.bindingType === 'channels' && (
              <div className="space-y-2 md:col-span-2">
                <Label>渠道</Label>
                <SearchableMultiSelect values={draft.channelIds} onValuesChange={(values) => setDraft((current) => ({ ...current, channelIds: values }))} options={channelOptions} />
              </div>
            )}
            {draft.bindingType === 'groups' && (
              <div className="space-y-2 md:col-span-2">
                <Label>分组</Label>
                <SearchableMultiSelect values={draft.groupIds} onValuesChange={(values) => setDraft((current) => ({ ...current, groupIds: values }))} options={groupOptions} />
              </div>
            )}
            <div className="space-y-2">
              <Label>模式</Label>
              <Select value={draft.ruleMode} onValueChange={(value: RequestFilter['ruleMode']) => setDraft((current) => ({ ...current, ruleMode: value, executionPhase: value === 'advanced' ? 'final' : current.executionPhase }))}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="simple">Simple</SelectItem>
                  <SelectItem value="advanced">Advanced</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>阶段</Label>
              <Select value={draft.executionPhase} onValueChange={(value: RequestFilter['executionPhase']) => setDraft((current) => ({ ...current, executionPhase: value }))} disabled={draft.ruleMode === 'advanced'}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="guard">Guard</SelectItem>
                  <SelectItem value="final">Final</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>优先级</Label>
              <Input type="number" value={draft.priority} onChange={(event) => setDraft((current) => ({ ...current, priority: Number.parseInt(event.target.value || '0', 10) || 0 }))} />
            </div>
            <div className="flex items-center justify-between rounded-lg border px-3 py-2">
              <span className="text-sm">启用</span>
              <Switch checked={draft.isEnabled} onCheckedChange={(checked) => setDraft((current) => ({ ...current, isEnabled: checked }))} />
            </div>
            <div className="space-y-2 md:col-span-2">
              <Label>说明</Label>
              <Input value={draft.description} onChange={(event) => setDraft((current) => ({ ...current, description: event.target.value }))} />
            </div>

            {draft.ruleMode === 'simple' ? (
              <>
                <div className="space-y-2">
                  <Label>Scope</Label>
                  <Select value={draft.scope} onValueChange={(value: RequestFilter['scope']) => setDraft((current) => ({ ...current, scope: value }))}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="header">Header</SelectItem>
                      <SelectItem value="body">Body</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label>Action</Label>
                  <Select value={draft.action} onValueChange={(value: RequestFilter['action']) => setDraft((current) => ({ ...current, action: value }))}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="remove">remove</SelectItem>
                      <SelectItem value="set">set</SelectItem>
                      <SelectItem value="json_path">json_path</SelectItem>
                      <SelectItem value="text_replace">text_replace</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label>MatchType</Label>
                  <Select value={draft.matchType || 'contains'} onValueChange={(value: RequestFilterMatchType) => setDraft((current) => ({ ...current, matchType: value }))}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="contains">contains</SelectItem>
                      <SelectItem value="exact">exact</SelectItem>
                      <SelectItem value="regex">regex</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2 md:col-span-2">
                  <Label>Target</Label>
                  <Input value={draft.target} onChange={(event) => setDraft((current) => ({ ...current, target: event.target.value }))} />
                </div>
                <div className="space-y-2 md:col-span-2">
                  <Label>Replacement JSON</Label>
                  <Textarea className="min-h-[120px] font-mono text-xs" value={replacementText} onChange={(event) => setReplacementText(event.target.value)} />
                </div>
              </>
            ) : (
              <div className="space-y-2 md:col-span-2">
                <Label>Operations JSON</Label>
                <Textarea className="min-h-[220px] font-mono text-xs" value={operationsText} onChange={(event) => setOperationsText(event.target.value)} />
              </div>
            )}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>取消</Button>
            <Button type="button" onClick={() => void handleSave()} disabled={saving}>{saving ? '保存中...' : '保存'}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
