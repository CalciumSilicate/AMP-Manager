import { useState, useEffect } from 'react'
import { motion, AnimatePresence, tableStaggerContainer, tableRowVariants, fadeInScale } from '@/lib/motion'
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
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
import { CreditCard, Plus, Pencil, Trash2, CheckCircle2, XCircle } from 'lucide-react'

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

function usdToMicros(usd: string): number {
  const val = parseFloat(usd)
  if (isNaN(val)) return 0
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

  useEffect(() => {
    fetchPlans()
  }, [])

  const fetchPlans = async () => {
    try {
      const data = await getPlans()
      setPlans(data)
    } catch (err) {
      showMsg('error', err instanceof Error ? err.message : '获取套餐列表失败')
    } finally {
      setLoading(false)
    }
  }

  const showMsg = (type: 'success' | 'error', text: string) => {
    setMessage({ type, text })
    setTimeout(() => setMessage(null), 3000)
  }

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
      (plan.limits || []).map((l) => ({
        limitType: l.limitType,
        windowMode: l.windowMode,
        amountUsd: microsToUsd(l.limitMicros),
        fixedResetTime: l.fixedResetTime || '',
      }))
    )
    setShowForm(true)
  }

  const handleSubmit = async () => {
    if (!formName.trim()) {
      showMsg('error', '请填写套餐名称')
      return
    }

    const usedTypes = formLimits.map((l) => l.limitType)
    if (new Set(usedTypes).size !== usedTypes.length) {
      showMsg('error', '限制类型不能重复')
      return
    }

    const limits: PlanLimitRequest[] = formLimits.map((l) => ({
      limitType: l.limitType,
      windowMode: l.windowMode,
      limitMicros: usdToMicros(l.amountUsd),
      fixedResetTime: l.limitType === 'daily' && l.windowMode === 'fixed' && l.fixedResetTime ? l.fixedResetTime : undefined,
    }))

    const req: SubscriptionPlanRequest = {
      name: formName.trim(),
      description: formDescription.trim(),
      enabled: formEnabled,
      limits,
    }

    setSaving(true)
    try {
      if (editingPlan) {
        await updatePlan(editingPlan.id, req)
        showMsg('success', '套餐已更新')
      } else {
        await createPlan(req)
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
    const usedTypes = formLimits.map((l) => l.limitType)
    const available = ALL_LIMIT_TYPES.filter((t) => !usedTypes.includes(t))
    if (available.length === 0) {
      showMsg('error', '已添加所有限制类型')
      return
    }
    setFormLimits([...formLimits, { limitType: available[0], windowMode: 'fixed', amountUsd: '', fixedResetTime: '' }])
  }

  const removeLimitRow = (index: number) => {
    setFormLimits(formLimits.filter((_, i) => i !== index))
  }

  const updateLimitRow = (index: number, field: keyof LimitRow, value: string) => {
    setFormLimits(formLimits.map((l, i) => {
      if (i !== index) return l

      const next = { ...l, [field]: value }
      if (!(next.limitType === 'daily' && next.windowMode === 'fixed')) {
        next.fixedResetTime = ''
      }
      return next
    }))
  }

  if (loading) {
    return <div className="text-center text-muted-foreground">加载中...</div>
  }

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="space-y-6">
      <AnimatePresence>
        {message && (
          <motion.div initial={{ opacity: 0, y: -20, scale: 0.95 }} animate={{ opacity: 1, y: 0, scale: 1 }} exit={{ opacity: 0, y: -20, scale: 0.95 }} transition={{ type: 'spring', bounce: 0.3, duration: 0.5 }}>
            <Alert variant={message.type === 'success' ? 'default' : 'destructive'}>
              {message.type === 'success' ? (
                <CheckCircle2 className="h-4 w-4" />
              ) : (
                <XCircle className="h-4 w-4" />
              )}
              <AlertDescription>{message.text}</AlertDescription>
            </Alert>
          </motion.div>
        )}
      </AnimatePresence>

      <motion.div initial={{ opacity: 0, y: 30 }} animate={{ opacity: 1, y: 0 }} transition={{ type: 'spring', bounce: 0.2, duration: 0.6, delay: 0.1 }}>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-4">
            <CardTitle className="flex items-center gap-2">
              <CreditCard className="h-5 w-5" />
              订阅套餐列表
            </CardTitle>
            <Button onClick={handleCreate}>
              <Plus className="mr-1 h-4 w-4" />
              添加套餐
            </Button>
          </CardHeader>
          <CardContent>
            <AnimatePresence mode="wait">
              {plans.length === 0 ? (
                <AnimatePresence>
                  <motion.div
                    key="empty"
                    variants={fadeInScale}
                    initial="hidden"
                    animate="visible"
                    exit="hidden"
                    className="rounded-md border border-dashed p-8 text-center text-muted-foreground"
                  >
                    暂无套餐，点击上方按钮添加
                  </motion.div>
                </AnimatePresence>
              ) : (
                <motion.div key="table" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>名称</TableHead>
                        <TableHead>描述</TableHead>
                        <TableHead>状态</TableHead>
                        <TableHead>限制数量</TableHead>
                        <TableHead>限制详情</TableHead>
                        <TableHead>创建时间</TableHead>
                        <TableHead>操作</TableHead>
                      </TableRow>
                    </TableHeader>
                    <motion.tbody variants={tableStaggerContainer} initial="hidden" animate="visible" key={plans.length}>
                      {plans.map((plan) => (
                        <motion.tr key={plan.id} variants={tableRowVariants} className="border-b transition-colors hover:bg-muted/50 data-[state=selected]:bg-muted">
                          <TableCell className="font-medium">{plan.name}</TableCell>
                          <TableCell className="text-muted-foreground max-w-[200px] truncate">
                            {plan.description || '-'}
                          </TableCell>
                          <TableCell>
                            <div className="flex items-center gap-2">
                              <Switch
                                checked={plan.enabled}
                                onCheckedChange={() => handleToggleEnabled(plan)}
                              />
                              <Badge variant={plan.enabled ? 'default' : 'secondary'}>
                                {plan.enabled ? '启用' : '禁用'}
                              </Badge>
                            </div>
                          </TableCell>
                          <TableCell>
                            <Badge variant="secondary">{(plan.limits || []).length}</Badge>
                          </TableCell>
                          <TableCell>
                            <div className="flex flex-wrap gap-1">
                              {(plan.limits || []).map((l) => (
                                <Badge key={l.id} variant="outline" className="text-xs">
                                  {LIMIT_TYPE_LABELS[l.limitType]}: ${microsToUsd(l.limitMicros)}
                                  {l.fixedResetTime ? ` @ ${l.fixedResetTime}` : ''}
                                </Badge>
                              ))}
                            </div>
                          </TableCell>
                          <TableCell className="text-muted-foreground">
                            {formatDateTime(plan.createdAt)}
                          </TableCell>
                          <TableCell>
                            <div className="flex gap-2">
                              <Button variant="outline" size="sm" onClick={() => handleEdit(plan)}>
                                <Pencil className="mr-1 h-4 w-4" />
                                编辑
                              </Button>
                              <Button variant="destructive" size="sm" onClick={() => setDeleteConfirmModal(plan)}>
                                <Trash2 className="mr-1 h-4 w-4" />
                                删除
                              </Button>
                            </div>
                          </TableCell>
                        </motion.tr>
                      ))}
                    </motion.tbody>
                  </Table>
                </motion.div>
              )}
            </AnimatePresence>
          </CardContent>
        </Card>
      </motion.div>

      {/* Create/Edit Dialog */}
      <Dialog open={showForm} onOpenChange={setShowForm}>
        <DialogContent className="max-w-2xl max-h-[85vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{editingPlan ? '编辑套餐' : '添加套餐'}</DialogTitle>
            <DialogDescription>
              {editingPlan ? '修改订阅套餐配置' : '创建一个新的订阅套餐'}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="planName">名称 *</Label>
              <Input
                id="planName"
                placeholder="套餐名称"
                value={formName}
                onChange={(e) => setFormName(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="planDesc">描述</Label>
              <Textarea
                id="planDesc"
                placeholder="套餐描述（可选）"
                value={formDescription}
                onChange={(e) => setFormDescription(e.target.value)}
              />
            </div>
            <div className="flex items-center gap-3">
              <Label htmlFor="planEnabled">启用</Label>
              <Switch
                id="planEnabled"
                checked={formEnabled}
                onCheckedChange={setFormEnabled}
              />
            </div>

            {/* Limits */}
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <Label>额度限制</Label>
                <Button type="button" variant="outline" size="sm" onClick={addLimitRow}>
                  <Plus className="mr-1 h-4 w-4" />
                  添加限制
                </Button>
              </div>
              {formLimits.length === 0 && (
                <p className="text-sm text-muted-foreground">暂未配置额度限制</p>
              )}
              {formLimits.map((limit, index) => {
                const usedTypes = formLimits.filter((_, i) => i !== index).map((l) => l.limitType)
                const showFixedResetTime = limit.limitType === 'daily' && limit.windowMode === 'fixed'
                return (
                  <div key={index} className="flex items-end gap-2 rounded-lg border p-3">
                    <div className="flex-1 space-y-1">
                      <Label className="text-xs">限制类型</Label>
                      <Select
                        value={limit.limitType}
                        onValueChange={(v) => updateLimitRow(index, 'limitType', v)}
                      >
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {ALL_LIMIT_TYPES.map((t) => (
                            <SelectItem key={t} value={t} disabled={usedTypes.includes(t)}>
                              {LIMIT_TYPE_LABELS[t]}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="flex-1 space-y-1">
                      <Label className="text-xs">窗口模式</Label>
                      <Select
                        value={limit.windowMode}
                        onValueChange={(v) => updateLimitRow(index, 'windowMode', v)}
                      >
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {ALL_WINDOW_MODES.map((m) => (
                            <SelectItem key={m} value={m}>
                              {WINDOW_MODE_LABELS[m]}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="flex-1 space-y-1">
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
                    {showFixedResetTime && (
                      <div className="w-[132px] space-y-1">
                        <Label className="text-xs">重置时间</Label>
                        <Input
                          type="time"
                          step="60"
                          value={limit.fixedResetTime}
                          onChange={(e) => updateLimitRow(index, 'fixedResetTime', e.target.value)}
                        />
                      </div>
                    )}
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="text-destructive hover:text-destructive"
                      onClick={() => removeLimitRow(index)}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                )
              })}
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

      {/* Delete Confirmation */}
      <Dialog open={!!deleteConfirmModal} onOpenChange={(open) => !open && setDeleteConfirmModal(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>确认删除</DialogTitle>
            <DialogDescription>
              确定要删除套餐 <span className="font-medium">{deleteConfirmModal?.name}</span> 吗？此操作不可撤销。
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
    </motion.div>
  )
}
