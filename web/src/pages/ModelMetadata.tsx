import { useEffect, useState } from 'react'

import { motion, tableStaggerContainer, tableRowVariants } from '@/lib/motion'
import {
  listModelMetadata,
  createModelMetadata,
  updateModelMetadata,
  deleteModelMetadata,
  ModelMetadata,
  ModelMetadataRequest,
} from '../api/modelMetadata'
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
import { formatDate } from '@/lib/formatters'

const PROVIDERS = ['anthropic', 'openai', 'google', 'deepseek', 'alibaba', 'other']

function formatTokenCount(count: number): string {
  if (count >= 1000000) return `${(count / 1000000).toFixed(count % 1000000 === 0 ? 0 : 1)}M`
  if (count >= 1000) return `${(count / 1000).toFixed(count % 1000 === 0 ? 0 : 1)}k`
  return count.toString()
}

const providerVariants: Record<string, 'default' | 'secondary' | 'destructive' | 'outline'> = {
  anthropic: 'default',
  openai: 'secondary',
  google: 'outline',
  deepseek: 'default',
  alibaba: 'secondary',
  other: 'outline',
}

export default function ModelMetadataPage() {
  const [metadata, setMetadata] = useState<ModelMetadata[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [showForm, setShowForm] = useState(false)
  const [editingItem, setEditingItem] = useState<ModelMetadata | null>(null)
  const [formData, setFormData] = useState<ModelMetadataRequest>({
    modelPattern: '',
    displayName: '',
    contextLength: 200000,
    maxCompletionTokens: 8192,
    provider: 'anthropic',
  })
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    loadMetadata()
  }, [])

  const loadMetadata = async () => {
    try {
      const data = await listModelMetadata()
      setMetadata(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }

  const handleCreate = () => {
    setEditingItem(null)
    setFormData({
      modelPattern: '',
      displayName: '',
      contextLength: 200000,
      maxCompletionTokens: 8192,
      provider: 'anthropic',
    })
    setShowForm(true)
  }

  const handleEdit = (item: ModelMetadata) => {
    setEditingItem(item)
    setFormData({
      modelPattern: item.modelPattern,
      displayName: item.displayName,
      contextLength: item.contextLength,
      maxCompletionTokens: item.maxCompletionTokens,
      provider: item.provider,
    })
    setShowForm(true)
  }

  const handleSubmit = async () => {
    if (!formData.modelPattern.trim() || !formData.displayName.trim()) {
      setError('请填写必填字段')
      return
    }

    setSaving(true)
    setError('')

    try {
      if (editingItem) {
        await updateModelMetadata(editingItem.id, formData)
      } else {
        await createModelMetadata(formData)
      }
      setShowForm(false)
      loadMetadata()
    } catch (err) {
      setError(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (id: string, pattern: string) => {
    if (!confirm(`确定要删除模型元数据 "${pattern}" 吗？`)) return

    try {
      await deleteModelMetadata(id)
      loadMetadata()
    } catch (err) {
      setError(err instanceof Error ? err.message : '删除失败')
    }
  }

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <div className="text-muted-foreground">加载中...</div>
      </div>
    )
  }

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
      <AdminPageShell
        title="模型元数据管理"
        description="维护模型显示名、上下文长度和最大输出限制。"
        actions={<Button onClick={handleCreate}>添加模型元数据</Button>}
      >
        <motion.div initial={{ opacity: 0, y: 24 }} animate={{ opacity: 1, y: 0 }} transition={{ type: 'spring', bounce: 0.2, duration: 0.55 }}>
          <AdminSurface>
            {error && (
              <div className="admin-surface-body pb-0">
                <Alert variant="destructive">
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              </div>
            )}
            {metadata.length === 0 ? (
              <div className="admin-surface-body">
                <div className="rounded-md border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">
                  暂无模型元数据
                </div>
              </div>
            ) : (
              <>
                <div className="admin-surface-header">
                  <div className="space-y-1">
                    <p className="text-sm font-medium text-foreground">{metadata.length} 条规则</p>
                    <p className="admin-inline-note">使用前缀匹配覆盖模型默认参数。</p>
                  </div>
                </div>
                <div className="overflow-x-auto">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>模型模式</TableHead>
                        <TableHead>显示名称</TableHead>
                        <TableHead>上下文</TableHead>
                        <TableHead>最大输出</TableHead>
                        <TableHead>提供商</TableHead>
                        <TableHead>更新时间</TableHead>
                        <TableHead className="text-right">操作</TableHead>
                      </TableRow>
                    </TableHeader>
                    <motion.tbody variants={tableStaggerContainer} initial="hidden" animate="visible" key={metadata.length}>
                      {metadata.map((item) => (
                        <motion.tr key={item.id} variants={tableRowVariants}>
                          <TableCell className="font-mono text-sm">{item.modelPattern}</TableCell>
                          <TableCell>{item.displayName}</TableCell>
                          <TableCell>{formatTokenCount(item.contextLength)}</TableCell>
                          <TableCell>{formatTokenCount(item.maxCompletionTokens)}</TableCell>
                          <TableCell>
                            <Badge variant={providerVariants[item.provider] || 'outline'}>{item.provider}</Badge>
                          </TableCell>
                          <TableCell className="text-muted-foreground">{formatDate(item.updatedAt)}</TableCell>
                          <TableCell className="text-right">
                            <div className="flex justify-end gap-2">
                              <Button variant="ghost" size="sm" onClick={() => handleEdit(item)}>
                                编辑
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                className="text-destructive hover:text-destructive"
                                onClick={() => handleDelete(item.id, item.modelPattern)}
                              >
                                删除
                              </Button>
                            </div>
                          </TableCell>
                        </motion.tr>
                      ))}
                    </motion.tbody>
                  </Table>
                </div>
              </>
            )}
          </AdminSurface>
        </motion.div>

        <Dialog open={showForm} onOpenChange={setShowForm}>
          <DialogContent className="sm:max-w-[600px]">
            <DialogHeader>
              <DialogTitle>{editingItem ? '编辑模型元数据' : '添加模型元数据'}</DialogTitle>
              <DialogDescription>配置模型匹配规则和关键参数。</DialogDescription>
            </DialogHeader>
            <div className="grid grid-cols-2 gap-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="modelPattern">模型模式</Label>
                <Input
                  id="modelPattern"
                  value={formData.modelPattern}
                  onChange={(e) => setFormData((prev) => ({ ...prev, modelPattern: e.target.value }))}
                  placeholder="例如 claude-sonnet"
                />
                <p className="text-xs text-muted-foreground">前缀匹配。</p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="displayName">显示名称</Label>
                <Input
                  id="displayName"
                  value={formData.displayName}
                  onChange={(e) => setFormData((prev) => ({ ...prev, displayName: e.target.value }))}
                  placeholder="例如 Claude Sonnet 4"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="contextLength">上下文长度</Label>
                <Input
                  id="contextLength"
                  type="number"
                  min="1"
                  value={formData.contextLength}
                  onChange={(e) => setFormData((prev) => ({ ...prev, contextLength: parseInt(e.target.value) || 0 }))}
                />
                <p className="text-xs text-muted-foreground">当前 {formatTokenCount(formData.contextLength)}。</p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="maxCompletionTokens">最大输出</Label>
                <Input
                  id="maxCompletionTokens"
                  type="number"
                  min="1"
                  value={formData.maxCompletionTokens}
                  onChange={(e) =>
                    setFormData((prev) => ({ ...prev, maxCompletionTokens: parseInt(e.target.value) || 0 }))
                  }
                />
                <p className="text-xs text-muted-foreground">当前 {formatTokenCount(formData.maxCompletionTokens)}。</p>
              </div>
              <div className="col-span-2 space-y-2">
                <Label htmlFor="provider">提供商</Label>
                <select
                  id="provider"
                  value={formData.provider}
                  onChange={(e) => setFormData((prev) => ({ ...prev, provider: e.target.value }))}
                  className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                >
                  {PROVIDERS.map((provider) => (
                    <option key={provider} value={provider}>
                      {provider}
                    </option>
                  ))}
                </select>
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
      </AdminPageShell>
    </motion.div>
  )
}
