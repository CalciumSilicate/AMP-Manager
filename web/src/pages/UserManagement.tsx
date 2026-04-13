import { useState, useEffect } from 'react'
import { motion, AnimatePresence, tableStaggerContainer, tableRowVariants } from '@/lib/motion'
import {
  listUsersPaged,
  setUserAdmin,
  deleteUser,
  resetUserPassword,
  setUserGroups,
  topUpUser,
  UserInfo,
} from '../api/users'
import { listGroups, Group } from '../api/groups'
import {
  getPlans,
  getUserSubscription,
  assignSubscription,
  cancelSubscription,
  updateSubscriptionExpiry,
  SubscriptionPlanResponse,
  UserSubscriptionResponse,
  LimitType,
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
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Checkbox } from '@/components/ui/checkbox'
import { formatDateTime } from '@/lib/formatters'
import { CheckCircle2, XCircle, Trash2, KeyRound, Wallet, CreditCard, CalendarClock, Eye, X } from 'lucide-react'
import { DateTimePicker } from '@/components/ui/datetime-picker'
import { PageSizeSlider } from '@/components/PageSizeSlider'

const LIMIT_TYPE_LABELS: Record<LimitType, string> = {
  daily: '日限制',
  weekly: '周限制',
  monthly: '月限制',
  rolling_5h: '5小时滚动',
  total: '总量限制',
}

