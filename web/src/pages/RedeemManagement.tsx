import { useEffect, useState } from 'react'

import {
  createRedeemBatch,
  createRedeemCampaign,
  deleteRedeemCampaign,
  downloadRedeemBatchCSV,
  listRedeemBatches,
  listRedeemCampaigns,
  listRedeemCodes,
  listRedeemRedemptions,
  setRedeemCodeEnabled,
  updateRedeemCampaign,
  type RedeemBatch,
  type RedeemBatchRequest,
  type RedeemCampaign,
  type RedeemCampaignRequest,
  type RedeemCode,
  type RedeemRedemption,
} from '@/api/redeem'
import { getPlans, type SubscriptionPlanResponse } from '@/api/subscription'
import { AdminPageShell, AdminSurface, AdminToolbarRow } from '@/components/admin/AdminPageShell'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { formatDateTime, formatDecimal } from '@/lib/formatters'
import { motion } from '@/lib/motion'
import { CheckCircle2, Copy, Download, Plus, RefreshCw, TicketPercent, Trash2 } from 'lucide-react'

type CampaignFormState = {
  name: string
  description: string
  codeMode: 'single_use' | 'shared'
  sharedCode: string
  subscriptionPlanId: string
  subscriptionDurationDays: number
  balanceUsd: string
  totalRedemptionsLimit: number
  perUserLimit: number
  startsAt: string
  endsAt: string
  enabled: boolean
}

const initialCampaignForm = (): CampaignFormState => ({
  name: '',
  description: '',
  codeMode: 'single_use',
  sharedCode: '',
  subscriptionPlanId: '',
  subscriptionDurationDays: 30,
  balanceUsd: '',
  totalRedemptionsLimit: 0,
  perUserLimit: 1,
  startsAt: '',
  endsAt: '',
  enabled: true,
})

const initialBatchForm = (): RedeemBatchRequest => ({
  name: '',
  prefix: '',
  codeCount: 50,
  codeLength: 10,
})

function microsToUsdLabel(value: number): string {
  if (value <= 0) return '-'
  return `$${formatDecimal(value / 1_000_000, 2)}`
}

function usdToMicros(value: string): number {
  const amount = Number.parseFloat(value)
  if (Number.isNaN(amount) || amount <= 0) return 0
  return Math.round(amount * 1_000_000)
}

function normalizeDatetimeLocal(value: string): string | undefined {
  if (!value) return undefined
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return undefined
  return parsed.toISOString()
}

function toDatetimeLocal(value?: string | null): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const offset = date.getTimezoneOffset()
  const local = new Date(date.getTime() - offset * 60_000)
  return local.toISOString().slice(0, 16)
}

function campaignReward(item: RedeemCampaign): string {
  const parts: string[] = []
  if (item.subscriptionPlanName) {
    parts.push(`${item.subscriptionPlanName} ${item.subscriptionDurationDays}天`)
  }
  if (item.balanceMicros > 0) {
    parts.push(microsToUsdLabel(item.balanceMicros))
  }
  return parts.join(' + ') || '-'
}

