import { useCallback, useEffect, useState } from 'react'
import { AnimatePresence, motion, tableRowVariants, tableStaggerContainer } from '@/lib/motion'
import { register } from '../api/auth'
import {
  deleteUser,
  listUsersPaged,
  resetUserPassword,
  setUserAdmin,
  setUserGroups,
  topUpUser,
  UserInfo,
} from '../api/users'
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
  KeyRound,
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
  }, [page, pageSize, showMessage])

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

  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  if (loading && !hasLoadedUsers) {
    return <div className="text-center text-muted-foreground">加载中...</div>
  }

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
      <AdminPageShell
        title="用户列表"
        description="管理用户权限、余额、分组与订阅。"
        width="7xl"
        actions={(
          <Button size="sm" onClick={() => setCreateUserOpen(true)}>
            <UserPlus className="mr-1.5 h-4 w-4" />
            新建用户
          </Button>
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
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>用户名</TableHead>
                    <TableHead>角色</TableHead>
                    <TableHead>分组</TableHead>
                    <TableHead>余额 (USD)</TableHead>
                    <TableHead>管理员权限</TableHead>
                    <TableHead>创建时间</TableHead>
                    <TableHead className="w-[240px]">操作</TableHead>
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
