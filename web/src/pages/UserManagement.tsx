import { useCallback, useEffect, useState } from 'react'
import { AnimatePresence, motion, tableRowVariants, tableStaggerContainer } from '@/lib/motion'
import { register } from '../api/auth'
import {
  applyUserBatch,
  deleteUser,
  listUsersPaged,
  previewUserBatch,
  resetUserPassword,
  setUserAdmin,
  setUserConcurrencyLimit,
  setUserGroups,
  topUpUser,
  type GroupBatchChangeMode,
  type SubscriptionBatchExpiryMode,
  type SubscriptionBatchPlanMode,
  UserInfo,
  type UserBatchFilter,
  type UserBatchPreviewResponse,
  type UserBatchTargetMode,
} from '../api/users'
import {
  deleteAdminUserAPIKey,
  getAdminUserAPIKeys,
  type APIKey,
  updateAdminUserAPIKey,
} from '../api/amp'
import { Group, listGroups } from '../api/groups'
import {
  assignSubscription,
  cancelSubscription,
  getPlans,
  getUserSubscription,
  LimitType,
  SubscriptionPlanResponse,
  SubscriptionStatus,
  updateSubscriptionExpiry,
  UserSubscriptionResponse,
} from '../api/subscription'
import { AdminPageShell, AdminSurface } from '@/components/admin/AdminPageShell'
import { Num } from '@/components/Num'
import { OverflowCopyText } from '@/components/OverflowCopyText'
import { PageSizeSlider } from '@/components/PageSizeSlider'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Table, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { DateTimePicker } from '@/components/ui/datetime-picker'
import { formatDateTime, formatGroupedNumericString } from '@/lib/formatters'
import {
  CheckCircle2,
  CreditCard,
  Key,
  KeyRound,
  Save,
  MoreHorizontal,
  Trash2,
  UserPlus,
  Wallet,
  XCircle,
} from 'lucide-react'

const LIMIT_TYPE_LABELS: Record<LimitType, string> = {
  daily: '日限制',
  weekly: '周限制',
  monthly: '月限制',
  rolling_5h: '5小时滚动',
  total: '总量限制',
}

function microsToUsd(micros: number): number {
  return micros / 1_000_000
}

function formatUsdExact(value: number, fractionDigits = 4): string {
  return `$${formatGroupedNumericString(value.toFixed(fractionDigits))}`
}

function getSubscriptionStatusLabel(status: SubscriptionStatus): string {
  switch (status) {
    case 'active':
      return '生效中'
    case 'paused':
      return '已暂停'
    case 'expired':
      return '已过期'
    case 'cancelled':
      return '已取消'
    default:
      return status
  }
}

