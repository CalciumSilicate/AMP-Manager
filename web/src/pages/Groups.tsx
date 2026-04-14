import { useState, useEffect, useCallback } from 'react'
import { motion, AnimatePresence, staggerContainer, staggerItem, fadeInScale } from '@/lib/motion'
import {
  listGroups,
  createGroup,
  updateGroup,
  deleteGroup,
  Group,
  GroupRequest,
} from '../api/groups'
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
import { Textarea } from '@/components/ui/textarea'
import { FolderOpen, Plus, Pencil, Trash2, CheckCircle2, XCircle } from 'lucide-react'

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

  const formatDate = (dateStr: string) => {
    return new Date(dateStr).toLocaleString('zh-CN')
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
              <FolderOpen className="h-5 w-5" />
              分组列表
            </CardTitle>
            <Button onClick={handleCreate}>
              <Plus className="mr-1 h-4 w-4" />
              添加分组
            </Button>
          </CardHeader>
          <CardContent>
            <AnimatePresence mode="wait">
              {groups.length === 0 ? (
                <AnimatePresence>
                  <motion.div
                    key="empty"
                    variants={fadeInScale}
                    initial="hidden"
                    animate="visible"
                    exit="hidden"
                    className="rounded-md border border-dashed p-8 text-center text-muted-foreground"
                  >
                    暂无分组，点击上方按钮添加
                  </motion.div>
                </AnimatePresence>
              ) : (
                <motion.div key="table" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>名称</TableHead>
                        <TableHead>描述</TableHead>
                        <TableHead>倍率</TableHead>
                        <TableHead>用户数</TableHead>
                        <TableHead>渠道数</TableHead>
                        <TableHead>创建时间</TableHead>
                        <TableHead>操作</TableHead>
                      </TableRow>
                    </TableHeader>
                    <motion.tbody variants={staggerContainer} initial="hidden" animate="visible" key={groups.length}>
                      {groups.map((group) => (
                        <motion.tr key={group.id} variants={staggerItem} layout className="border-b transition-colors hover:bg-muted/50 data-[state=selected]:bg-muted">
                          <TableCell className="font-medium">{group.name}</TableCell>
                          <TableCell className="text-muted-foreground">{group.description || '-'}</TableCell>
                          <TableCell>
                            <Badge variant={group.rateMultiplier === 1 ? 'outline' : 'default'}>
                              {group.rateMultiplier}x
                            </Badge>
                          </TableCell>
                          <TableCell>
                            <Badge variant="secondary">{group.userCount}</Badge>
                          </TableCell>
                          <TableCell>
                            <Badge variant="secondary">{group.channelCount}</Badge>
                          </TableCell>
                          <TableCell className="text-muted-foreground">
                            {formatDate(group.createdAt)}
                          </TableCell>
                          <TableCell>
                            <div className="flex gap-2">
                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() => handleEdit(group)}
                              >
                                <Pencil className="mr-1 h-4 w-4" />
                                编辑
                              </Button>
                              <Button
                                variant="destructive"
                                size="sm"
                                onClick={() => setDeleteConfirmModal(group)}
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
                </motion.div>
              )}
            </AnimatePresence>
          </CardContent>
        </Card>
      </motion.div>

      <Dialog open={showForm} onOpenChange={setShowForm}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editingGroup ? '编辑分组' : '添加分组'}</DialogTitle>
            <DialogDescription>
              {editingGroup ? '修改分组信息' : '创建一个新的分组'}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="groupName">名称 *</Label>
              <Input
                id="groupName"
                placeholder="分组名称"
                value={formData.name}
                onChange={(e) => setFormData(prev => ({ ...prev, name: e.target.value }))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="groupDesc">描述</Label>
              <Textarea
                id="groupDesc"
                placeholder="分组描述（可选）"
                value={formData.description}
                onChange={(e) => setFormData(prev => ({ ...prev, description: e.target.value }))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="groupRate">倍率</Label>
              <Input
                id="groupRate"
                type="number"
                step="0.1"
                min="0"
                placeholder="1.0"
                value={formData.rateMultiplier}
                onChange={(e) => setFormData(prev => ({ ...prev, rateMultiplier: parseFloat(e.target.value) || 1 }))}
              />
              <p className="text-xs text-muted-foreground">
                费用倍率，1.0 表示原价，2.0 表示双倍计费
              </p>
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
              确定要删除分组 <span className="font-medium">{deleteConfirmModal?.name}</span> 吗？此操作不可撤销。
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
