import { useState, useEffect, useCallback } from 'react'
import { motion, AnimatePresence, tableStaggerContainer, tableRowVariants } from '@/lib/motion'
import {
  getPlans,
  createPlan,
  updatePlan,
  deletePlan,
  setPlanEnabled,
  SubscriptionPlanResponse,
  SubscriptionPlanRequest,
  PlanLimitRequest,
  LimitType,
  WindowMode,
} from '../api/subscription'
import { AdminPageShell, AdminSurface } from '@/components/admin/AdminPageShell'
import { Num } from '@/components/Num'
import { OverflowCopyText } from '@/components/OverflowCopyText'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Table,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { formatDateTime } from '@/lib/formatters'
import { CheckCircle2, XCircle, Plus, Trash2 } from 'lucide-react'

const LIMIT_TYPE_LABELS: Record<LimitType, string> = {
  daily: '日限制',
  weekly: '周限制',
  monthly: '月限制',
  rolling_5h: '5小时滚动',
  total: '总量限制',
}

const WINDOW_MODE_LABELS: Record<WindowMode, string> = {
  fixed: '固定窗口',
  sliding: '滑动窗口',
}

const ALL_LIMIT_TYPES: LimitType[] = ['daily', 'weekly', 'monthly', 'rolling_5h', 'total']
const ALL_WINDOW_MODES: WindowMode[] = ['fixed', 'sliding']

function microsToUsd(micros: number): string {
  return (micros / 1_000_000).toFixed(2)
}

function formatUsdLabel(micros: number): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(micros / 1_000_000)
}

function usdToMicros(usd: string): number {
  const val = parseFloat(usd)
  if (Number.isNaN(val)) return 0
  return Math.round(val * 1_000_000)
}

interface LimitRow {
  limitType: LimitType
  windowMode: WindowMode
  amountUsd: string
  fixedResetTime: string
}