function downloadTextFile(filename: string, content: string): void {
  const blob = new Blob([content], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  anchor.click()
  URL.revokeObjectURL(url)
}

export default function RedeemManagement() {
  const [loading, setLoading] = useState(true)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [campaigns, setCampaigns] = useState<RedeemCampaign[]>([])
  const [plans, setPlans] = useState<SubscriptionPlanResponse[]>([])
  const [batches, setBatches] = useState<RedeemBatch[]>([])
  const [codes, setCodes] = useState<RedeemCode[]>([])
  const [redemptions, setRedemptions] = useState<RedeemRedemption[]>([])

  const [campaignDialogOpen, setCampaignDialogOpen] = useState(false)
  const [editingCampaign, setEditingCampaign] = useState<RedeemCampaign | null>(null)
  const [campaignForm, setCampaignForm] = useState<CampaignFormState>(initialCampaignForm())
  const [savingCampaign, setSavingCampaign] = useState(false)

  const [batchDialogCampaign, setBatchDialogCampaign] = useState<RedeemCampaign | null>(null)
  const [batchForm, setBatchForm] = useState<RedeemBatchRequest>(initialBatchForm())
  const [savingBatch, setSavingBatch] = useState(false)
  const [latestBatchCodes, setLatestBatchCodes] = useState<string[]>([])

  const [codeFilters, setCodeFilters] = useState({
    campaignId: 'all',
    status: 'all',
    keyword: '',
  })
  const [redemptionFilters, setRedemptionFilters] = useState({
    campaignId: 'all',
    status: 'all',
    username: '',
  })
  const [togglingCodeId, setTogglingCodeId] = useState<string | null>(null)

  useEffect(() => {
    void loadAll()
  }, [])

  const showMessage = (type: 'success' | 'error', text: string) => {
    setMessage({ type, text })
    window.setTimeout(() => setMessage(null), 3200)
  }

  const loadAll = async () => {
    setLoading(true)
    try {
      const [campaignList, planList, batchList, codeList, redemptionList] = await Promise.all([
        listRedeemCampaigns(),
        getPlans(),
        listRedeemBatches(),
        listRedeemCodes({ limit: 200 }),
        listRedeemRedemptions({ limit: 200 }),
      ])
      setCampaigns(campaignList)
      setPlans(planList)
      setBatches(batchList)
      setCodes(codeList)
      setRedemptions(redemptionList)
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }

  const refreshCodes = async () => {
    const codeList = await listRedeemCodes({
      campaignId: codeFilters.campaignId === 'all' ? '' : codeFilters.campaignId,
      status: codeFilters.status === 'all' ? '' : codeFilters.status as 'active' | 'disabled' | 'consumed',
      keyword: codeFilters.keyword.trim(),
      limit: 200,
    })
    setCodes(codeList)
  }

  const refreshRedemptions = async () => {
    const redemptionList = await listRedeemRedemptions({
      campaignId: redemptionFilters.campaignId === 'all' ? '' : redemptionFilters.campaignId,
      status: redemptionFilters.status === 'all' ? '' : redemptionFilters.status as 'success' | 'rejected',
      username: redemptionFilters.username.trim(),
      limit: 200,
    })
    setRedemptions(redemptionList)
  }

  const handleOpenCreateCampaign = () => {
    setEditingCampaign(null)
    setCampaignForm(initialCampaignForm())
    setCampaignDialogOpen(true)
  }

  const handleOpenEditCampaign = (item: RedeemCampaign) => {
    setEditingCampaign(item)
    setCampaignForm({
      name: item.name,
      description: item.description,
      codeMode: item.codeMode,
      sharedCode: item.sharedCode || '',
      subscriptionPlanId: item.subscriptionPlanId || '',
      subscriptionDurationDays: item.subscriptionDurationDays || 30,
      balanceUsd: item.balanceMicros > 0 ? String(item.balanceMicros / 1_000_000) : '',
      totalRedemptionsLimit: item.totalRedemptionsLimit,
      perUserLimit: item.perUserLimit,
      startsAt: toDatetimeLocal(item.startsAt),
      endsAt: toDatetimeLocal(item.endsAt),
      enabled: item.enabled,
    })
    setCampaignDialogOpen(true)
  }

  const handleSaveCampaign = async () => {
    if (!campaignForm.name.trim()) {
      showMessage('error', '请填写活动名称')
      return
    }

    const payload: RedeemCampaignRequest = {
      name: campaignForm.name.trim(),
      description: campaignForm.description.trim(),
      codeMode: campaignForm.codeMode,
      sharedCode: campaignForm.codeMode === 'shared' ? campaignForm.sharedCode.trim() : '',
      subscriptionPlanId: campaignForm.subscriptionPlanId,
      subscriptionDurationDays: campaignForm.subscriptionPlanId ? campaignForm.subscriptionDurationDays : 0,
      balanceMicros: usdToMicros(campaignForm.balanceUsd),
      totalRedemptionsLimit: campaignForm.totalRedemptionsLimit,
      perUserLimit: campaignForm.perUserLimit,
      startsAt: normalizeDatetimeLocal(campaignForm.startsAt),
      endsAt: normalizeDatetimeLocal(campaignForm.endsAt),
      enabled: campaignForm.enabled,
    }

    setSavingCampaign(true)
    try {
      if (editingCampaign) {
        await updateRedeemCampaign(editingCampaign.id, payload)
        showMessage('success', '兑换活动已更新')
      } else {
        await createRedeemCampaign(payload)
        showMessage('success', '兑换活动已创建')
      }
      setCampaignDialogOpen(false)
      await loadAll()
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '保存失败')
    } finally {
      setSavingCampaign(false)
    }
  }

  const handleDeleteCampaign = async (item: RedeemCampaign) => {
    if (!window.confirm(`确定删除活动 ${item.name} 吗？`)) return
    try {
      await deleteRedeemCampaign(item.id)
      showMessage('success', '兑换活动已删除')
      await loadAll()
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '删除失败')
    }
  }

  const handleCreateBatch = async () => {
    if (!batchDialogCampaign) return
    setSavingBatch(true)
    try {
      const result = await createRedeemBatch(batchDialogCampaign.id, batchForm)
      setLatestBatchCodes(result.codes)
      showMessage('success', `已生成 ${result.codes.length} 个兑换码`)
      setBatchDialogCampaign(null)
      setBatchForm(initialBatchForm())
      await loadAll()
      downloadTextFile(`${result.batch.name}.txt`, result.codes.join('\n'))
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '生成失败')
    } finally {
      setSavingBatch(false)
    }
  }

  const handleExportBatch = async (item: RedeemBatch) => {
    try {
      const blob = await downloadRedeemBatchCSV(item.id)
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `${item.name}.csv`
      anchor.click()
      URL.revokeObjectURL(url)
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '导出失败')
    }
  }

  const handleToggleCode = async (item: RedeemCode, enabled: boolean) => {
    setTogglingCodeId(item.id)
    try {
      await setRedeemCodeEnabled(item.id, enabled)
      showMessage('success', enabled ? '兑换码已启用' : '兑换码已停用')
      await refreshCodes()
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '更新失败')
    } finally {
      setTogglingCodeId(null)
    }
  }

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <RefreshCw className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
      <AdminPageShell
        title="兑换码管理"
        description="管理活动、批次、码明细与兑换记录。"
        width="7xl"
        actions={(
          <Button onClick={handleOpenCreateCampaign}>
            <Plus className="mr-2 h-4 w-4" />
            新建活动
          </Button>
        )}
      >
        {message && (
          <Alert variant={message.type === 'error' ? 'destructive' : 'default'}>
            {message.type === 'success' ? <CheckCircle2 className="h-4 w-4" /> : null}
            <AlertDescription>{message.text}</AlertDescription>
          </Alert>
        )}

        {latestBatchCodes.length > 0 && (
          <Alert>
            <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
              <span>最近一次批量生成了 {latestBatchCodes.length} 个兑换码。</span>
              <Button variant="outline" size="sm" onClick={() => navigator.clipboard.writeText(latestBatchCodes.join('\n')).catch(() => undefined)}>
                <Copy className="mr-2 h-4 w-4" />
                复制全部
              </Button>
            </AlertDescription>
          </Alert>
        )}

        <AdminSurface>
          <div className="admin-surface-header">
            <div className="space-y-1">
              <p className="text-sm font-medium text-foreground">{campaigns.length} 个活动</p>
              <p className="admin-inline-note">共享码和单次码共用一套奖励配置。</p>
            </div>
            <Button variant="outline" size="sm" onClick={() => void loadAll()}>
              <RefreshCw className="mr-2 h-4 w-4" />
              刷新
            </Button>
          </div>
          <div className="overflow-hidden rounded-b-xl">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>活动</TableHead>
                  <TableHead>模式</TableHead>
                  <TableHead>奖励</TableHead>
                  <TableHead>限制</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead>码量</TableHead>
                  <TableHead className="text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {campaigns.map((item) => (
                  <TableRow key={item.id}>
                    <TableCell>
                      <div>
                        <p className="font-medium">{item.name}</p>
                        <p className="text-xs text-muted-foreground">{item.description || '-'}</p>
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-wrap gap-2">
                        <Badge variant="outline">{item.codeMode === 'shared' ? '共享码' : '单次码'}</Badge>
                        {item.codeMode === 'shared' && item.sharedCodeMask ? (
                          <Badge variant="secondary">{item.sharedCodeMask}</Badge>
                        ) : null}
                      </div>
                    </TableCell>
                    <TableCell>{campaignReward(item)}</TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      <div>总上限 {item.totalRedemptionsLimit > 0 ? item.totalRedemptionsLimit : '不限'}</div>
                      <div>单用户 {item.perUserLimit}</div>
                    </TableCell>
                    <TableCell>
                      <Badge variant={item.enabled ? 'default' : 'secondary'}>
                        {item.enabled ? '启用' : '停用'}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      <div>{item.redeemedCount} / {item.totalRedemptionsLimit > 0 ? item.totalRedemptionsLimit : '∞'}</div>
                      <div>{item.codeCount} 个码 / {item.batchCount} 批</div>
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex flex-wrap justify-end gap-2">
                        {item.codeMode === 'shared' && item.sharedCode ? (
                          <Button variant="outline" size="sm" onClick={() => navigator.clipboard.writeText(item.sharedCode || '').catch(() => undefined)}>
                            <Copy className="mr-2 h-4 w-4" />
                            复制码
                          </Button>
                        ) : null}
                        {item.codeMode === 'single_use' ? (
                          <Button variant="outline" size="sm" onClick={() => setBatchDialogCampaign(item)}>
                            <TicketPercent className="mr-2 h-4 w-4" />
                            生成批次
                          </Button>
                        ) : null}
                        <Button variant="outline" size="sm" onClick={() => handleOpenEditCampaign(item)}>编辑</Button>
                        <Button variant="outline" size="sm" onClick={() => handleDeleteCampaign(item)} className="text-destructive hover:text-destructive">
                          <Trash2 className="mr-2 h-4 w-4" />
                          删除
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </AdminSurface>

        <div className="grid gap-5 xl:grid-cols-2">
          <AdminSurface>
            <AdminToolbarRow>
              <div className="space-y-1">
                <p className="text-sm font-medium text-foreground">批次</p>
                <p className="admin-inline-note">单次码批量生成与导出。</p>
              </div>
              <Select
                value={codeFilters.campaignId}
                onValueChange={async (value) => {
                  setCodeFilters((current) => ({ ...current, campaignId: value }))
                  setBatches(await listRedeemBatches(value === 'all' ? '' : value))
                }}
              >
                <SelectTrigger className="w-[220px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">全部活动</SelectItem>
                  {campaigns.filter((item) => item.codeMode === 'single_use').map((item) => (
                    <SelectItem key={item.id} value={item.id}>{item.name}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </AdminToolbarRow>
            <div className="overflow-hidden rounded-b-xl">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>批次</TableHead>
                    <TableHead>活动</TableHead>
                    <TableHead>数量</TableHead>
                    <TableHead>前缀</TableHead>
                    <TableHead>创建时间</TableHead>
                    <TableHead className="text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {batches.map((item) => (
                    <TableRow key={item.id}>
                      <TableCell className="font-medium">{item.name}</TableCell>
                      <TableCell>{item.campaignName}</TableCell>
                      <TableCell>{item.codeCount}</TableCell>
                      <TableCell>{item.prefix || '-'}</TableCell>
                      <TableCell className="text-muted-foreground">{formatDateTime(item.createdAt)}</TableCell>
                      <TableCell className="text-right">
                        <Button variant="outline" size="sm" onClick={() => void handleExportBatch(item)}>
                          <Download className="mr-2 h-4 w-4" />
                          导出
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </AdminSurface>

          <AdminSurface>
            <AdminToolbarRow>
              <div className="space-y-1">
                <p className="text-sm font-medium text-foreground">兑换码明细</p>
                <p className="admin-inline-note">支持筛选、搜索和停用。</p>
              </div>
              <div className="flex flex-wrap gap-2">
                <Select
                  value={codeFilters.status}
                  onValueChange={(value) => setCodeFilters((current) => ({ ...current, status: value }))}
                >
                  <SelectTrigger className="w-[150px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">全部状态</SelectItem>
                    <SelectItem value="active">启用</SelectItem>
                    <SelectItem value="disabled">停用</SelectItem>
                    <SelectItem value="consumed">已用尽</SelectItem>
                  </SelectContent>
                </Select>
                <Input
                  value={codeFilters.keyword}
                  onChange={(event) => setCodeFilters((current) => ({ ...current, keyword: event.target.value }))}
                  placeholder="搜索码或活动"
                  className="w-[220px]"
                />
                <Button variant="outline" size="sm" onClick={() => void refreshCodes()}>
                  查询
                </Button>
              </div>
            </AdminToolbarRow>
            <div className="overflow-hidden rounded-b-xl">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>兑换码</TableHead>
                    <TableHead>活动</TableHead>
                    <TableHead>批次</TableHead>
                    <TableHead>次数</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead className="text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {codes.map((item) => (
                    <TableRow key={item.id}>
                      <TableCell className="font-mono text-xs">{item.codeMode === 'shared' ? item.codeValue : item.codeMask}</TableCell>
                      <TableCell>{item.campaignName}</TableCell>
                      <TableCell>{item.batchName || '-'}</TableCell>
                      <TableCell>{item.redeemedCount} / {item.maxRedemptions > 0 ? item.maxRedemptions : '∞'}</TableCell>
                      <TableCell>
                        <Badge variant={item.status === 'active' ? 'default' : 'secondary'}>{item.status}</Badge>
                      </TableCell>
                      <TableCell className="text-right">
                        {item.status !== 'consumed' ? (
                          <div className="flex items-center justify-end gap-3">
                            <span className="text-xs text-muted-foreground">{item.status === 'active' ? '启用' : '停用'}</span>
                            <Switch
                              checked={item.status === 'active'}
                              disabled={togglingCodeId === item.id}
                              onCheckedChange={(checked) => void handleToggleCode(item, checked)}
                            />
                          </div>
                        ) : (
                          <span className="text-xs text-muted-foreground">已用尽</span>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </AdminSurface>
        </div>

        <AdminSurface>
          <AdminToolbarRow>
            <div className="space-y-1">
              <p className="text-sm font-medium text-foreground">兑换记录</p>
              <p className="admin-inline-note">包含成功与拒绝记录。</p>
            </div>
            <div className="flex flex-wrap gap-2">
              <Select
                value={redemptionFilters.campaignId}
                onValueChange={(value) => setRedemptionFilters((current) => ({ ...current, campaignId: value }))}
              >
                <SelectTrigger className="w-[220px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">全部活动</SelectItem>
                  {campaigns.map((item) => (
                    <SelectItem key={item.id} value={item.id}>{item.name}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Select
                value={redemptionFilters.status}
                onValueChange={(value) => setRedemptionFilters((current) => ({ ...current, status: value }))}
              >
                <SelectTrigger className="w-[150px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">全部结果</SelectItem>
                  <SelectItem value="success">成功</SelectItem>
                  <SelectItem value="rejected">拒绝</SelectItem>
                </SelectContent>
              </Select>
              <Input
                value={redemptionFilters.username}
                onChange={(event) => setRedemptionFilters((current) => ({ ...current, username: event.target.value }))}
                placeholder="用户名"
                className="w-[180px]"
              />
              <Button variant="outline" size="sm" onClick={() => void refreshRedemptions()}>
                查询
              </Button>
            </div>
          </AdminToolbarRow>
          <div className="overflow-hidden rounded-b-xl">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>时间</TableHead>
                  <TableHead>用户</TableHead>
                  <TableHead>活动</TableHead>
                  <TableHead>码</TableHead>
                  <TableHead>奖励</TableHead>
                  <TableHead>结果</TableHead>
                  <TableHead>说明</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {redemptions.map((item) => (
                  <TableRow key={item.id}>
                    <TableCell className="text-muted-foreground">{formatDateTime(item.createdAt)}</TableCell>
                    <TableCell>{item.username}</TableCell>
                    <TableCell>{item.campaignName || '-'}</TableCell>
                    <TableCell className="font-mono text-xs">{item.codeMask || '-'}</TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      <div>{item.subscriptionPlanName ? `${item.subscriptionPlanName} ${item.subscriptionDurationDays}天` : '-'}</div>
                      {item.balanceMicros > 0 ? <div>{microsToUsdLabel(item.balanceMicros)}</div> : null}
                    </TableCell>
                    <TableCell>
                      <Badge variant={item.status === 'success' ? 'default' : 'secondary'}>
                        {item.status === 'success' ? '成功' : '拒绝'}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {item.status === 'success'
                        ? item.grantedExpiresAt
                          ? `到期 ${formatDateTime(item.grantedExpiresAt)}`
                          : item.balanceAfterMicros > 0
                            ? `余额 ${microsToUsdLabel(item.balanceAfterMicros)}`
                            : '-'
                        : item.failureReason || '-'}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </AdminSurface>

        <Dialog open={campaignDialogOpen} onOpenChange={setCampaignDialogOpen}>
          <DialogContent className="sm:max-w-2xl">
            <DialogHeader>
              <DialogTitle>{editingCampaign ? '编辑兑换活动' : '新建兑换活动'}</DialogTitle>
              <DialogDescription>奖励支持订阅和余额，可单独发放，也可组合发放。</DialogDescription>
            </DialogHeader>
            <div className="grid gap-4 py-2 sm:grid-cols-2">
              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="campaignName">活动名称</Label>
                <Input
                  id="campaignName"
                  value={campaignForm.name}
                  onChange={(event) => setCampaignForm((current) => ({ ...current, name: event.target.value }))}
                />
              </div>
              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="campaignDesc">说明</Label>
                <Input
                  id="campaignDesc"
                  value={campaignForm.description}
                  onChange={(event) => setCampaignForm((current) => ({ ...current, description: event.target.value }))}
                />
              </div>
              <div className="space-y-2">
                <Label>码模式</Label>
                <Select
                  value={campaignForm.codeMode}
                  onValueChange={(value: 'single_use' | 'shared') => setCampaignForm((current) => ({ ...current, codeMode: value }))}
                  disabled={!!editingCampaign}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="single_use">单次码</SelectItem>
                    <SelectItem value="shared">共享码</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="perUserLimit">单用户次数</Label>
                <Input
                  id="perUserLimit"
                  type="number"
                  min="1"
                  value={campaignForm.perUserLimit}
                  onChange={(event) => setCampaignForm((current) => ({ ...current, perUserLimit: Number(event.target.value || 1) }))}
                />
              </div>
              {campaignForm.codeMode === 'shared' ? (
                <div className="space-y-2 sm:col-span-2">
                  <Label htmlFor="sharedCode">共享兑换码</Label>
                  <Input
                    id="sharedCode"
                    value={campaignForm.sharedCode}
                    onChange={(event) => setCampaignForm((current) => ({ ...current, sharedCode: event.target.value.toUpperCase() }))}
                  />
                </div>
              ) : null}
              <div className="space-y-2">
                <Label>订阅套餐</Label>
                <Select
                  value={campaignForm.subscriptionPlanId || 'none'}
                  onValueChange={(value) => setCampaignForm((current) => ({ ...current, subscriptionPlanId: value === 'none' ? '' : value }))}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">不发订阅</SelectItem>
                    {plans.map((plan) => (
                      <SelectItem key={plan.id} value={plan.id}>{plan.name}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="durationDays">订阅时长</Label>
                <Input
                  id="durationDays"
                  type="number"
                  min="0"
                  value={campaignForm.subscriptionDurationDays}
                  onChange={(event) => setCampaignForm((current) => ({ ...current, subscriptionDurationDays: Number(event.target.value || 0) }))}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="balanceUsd">余额奖励 (USD)</Label>
                <Input
                  id="balanceUsd"
                  value={campaignForm.balanceUsd}
                  onChange={(event) => setCampaignForm((current) => ({ ...current, balanceUsd: event.target.value }))}
                  placeholder="例如 5"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="totalLimit">总兑换上限</Label>
                <Input
                  id="totalLimit"
                  type="number"
                  min="0"
                  value={campaignForm.totalRedemptionsLimit}
                  onChange={(event) => setCampaignForm((current) => ({ ...current, totalRedemptionsLimit: Number(event.target.value || 0) }))}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="startsAt">开始时间</Label>
                <Input
                  id="startsAt"
                  type="datetime-local"
                  value={campaignForm.startsAt}
                  onChange={(event) => setCampaignForm((current) => ({ ...current, startsAt: event.target.value }))}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="endsAt">结束时间</Label>
                <Input
                  id="endsAt"
                  type="datetime-local"
                  value={campaignForm.endsAt}
                  onChange={(event) => setCampaignForm((current) => ({ ...current, endsAt: event.target.value }))}
                />
              </div>
              <div className="flex items-center justify-between rounded-xl border border-border/70 px-4 py-3 sm:col-span-2">
                <div>
                  <p className="text-sm font-medium">启用活动</p>
                  <p className="text-xs text-muted-foreground">停用后用户不可兑换</p>
                </div>
                <Switch
                  checked={campaignForm.enabled}
                  onCheckedChange={(checked) => setCampaignForm((current) => ({ ...current, enabled: checked }))}
                />
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setCampaignDialogOpen(false)}>取消</Button>
              <Button onClick={handleSaveCampaign} disabled={savingCampaign}>
                {savingCampaign ? <RefreshCw className="mr-2 h-4 w-4 animate-spin" /> : null}
                保存
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={!!batchDialogCampaign} onOpenChange={(open) => !open && setBatchDialogCampaign(null)}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>生成兑换批次</DialogTitle>
              <DialogDescription>{batchDialogCampaign?.name || '-'}</DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-2">
              <div className="space-y-2">
                <Label htmlFor="batchName">批次名称</Label>
                <Input
                  id="batchName"
                  value={batchForm.name}
                  onChange={(event) => setBatchForm((current) => ({ ...current, name: event.target.value }))}
                />
              </div>
              <div className="grid gap-4 sm:grid-cols-3">
                <div className="space-y-2">
                  <Label htmlFor="batchPrefix">前缀</Label>
                  <Input
                    id="batchPrefix"
                    value={batchForm.prefix}
                    onChange={(event) => setBatchForm((current) => ({ ...current, prefix: event.target.value.toUpperCase() }))}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="batchCount">数量</Label>
                  <Input
                    id="batchCount"
                    type="number"
                    min="1"
                    max="5000"
                    value={batchForm.codeCount}
                    onChange={(event) => setBatchForm((current) => ({ ...current, codeCount: Number(event.target.value || 1) }))}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="codeLength">随机长度</Label>
                  <Input
                    id="codeLength"
                    type="number"
                    min="6"
                    max="32"
                    value={batchForm.codeLength}
                    onChange={(event) => setBatchForm((current) => ({ ...current, codeLength: Number(event.target.value || 10) }))}
                  />
                </div>
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setBatchDialogCampaign(null)}>取消</Button>
              <Button onClick={handleCreateBatch} disabled={savingBatch}>
                {savingBatch ? <RefreshCw className="mr-2 h-4 w-4 animate-spin" /> : null}
                生成
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </AdminPageShell>
    </motion.div>
  )
}
