import { type ReactNode, useEffect, useState } from 'react'

import {
  createRedeemBatch,
  createRedeemCampaign,
  createManualRedeemCode,
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
  type ManualRedeemCodeRequest,
  type RedeemRedemption,
} from '@/api/redeem'
import { getPlans, type SubscriptionPlanResponse } from '@/api/subscription'
import { TablePagination } from '@/components/TablePagination'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { TabbedSettingsPage, type TabbedSettingsPageTab } from '@/components/layout/TabbedSettingsPage'
import { formatDateTime, formatDecimal } from '@/lib/formatters'
import { Copy, Download, Plus, RefreshCw, TicketPercent, Trash2 } from 'lucide-react'

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

const initialManualCodeForm = (): ManualRedeemCodeRequest => ({
  campaignId: '',
  codeValue: '',
  subscriptionPlanId: '',
  subscriptionDurationDays: 30,
  balanceMicros: 0,
  perUserLimit: 1,
  maxRedemptions: 1,
  enabled: true,
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
  return rewardSummary(item.subscriptionPlanName, item.subscriptionDurationDays, item.balanceMicros)
}

function rewardSummary(subscriptionPlanName?: string | null, subscriptionDurationDays = 0, balanceMicros = 0): string {
  const parts: string[] = []
  if (subscriptionPlanName) {
    parts.push(`${subscriptionPlanName} ${subscriptionDurationDays}天`)
  }
  if (balanceMicros > 0) {
    parts.push(microsToUsdLabel(balanceMicros))
  }
  return parts.join(' + ') || '-'
}

function formatRedeemWindow(startsAt?: string | null, endsAt?: string | null): string {
  if (!startsAt && !endsAt) return '长期有效'
  if (startsAt && endsAt) return `${formatDateTime(startsAt)} - ${formatDateTime(endsAt)}`
  if (startsAt) return `${formatDateTime(startsAt)} 起`
  return endsAt ? `${formatDateTime(endsAt)} 止` : '长期有效'
}

function getCodeSourceLabel(sourceType: RedeemCode['sourceType']): string {
  if (sourceType === 'purchase_order') return '购后码'
  if (sourceType === 'free') return '自由码'
  return '活动码'
}

function getCodeStatusLabel(status: RedeemCode['status']): string {
  switch (status) {
    case 'active':
      return '启用'
    case 'disabled':
      return '停用'
    default:
      return '已用尽'
  }
}

function redemptionResultLabel(status: RedeemRedemption['status']): string {
  return status === 'success' ? '成功' : '拒绝'
}

function redemptionResultDescription(item: RedeemRedemption): string {
  if (item.status !== 'success') return item.failureReason || '-'
  if (item.grantedExpiresAt) return `到期 ${formatDateTime(item.grantedExpiresAt)}`
  if (item.balanceAfterMicros > 0) return `余额 ${microsToUsdLabel(item.balanceAfterMicros)}`
  return '-'
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

function downloadTextFile(filename: string, content: string): void {
  const blob = new Blob([content], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  anchor.click()
  URL.revokeObjectURL(url)
}

function paginateItems<T>(items: T[], page: number, pageSize: number) {
  const totalPages = Math.max(1, Math.ceil(items.length / pageSize))
  const currentPage = Math.min(page, totalPages)
  const visibleItems = items.slice((currentPage - 1) * pageSize, currentPage * pageSize)

  return {
    currentPage,
    visibleItems,
  }
}

type RedeemManagementTab = 'campaigns' | 'batches' | 'codes' | 'redemptions'

const tabs: TabbedSettingsPageTab<RedeemManagementTab>[] = [
  { key: 'campaigns', label: '活动' },
  { key: 'batches', label: '批次' },
  { key: 'codes', label: '兑换码明细' },
  { key: 'redemptions', label: '兑换记录' },
]

export default function RedeemManagement() {
  const [activeTab, setActiveTab] = useState<RedeemManagementTab>('campaigns')
  const [loading, setLoading] = useState(true)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [campaigns, setCampaigns] = useState<RedeemCampaign[]>([])
  const [plans, setPlans] = useState<SubscriptionPlanResponse[]>([])
  const [batches, setBatches] = useState<RedeemBatch[]>([])
  const [codes, setCodes] = useState<RedeemCode[]>([])
  const [redemptions, setRedemptions] = useState<RedeemRedemption[]>([])
  const [campaignPage, setCampaignPage] = useState(1)
  const [campaignPageSize, setCampaignPageSize] = useState(10)
  const [batchPage, setBatchPage] = useState(1)
  const [batchPageSize, setBatchPageSize] = useState(10)
  const [codePage, setCodePage] = useState(1)
  const [codePageSize, setCodePageSize] = useState(10)
  const [redemptionPage, setRedemptionPage] = useState(1)
  const [redemptionPageSize, setRedemptionPageSize] = useState(10)

  const [campaignDialogOpen, setCampaignDialogOpen] = useState(false)
  const [editingCampaign, setEditingCampaign] = useState<RedeemCampaign | null>(null)
  const [campaignForm, setCampaignForm] = useState<CampaignFormState>(initialCampaignForm())
  const [savingCampaign, setSavingCampaign] = useState(false)

  const [batchDialogCampaign, setBatchDialogCampaign] = useState<RedeemCampaign | null>(null)
  const [batchForm, setBatchForm] = useState<RedeemBatchRequest>(initialBatchForm())
  const [savingBatch, setSavingBatch] = useState(false)
  const [latestBatchCodes, setLatestBatchCodes] = useState<string[]>([])
  const [manualCodeDialogOpen, setManualCodeDialogOpen] = useState(false)
  const [manualCodeForm, setManualCodeForm] = useState<ManualRedeemCodeRequest>(initialManualCodeForm())
  const [manualCodeBalanceUsd, setManualCodeBalanceUsd] = useState('')
  const [savingManualCode, setSavingManualCode] = useState(false)
  const [batchCampaignId, setBatchCampaignId] = useState('all')

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
    window.setTimeout(() => setMessage(null), 5000)
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
    setCodePage(1)
  }

  const refreshRedemptions = async () => {
    const redemptionList = await listRedeemRedemptions({
      campaignId: redemptionFilters.campaignId === 'all' ? '' : redemptionFilters.campaignId,
      status: redemptionFilters.status === 'all' ? '' : redemptionFilters.status as 'success' | 'rejected',
      username: redemptionFilters.username.trim(),
      limit: 200,
    })
    setRedemptions(redemptionList)
    setRedemptionPage(1)
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

  const handleOpenCreateManualCode = () => {
    setManualCodeForm(initialManualCodeForm())
    setManualCodeBalanceUsd('')
    setManualCodeDialogOpen(true)
  }

  const handleSaveManualCode = async () => {
    if (!manualCodeForm.codeValue.trim()) {
      showMessage('error', '请填写兑换码')
      return
    }
    if (!manualCodeForm.campaignId && !manualCodeForm.subscriptionPlanId && !manualCodeBalanceUsd.trim()) {
      showMessage('error', '自由码至少需要一种奖励')
      return
    }

    setSavingManualCode(true)
    try {
      await createManualRedeemCode({
        ...manualCodeForm,
        campaignId: manualCodeForm.campaignId || '',
        codeValue: manualCodeForm.codeValue.trim().toUpperCase(),
        subscriptionPlanId: manualCodeForm.campaignId ? '' : manualCodeForm.subscriptionPlanId,
        subscriptionDurationDays: manualCodeForm.campaignId ? 0 : manualCodeForm.subscriptionDurationDays,
        balanceMicros: manualCodeForm.campaignId ? 0 : usdToMicros(manualCodeBalanceUsd),
        startsAt: manualCodeForm.campaignId ? undefined : normalizeDatetimeLocal(manualCodeForm.startsAt || ''),
        endsAt: manualCodeForm.campaignId ? undefined : normalizeDatetimeLocal(manualCodeForm.endsAt || ''),
      })
      setManualCodeDialogOpen(false)
      showMessage('success', '兑换码已创建')
      await loadAll()
    } catch (error) {
      showMessage('error', error instanceof Error ? error.message : '创建失败')
    } finally {
      setSavingManualCode(false)
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

  const { currentPage: currentCampaignPage, visibleItems: visibleCampaigns } = paginateItems(campaigns, campaignPage, campaignPageSize)
  const { currentPage: currentBatchPage, visibleItems: visibleBatches } = paginateItems(batches, batchPage, batchPageSize)
  const { currentPage: currentCodePage, visibleItems: visibleCodes } = paginateItems(codes, codePage, codePageSize)
  const { currentPage: currentRedemptionPage, visibleItems: visibleRedemptions } = paginateItems(redemptions, redemptionPage, redemptionPageSize)

  return (
    <>
      <TabbedSettingsPage
        title="兑换码管理"
        description="管理活动、批次、兑换码明细与兑换记录"
        tabs={tabs}
        activeTab={activeTab}
        onTabChange={setActiveTab}
        indicatorId="redeem-management-tab-indicator"
        message={message}
        extraContent={activeTab === 'batches' && latestBatchCodes.length > 0 ? (
          <Alert>
            <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
              <span>最近一次批量生成了 {latestBatchCodes.length} 个兑换码。</span>
              <Button type="button" variant="outline" size="sm" onClick={() => navigator.clipboard.writeText(latestBatchCodes.join('\n')).catch(() => undefined)}>
                <Copy className="mr-2 h-4 w-4" />
                复制全部
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
      >
        {activeTab === 'campaigns' && (
          <Card>
            <CardHeader>
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="space-y-1">
                  <CardTitle>活动</CardTitle>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button type="button" variant="outline" size="sm" onClick={() => void loadAll()}>
                    <RefreshCw className="mr-2 h-4 w-4" />
                    刷新
                  </Button>
                  <Button type="button" onClick={handleOpenCreateCampaign}>
                    <Plus className="mr-2 h-4 w-4" />
                    新建活动
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-3 md:hidden">
                {visibleCampaigns.map((item) => (
                  <div key={item.id} className="rounded-xl border border-border/70 px-4 py-4">
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <p className="font-medium">{item.name}</p>
                        <p className="mt-1 text-xs text-muted-foreground">{item.description || '-'}</p>
                      </div>
                      <Badge variant={item.enabled ? 'default' : 'secondary'}>
                        {item.enabled ? '启用' : '停用'}
                      </Badge>
                    </div>

                    <div className="mt-3 flex flex-wrap gap-2">
                      <Badge variant="outline">{item.codeMode === 'shared' ? '共享码' : '单次码'}</Badge>
                      {item.codeMode === 'shared' && item.sharedCodeMask ? (
                        <Badge variant="secondary">{item.sharedCodeMask}</Badge>
                      ) : null}
                    </div>

                    <div className="mt-3 grid gap-2 text-sm">
                      <MobileInfoRow label="奖励" value={campaignReward(item)} />
                      <MobileInfoRow
                        label="限制"
                        value={(
                          <div className="text-right">
                            <div>总上限 {item.totalRedemptionsLimit > 0 ? item.totalRedemptionsLimit : '不限'}</div>
                            <div>单用户 {item.perUserLimit}</div>
                          </div>
                        )}
                      />
                      <MobileInfoRow label="有效期" value={formatRedeemWindow(item.startsAt, item.endsAt)} />
                      <MobileInfoRow
                        label="码量"
                        value={(
                          <div className="text-right">
                            <div>{item.redeemedCount} / {item.totalRedemptionsLimit > 0 ? item.totalRedemptionsLimit : '∞'}</div>
                            <div>{item.codeCount} 个码 / {item.batchCount} 批</div>
                          </div>
                        )}
                      />
                    </div>

                    <div className="mt-3 flex flex-wrap gap-2">
                      {item.codeMode === 'shared' && item.sharedCode ? (
                        <Button type="button" variant="outline" size="sm" onClick={() => navigator.clipboard.writeText(item.sharedCode || '').catch(() => undefined)}>
                          <Copy className="mr-2 h-4 w-4" />
                          复制码
                        </Button>
                      ) : null}
                      {item.codeMode === 'single_use' ? (
                        <Button type="button" variant="outline" size="sm" onClick={() => setBatchDialogCampaign(item)}>
                          <TicketPercent className="mr-2 h-4 w-4" />
                          生成批次
                        </Button>
                      ) : null}
                      <Button type="button" variant="outline" size="sm" onClick={() => handleOpenEditCampaign(item)}>编辑</Button>
                      <Button type="button" variant="outline" size="sm" onClick={() => handleDeleteCampaign(item)} className="text-destructive hover:text-destructive">
                        <Trash2 className="mr-2 h-4 w-4" />
                        删除
                      </Button>
                    </div>
                  </div>
                ))}
              </div>

              <div className="hidden overflow-x-auto rounded-lg border md:block">
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
                    {visibleCampaigns.map((item) => (
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
                              <Button type="button" variant="outline" size="sm" onClick={() => navigator.clipboard.writeText(item.sharedCode || '').catch(() => undefined)}>
                                <Copy className="mr-2 h-4 w-4" />
                                复制码
                              </Button>
                            ) : null}
                            {item.codeMode === 'single_use' ? (
                              <Button type="button" variant="outline" size="sm" onClick={() => setBatchDialogCampaign(item)}>
                                <TicketPercent className="mr-2 h-4 w-4" />
                                生成批次
                              </Button>
                            ) : null}
                            <Button type="button" variant="outline" size="sm" onClick={() => handleOpenEditCampaign(item)}>编辑</Button>
                            <Button type="button" variant="outline" size="sm" onClick={() => handleDeleteCampaign(item)} className="text-destructive hover:text-destructive">
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
              <TablePagination
                page={currentCampaignPage}
                pageSize={campaignPageSize}
                total={campaigns.length}
                onPageChange={setCampaignPage}
                onPageSizeChange={(nextPageSize) => {
                  setCampaignPageSize(nextPageSize)
                  setCampaignPage(1)
                }}
              />
            </CardContent>
          </Card>
        )}

        {activeTab === 'batches' && (
          <Card>
            <CardHeader>
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="space-y-1">
                  <CardTitle>批次</CardTitle>
                </div>
                <Button type="button" variant="outline" size="sm" onClick={() => void loadAll()}>
                  <RefreshCw className="mr-2 h-4 w-4" />
                  刷新
                </Button>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap">
                <Select
                  value={batchCampaignId}
                  onValueChange={async (value) => {
                    setBatchCampaignId(value)
                    setBatchPage(1)
                    setBatches(await listRedeemBatches(value === 'all' ? '' : value))
                  }}
                >
                  <SelectTrigger className="w-full sm:w-[220px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">全部活动</SelectItem>
                    {campaigns.filter((item) => item.codeMode === 'single_use').map((item) => (
                      <SelectItem key={item.id} value={item.id}>{item.name}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className="space-y-3 md:hidden">
                {visibleBatches.map((item) => (
                  <div key={item.id} className="rounded-xl border border-border/70 px-4 py-4">
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <p className="font-medium">{item.name}</p>
                        <p className="mt-1 text-xs text-muted-foreground">{item.campaignName}</p>
                      </div>
                      <Button type="button" variant="outline" size="sm" onClick={() => void handleExportBatch(item)}>
                        <Download className="mr-2 h-4 w-4" />
                        导出
                      </Button>
                    </div>

                    <div className="mt-3 grid gap-2 text-sm">
                      <MobileInfoRow label="数量" value={item.codeCount} />
                      <MobileInfoRow label="前缀" value={item.prefix || '-'} />
                      <MobileInfoRow label="创建时间" value={formatDateTime(item.createdAt)} />
                    </div>
                  </div>
                ))}
              </div>

              <div className="hidden overflow-x-auto rounded-lg border md:block">
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
                    {visibleBatches.map((item) => (
                      <TableRow key={item.id}>
                        <TableCell className="font-medium">{item.name}</TableCell>
                        <TableCell>{item.campaignName}</TableCell>
                        <TableCell>{item.codeCount}</TableCell>
                        <TableCell>{item.prefix || '-'}</TableCell>
                        <TableCell className="text-muted-foreground">{formatDateTime(item.createdAt)}</TableCell>
                        <TableCell className="text-right">
                          <Button type="button" variant="outline" size="sm" onClick={() => void handleExportBatch(item)}>
                            <Download className="mr-2 h-4 w-4" />
                            导出
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
              <TablePagination
                page={currentBatchPage}
                pageSize={batchPageSize}
                total={batches.length}
                onPageChange={setBatchPage}
                onPageSizeChange={(nextPageSize) => {
                  setBatchPageSize(nextPageSize)
                  setBatchPage(1)
                }}
              />
            </CardContent>
          </Card>
        )}

        {activeTab === 'codes' && (
          <Card>
            <CardHeader>
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="space-y-1">
                  <CardTitle>兑换码明细</CardTitle>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button type="button" size="sm" onClick={handleOpenCreateManualCode}>
                    <Plus className="mr-2 h-4 w-4" />
                    新建兑换码
                  </Button>
                  <Button type="button" variant="outline" size="sm" onClick={() => void refreshCodes()}>
                    <RefreshCw className="mr-2 h-4 w-4" />
                    刷新
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap">
                <Select
                  value={codeFilters.status}
                  onValueChange={(value) => setCodeFilters((current) => ({ ...current, status: value }))}
                >
                  <SelectTrigger className="w-full sm:w-[150px]">
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
                  className="w-full sm:w-[220px]"
                />
                <Button type="button" variant="outline" size="sm" onClick={() => void refreshCodes()}>
                  查询
                </Button>
              </div>

              <div className="space-y-3 md:hidden">
                {visibleCodes.map((item) => (
                  <div key={item.id} className="rounded-xl border border-border/70 px-4 py-4">
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <p className="font-mono text-xs">{item.codeValue}</p>
                        <p className="mt-1 text-xs text-muted-foreground">{item.campaignName || '未关联活动'}</p>
                      </div>
                      <Badge variant={item.status === 'active' ? 'default' : 'secondary'}>
                        {getCodeStatusLabel(item.status)}
                      </Badge>
                    </div>

                    <div className="mt-3 flex flex-wrap gap-2">
                      <Badge variant="outline">{getCodeSourceLabel(item.sourceType)}</Badge>
                      {item.batchName ? <Badge variant="secondary">{item.batchName}</Badge> : null}
                    </div>

                    <div className="mt-3 grid gap-2 text-sm">
                      <MobileInfoRow label="奖励" value={rewardSummary(item.subscriptionPlanName, item.subscriptionDurationDays, item.balanceMicros)} />
                      <MobileInfoRow label="次数" value={`${item.redeemedCount} / ${item.maxRedemptions > 0 ? item.maxRedemptions : '∞'}`} />
                      <MobileInfoRow label="有效期" value={formatRedeemWindow(item.startsAt, item.endsAt)} />
                    </div>

                    <div className="mt-3 flex items-center justify-between gap-3">
                      {item.status !== 'consumed' ? (
                        <>
                          <span className="text-xs text-muted-foreground">{getCodeStatusLabel(item.status)}</span>
                          <Switch
                            checked={item.status === 'active'}
                            disabled={togglingCodeId === item.id}
                            onCheckedChange={(checked) => void handleToggleCode(item, checked)}
                          />
                        </>
                      ) : (
                        <span className="text-xs text-muted-foreground">已用尽</span>
                      )}
                    </div>
                  </div>
                ))}
              </div>

              <div className="hidden overflow-x-auto rounded-lg border md:block">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>兑换码</TableHead>
                      <TableHead>类型</TableHead>
                      <TableHead>活动</TableHead>
                      <TableHead>批次</TableHead>
                      <TableHead>奖励</TableHead>
                      <TableHead>次数</TableHead>
                      <TableHead>状态</TableHead>
                      <TableHead className="text-right">操作</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {visibleCodes.map((item) => (
                      <TableRow key={item.id}>
                        <TableCell className="font-mono text-xs">{item.codeValue}</TableCell>
                        <TableCell>{getCodeSourceLabel(item.sourceType)}</TableCell>
                        <TableCell>{item.campaignName || '-'}</TableCell>
                        <TableCell>{item.batchName || '-'}</TableCell>
                        <TableCell className="text-sm text-muted-foreground">
                          {rewardSummary(item.subscriptionPlanName, item.subscriptionDurationDays, item.balanceMicros)}
                        </TableCell>
                        <TableCell>{item.redeemedCount} / {item.maxRedemptions > 0 ? item.maxRedemptions : '∞'}</TableCell>
                        <TableCell>
                          <Badge variant={item.status === 'active' ? 'default' : 'secondary'}>{getCodeStatusLabel(item.status)}</Badge>
                        </TableCell>
                        <TableCell className="text-right">
                          {item.status !== 'consumed' ? (
                            <div className="flex items-center justify-end gap-3">
                              <span className="text-xs text-muted-foreground">{getCodeStatusLabel(item.status)}</span>
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
              <TablePagination
                page={currentCodePage}
                pageSize={codePageSize}
                total={codes.length}
                onPageChange={setCodePage}
                onPageSizeChange={(nextPageSize) => {
                  setCodePageSize(nextPageSize)
                  setCodePage(1)
                }}
              />
            </CardContent>
          </Card>
        )}

        {activeTab === 'redemptions' && (
          <Card>
            <CardHeader>
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="space-y-1">
                  <CardTitle>兑换记录</CardTitle>
                </div>
                <Button type="button" variant="outline" size="sm" onClick={() => void refreshRedemptions()}>
                  <RefreshCw className="mr-2 h-4 w-4" />
                  刷新
                </Button>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap">
                <Select
                  value={redemptionFilters.campaignId}
                  onValueChange={(value) => setRedemptionFilters((current) => ({ ...current, campaignId: value }))}
                >
                  <SelectTrigger className="w-full sm:w-[220px]">
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
                  <SelectTrigger className="w-full sm:w-[150px]">
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
                  className="w-full sm:w-[180px]"
                />
                <Button type="button" variant="outline" size="sm" onClick={() => void refreshRedemptions()}>
                  查询
                </Button>
              </div>

              <div className="space-y-3 md:hidden">
                {visibleRedemptions.map((item) => (
                  <div key={item.id} className="rounded-xl border border-border/70 px-4 py-4">
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <p className="font-medium">{item.username}</p>
                        <p className="mt-1 text-xs text-muted-foreground">{item.campaignName || '未关联活动'}</p>
                      </div>
                      <Badge variant={item.status === 'success' ? 'default' : 'secondary'}>
                        {redemptionResultLabel(item.status)}
                      </Badge>
                    </div>

                    <div className="mt-3 grid gap-2 text-sm">
                      <MobileInfoRow label="时间" value={formatDateTime(item.createdAt)} />
                      <MobileInfoRow label="兑换码" value={<span className="font-mono text-xs">{item.codeMask || '-'}</span>} />
                      <MobileInfoRow label="奖励" value={rewardSummary(item.subscriptionPlanName, item.subscriptionDurationDays, item.balanceMicros)} />
                      <MobileInfoRow label="说明" value={redemptionResultDescription(item)} />
                    </div>
                  </div>
                ))}
              </div>

              <div className="hidden overflow-x-auto rounded-lg border md:block">
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
                    {visibleRedemptions.map((item) => (
                      <TableRow key={item.id}>
                        <TableCell className="text-muted-foreground">{formatDateTime(item.createdAt)}</TableCell>
                        <TableCell>{item.username}</TableCell>
                        <TableCell>{item.campaignName || '-'}</TableCell>
                        <TableCell className="font-mono text-xs">{item.codeMask || '-'}</TableCell>
                        <TableCell className="text-sm text-muted-foreground">{rewardSummary(item.subscriptionPlanName, item.subscriptionDurationDays, item.balanceMicros)}</TableCell>
                        <TableCell>
                          <Badge variant={item.status === 'success' ? 'default' : 'secondary'}>
                            {redemptionResultLabel(item.status)}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-sm text-muted-foreground">{redemptionResultDescription(item)}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
              <TablePagination
                page={currentRedemptionPage}
                pageSize={redemptionPageSize}
                total={redemptions.length}
                onPageChange={setRedemptionPage}
                onPageSizeChange={(nextPageSize) => {
                  setRedemptionPageSize(nextPageSize)
                  setRedemptionPage(1)
                }}
              />
            </CardContent>
          </Card>
        )}
      </TabbedSettingsPage>

      <Dialog open={manualCodeDialogOpen} onOpenChange={setManualCodeDialogOpen}>
        <DialogContent className="max-h-[calc(100vh-2rem)] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>新建兑换码</DialogTitle>
          </DialogHeader>
          <div className="grid gap-4 py-2 sm:grid-cols-2">
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="manualCodeValue">兑换码</Label>
              <Input
                id="manualCodeValue"
                value={manualCodeForm.codeValue}
                onChange={(event) => setManualCodeForm((current) => ({ ...current, codeValue: event.target.value.toUpperCase() }))}
                placeholder="输入自定义码值"
              />
            </div>
            <div className="space-y-2 sm:col-span-2">
              <Label>所属活动</Label>
              <Select
                value={manualCodeForm.campaignId || 'none'}
                onValueChange={(value) => setManualCodeForm((current) => ({ ...current, campaignId: value === 'none' ? '' : value }))}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">自由码</SelectItem>
                  {campaigns.map((campaign) => (
                    <SelectItem key={campaign.id} value={campaign.id}>{campaign.name}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {!manualCodeForm.campaignId ? (
              <>
                <div className="space-y-2">
                  <Label>订阅套餐</Label>
                  <Select
                    value={manualCodeForm.subscriptionPlanId || 'none'}
                    onValueChange={(value) => setManualCodeForm((current) => ({ ...current, subscriptionPlanId: value === 'none' ? '' : value }))}
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
                  <Label htmlFor="manualDurationDays">订阅时长</Label>
                  <Input
                    id="manualDurationDays"
                    type="number"
                    min="0"
                    value={manualCodeForm.subscriptionDurationDays}
                    onChange={(event) => setManualCodeForm((current) => ({ ...current, subscriptionDurationDays: Number(event.target.value || 0) }))}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="manualBalanceUsd">余额奖励 (USD)</Label>
                  <Input
                    id="manualBalanceUsd"
                    value={manualCodeBalanceUsd}
                    onChange={(event) => setManualCodeBalanceUsd(event.target.value)}
                    placeholder="例如 5"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="manualPerUserLimit">单用户次数</Label>
                  <Input
                    id="manualPerUserLimit"
                    type="number"
                    min="1"
                    value={manualCodeForm.perUserLimit}
                    onChange={(event) => setManualCodeForm((current) => ({ ...current, perUserLimit: Number(event.target.value || 1) }))}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="manualStartsAt">开始时间</Label>
                  <Input
                    id="manualStartsAt"
                    type="datetime-local"
                    value={manualCodeForm.startsAt || ''}
                    onChange={(event) => setManualCodeForm((current) => ({ ...current, startsAt: event.target.value }))}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="manualEndsAt">结束时间</Label>
                  <Input
                    id="manualEndsAt"
                    type="datetime-local"
                    value={manualCodeForm.endsAt || ''}
                    onChange={(event) => setManualCodeForm((current) => ({ ...current, endsAt: event.target.value }))}
                  />
                </div>
              </>
            ) : null}
            <div className="space-y-2">
              <Label htmlFor="manualMaxRedemptions">最大次数</Label>
              <Input
                id="manualMaxRedemptions"
                type="number"
                min="1"
                value={manualCodeForm.maxRedemptions}
                onChange={(event) => setManualCodeForm((current) => ({ ...current, maxRedemptions: Number(event.target.value || 1) }))}
              />
            </div>
            <div className="flex items-center justify-between rounded-xl border border-border/70 px-4 py-3">
              <div>
                <p className="text-sm font-medium">启用</p>
              </div>
              <Switch
                checked={manualCodeForm.enabled}
                onCheckedChange={(checked) => setManualCodeForm((current) => ({ ...current, enabled: checked }))}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setManualCodeDialogOpen(false)}>取消</Button>
            <Button onClick={() => void handleSaveManualCode()} disabled={savingManualCode}>
              {savingManualCode ? <RefreshCw className="mr-2 h-4 w-4 animate-spin" /> : null}
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={campaignDialogOpen} onOpenChange={setCampaignDialogOpen}>
          <DialogContent className="max-h-[calc(100vh-2rem)] overflow-y-auto sm:max-w-2xl">
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
          <DialogContent className="max-h-[calc(100vh-2rem)] overflow-y-auto">
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
    </>
  )
}