function microsToUsd(micros: number): string {
  return (micros / 1_000_000).toFixed(2)
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
  const [assignSubModal, setAssignSubModal] = useState<{ userId: string; username: string } | null>(null)
  const [assignPlanId, setAssignPlanId] = useState('')
  const [assignExpiresAt, setAssignExpiresAt] = useState('')
  const [viewSubModal, setViewSubModal] = useState<{ userId: string; username: string } | null>(null)
  const [viewingSub, setViewingSub] = useState<UserSubscriptionResponse | null>(null)
  const [viewSubLoading, setViewSubLoading] = useState(false)
  const [extendModal, setExtendModal] = useState<{ userId: string; username: string } | null>(null)
  const [extendDate, setExtendDate] = useState('')
  const [cancelSubConfirm, setCancelSubConfirm] = useState<{ userId: string; username: string } | null>(null)

  useEffect(() => {
    fetchUsers()
  }, [page, pageSize])

  useEffect(() => {
    fetchGroups()
    fetchPlansList()
  }, [])

  const fetchUsers = async (targetPage = page, targetPageSize = pageSize) => {
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
  }

  const fetchGroups = async () => {
    try {
      const data = await listGroups()
      setGroups(data)
    } catch {}
  }

  const fetchPlansList = async () => {
    try {
      const data = await getPlans()
      setPlans(data.filter((p) => p.enabled))
    } catch {}
  }

  const handleOpenAssignSub = (userId: string, username: string) => {
    setAssignPlanId('')
    setAssignExpiresAt('')
    setAssignSubModal({ userId, username })
  }

  const handleAssignSub = async () => {
    if (!assignSubModal || !assignPlanId) return
    try {
      await assignSubscription(assignSubModal.userId, {
        planId: assignPlanId,
        expiresAt: assignExpiresAt || undefined,
      })
      showMessage('success', '订阅已分配')
      setAssignSubModal(null)
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '分配订阅失败')
    }
  }

  const handleViewSub = async (userId: string, username: string) => {
    setViewSubModal({ userId, username })
    setViewSubLoading(true)
    setViewingSub(null)
    try {
      const sub = await getUserSubscription(userId)
      setViewingSub(sub)
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '获取订阅失败')
      setViewSubModal(null)
    } finally {
      setViewSubLoading(false)
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

  const handleOpenExtend = (userId: string, username: string) => {
    setExtendDate('')
    setExtendModal({ userId, username })
  }

  const handleExtend = async () => {
    if (!extendModal || !extendDate) return
    try {
      await updateSubscriptionExpiry(extendModal.userId, extendDate)
      showMessage('success', '到期时间已更新')
      setExtendModal(null)
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '更新到期时间失败')
    }
  }

  const showMessage = (type: 'success' | 'error', text: string) => {
    setMessage({ type, text })
    setTimeout(() => setMessage(null), 3000)
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
    const amount = parseFloat(topUpAmount)
    if (isNaN(amount) || amount <= 0) {
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

  const handlePageSizeChange = (value: number) => {
    setPageSize(value)
    setPage(1)
  }

  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  if (loading && !hasLoadedUsers) {
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
        <CardHeader>
          <CardTitle>用户列表</CardTitle>
        </CardHeader>
        <CardContent className={fetching ? 'opacity-60 transition-opacity' : 'transition-opacity'}>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>用户名</TableHead>
                <TableHead>角色</TableHead>
                <TableHead>分组</TableHead>
                <TableHead>余额 (USD)</TableHead>
                <TableHead>管理员权限</TableHead>
                <TableHead>创建时间</TableHead>
                <TableHead>操作</TableHead>
              </TableRow>
            </TableHeader>
            <motion.tbody key="user-table-body" variants={tableStaggerContainer} initial="hidden" animate="visible">
              {users.map((user) => (
                <motion.tr key={user.id} variants={tableRowVariants} className="border-b transition-colors hover:bg-muted/50 data-[state=selected]:bg-muted">
                  <TableCell className="font-medium">{user.username}</TableCell>
                  <TableCell>
                    <Badge variant={user.isAdmin ? 'default' : 'secondary'}>
                      {user.isAdmin ? '管理员' : '普通用户'}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <Popover>
                      <PopoverTrigger asChild>
                        <Button variant="outline" size="sm" className="h-auto min-h-[32px] py-1 px-2">
                          {user.groupNames && user.groupNames.length > 0 ? (
                            <div className="flex flex-wrap gap-1">
                              <AnimatePresence mode="popLayout">
                                {user.groupNames.map((name) => (
                                  <motion.span
                                    key={name}
                                    initial={{ opacity: 0, scale: 0.8 }}
                                    animate={{ opacity: 1, scale: 1 }}
                                    exit={{ opacity: 0, scale: 0.8 }}
                                    transition={{ type: 'spring', bounce: 0.3, duration: 0.3 }}
                                  >
                                    <Badge variant="secondary" className="text-xs">{name}</Badge>
                                  </motion.span>
                                ))}
                              </AnimatePresence>
                            </div>
                          ) : (
                            <span className="text-muted-foreground text-xs">未分组</span>
                          )}
                        </Button>
                      </PopoverTrigger>
                      <PopoverContent className="w-[200px] p-2" align="start">
                        <div className="space-y-2">
                          <p className="text-sm font-medium px-1">选择分组</p>
                          {groups.map(g => (
                            <label key={g.id} className="flex items-center gap-2 px-1 py-1 rounded hover:bg-muted cursor-pointer">
                              <Checkbox
                                checked={(user.groupIds || []).includes(g.id)}
                                onCheckedChange={async (checked) => {
                                  const currentIds = user.groupIds || []
                                  const newIds = checked
                                    ? [...currentIds, g.id]
                                    : currentIds.filter(id => id !== g.id)
                                  try {
                                    await setUserGroups(user.id, newIds)
                                    showMessage('success', '分组已更新')
                                    fetchUsers()
                                  } catch (err) {
                                    showMessage('error', err instanceof Error ? err.message : '设置分组失败')
                                  }
                                }}
                              />
                              <span className="text-sm">{g.name}</span>
                            </label>
                          ))}
                          {groups.length === 0 && (
                            <p className="text-xs text-muted-foreground px-1">暂无分组</p>
                          )}
                        </div>
                      </PopoverContent>
                    </Popover>
                  </TableCell>
                  <TableCell className="font-mono text-sm">
                    ${parseFloat(user.balanceUsd || '0').toFixed(4)}
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-2">
                      <Switch
                        checked={user.isAdmin}
                        onCheckedChange={() => handleToggleAdmin(user)}
                      />
                      <span className="text-sm text-muted-foreground">
                        {user.isAdmin ? '已启用' : '已禁用'}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatDateTime(user.createdAt)}
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => handleOpenAssignSub(user.id, user.username)}
                      >
                        <CreditCard className="mr-1 h-4 w-4" />
                        分配订阅
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => handleViewSub(user.id, user.username)}
                      >
                        <Eye className="mr-1 h-4 w-4" />
                        查看订阅
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => handleOpenExtend(user.id, user.username)}
                      >
                        <CalendarClock className="mr-1 h-4 w-4" />
                        延期
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        className="text-destructive hover:text-destructive"
                        onClick={() => setCancelSubConfirm({ userId: user.id, username: user.username })}
                      >
                        <X className="mr-1 h-4 w-4" />
                        取消订阅
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setTopUpModal({ userId: user.id, username: user.username })}
                      >
                        <Wallet className="mr-1 h-4 w-4" />
                        充值
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setResetPasswordModal({ userId: user.id, username: user.username })}
                      >
                        <KeyRound className="mr-1 h-4 w-4" />
                        重置密码
                      </Button>
                      <Button
                        variant="destructive"
                        size="sm"
                        onClick={() => setDeleteConfirmModal(user)}
                      >
                        <Trash2 className="mr-1 h-4 w-4" />
                        删除
                      </Button>
                    </div>
                  </TableCell>
                </motion.tr>
              ))}
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
        </CardContent>
      </Card>
      </motion.div>

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
                onChange={(e) => setNewPassword(e.target.value)}
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
                onChange={(e) => setTopUpAmount(e.target.value)}
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
            <Button onClick={handleTopUp} disabled={!topUpAmount || parseFloat(topUpAmount) <= 0}>
              确认充值
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Assign Subscription Dialog */}
      <Dialog open={!!assignSubModal} onOpenChange={(open) => !open && setAssignSubModal(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>分配订阅</DialogTitle>
            <DialogDescription>
              为用户 <span className="font-medium">{assignSubModal?.username}</span> 分配订阅套餐
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label>选择套餐</Label>
              <Select value={assignPlanId} onValueChange={setAssignPlanId}>
                <SelectTrigger>
                  <SelectValue placeholder="选择套餐..." />
                </SelectTrigger>
                <SelectContent>
                  {plans.map((p) => (
                    <SelectItem key={p.id} value={p.id}>{p.name}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {plans.length === 0 && (
                <p className="text-xs text-muted-foreground">暂无可用套餐，请先创建</p>
              )}
            </div>
            <div className="space-y-2">
              <Label>到期时间（可选）</Label>
              <DateTimePicker
                value={assignExpiresAt}
                onChange={setAssignExpiresAt}
                placeholder="选择到期时间"
              />
              <p className="text-xs text-muted-foreground">留空表示永不过期</p>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setAssignSubModal(null)}>
              取消
            </Button>
            <Button onClick={handleAssignSub} disabled={!assignPlanId}>
              确认分配
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* View Subscription Dialog */}
      <Dialog open={!!viewSubModal} onOpenChange={(open) => !open && setViewSubModal(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>用户订阅详情</DialogTitle>
            <DialogDescription>
              用户 <span className="font-medium">{viewSubModal?.username}</span> 的订阅信息
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            {viewSubLoading ? (
              <p className="text-center text-muted-foreground">加载中...</p>
            ) : viewingSub ? (
              <div className="space-y-3">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">套餐名称</span>
                  <span className="font-medium">{viewingSub.planName}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">状态</span>
                  <Badge variant={viewingSub.status === 'active' ? 'default' : 'secondary'}>
                    {viewingSub.status === 'active' ? '活跃' : viewingSub.status === 'expired' ? '已过期' : viewingSub.status === 'cancelled' ? '已取消' : viewingSub.status}
                  </Badge>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">开始时间</span>
                  <span>{formatDateTime(viewingSub.startsAt)}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">到期时间</span>
                  <span>{viewingSub.expiresAt ? formatDateTime(viewingSub.expiresAt) : '永不过期'}</span>
                </div>
                {viewingSub.limits && viewingSub.limits.length > 0 && (
                  <div className="space-y-2 pt-2 border-t">
                    <span className="text-sm font-medium">额度限制</span>
                    {viewingSub.limits.map((l) => (
                      <div key={l.id} className="flex justify-between text-sm">
                        <span className="text-muted-foreground">
                          {LIMIT_TYPE_LABELS[l.limitType] || l.limitType}
                          {l.fixedResetTime ? ` @ ${l.fixedResetTime}` : ''}
                        </span>
                        <span>${microsToUsd(l.limitMicros)}</span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            ) : (
              <p className="text-center text-muted-foreground">该用户暂无订阅</p>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setViewSubModal(null)}>
              关闭
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Extend Subscription Dialog */}
      <Dialog open={!!extendModal} onOpenChange={(open) => !open && setExtendModal(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>延长订阅</DialogTitle>
            <DialogDescription>
              为用户 <span className="font-medium">{extendModal?.username}</span> 设置新的到期时间
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label>新到期时间</Label>
              <DateTimePicker
                value={extendDate}
                onChange={setExtendDate}
                placeholder="选择到期时间"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setExtendModal(null)}>
              取消
            </Button>
            <Button onClick={handleExtend} disabled={!extendDate}>
              确认延期
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Cancel Subscription Confirm */}
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
    </motion.div>
  )
}
