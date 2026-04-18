import { type ReactNode, useCallback, useEffect, useMemo, useState } from 'react'

import {
  createErrorRule,
  deleteErrorRule,
  getErrorRuleCacheStats,
  getErrorRules,
  refreshErrorRules,
  testErrorRule,
  updateErrorRule,
  type ErrorRule,
  type ErrorRuleCacheStats,
  type ErrorRuleRequestType,
  type ErrorRuleUpsertRequest,
} from '@/api/system'
import { OverflowCopyText } from '@/components/OverflowCopyText'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'

const requestTypeOptions: { value: ErrorRuleRequestType; label: string }[] = [
  { value: 'responses', label: 'Responses' },
  { value: 'chat_completions', label: 'Chat' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'messages', label: 'Anthropic' },
]

const matchTypeOptions = [
  { value: 'contains', label: 'Contains' },
  { value: 'exact', label: 'Exact' },
  { value: 'regex', label: 'Regex' },
] as const

const categoryOptions = [
  'model_error',
  'prompt_limit',
  'input_limit',
  'validation_error',
  'context_limit',
  'content_filter',
  'invalid_request',
]

function buildDraft(): ErrorRuleUpsertRequest {
  return {
    name: '',
    description: '',
    requestType: 'responses',
    upstreamStatus: '200',
    pattern: '',
    matchType: 'contains',
    category: 'model_error',
    priority: 0,
    isEnabled: true,
    overrideStatusCode: 502,
    overrideMessage: '',
    overrideResponse: null,
  }
}

interface Props {
  onMessage: (type: 'success' | 'error', text: string) => void
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

export function ErrorRulesPanel({ onMessage }: Props) {
  const [rules, setRules] = useState<ErrorRule[]>([])
  const [cacheStats, setCacheStats] = useState<ErrorRuleCacheStats | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingRule, setEditingRule] = useState<ErrorRule | null>(null)
  const [draft, setDraft] = useState<ErrorRuleUpsertRequest>(buildDraft())
  const [overrideResponseText, setOverrideResponseText] = useState('')
  const [testBody, setTestBody] = useState('')
  const [testStatus, setTestStatus] = useState('400')
  const [testRequestType, setTestRequestType] = useState<ErrorRuleRequestType>('responses')
  const [testResult, setTestResult] = useState<string>('')