export default function SubscriptionPlans() {
  const [plans, setPlans] = useState<SubscriptionPlanResponse[]>([])
  const [loading, setLoading] = useState(true)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [editingPlan, setEditingPlan] = useState<SubscriptionPlanResponse | null>(null)
  const [formName, setFormName] = useState('')
  const [formDescription, setFormDescription] = useState('')
  const [formEnabled, setFormEnabled] = useState(true)
  const [formLimits, setFormLimits] = useState<LimitRow[]>([])
  const [saving, setSaving] = useState(false)
  const [deleteConfirmModal, setDeleteConfirmModal] = useState<SubscriptionPlanResponse | null>(null)

  const showMsg = useCallback((type: 'success' | 'error', text: string) => {
    setMessage({ type, text })
    setTimeout(() => setMessage(null), 3000)
  }, [])

  const fetchPlans = useCallback(async () => {
    try {
      const data = await getPlans()
      setPlans(data)
    } catch (err) {
      showMsg('error', err instanceof Error ? err.message : '获取套餐列表失败')
    } finally {
      setLoading(false)
    }
  }, [showMsg])

  useEffect(() => {
    fetchPlans()
  }, [fetchPlans])

  const handleCreate = () => {
    setEditingPlan(null)
    setFormName('')
    setFormDescription('')
    setFormEnabled(true)
    setFormLimits([])
    setShowForm(true)
  }

  const handleEdit = (plan: SubscriptionPlanResponse) => {
    setEditingPlan(plan)
    setFormName(plan.name)
    setFormDescription(plan.description)
    setFormEnabled(plan.enabled)
    setFormLimits(
      (plan.limits || []).map((limit) => ({
        limitType: limit.limitType,
        windowMode: limit.windowMode,
        amountUsd: microsToUsd(limit.limitMicros),
        fixedResetTime: limit.fixedResetTime || '',
      })),
    )
    setShowForm(true)
  }

  const handleSubmit = async () => {
    if (!formName.trim()) {
      showMsg('error', '请填写套餐名称')
      return
    }

    const usedTypes = formLimits.map((limit) => limit.limitType)
    if (new Set(usedTypes).size !== usedTypes.length) {
      showMsg('error', '限制类型不能重复')
      return
    }

    const limits: PlanLimitRequest[] = formLimits.map((limit) => ({
      limitType: limit.limitType,
      windowMode: limit.windowMode,
      limitMicros: usdToMicros(limit.amountUsd),
      fixedResetTime:
        limit.limitType === 'daily' && limit.windowMode === 'fixed' && limit.fixedResetTime
          ? limit.fixedResetTime
          : undefined,
    }))

    const payload: SubscriptionPlanRequest = {
      name: formName.trim(),
      description: formDescription.trim(),
      enabled: formEnabled,
      limits,
    }

    setSaving(true)
    try {
      if (editingPlan) {
        await updatePlan(editingPlan.id, payload)
        showMsg('success', '套餐已更新')
      } else {
        await createPlan(payload)
        showMsg('success', '套餐已创建')
      }
      setShowForm(false)
      fetchPlans()
    } catch (err) {
      showMsg('error', err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    if (!deleteConfirmModal) return
    try {
      await deletePlan(deleteConfirmModal.id)
      showMsg('success', '套餐已删除')
      setDeleteConfirmModal(null)
      fetchPlans()
    } catch (err) {
      showMsg('error', err instanceof Error ? err.message : '删除失败')
    }
  }

  const handleToggleEnabled = async (plan: SubscriptionPlanResponse) => {
    try {
      await setPlanEnabled(plan.id, !plan.enabled)
      showMsg('success', `套餐已${plan.enabled ? '禁用' : '启用'}`)
      fetchPlans()
    } catch (err) {
      showMsg('error', err instanceof Error ? err.message : '操作失败')
    }
  }

  const addLimitRow = () => {
    const usedTypes = formLimits.map((limit) => limit.limitType)
    const available = ALL_LIMIT_TYPES.filter((type) => !usedTypes.includes(type))
    if (available.length === 0) {
      showMsg('error', '已添加所有限制类型')
      return
    }

    setFormLimits([
      ...formLimits,
      { limitType: available[0], windowMode: 'fixed', amountUsd: '', fixedResetTime: '' },
    ])
  }

  const removeLimitRow = (index: number) => {
    setFormLimits(formLimits.filter((_, currentIndex) => currentIndex !== index))
  }

  const updateLimitRow = (index: number, field: keyof LimitRow, value: string) => {
    setFormLimits(
      formLimits.map((limit, currentIndex) => {
        if (currentIndex !== index) return limit
        const next = { ...limit, [field]: value }
        if (!(next.limitType === 'daily' && next.windowMode === 'fixed')) {
          next.fixedResetTime = ''
        }
        return next
      }),
    )
  }

  if (loading) {
    return <div className="text-center text-muted-foreground">加载中...</div>
  }

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
      <AdminPageShell
        title="订阅套餐"
        description="配置可分配的订阅套餐与额度限制。"
        actions={<Button onClick={handleCreate}>添加套餐</Button>}
      >
        <AnimatePresence>
          {message && (
            <motion.div
              initial={{ opacity: 0, y: -16, scale: 0.98 }}
              animate={{ opacity: 1, y: 0, scale: 1 }}
              exit={{ opacity: 0, y: -16, scale: 0.98 }}
              transition={{ type: 'spring', bounce: 0.25, duration: 0.35 }}
            >
              <Alert variant={message.type === 'success' ? 'default' : 'destructive'}>
                {message.type === 'success' ? <CheckCircle2 className="h-4 w-4" /> : <XCircle className="h-4 w-4" />}
                <AlertDescription>{message.text}</AlertDescription>
              </Alert>
            </motion.div>
          )}
        </AnimatePresence>

        <motion.div initial={{ opacity: 0, y: 24 }} animate={{ opacity: 1, y: 0 }} transition={{ type: 'spring', bounce: 0.2, duration: 0.55 }}>
          <AdminSurface>
            {plans.length === 0 ? (
              <div className="admin-surface-body">
                <div className="rounded-md border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">
                  暂无套餐
                </div>
              </div>
            ) : (
              <>
                <div className="admin-surface-header">
                  <p className="admin-inline-note">支持开关、编辑和删除。</p>
                </div>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>名称</TableHead>
                      <TableHead>描述</TableHead>
                      <TableHead>状态</TableHead>
                      <TableHead>限制数量</TableHead>
                      <TableHead>限制详情</TableHead>
                      <TableHead>创建时间</TableHead>
                      <TableHead className="text-right">操作</TableHead>
                    </TableRow>
                  </TableHeader>
                  <motion.tbody variants={tableStaggerContainer} initial="hidden" animate="visible" key={plans.length}>
                    {plans.map((plan) => (
                      <motion.tr key={plan.id} variants={tableRowVariants}>
                        <TableCell className="max-w-[220px] font-medium">
                          <OverflowCopyText text={plan.name} className="max-w-[220px]" />
                        </TableCell>
                        <TableCell className="max-w-[260px] text-muted-foreground">
                          <OverflowCopyText
                            text={plan.description || '-'}
                            copyValue={plan.description || '-'}
                            tooltipText={plan.description || '暂无描述'}
                            className="max-w-[260px] text-muted-foreground"
                          />
                        </TableCell>
                        <TableCell>
                          <Switch checked={plan.enabled} onCheckedChange={() => handleToggleEnabled(plan)} />
                        </TableCell>
                        <TableCell>{(plan.limits || []).length}</TableCell>
                        <TableCell>
                          <div className="space-y-1.5">
                            {(plan.limits || []).map((limit) => (
                              <div key={limit.id} className="flex items-center gap-2 text-sm">
                                <span className="text-muted-foreground">{LIMIT_TYPE_LABELS[limit.limitType]}</span>
                                <Num
                                  value={limit.limitMicros / 1_000_000}
                                  fullTextOverride={formatUsdLabel(limit.limitMicros)}
                                  interactive
                                  copyable
                                  className="font-medium"
                                />
                              </div>
                            ))}
                          </div>
                        </TableCell>
                        <TableCell className="text-muted-foreground">{formatDateTime(plan.createdAt)}</TableCell>
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-2">
                            <Button variant="ghost" size="sm" onClick={() => handleEdit(plan)}>
                              编辑
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              className="text-destructive hover:text-destructive"
                              onClick={() => setDeleteConfirmModal(plan)}
                            >
                              删除
                            </Button>
                          </div>
                        </TableCell>
                      </motion.tr>
                    ))}
                  </motion.tbody>
                </Table>
              </>
            )}
          </AdminSurface>
        </motion.div>

        <Dialog open={showForm} onOpenChange={setShowForm}>
          <DialogContent className="max-w-2xl max-h-[85vh] overflow-y-auto">
            <DialogHeader>
              <DialogTitle>{editingPlan ? '编辑套餐' : '添加套餐'}</DialogTitle>
              <DialogDescription>{editingPlan ? '修改订阅套餐配置。' : '创建一个新的订阅套餐。'}</DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="planName">名称</Label>
                <Input id="planName" placeholder="套餐名称" value={formName} onChange={(e) => setFormName(e.target.value)} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="planDesc">描述</Label>
                <Textarea
                  id="planDesc"
                  placeholder="可选"
                  value={formDescription}
                  onChange={(e) => setFormDescription(e.target.value)}
                />
              </div>
              <div className="flex items-center justify-between rounded-xl border border-border/70 px-4 py-3">
                <div className="space-y-0.5">
                  <Label htmlFor="planEnabled">启用状态</Label>
                  <p className="text-xs text-muted-foreground">关闭后不会出现在分配列表中。</p>
                </div>
                <Switch id="planEnabled" checked={formEnabled} onCheckedChange={setFormEnabled} />
              </div>

              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <Label>额度限制</Label>
                  <Button type="button" variant="outline" size="sm" onClick={addLimitRow}>
                    <Plus className="mr-1 h-4 w-4" />
                    添加限制
                  </Button>
                </div>
                {formLimits.length === 0 ? (
                  <p className="text-sm text-muted-foreground">暂未配置额度限制</p>
                ) : (
                  <div className="admin-list-divider rounded-xl border border-border/70">
                    {formLimits.map((limit, index) => {
                      const usedTypes = formLimits.filter((_, currentIndex) => currentIndex !== index).map((row) => row.limitType)
                      const showFixedResetTime = limit.limitType === 'daily' && limit.windowMode === 'fixed'
                      return (
                        <div key={`${limit.limitType}-${index}`} className="grid gap-3 px-4 py-4 md:grid-cols-[1fr_1fr_1fr_auto] md:items-end">
                          <div className="space-y-1">
                            <Label className="text-xs">限制类型</Label>
                            <Select value={limit.limitType} onValueChange={(value) => updateLimitRow(index, 'limitType', value)}>
                              <SelectTrigger>
                                <SelectValue />
                              </SelectTrigger>
                              <SelectContent>
                                {ALL_LIMIT_TYPES.map((type) => (
                                  <SelectItem key={type} value={type} disabled={usedTypes.includes(type)}>
                                    {LIMIT_TYPE_LABELS[type]}
                                  </SelectItem>
                                ))}
                              </SelectContent>
                            </Select>
                          </div>
                          <div className="space-y-1">
                            <Label className="text-xs">窗口模式</Label>
                            <Select value={limit.windowMode} onValueChange={(value) => updateLimitRow(index, 'windowMode', value)}>
                              <SelectTrigger>
                                <SelectValue />
                              </SelectTrigger>
                              <SelectContent>
                                {ALL_WINDOW_MODES.map((mode) => (
                                  <SelectItem key={mode} value={mode}>
                                    {WINDOW_MODE_LABELS[mode]}
                                  </SelectItem>
                                ))}
                              </SelectContent>
                            </Select>
                          </div>
                          <div className="space-y-1">
                            <Label className="text-xs">额度 (USD)</Label>
                            <Input
                              type="number"
                              step="0.01"
                              min="0"
                              placeholder="0.00"
                              value={limit.amountUsd}
                              onChange={(e) => updateLimitRow(index, 'amountUsd', e.target.value)}
                            />
                          </div>
                          <Button
                            type="button"
                            variant="ghost"
                            size="sm"
                            className="text-destructive hover:text-destructive"
                            onClick={() => removeLimitRow(index)}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                          {showFixedResetTime ? (
                            <div className="space-y-1 md:col-span-2">
                              <Label className="text-xs">重置时间</Label>
                              <Input
                                type="time"
                                step="60"
                                value={limit.fixedResetTime}
                                onChange={(e) => updateLimitRow(index, 'fixedResetTime', e.target.value)}
                              />
                            </div>
                          ) : null}
                        </div>
                      )
                    })}
                  </div>
                )}
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setShowForm(false)}>
                取消
              </Button>
              <Button onClick={handleSubmit} disabled={saving}>
                {saving ? '保存中...' : '保存'}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={!!deleteConfirmModal} onOpenChange={(open) => !open && setDeleteConfirmModal(null)}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>确认删除</DialogTitle>
              <DialogDescription>
                确定要删除套餐 <span className="font-medium">{deleteConfirmModal?.name}</span> 吗？
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" onClick={() => setDeleteConfirmModal(null)}>
                取消
              </Button>
              <Button variant="destructive" onClick={handleDelete}>
                确认删除
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </AdminPageShell>
    </motion.div>
  )
}
