import { useState, useEffect, useCallback } from 'react'
import { motion, AnimatePresence, tableStaggerContainer, tableRowVariants } from '@/lib/motion'
import {
  listGroups,
  createGroup,
  updateGroup,
  deleteGroup,
  Group,
  GroupRequest,
} from '../api/groups'
import { AdminPageShell, AdminSurface } from '@/components/admin/AdminPageShell'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
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
import { Textarea } from '@/components/ui/textarea'
import { formatDate } from '@/lib/formatters'
import { CheckCircle2, XCircle } from 'lucide-react'

export default function Groups() {
  const [groups, setGroups] = useState<Group[]>([])
  const [loading, setLoading] = useState(true)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [editingGroup, setEditingGroup] = useState<Group | null>(null)
  const [formData, setFormData] = useState<GroupRequest>({ name: '', description: '', rateMultiplier: 1 })
  const [saving, setSaving] = useState(false)
  const [deleteConfirmModal, setDeleteConfirmModal] = useState<Group | null>(null)

  const showMessage = useCallback((type: 'success' | 'error', text: string) => {
    setMessage({ type, text })
    setTimeout(() => setMessage(null), 3000)
  }, [])

  const fetchGroups = useCallback(async () => {
    try {
      const data = await listGroups()
      setGroups(data)
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '获取分组列表失败')
    } finally {
      setLoading(false)
    }
  }, [showMessage])

  useEffect(() => {
    fetchGroups()
  }, [fetchGroups])

  const handleCreate = () => {
    setEditingGroup(null)
    setFormData({ name: '', description: '', rateMultiplier: 1 })
    setShowForm(true)
  }

  const handleEdit = (group: Group) => {
    setEditingGroup(group)
    setFormData({ name: group.name, description: group.description, rateMultiplier: group.rateMultiplier })
    setShowForm(true)
  }

  const handleSubmit = async () => {
    if (!formData.name.trim()) {
      showMessage('error', '请填写分组名称')
      return
    }

    setSaving(true)
    try {
      if (editingGroup) {
        await updateGroup(editingGroup.id, formData)
        showMessage('success', '分组已更新')
      } else {
        await createGroup(formData)
        showMessage('success', '分组已创建')
      }
      setShowForm(false)
      fetchGroups()
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    if (!deleteConfirmModal) return

    try {
      await deleteGroup(deleteConfirmModal.id)
      showMessage('success', '分组已删除')
      setDeleteConfirmModal(null)
      fetchGroups()
    } catch (err) {
      showMessage('error', err instanceof Error ? err.message : '删除失败')
    }
  }

  if (loading) {
    return <div className="text-center text-muted-foreground">加载中...</div>
  }

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
      <AdminPageShell
        title="分组管理"
        description="维护用户分组与倍率规则。"
        actions={<Button onClick={handleCreate}>添加分组</Button>}
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
            {groups.length === 0 ? (
              <div className="admin-surface-body">
                <div className="rounded-md border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">
                  暂无分组
                </div>
              </div>
            ) : (
              <>
                <div className="admin-surface-header">
                  <div className="space-y-1">
                    <p className="text-sm font-medium text-foreground">{groups.length} 个分组</p>
                    <p className="admin-inline-note">用于倍率控制与权限归类。</p>
                  </div>
                </div>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>名称</TableHead>
                      <TableHead>描述</TableHead>
                      <TableHead>倍率</TableHead>
                      <TableHead>用户数</TableHead>
                      <TableHead>渠道数</TableHead>
                      <TableHead>创建时间</TableHead>
                      <TableHead className="text-right">操作</TableHead>
                    </TableRow>
                  </TableHeader>
                  <motion.tbody variants={tableStaggerContainer} initial="hidden" animate="visible" key={groups.length}>
                    {groups.map((group) => (
                      <motion.tr key={group.id} variants={tableRowVariants}>
                        <TableCell className="font-medium">{group.name}</TableCell>
                        <TableCell className="text-muted-foreground">{group.description || '-'}</TableCell>
                        <TableCell>
                          <Badge variant={group.rateMultiplier === 1 ? 'outline' : 'default'}>{group.rateMultiplier}x</Badge>
                        </TableCell>
                        <TableCell>{group.userCount}</TableCell>
                        <TableCell>{group.channelCount}</TableCell>
                        <TableCell className="text-muted-foreground">{formatDate(group.createdAt)}</TableCell>
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-2">
                            <Button variant="ghost" size="sm" onClick={() => handleEdit(group)}>
                              编辑
                            </Button>
                            <Button variant="ghost" size="sm" className="text-destructive hover:text-destructive" onClick={() => setDeleteConfirmModal(group)}>
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
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{editingGroup ? '编辑分组' : '添加分组'}</DialogTitle>
              <DialogDescription>{editingGroup ? '修改分组信息。' : '创建一个新的分组。'}</DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="groupName">名称</Label>
                <Input
                  id="groupName"
                  placeholder="分组名称"
                  value={formData.name}
                  onChange={(e) => setFormData((prev) => ({ ...prev, name: e.target.value }))}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="groupDesc">描述</Label>
                <Textarea
                  id="groupDesc"
                  placeholder="可选"
                  value={formData.description}
                  onChange={(e) => setFormData((prev) => ({ ...prev, description: e.target.value }))}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="groupRate">倍率</Label>
                <Input
                  id="groupRate"
                  type="number"
                  step="0.1"
                  min="0"
                  value={formData.rateMultiplier}
                  onChange={(e) => setFormData((prev) => ({ ...prev, rateMultiplier: Number(e.target.value) || 0 }))}
                />
                <p className="text-xs text-muted-foreground">默认 1.0。</p>
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
                确定要删除分组 <span className="font-medium">{deleteConfirmModal?.name}</span> 吗？
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