export default function UserManagement() {
  const [users, setUsers] = useState<UserInfo[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [loading, setLoading] = useState(true)
  const [fetching, setFetching] = useState(false)
  const [hasLoadedUsers, setHasLoadedUsers] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [total, setTotal] = useState(0)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [resetPasswordModal, setResetPasswordModal] = useState<{ userId: string; username: string } | null>(null)
  const [newPassword, setNewPassword] = useState('')
  const [deleteConfirmModal, setDeleteConfirmModal] = useState<UserInfo | null>(null)
  const [topUpModal, setTopUpModal] = useState<{ userId: string; username: string } | null>(null)
  const [topUpAmount, setTopUpAmount] = useState('')
  const [plans, setPlans] = useState<SubscriptionPlanResponse[]>([])
  const [editSubscriptionModal, setEditSubscriptionModal] = useState<{ userId: string; username: string } | null>(null)
  const [editSubscriptionLoading, setEditSubscriptionLoading] = useState(false)
  const [editSubscriptionSaving, setEditSubscriptionSaving] = useState(false)
  const [currentSubscription, setCurrentSubscription] = useState<UserSubscriptionResponse | null>(null)
  const [selectedPlanId, setSelectedPlanId] = useState('')
  const [selectedExpiresAt, setSelectedExpiresAt] = useState('')
  const [cancelSubConfirm, setCancelSubConfirm] = useState<{ userId: string; username: string } | null>(null)
  const [createUserOpen, setCreateUserOpen] = useState(false)
  const [createUsername, setCreateUsername] = useState('')
  const [createPassword, setCreatePassword] = useState('')
  const [creatingUser, setCreatingUser] = useState(false)
  const [selectedUserIds, setSelectedUserIds] = useState<string[]>([])
  const [concurrencyDrafts, setConcurrencyDrafts] = useState<Record<string, string>>({})
  const [apiKeysModal, setAPIKeysModal] = useState<{ userId: string; username: string } | null>(null)
  const [userAPIKeys, setUserAPIKeys] = useState<APIKey[]>([])
  const [userAPIKeysLoading, setUserAPIKeysLoading] = useState(false)
  const [editingUserAPIKey, setEditingUserAPIKey] = useState<APIKey | null>(null)
  const [editingUserAPIKeyName, setEditingUserAPIKeyName] = useState('')
  const [editingUserAPIKeyExpiresAt, setEditingUserAPIKeyExpiresAt] = useState('')
  const [savingUserAPIKey, setSavingUserAPIKey] = useState(false)
  const [deletingUserAPIKeyId, setDeletingUserAPIKeyId] = useState<string | null>(null)
  const [batchOpen, setBatchOpen] = useState(false)
  const [batchTargetMode, setBatchTargetMode] = useState<UserBatchTargetMode>('selected')
  const [batchFilterKeyword, setBatchFilterKeyword] = useState('')
  const [batchFilterIsAdmin, setBatchFilterIsAdmin] = useState<'all' | 'true' | 'false'>('all')
  const [batchFilterGroupId, setBatchFilterGroupId] = useState<string>('all')
  const [batchFilterSubscriptionStatus, setBatchFilterSubscriptionStatus] = useState<SubscriptionStatus | 'all'>('all')
  const [batchFilterPlanId, setBatchFilterPlanId] = useState<string>('all')
  const [batchFilterBalanceMin, setBatchFilterBalanceMin] = useState('')
  const [batchFilterBalanceMax, setBatchFilterBalanceMax] = useState('')
  const [batchFilterExpiresAfter, setBatchFilterExpiresAfter] = useState('')
  const [batchFilterExpiresBefore, setBatchFilterExpiresBefore] = useState('')
  const [batchFilterLimitType, setBatchFilterLimitType] = useState<LimitType | 'all'>('all')
  const [batchFilterLimitMin, setBatchFilterLimitMin] = useState('')
  const [batchFilterLimitMax, setBatchFilterLimitMax] = useState('')
  const [batchBalanceMode, setBatchBalanceMode] = useState<'none' | 'add' | 'set' | 'subtract'>('none')
  const [batchBalanceAmount, setBatchBalanceAmount] = useState('')
  const [batchGroupMode, setBatchGroupMode] = useState<'none' | GroupBatchChangeMode>('none')
  const [batchGroupIds, setBatchGroupIds] = useState<string[]>([])
  const [batchSubscriptionPlanMode, setBatchSubscriptionPlanMode] = useState<'none' | SubscriptionBatchPlanMode>('none')
  const [batchSubscriptionPlanId, setBatchSubscriptionPlanId] = useState('')
  const [batchSubscriptionExpiryMode, setBatchSubscriptionExpiryMode] = useState<SubscriptionBatchExpiryMode>('keep')
  const [batchSubscriptionDays, setBatchSubscriptionDays] = useState('')
  const [batchSubscriptionExpiresAt, setBatchSubscriptionExpiresAt] = useState('')
  const [batchConcurrencyLimit, setBatchConcurrencyLimit] = useState('')
  const [batchPreview, setBatchPreview] = useState<UserBatchPreviewResponse | null>(null)
  const [batchPreviewing, setBatchPreviewing] = useState(false)
  const [batchApplying, setBatchApplying] = useState(false)

  const showMessage = useCallback((type: 'success' | 'error', text: string) => {
    setMessage({ type, text })
    setTimeout(() => setMessage(null), 3000)
  }, [])

  const fetchUsers = useCallback(async (targetPage = page, targetPageSize = pageSize) => {
    if (!hasLoadedUsers) {
      setLoading(true)
    } else {
      setFetching(true)
    }
    try {
      const data = await listUsersPaged(targetPage, targetPageSize)
      setUsers(data.items || [])
      setTotal(data.total || 0)
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '获取用户列表失败')
    } finally {
      setLoading(false)
      setFetching(false)
      setHasLoadedUsers(true)
    }
  }, [hasLoadedUsers, page, pageSize, showMessage])

  const fetchGroups = useCallback(async () => {
    try {
      const data = await listGroups()
      setGroups(data)
    } catch {}
  }, [])

  const fetchPlansList = useCallback(async () => {
    try {
      const data = await getPlans()
      setPlans(data.filter((plan) => plan.enabled))
    } catch {}
  }, [])

  useEffect(() => {
    fetchUsers()
  }, [fetchUsers])

  useEffect(() => {
    fetchGroups()
    fetchPlansList()
  }, [fetchGroups, fetchPlansList])

  useEffect(() => {
    setConcurrencyDrafts((current) => {
      const next = { ...current }
      for (const user of users) {
        if (!(user.id in next)) {
          next[user.id] = String(user.concurrencyLimit ?? 0)
        }
      }
      return next
    })
  }, [users])

  const buildBatchFilters = useCallback((): UserBatchFilter | undefined => {
    const filters: UserBatchFilter = {}
    if (batchFilterKeyword.trim()) {
      filters.keyword = batchFilterKeyword.trim()
    }
    if (batchFilterIsAdmin !== 'all') {
      filters.isAdmin = batchFilterIsAdmin === 'true'
    }
    if (batchFilterGroupId !== 'all') {
      filters.groupIds = [batchFilterGroupId]
    }
    if (batchFilterSubscriptionStatus !== 'all') {
      filters.subscriptionStatuses = [batchFilterSubscriptionStatus]
    }
    if (batchFilterPlanId !== 'all') {
      filters.planIds = [batchFilterPlanId]
    }
    if (batchFilterBalanceMin.trim()) {
      filters.balanceMinMicros = Math.round(Number.parseFloat(batchFilterBalanceMin || '0') * 1_000_000)
    }
    if (batchFilterBalanceMax.trim()) {
      filters.balanceMaxMicros = Math.round(Number.parseFloat(batchFilterBalanceMax || '0') * 1_000_000)
    }
    if (batchFilterExpiresAfter.trim()) {
      filters.subscriptionExpiresAfter = batchFilterExpiresAfter
    }
    if (batchFilterExpiresBefore.trim()) {
      filters.subscriptionExpiresBefore = batchFilterExpiresBefore
    }
    if (batchFilterLimitType !== 'all') {
      filters.limitType = batchFilterLimitType
    }
    if (batchFilterLimitMin.trim()) {
      filters.limitMinMicros = Math.round(Number.parseFloat(batchFilterLimitMin || '0') * 1_000_000)
    }
    if (batchFilterLimitMax.trim()) {
      filters.limitMaxMicros = Math.round(Number.parseFloat(batchFilterLimitMax || '0') * 1_000_000)
    }
    return Object.keys(filters).length > 0 ? filters : undefined
  }, [
    batchFilterBalanceMax,
    batchFilterBalanceMin,
    batchFilterExpiresAfter,
    batchFilterExpiresBefore,
    batchFilterGroupId,
    batchFilterIsAdmin,
    batchFilterKeyword,
    batchFilterLimitMax,
    batchFilterLimitMin,
    batchFilterLimitType,
    batchFilterPlanId,
    batchFilterSubscriptionStatus,
  ])

  const loadUserAPIKeys = useCallback(async (userId: string) => {
    setUserAPIKeysLoading(true)
    try {
      const items = await getAdminUserAPIKeys(userId)
      setUserAPIKeys(items)
    } finally {
      setUserAPIKeysLoading(false)
    }
  }, [])

  const closeEditSubscriptionModal = () => {
    setEditSubscriptionModal(null)
    setEditSubscriptionLoading(false)
    setEditSubscriptionSaving(false)
    setCurrentSubscription(null)
    setSelectedPlanId('')
    setSelectedExpiresAt('')
  }

  const handleOpenEditSubscription = async (userId: string, username: string) => {
    setEditSubscriptionModal({ userId, username })
    setEditSubscriptionLoading(true)
    setCurrentSubscription(null)
    setSelectedPlanId('')
    setSelectedExpiresAt('')

    try {
      const subscription = await getUserSubscription(userId)
      setCurrentSubscription(subscription)
      setSelectedPlanId(subscription?.planId || '')
      setSelectedExpiresAt(subscription?.expiresAt || '')
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '获取订阅失败')
      closeEditSubscriptionModal()
    } finally {
      setEditSubscriptionLoading(false)
    }
  }

  const handleSaveSubscription = async () => {
    if (!editSubscriptionModal || !selectedPlanId) return

    const hasCurrentSubscription = !!currentSubscription
    const currentExpiresAt = currentSubscription?.expiresAt || ''
    const currentPlanId = currentSubscription?.planId || ''
    const shouldReassign =
      !hasCurrentSubscription ||
      currentPlanId !== selectedPlanId ||
      (!selectedExpiresAt && !!currentExpiresAt)

    try {
      setEditSubscriptionSaving(true)

      if (!shouldReassign) {
        if (currentExpiresAt === selectedExpiresAt) {
          showMessage('success', '订阅信息未变更')
          closeEditSubscriptionModal()
          return
        }

        await updateSubscriptionExpiry(editSubscriptionModal.userId, selectedExpiresAt)
        showMessage('success', '订阅已更新')
      } else {
        await assignSubscription(editSubscriptionModal.userId, {
          planId: selectedPlanId,
          expiresAt: selectedExpiresAt || undefined,
        })
        showMessage('success', hasCurrentSubscription ? '订阅套餐已更新' : '订阅已分配')
      }

      closeEditSubscriptionModal()
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '保存订阅失败')
    } finally {
      setEditSubscriptionSaving(false)
    }
  }

  const handleCancelSub = async () => {
    if (!cancelSubConfirm) return

    try {
      await cancelSubscription(cancelSubConfirm.userId)
      showMessage('success', '订阅已取消')
      setCancelSubConfirm(null)
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '取消订阅失败')
    }
  }

  const handleToggleAdmin = async (user: UserInfo) => {
    try {
      await setUserAdmin(user.id, !user.isAdmin)
      showMessage('success', `已${user.isAdmin ? '取消' : '设置'}管理员权限`)
      fetchUsers()
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '操作失败')
    }
  }

  const handleDelete = async () => {
    if (!deleteConfirmModal) return

    try {
      await deleteUser(deleteConfirmModal.id)
      showMessage('success', '用户已删除')
      setDeleteConfirmModal(null)

      if (users.length === 1 && page > 1) {
        setPage(page - 1)
      } else {
        fetchUsers()
      }
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '删除失败')
    }
  }

  const handleResetPassword = async () => {
    if (!resetPasswordModal || !newPassword) return

    try {
      await resetUserPassword(resetPasswordModal.userId, newPassword)
      showMessage('success', '密码已重置')
      setResetPasswordModal(null)
      setNewPassword('')
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '重置密码失败')
    }
  }

  const handleTopUp = async () => {
    if (!topUpModal || !topUpAmount) return

    const amount = Number.parseFloat(topUpAmount)
    if (Number.isNaN(amount) || amount <= 0) {
      showMessage('error', '请输入有效金额')
      return
    }

    try {
      await topUpUser(topUpModal.userId, amount)
      showMessage('success', '充值成功')
      setTopUpModal(null)
      setTopUpAmount('')
      fetchUsers()
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '充值失败')
    }
  }

  const handleCreateUser = async () => {
    if (!createUsername.trim() || createPassword.length < 6) return

    try {
      setCreatingUser(true)
      await register({
        username: createUsername.trim(),
        password: createPassword,
      })

      showMessage('success', '用户已创建')
      setCreateUserOpen(false)
      setCreateUsername('')
      setCreatePassword('')

      if (page !== 1) {
        setPage(1)
      }
      await fetchUsers(1, pageSize)
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '创建用户失败')
    } finally {
      setCreatingUser(false)
    }
  }

  const handlePageSizeChange = (value: number) => {
    setPageSize(value)
    setPage(1)
  }

  const toggleSelectedUser = (userId: string, checked: boolean) => {
    setSelectedUserIds((current) => (
      checked ? Array.from(new Set([...current, userId])) : current.filter((id) => id !== userId)
    ))
  }

  const toggleSelectCurrentPage = (checked: boolean) => {
    const pageUserIds = users.map((user) => user.id)
    setSelectedUserIds((current) => {
      if (checked) {
        return Array.from(new Set([...current, ...pageUserIds]))
      }
      return current.filter((id) => !pageUserIds.includes(id))
    })
  }

  const handleSaveConcurrency = async (user: UserInfo) => {
    const draftValue = Number.parseInt(concurrencyDrafts[user.id] ?? String(user.concurrencyLimit ?? 0), 10)
    const nextLimit = Number.isNaN(draftValue) ? 0 : draftValue

    try {
      await setUserConcurrencyLimit(user.id, nextLimit)
      showMessage('success', '并发限制已更新')
      await fetchUsers()
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '设置并发限制失败')
    }
  }

  const handleOpenUserAPIKeys = async (user: UserInfo) => {
    setAPIKeysModal({ userId: user.id, username: user.username })
    try {
      await loadUserAPIKeys(user.id)
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '获取 API Key 失败')
      setAPIKeysModal(null)
    }
  }

  const handleOpenEditUserAPIKey = (key: APIKey) => {
    setEditingUserAPIKey(key)
    setEditingUserAPIKeyName(key.name)
    setEditingUserAPIKeyExpiresAt(key.expiresAt || '')
  }

  const handleSaveUserAPIKey = async () => {
    if (!apiKeysModal || !editingUserAPIKey || !editingUserAPIKeyName.trim()) return

    try {
      setSavingUserAPIKey(true)
      await updateAdminUserAPIKey(apiKeysModal.userId, editingUserAPIKey.id, {
        name: editingUserAPIKeyName.trim(),
        ...(editingUserAPIKeyExpiresAt ? { expiresAt: editingUserAPIKeyExpiresAt } : { clearExpiry: true }),
      })
      await loadUserAPIKeys(apiKeysModal.userId)
      setEditingUserAPIKey(null)
      showMessage('success', 'API Key 已更新')
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '更新 API Key 失败')
    } finally {
      setSavingUserAPIKey(false)
    }
  }

  const handleDeleteUserAPIKey = async (key: APIKey) => {
    if (!apiKeysModal) return
    if (!confirm(`确定删除 API Key "${key.name}" 吗？`)) return

    try {
      setDeletingUserAPIKeyId(key.id)
      await deleteAdminUserAPIKey(apiKeysModal.userId, key.id)
      await loadUserAPIKeys(apiKeysModal.userId)
      showMessage('success', 'API Key 已删除')
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '删除 API Key 失败')
    } finally {
      setDeletingUserAPIKeyId(null)
    }
  }

  const handlePreviewBatch = async () => {
    try {
      setBatchPreviewing(true)
      const result = await previewUserBatch({
        targetMode: batchTargetMode,
        ...(batchTargetMode === 'selected' ? { selectedUserIds } : { filters: buildBatchFilters() }),
      })
      setBatchPreview(result)
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '批量预览失败')
    } finally {
      setBatchPreviewing(false)
    }
  }

  const handleApplyBatch = async () => {
    const changes: Parameters<typeof applyUserBatch>[0]['changes'] = {}
    if (batchBalanceMode !== 'none') {
      const amountUsd = Number.parseFloat(batchBalanceAmount)
      if (Number.isNaN(amountUsd) || amountUsd < 0) {
        showMessage('error', '请输入有效的批量余额金额')
        return
      }
      changes.balance = {
        mode: batchBalanceMode,
        amountMicros: Math.round(amountUsd * 1_000_000),
      }
    }
    if (batchGroupMode !== 'none') {
      changes.groups = {
        mode: batchGroupMode,
        groupIds: batchGroupIds,
      }
    }
    if (batchSubscriptionPlanMode !== 'none' || batchSubscriptionExpiryMode !== 'keep') {
      if (batchSubscriptionPlanMode === 'assign' && !batchSubscriptionPlanId) {
        showMessage('error', '请选择要应用的订阅套餐')
        return
      }
      if (batchSubscriptionExpiryMode === 'set' && !batchSubscriptionExpiresAt) {
        showMessage('error', '请设置目标到期时间')
        return
      }
      if ((batchSubscriptionExpiryMode === 'extend_days' || batchSubscriptionExpiryMode === 'shorten_days')) {
        const days = Number.parseInt(batchSubscriptionDays, 10)
        if (Number.isNaN(days) || days <= 0) {
          showMessage('error', '请输入有效的订阅天数')
          return
        }
      }
      changes.subscription = {
        planMode: batchSubscriptionPlanMode === 'none' ? 'keep' : batchSubscriptionPlanMode,
        planId: batchSubscriptionPlanId || undefined,
        expiryMode: batchSubscriptionExpiryMode,
        expiresAt: batchSubscriptionExpiresAt || undefined,
        days: batchSubscriptionDays.trim() ? Number.parseInt(batchSubscriptionDays, 10) : undefined,
      }
    }
    if (batchConcurrencyLimit.trim() !== '') {
      const limit = Number.parseInt(batchConcurrencyLimit, 10)
      if (Number.isNaN(limit) || limit < 0) {
        showMessage('error', '请输入有效的并发限制')
        return
      }
      changes.concurrency = {
        concurrencyLimit: limit,
      }
    }
    if (Object.keys(changes).length === 0) {
      showMessage('error', '请至少选择一个批量修改项')
      return
    }

    try {
      setBatchApplying(true)
      const result = await applyUserBatch({
        targetMode: batchTargetMode,
        ...(batchTargetMode === 'selected' ? { selectedUserIds } : { filters: buildBatchFilters() }),
        changes,
      })
      showMessage('success', `已完成批量修改：${result.appliedCount}/${result.matchedCount}`)
      setBatchOpen(false)
      setBatchPreview(null)
      await fetchUsers()
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '批量修改失败')
    } finally {
      setBatchApplying(false)
    }
  }

  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  const currentPageSelectedCount = users.filter((user) => selectedUserIds.includes(user.id)).length

  if (loading && !hasLoadedUsers) {
    return <div className="text-center text-muted-foreground">加载中...</div>
  }

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
      <AdminPageShell
        title="用户列表"
        description="管理用户权限、余额、分组、并发限制与订阅。"
        width="7xl"
        actions={(
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="outline" size="sm" onClick={() => setBatchOpen(true)}>
              批量修改
              {selectedUserIds.length > 0 ? ` (${selectedUserIds.length})` : ''}
            </Button>
            <Button size="sm" onClick={() => setCreateUserOpen(true)}>
              <UserPlus className="mr-1.5 h-4 w-4" />
              新建用户
            </Button>
          </div>
        )}
      >
        <AnimatePresence>
          {message ? (
            <motion.div
              initial={{ opacity: 0, y: -20, scale: 0.95 }}
              animate={{ opacity: 1, y: 0, scale: 1 }}
              exit={{ opacity: 0, y: -20, scale: 0.95 }}
              transition={{ type: 'spring', bounce: 0.3, duration: 0.5 }}
            >
              <Alert variant={message.type === 'success' ? 'default' : 'destructive'}>
                {message.type === 'success' ? (
                  <CheckCircle2 className="h-4 w-4" />
                ) : (
                  <XCircle className="h-4 w-4" />
                )}
                <AlertDescription>{message.text}</AlertDescription>
              </Alert>
            </motion.div>
          ) : null}
        </AnimatePresence>

        <motion.div
          initial={{ opacity: 0, y: 30 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ type: 'spring', bounce: 0.2, duration: 0.6, delay: 0.1 }}
        >
          <AdminSurface>
            <div className={fetching ? 'admin-surface-body opacity-60 transition-opacity' : 'admin-surface-body transition-opacity'}>
              <div className="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-dashed px-4 py-3 text-sm">
                <div className="text-muted-foreground">
                  已选中 <span className="font-medium text-foreground">{selectedUserIds.length}</span> 个用户，
                  当前页选中 <span className="font-medium text-foreground">{currentPageSelectedCount}</span> 个。
                </div>
                <Button variant="outline" size="sm" onClick={() => setSelectedUserIds([])} disabled={selectedUserIds.length === 0}>
                  清空选择
                </Button>
              </div>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-12">
                      <Checkbox
                        checked={users.length > 0 && currentPageSelectedCount === users.length}
                        onCheckedChange={(checked) => toggleSelectCurrentPage(checked === true)}
                        aria-label="选择当前页全部用户"
                      />
                    </TableHead>
                    <TableHead>用户名</TableHead>
                    <TableHead>角色</TableHead>
                    <TableHead>分组</TableHead>
                    <TableHead>余额 (USD)</TableHead>
                    <TableHead>并发限制</TableHead>
                    <TableHead>管理员权限</TableHead>
                    <TableHead>创建时间</TableHead>
                    <TableHead className="w-[280px]">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <motion.tbody key="user-table-body" variants={tableStaggerContainer} initial="hidden" animate="visible">
                  {users.map((user) => {
                    const balanceValue = Number.parseFloat(user.balanceUsd || '0') || 0

                    return (
                      <motion.tr
                        key={user.id}
                        variants={tableRowVariants}
                        className="border-b transition-colors hover:bg-muted/50 data-[state=selected]:bg-muted"
                      >
                        <TableCell>
                          <Checkbox
                            checked={selectedUserIds.includes(user.id)}
                            onCheckedChange={(checked) => toggleSelectedUser(user.id, checked === true)}
                            aria-label={`选择用户 ${user.username}`}
                          />
                        </TableCell>
                        <TableCell className="font-medium">
                          <div className="max-w-[220px]">
                            <OverflowCopyText text={user.username} className="font-medium" />
                          </div>
                        </TableCell>
                        <TableCell>
                          <Badge variant={user.isAdmin ? 'default' : 'secondary'}>
                            {user.isAdmin ? '管理员' : '普通用户'}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          <Popover>
                            <PopoverTrigger asChild>
                              <Button
                                variant="outline"
                                size="sm"
                                className="h-8 max-w-[180px] justify-start rounded-md px-2.5 text-left"
                              >
                                <span className="truncate text-xs">
                                  {user.groupNames && user.groupNames.length > 0 ? user.groupNames.join(' / ') : '未分组'}
                                </span>
                              </Button>
                            </PopoverTrigger>
                            <PopoverContent className="w-[200px] p-2" align="start">
                              <div className="space-y-2">
                                <p className="px-1 text-sm font-medium">选择分组</p>
                                {groups.map((group) => (
                                  <label
                                    key={group.id}
                                    className="flex cursor-pointer items-center gap-2 rounded px-1 py-1 hover:bg-muted"
                                  >
                                    <Checkbox
                                      checked={(user.groupIds || []).includes(group.id)}
                                      onCheckedChange={async (checked) => {
                                        const currentIds = user.groupIds || []
                                        const nextIds = checked
                                          ? [...currentIds, group.id]
                                          : currentIds.filter((id) => id !== group.id)

                                        try {
                                          await setUserGroups(user.id, nextIds)
                                          showMessage('success', '分组已更新')
                                          fetchUsers()
                                        } catch (err) {
                                          showMessage('error', err instanceof Error ? err.message : '设置分组失败')
                                        }
                                      }}
                                    />
                                    <span className="text-sm">{group.name}</span>
                                  </label>
                                ))}
                                {groups.length === 0 ? (
                                  <p className="px-1 text-xs text-muted-foreground">暂无分组</p>
                                ) : null}
                              </div>
                            </PopoverContent>
                          </Popover>
                        </TableCell>
                        <TableCell className="font-mono text-sm">
                          <Num
                            value={balanceValue}
                            interactive
                            copyable
                            className="font-mono text-sm"
                            fullTextOverride={formatUsdExact(balanceValue)}
                          />
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            <Input
                              value={concurrencyDrafts[user.id] ?? String(user.concurrencyLimit ?? 0)}
                              onChange={(event) => setConcurrencyDrafts((current) => ({ ...current, [user.id]: event.target.value }))}
                              className="h-8 w-20"
                              inputMode="numeric"
                            />
                            <Button variant="outline" size="sm" onClick={() => void handleSaveConcurrency(user)}>
                              <Save className="h-3.5 w-3.5" />
                            </Button>
                          </div>
                        </TableCell>
                        <TableCell>
                          <Switch
                            checked={user.isAdmin}
                            onCheckedChange={() => handleToggleAdmin(user)}
                            aria-label={user.isAdmin ? `取消 ${user.username} 的管理员权限` : `授予 ${user.username} 管理员权限`}
                          />
                        </TableCell>
                        <TableCell className="text-muted-foreground">
                          {formatDateTime(user.createdAt)}
                        </TableCell>
                        <TableCell>
                          <div className="flex flex-wrap items-center gap-2">
                            <Button variant="outline" size="sm" onClick={() => handleOpenEditSubscription(user.id, user.username)}>
                              <CreditCard className="mr-1.5 h-4 w-4" />
                              编辑订阅
                            </Button>
                            <Button variant="outline" size="sm" onClick={() => void handleOpenUserAPIKeys(user)}>
                              <Key className="mr-1.5 h-4 w-4" />
                              API Keys
                            </Button>
                            <DropdownMenu>
                              <DropdownMenuTrigger asChild>
                                <Button variant="outline" size="sm">
                                  <MoreHorizontal className="mr-1.5 h-4 w-4" />
                                  更多操作
                                </Button>
                              </DropdownMenuTrigger>
                              <DropdownMenuContent align="end" className="w-40">
                                <DropdownMenuItem onClick={() => setTopUpModal({ userId: user.id, username: user.username })}>
                                  <Wallet className="mr-2 h-4 w-4" />
                                  充值
                                </DropdownMenuItem>
                                <DropdownMenuItem onClick={() => setResetPasswordModal({ userId: user.id, username: user.username })}>
                                  <KeyRound className="mr-2 h-4 w-4" />
                                  重置密码
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                  onClick={() => setCancelSubConfirm({ userId: user.id, username: user.username })}
                                  className="text-destructive focus:text-destructive"
                                >
                                  <XCircle className="mr-2 h-4 w-4" />
                                  取消订阅
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                  onClick={() => setDeleteConfirmModal(user)}
                                  className="text-destructive focus:text-destructive"
                                >
                                  <Trash2 className="mr-2 h-4 w-4" />
                                  删除
                                </DropdownMenuItem>
                              </DropdownMenuContent>
                            </DropdownMenu>
                          </div>
                        </TableCell>
                      </motion.tr>
                    )
                  })}
                </motion.tbody>
              </Table>

              <div className="mt-4 flex items-center justify-between">
                <div className="flex items-center gap-4">
                  <p className="text-sm text-muted-foreground">
                    第 {page} 页，共 {totalPages} 页（{total} 条）
                  </p>
                  <PageSizeSlider value={pageSize} onChange={handlePageSizeChange} />
                </div>
                <div className="flex gap-2">
                  <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((current) => current - 1)}>
                    上一页
                  </Button>
                  <Button variant="outline" size="sm" disabled={page >= totalPages} onClick={() => setPage((current) => current + 1)}>
                    下一页
                  </Button>
                </div>
              </div>
            </div>
          </AdminSurface>
        </motion.div>

        <Dialog
          open={createUserOpen}
          onOpenChange={(open) => {
            setCreateUserOpen(open)
            if (!open) {
              setCreateUsername('')
              setCreatePassword('')
            }
          }}
        >
          <DialogContent>
            <DialogHeader>
              <DialogTitle>新建用户</DialogTitle>
              <DialogDescription>使用用户名和密码直接创建一个新用户。</DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="createUsername">用户名</Label>
                <Input
                  id="createUsername"
                  placeholder="输入用户名"
                  value={createUsername}
                  onChange={(event) => setCreateUsername(event.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="createPassword">密码</Label>
                <Input
                  id="createPassword"
                  type="password"
                  placeholder="至少6位字符"
                  value={createPassword}
                  onChange={(event) => setCreatePassword(event.target.value)}
                />
              </div>
            </div>
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => {
                  setCreateUserOpen(false)
                  setCreateUsername('')
                  setCreatePassword('')
                }}
              >
                取消
              </Button>
              <Button onClick={handleCreateUser} disabled={creatingUser || !createUsername.trim() || createPassword.length < 6}>
                创建用户
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={!!resetPasswordModal} onOpenChange={(open) => !open && setResetPasswordModal(null)}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>重置密码</DialogTitle>
              <DialogDescription>
                为用户 <span className="font-medium">{resetPasswordModal?.username}</span> 设置新密码
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="newPassword">新密码</Label>
                <Input
                  id="newPassword"
                  type="password"
                  placeholder="至少6位字符"
                  value={newPassword}
                  onChange={(event) => setNewPassword(event.target.value)}
                />
              </div>
            </div>
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => {
                  setResetPasswordModal(null)
                  setNewPassword('')
                }}
              >
                取消
              </Button>
              <Button onClick={handleResetPassword} disabled={newPassword.length < 6}>
                确认重置
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={!!deleteConfirmModal} onOpenChange={(open) => !open && setDeleteConfirmModal(null)}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>确认删除</DialogTitle>
              <DialogDescription>
                确定要删除用户 <span className="font-medium">{deleteConfirmModal?.username}</span> 吗？此操作不可撤销。
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

        <Dialog open={!!topUpModal} onOpenChange={(open) => !open && setTopUpModal(null)}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>余额充值</DialogTitle>
              <DialogDescription>
                为用户 <span className="font-medium">{topUpModal?.username}</span> 充值
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="topUpAmount">充值金额 (USD)</Label>
                <Input
                  id="topUpAmount"
                  type="number"
                  step="0.01"
                  min="0.01"
                  placeholder="例如: 10.00"
                  value={topUpAmount}
                  onChange={(event) => setTopUpAmount(event.target.value)}
                />
              </div>
            </div>
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => {
                  setTopUpModal(null)
                  setTopUpAmount('')
                }}
              >
                取消
              </Button>
              <Button onClick={handleTopUp} disabled={!topUpAmount || Number.parseFloat(topUpAmount) <= 0}>
                确认充值
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={!!editSubscriptionModal} onOpenChange={(open) => !open && closeEditSubscriptionModal()}>
          <DialogContent className="sm:max-w-2xl">
            <DialogHeader>
              <DialogTitle>编辑订阅</DialogTitle>
              <DialogDescription>
                调整用户 <span className="font-medium">{editSubscriptionModal?.username}</span> 的订阅信息。
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              {editSubscriptionLoading ? (
                <p className="text-center text-muted-foreground">加载中...</p>
              ) : (
                <>
                  <div className="space-y-3 rounded-xl border border-border/60 px-4 py-3">
                    <div className="flex items-center justify-between gap-4">
                      <span className="text-sm text-muted-foreground">当前订阅</span>
                      <span className="text-sm font-medium">
                        {currentSubscription ? getSubscriptionStatusLabel(currentSubscription.status) : '未分配'}
                      </span>
                    </div>
                    {currentSubscription ? (
                      <>
                        <div className="grid gap-2 text-sm">
                          <div className="flex items-center justify-between gap-4">
                            <span className="text-muted-foreground">套餐</span>
                            <div className="max-w-[260px]">
                              <OverflowCopyText text={currentSubscription.planName} className="font-medium text-right" />
                            </div>
                          </div>
                          <div className="flex items-center justify-between gap-4">
                            <span className="text-muted-foreground">开始时间</span>
                            <span>{formatDateTime(currentSubscription.startsAt)}</span>
                          </div>
                          <div className="flex items-center justify-between gap-4">
                            <span className="text-muted-foreground">到期时间</span>
                            <span>{currentSubscription.expiresAt ? formatDateTime(currentSubscription.expiresAt) : '永不过期'}</span>
                          </div>
                        </div>

                        {currentSubscription.limits.length > 0 ? (
                          <div className="space-y-2 border-t border-border/60 pt-3">
                            <p className="text-sm font-medium">额度详情</p>
                            <div className="space-y-2">
                              {currentSubscription.limits.map((limit) => {
                                const amountUsd = microsToUsd(limit.limitMicros)
                                const label = `${LIMIT_TYPE_LABELS[limit.limitType] || limit.limitType}${limit.fixedResetTime ? ` @ ${limit.fixedResetTime}` : ''}`

                                return (
                                  <div key={limit.id} className="flex items-center justify-between gap-4 text-sm">
                                    <span className="text-muted-foreground">{label}</span>
                                    <Num
                                      value={amountUsd}
                                      interactive
                                      copyable
                                      className="font-mono text-sm"
                                      fullTextOverride={formatUsdExact(amountUsd, 2)}
                                    />
                                  </div>
                                )
                              })}
                            </div>
                          </div>
                        ) : null}
                      </>
                    ) : (
                      <p className="text-sm text-muted-foreground">该用户当前没有订阅，可直接分配套餐。</p>
                    )}
                  </div>

                  <div className="grid gap-4 md:grid-cols-2">
                    <div className="space-y-2">
                      <Label>选择套餐</Label>
                      <Select value={selectedPlanId} onValueChange={setSelectedPlanId}>
                        <SelectTrigger>
                          <SelectValue placeholder="选择套餐..." />
                        </SelectTrigger>
                        <SelectContent>
                          {plans.map((plan) => (
                            <SelectItem key={plan.id} value={plan.id}>
                              {plan.name}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      {plans.length === 0 ? (
                        <p className="text-xs text-muted-foreground">暂无可用套餐，请先创建</p>
                      ) : null}
                    </div>
                    <div className="space-y-2">
                      <Label>到期时间</Label>
                      <DateTimePicker
                        value={selectedExpiresAt}
                        onChange={setSelectedExpiresAt}
                        placeholder="选择到期时间"
                      />
                      <p className="text-xs text-muted-foreground">留空表示永不过期</p>
                    </div>
                  </div>
                </>
              )}
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={closeEditSubscriptionModal}>
                取消
              </Button>
              <Button onClick={handleSaveSubscription} disabled={editSubscriptionLoading || editSubscriptionSaving || !selectedPlanId}>
                保存订阅
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog
          open={batchOpen}
          onOpenChange={(open) => {
            setBatchOpen(open)
            if (!open) {
              setBatchPreview(null)
            }
          }}
        >
          <DialogContent className="sm:max-w-3xl">
            <DialogHeader>
              <DialogTitle>批量修改用户</DialogTitle>
              <DialogDescription>可针对已选用户，或按筛选条件匹配到的用户批量修改余额、分组和并发限制。</DialogDescription>
            </DialogHeader>
            <div className="space-y-5 py-4">
              <div className="grid gap-4 md:grid-cols-2">
                <div className="space-y-2">
                  <Label>目标范围</Label>
                  <Select value={batchTargetMode} onValueChange={(value) => setBatchTargetMode(value as UserBatchTargetMode)}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="selected">已选用户 ({selectedUserIds.length})</SelectItem>
                      <SelectItem value="filtered">筛选结果</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                {batchTargetMode === 'filtered' ? (
                  <div className="space-y-2">
                    <Label>用户名关键字</Label>
                    <Input value={batchFilterKeyword} onChange={(event) => setBatchFilterKeyword(event.target.value)} placeholder="留空表示不过滤" />
                  </div>
                ) : null}
              </div>

              {batchTargetMode === 'filtered' ? (
                <div className="grid gap-4 md:grid-cols-2">
                  <div className="space-y-2">
                    <Label>管理员状态</Label>
                    <Select value={batchFilterIsAdmin} onValueChange={(value) => setBatchFilterIsAdmin(value as 'all' | 'true' | 'false')}>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="all">全部</SelectItem>
                        <SelectItem value="true">仅管理员</SelectItem>
                        <SelectItem value="false">仅普通用户</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-2">
                    <Label>分组</Label>
                    <Select value={batchFilterGroupId} onValueChange={setBatchFilterGroupId}>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="all">全部分组</SelectItem>
                        {groups.map((group) => (
                          <SelectItem key={group.id} value={group.id}>
                            {group.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-2">
                    <Label>订阅状态</Label>
                    <Select value={batchFilterSubscriptionStatus} onValueChange={(value) => setBatchFilterSubscriptionStatus(value as SubscriptionStatus | 'all')}>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="all">全部状态</SelectItem>
                        <SelectItem value="active">生效中</SelectItem>
                        <SelectItem value="paused">已暂停</SelectItem>
                        <SelectItem value="expired">已过期</SelectItem>
                        <SelectItem value="cancelled">已取消</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-2">
                    <Label>订阅套餐</Label>
                    <Select value={batchFilterPlanId} onValueChange={setBatchFilterPlanId}>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="all">全部套餐</SelectItem>
                        {plans.map((plan) => (
                          <SelectItem key={plan.id} value={plan.id}>
                            {plan.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-2">
                    <Label>余额下限 (USD)</Label>
                    <Input value={batchFilterBalanceMin} onChange={(event) => setBatchFilterBalanceMin(event.target.value)} type="number" min="0" step="0.01" placeholder="可空" />
                  </div>
                  <div className="space-y-2">
                    <Label>余额上限 (USD)</Label>
                    <Input value={batchFilterBalanceMax} onChange={(event) => setBatchFilterBalanceMax(event.target.value)} type="number" min="0" step="0.01" placeholder="可空" />
                  </div>
                  <div className="space-y-2">
                    <Label>订阅到期晚于</Label>
                    <DateTimePicker value={batchFilterExpiresAfter} onChange={setBatchFilterExpiresAfter} />
                  </div>
                  <div className="space-y-2">
                    <Label>订阅到期早于</Label>
                    <DateTimePicker value={batchFilterExpiresBefore} onChange={setBatchFilterExpiresBefore} />
                  </div>
                  <div className="space-y-2">
                    <Label>额度类型</Label>
                    <Select value={batchFilterLimitType} onValueChange={(value) => setBatchFilterLimitType(value as LimitType | 'all')}>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="all">全部</SelectItem>
                        {(Object.keys(LIMIT_TYPE_LABELS) as LimitType[]).map((limitType) => (
                          <SelectItem key={limitType} value={limitType}>
                            {LIMIT_TYPE_LABELS[limitType]}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-2">
                    <Label>额度下限 (USD)</Label>
                    <Input value={batchFilterLimitMin} onChange={(event) => setBatchFilterLimitMin(event.target.value)} type="number" min="0" step="0.01" placeholder="例如日额大于 20" />
                  </div>
                  <div className="space-y-2">
                    <Label>额度上限 (USD)</Label>
                    <Input value={batchFilterLimitMax} onChange={(event) => setBatchFilterLimitMax(event.target.value)} type="number" min="0" step="0.01" placeholder="可空" />
                  </div>
                </div>
              ) : null}

              <div className="grid gap-4 md:grid-cols-4">
                <div className="space-y-2">
                  <Label>余额变更</Label>
                  <Select value={batchBalanceMode} onValueChange={(value) => setBatchBalanceMode(value as typeof batchBalanceMode)}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="none">不修改</SelectItem>
                      <SelectItem value="add">增加</SelectItem>
                      <SelectItem value="set">设为</SelectItem>
                      <SelectItem value="subtract">扣减</SelectItem>
                    </SelectContent>
                  </Select>
                  <Input
                    value={batchBalanceAmount}
                    onChange={(event) => setBatchBalanceAmount(event.target.value)}
                    placeholder="USD 金额"
                    type="number"
                    min="0"
                    step="0.01"
                    disabled={batchBalanceMode === 'none'}
                  />
                </div>
                <div className="space-y-2">
                  <Label>分组变更</Label>
                  <Select value={batchGroupMode} onValueChange={(value) => setBatchGroupMode(value as typeof batchGroupMode)}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="none">不修改</SelectItem>
                      <SelectItem value="add">追加分组</SelectItem>
                      <SelectItem value="set">覆盖分组</SelectItem>
                      <SelectItem value="remove">移除分组</SelectItem>
                    </SelectContent>
                  </Select>
                  <div className="space-y-2 rounded-lg border px-3 py-3">
                    {groups.map((group) => (
                      <label key={group.id} className="flex items-center gap-2 text-sm">
                        <Checkbox
                          checked={batchGroupIds.includes(group.id)}
                          onCheckedChange={(checked) => {
                            setBatchGroupIds((current) => (
                              checked ? Array.from(new Set([...current, group.id])) : current.filter((id) => id !== group.id)
                            ))
                          }}
                          disabled={batchGroupMode === 'none'}
                        />
                        <span>{group.name}</span>
                      </label>
                    ))}
                    {groups.length === 0 ? <p className="text-xs text-muted-foreground">暂无分组</p> : null}
                  </div>
                </div>
                <div className="space-y-2">
                  <Label>并发限制</Label>
                  <Input
                    value={batchConcurrencyLimit}
                    onChange={(event) => setBatchConcurrencyLimit(event.target.value)}
                    inputMode="numeric"
                    placeholder="留空表示不修改"
                  />
                  <p className="text-xs text-muted-foreground">填 0 表示不限制。</p>
                </div>
                <div className="space-y-2 md:col-span-2">
                  <Label>订阅变更</Label>
                  <div className="space-y-3 rounded-lg border px-3 py-3">
                    <Select value={batchSubscriptionPlanMode} onValueChange={(value) => setBatchSubscriptionPlanMode(value as typeof batchSubscriptionPlanMode)}>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="none">不修改</SelectItem>
                        <SelectItem value="assign">应用/替换订阅</SelectItem>
                        <SelectItem value="cancel">取消订阅</SelectItem>
                      </SelectContent>
                    </Select>
                    {batchSubscriptionPlanMode === 'assign' ? (
                      <Select value={batchSubscriptionPlanId} onValueChange={setBatchSubscriptionPlanId}>
                        <SelectTrigger>
                          <SelectValue placeholder="选择套餐" />
                        </SelectTrigger>
                        <SelectContent>
                          {plans.map((plan) => (
                            <SelectItem key={plan.id} value={plan.id}>
                              {plan.name}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    ) : null}
                    <Select value={batchSubscriptionExpiryMode} onValueChange={(value) => setBatchSubscriptionExpiryMode(value as SubscriptionBatchExpiryMode)}>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="keep">不改时长</SelectItem>
                        <SelectItem value="set">设定到期时间</SelectItem>
                        <SelectItem value="extend_days">续期天数</SelectItem>
                        <SelectItem value="shorten_days">缩短天数</SelectItem>
                      </SelectContent>
                    </Select>
                    {batchSubscriptionExpiryMode === 'set' ? (
                      <DateTimePicker value={batchSubscriptionExpiresAt} onChange={setBatchSubscriptionExpiresAt} />
                    ) : null}
                    {batchSubscriptionExpiryMode === 'extend_days' || batchSubscriptionExpiryMode === 'shorten_days' ? (
                      <Input
                        value={batchSubscriptionDays}
                        onChange={(event) => setBatchSubscriptionDays(event.target.value)}
                        inputMode="numeric"
                        placeholder="输入天数"
                      />
                    ) : null}
                  </div>
                </div>
              </div>

              <div className="rounded-lg border px-4 py-4">
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <p className="font-medium">预览匹配结果</p>
                    <p className="text-sm text-muted-foreground">先预览再执行，可确认筛选是否正确。</p>
                  </div>
                  <Button variant="outline" onClick={() => void handlePreviewBatch()} disabled={batchPreviewing}>
                    {batchPreviewing ? '预览中...' : '预览'}
                  </Button>
                </div>
                {batchPreview ? (
                  <div className="mt-4 space-y-3">
                    <p className="text-sm text-muted-foreground">共匹配 {batchPreview.count} 个用户，以下展示前 {batchPreview.items.length} 个。</p>
                    <div className="space-y-2">
                      {batchPreview.items.map((item) => (
                        <div key={item.id} className="flex items-center justify-between gap-4 rounded-md border px-3 py-2 text-sm">
                          <span className="font-medium">{item.username}</span>
                          <span className="text-muted-foreground">{item.isAdmin ? '管理员' : '普通用户'}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                ) : null}
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setBatchOpen(false)}>
                取消
              </Button>
              <Button onClick={() => void handleApplyBatch()} disabled={batchApplying}>
                {batchApplying ? '执行中...' : '确认批量修改'}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={!!apiKeysModal} onOpenChange={(open) => !open && setAPIKeysModal(null)}>
          <DialogContent className="sm:max-w-3xl">
            <DialogHeader>
              <DialogTitle>用户 API Keys</DialogTitle>
              <DialogDescription>
                管理用户 <span className="font-medium">{apiKeysModal?.username}</span> 的 API Key 名称和到期时间。
              </DialogDescription>
            </DialogHeader>
            <div className="py-4">
              {userAPIKeysLoading ? (
                <p className="text-center text-muted-foreground">加载中...</p>
              ) : userAPIKeys.length === 0 ? (
                <div className="rounded-lg border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">
                  该用户暂无 API Key
                </div>
              ) : (
                <div className="space-y-3">
                  {userAPIKeys.map((key) => (
                    <div key={key.id} className="flex flex-wrap items-center justify-between gap-3 rounded-lg border px-4 py-3">
                      <div className="space-y-1">
                        <p className="font-medium">{key.name}</p>
                        <p className="text-xs text-muted-foreground">
                          {key.prefix}... · {key.isActive ? '生效中' : '已过期'} · 到期 {key.expiresAt ? formatDateTime(key.expiresAt) : '永不过期'}
                        </p>
                      </div>
                      <div className="flex items-center gap-2">
                        <Button variant="outline" size="sm" onClick={() => handleOpenEditUserAPIKey(key)}>
                          编辑
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          className="text-destructive hover:text-destructive"
                          onClick={() => void handleDeleteUserAPIKey(key)}
                          disabled={deletingUserAPIKeyId === key.id}
                        >
                          {deletingUserAPIKeyId === key.id ? '删除中...' : '删除'}
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
            <DialogFooter>
              <Button onClick={() => setAPIKeysModal(null)}>关闭</Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={!!editingUserAPIKey} onOpenChange={(open) => !open && setEditingUserAPIKey(null)}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>编辑用户 API Key</DialogTitle>
              <DialogDescription>仅可修改名称和到期时间。</DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label>名称</Label>
                <Input value={editingUserAPIKeyName} onChange={(event) => setEditingUserAPIKeyName(event.target.value)} />
              </div>
              <div className="space-y-2">
                <Label>到期时间</Label>
                <DateTimePicker
                  value={editingUserAPIKeyExpiresAt}
                  onChange={setEditingUserAPIKeyExpiresAt}
                  placeholder="留空表示永不过期"
                />
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setEditingUserAPIKey(null)}>
                取消
              </Button>
              <Button onClick={() => void handleSaveUserAPIKey()} disabled={savingUserAPIKey || !editingUserAPIKeyName.trim()}>
                {savingUserAPIKey ? '保存中...' : '保存'}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={!!cancelSubConfirm} onOpenChange={(open) => !open && setCancelSubConfirm(null)}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>确认取消订阅</DialogTitle>
              <DialogDescription>
                确定要取消用户 <span className="font-medium">{cancelSubConfirm?.username}</span> 的订阅吗？
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" onClick={() => setCancelSubConfirm(null)}>
                取消
              </Button>
              <Button variant="destructive" onClick={handleCancelSub}>
                确认取消订阅
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </AdminPageShell>
    </motion.div>
  )
}