  const loadedAtLabel = useMemo(() => {
    if (!cacheStats?.loadedAt) return '-'
    return new Date(cacheStats.loadedAt).toLocaleString('zh-CN')
  }, [cacheStats])

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [rulesResult, statsResult] = await Promise.all([
        getErrorRules(),
        getErrorRuleCacheStats(),
      ])
      setRules(rulesResult.rules)
      setCacheStats(statsResult)
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '加载错误规则失败')
    } finally {
      setLoading(false)
    }
  }, [onMessage])

  useEffect(() => {
    void load()
  }, [load])

  const openCreate = () => {
    setEditingRule(null)
    setDraft(buildDraft())
    setOverrideResponseText('')
    setDialogOpen(true)
  }

  const openEdit = (rule: ErrorRule) => {
    setEditingRule(rule)
    setDraft({
      name: rule.name,
      description: rule.description,
      requestType: rule.requestType,
      upstreamStatus: rule.upstreamStatus,
      pattern: rule.pattern,
      matchType: rule.matchType,
      category: rule.category,
      priority: rule.priority,
      isEnabled: rule.isEnabled,
      overrideStatusCode: rule.overrideStatusCode ?? null,
      overrideMessage: rule.overrideMessage,
      overrideResponse: rule.overrideResponse ?? null,
    })
    setOverrideResponseText(rule.overrideResponse ? JSON.stringify(rule.overrideResponse, null, 2) : '')
    setDialogOpen(true)
  }

  const parseOverrideResponse = () => {
    if (!overrideResponseText.trim()) return null
    return JSON.parse(overrideResponseText) as Record<string, unknown>
  }

  const handleSave = async () => {
    setSaving(true)
    try {
      const payload: ErrorRuleUpsertRequest = {
        ...draft,
        overrideResponse: parseOverrideResponse(),
      }
      if (editingRule) {
        await updateErrorRule(editingRule.id, payload)
        onMessage('success', '错误规则已更新')
      } else {
        await createErrorRule(payload)
        onMessage('success', '错误规则已创建')
      }
      setDialogOpen(false)
      await load()
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (rule: ErrorRule) => {
    if (!window.confirm(`删除 ${rule.name}？`)) return
    try {
      await deleteErrorRule(rule.id)
      onMessage('success', '错误规则已删除')
      await load()
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '删除失败')
    }
  }

  const handleRefresh = async () => {
    try {
      const result = await refreshErrorRules()
      setCacheStats(result.cacheStats)
      onMessage('success', result.message)
      await load()
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '刷新失败')
    }
  }

  const handleTest = async () => {
    try {
      const result = await testErrorRule({
        requestType: testRequestType,
        upstreamStatus: Number.parseInt(testStatus || '0', 10) || 0,
        body: testBody,
      })
      setTestResult(JSON.stringify(result, null, 2))
    } catch (error) {
      onMessage('error', error instanceof Error ? error.message : '测试失败')
    }
  }

  return (
    <div className="space-y-4">
      <div className="rounded-lg border">
        <div className="grid gap-3 p-4 md:grid-cols-[180px_120px_1fr_auto]">
          <Select value={testRequestType} onValueChange={(value: ErrorRuleRequestType) => setTestRequestType(value)}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {requestTypeOptions.map((option) => (
                <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Input value={testStatus} onChange={(event) => setTestStatus(event.target.value)} placeholder="Status" />
          <Input value={testBody} onChange={(event) => setTestBody(event.target.value)} placeholder="Body" />
          <Button type="button" variant="outline" onClick={() => void handleTest()}>测试</Button>
        </div>
        {testResult && (
          <pre className="overflow-x-auto border-t bg-muted/30 p-4 text-xs">{testResult}</pre>
        )}
      </div>

      <div className="rounded-lg border">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b p-4">
          <div className="flex flex-wrap items-center gap-3 text-sm text-muted-foreground">
            <span>Loaded {loadedAtLabel}</span>
            <span>{cacheStats?.containsCount ?? 0}/{cacheStats?.exactCount ?? 0}/{cacheStats?.regexCount ?? 0}</span>
            {cacheStats?.lastReloadError && <span className="text-rose-600">{cacheStats.lastReloadError}</span>}
          </div>
          <div className="flex gap-2">
            <Button type="button" variant="outline" onClick={() => void handleRefresh()}>刷新缓存</Button>
            <Button type="button" onClick={openCreate}>新增</Button>
          </div>
        </div>
        <div className="space-y-3 p-4 md:hidden">
          {loading ? (
            <div className="rounded-lg border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">加载中...</div>
          ) : rules.length === 0 ? (
            <div className="rounded-lg border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">暂无规则</div>
          ) : (
            rules.map((rule) => (
              <div key={rule.id} className="rounded-xl border border-border/70 px-4 py-4">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="font-medium">{rule.name}</div>
                    <div className="mt-1 font-mono text-[11px] text-muted-foreground">{rule.upstreamStatus} · {rule.priority}</div>
                  </div>
                  <div className="flex flex-wrap justify-end gap-2">
                    <Badge variant={rule.isEnabled ? 'default' : 'secondary'}>{rule.isEnabled ? '启用' : '停用'}</Badge>
                    {rule.isDefault ? <Badge variant="outline">默认</Badge> : null}
                  </div>
                </div>

                <div className="mt-3 grid gap-2 text-sm">
                  <MobileInfoRow label="类型" value={requestTypeOptions.find((option) => option.value === rule.requestType)?.label || rule.requestType} />
                  <MobileInfoRow label="分类" value={rule.category} />
                  <MobileInfoRow label="匹配" value={`${rule.matchType} · ${rule.pattern}`} />
                </div>

                <div className="mt-3 flex flex-wrap gap-2">
                  <Button type="button" variant="outline" size="sm" onClick={() => openEdit(rule)}>编辑</Button>
                  {!rule.isDefault ? (
                    <Button type="button" variant="outline" size="sm" onClick={() => void handleDelete(rule)}>删除</Button>
                  ) : null}
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
                <TableHead>类型</TableHead>
                <TableHead>分类</TableHead>
                <TableHead>匹配</TableHead>
                <TableHead>状态</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading ? (
                <TableRow>
                  <TableCell colSpan={6} className="text-center text-sm text-muted-foreground">加载中...</TableCell>
                </TableRow>
              ) : rules.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={6} className="text-center text-sm text-muted-foreground">暂无规则</TableCell>
                </TableRow>
              ) : (
                rules.map((rule) => (
                  <TableRow key={rule.id}>
                    <TableCell>
                      <div className="flex min-w-0 items-center gap-2">
                        <OverflowCopyText text={rule.name} className="max-w-[180px] font-medium" />
                        <span className="shrink-0 font-mono text-[11px] text-muted-foreground">{rule.upstreamStatus} · {rule.priority}</span>
                      </div>
                    </TableCell>
                    <TableCell>{requestTypeOptions.find((option) => option.value === rule.requestType)?.label || rule.requestType}</TableCell>
                    <TableCell>{rule.category}</TableCell>
                    <TableCell className="max-w-[280px] truncate">{rule.matchType} · {rule.pattern}</TableCell>
                    <TableCell>
                      <div className="flex gap-2">
                        <Badge variant={rule.isEnabled ? 'default' : 'secondary'}>{rule.isEnabled ? '启用' : '停用'}</Badge>
                        {rule.isDefault && <Badge variant="outline">默认</Badge>}
                      </div>
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-2">
                        <Button type="button" variant="outline" size="sm" onClick={() => openEdit(rule)}>编辑</Button>
                        {!rule.isDefault && (
                          <Button type="button" variant="outline" size="sm" onClick={() => void handleDelete(rule)}>删除</Button>
                        )}
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
            <DialogTitle>{editingRule ? '编辑错误规则' : '新建错误规则'}</DialogTitle>
          </DialogHeader>
          <div className="grid gap-4 py-2 md:grid-cols-2">
            <div className="space-y-2">
              <Label>名称</Label>
              <Input value={draft.name} onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))} />
            </div>
            <div className="space-y-2">
              <Label>分类</Label>
              <Select value={draft.category} onValueChange={(value) => setDraft((current) => ({ ...current, category: value }))}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  {categoryOptions.map((option) => (
                    <SelectItem key={option} value={option}>{option}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>请求类型</Label>
              <Select value={draft.requestType} onValueChange={(value: ErrorRuleRequestType) => setDraft((current) => ({ ...current, requestType: value }))}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  {requestTypeOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>匹配类型</Label>
              <Select value={draft.matchType} onValueChange={(value: ErrorRuleUpsertRequest['matchType']) => setDraft((current) => ({ ...current, matchType: value }))}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  {matchTypeOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>上游状态</Label>
              <Input value={draft.upstreamStatus} onChange={(event) => setDraft((current) => ({ ...current, upstreamStatus: event.target.value }))} />
            </div>
            <div className="space-y-2">
              <Label>优先级</Label>
              <Input
                type="number"
                value={draft.priority}
                onChange={(event) => setDraft((current) => ({ ...current, priority: Number.parseInt(event.target.value || '0', 10) || 0 }))}
              />
            </div>
            <div className="space-y-2 md:col-span-2">
              <Label>Pattern</Label>
              <Input value={draft.pattern} onChange={(event) => setDraft((current) => ({ ...current, pattern: event.target.value }))} />
            </div>
            <div className="space-y-2 md:col-span-2">
              <Label>说明</Label>
              <Input value={draft.description} onChange={(event) => setDraft((current) => ({ ...current, description: event.target.value }))} />
            </div>
            <div className="space-y-2">
              <Label>状态码</Label>
              <Input
                type="number"
                value={draft.overrideStatusCode ?? ''}
                onChange={(event) => {
                  const value = event.target.value.trim()
                  setDraft((current) => ({ ...current, overrideStatusCode: value ? Number.parseInt(value, 10) : null }))
                }}
              />
            </div>
            <div className="flex items-center justify-between rounded-lg border px-3 py-2">
              <span className="text-sm">启用</span>
              <Switch checked={draft.isEnabled} onCheckedChange={(checked) => setDraft((current) => ({ ...current, isEnabled: checked }))} />
            </div>
            <div className="space-y-2 md:col-span-2">
              <Label>消息</Label>
              <Input value={draft.overrideMessage} onChange={(event) => setDraft((current) => ({ ...current, overrideMessage: event.target.value }))} />
            </div>
            <div className="space-y-2 md:col-span-2">
              <Label>Override JSON</Label>
              <Textarea className="min-h-[180px] font-mono text-xs" value={overrideResponseText} onChange={(event) => setOverrideResponseText(event.target.value)} />
            </div>
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
