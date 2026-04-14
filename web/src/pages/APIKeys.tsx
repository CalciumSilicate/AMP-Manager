import { useEffect, useState } from 'react'

import { motion, AnimatePresence, tableStaggerContainer, tableRowVariants } from '@/lib/motion'
import {
  getAPIKeys,
  createAPIKeyWithOptions,
  deleteAPIKey,
  getAPIKey,
  APIKey,
  CreateAPIKeyResponse,
  APIKeyRevealResponse,
} from '../api/amp'
import { AdminPageShell, AdminSurface } from '@/components/admin/AdminPageShell'
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
import { formatDateTime } from '@/lib/formatters'

export default function APIKeys() {
  const [keys, setKeys] = useState<APIKey[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [createName, setCreateName] = useState('')
  const [customKey, setCustomKey] = useState('')
  const [creating, setCreating] = useState(false)
  const [newKey, setNewKey] = useState<CreateAPIKeyResponse | null>(null)
  const [revealKey, setRevealKey] = useState<APIKeyRevealResponse | null>(null)
  const [copied, setCopied] = useState<string | null>(null)
  const [revealingId, setRevealingId] = useState<string | null>(null)
  const [deletingId, setDeletingId] = useState<string | null>(null)

  useEffect(() => {
    loadData()
  }, [])

  const loadData = async () => {
    try {
      const keysData = await getAPIKeys()
      setKeys(keysData)
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }

  const handleCreate = async () => {
    if (!createName.trim()) return
    if (customKey && !/^[A-Za-z0-9]{16,}$/.test(customKey)) {
      setError('自定义 API Key 只能包含字母和数字，且长度至少为 16')
      return
    }

    setCreating(true)
    setError('')

    try {
      const result = await createAPIKeyWithOptions({
        name: createName.trim(),
        ...(customKey ? { customKey } : {}),
      })
      setNewKey(result)
      setCreateName('')
      setCustomKey('')
      setShowCreate(false)
      loadData()
    } catch (err) {
      setError(err instanceof Error ? err.message : '创建失败')
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (id: string, name: string) => {
    if (!confirm(`确定要删除 API Key "${name}" 吗？此操作不可恢复。`)) return

    setDeletingId(id)
    setError('')

    try {
      await deleteAPIKey(id)
      loadData()
    } catch (err) {
      setError(err instanceof Error ? err.message : '删除失败')
    } finally {
      setDeletingId(null)
    }
  }

  const handleReveal = async (id: string) => {
    setRevealingId(id)
    setError('')

    try {
      const result = await getAPIKey(id)
      setRevealKey(result)
    } catch (err) {
      setError(err instanceof Error ? err.message : '获取失败')
    } finally {
      setRevealingId(null)
    }
  }

  const copyToClipboard = async (text: string, type: string) => {
    await navigator.clipboard.writeText(text)
    setCopied(type)
    setTimeout(() => setCopied(null), 2000)
  }

  const formatDate = (dateStr: string | null) => (dateStr ? formatDateTime(dateStr) : '-')
  const linuxEnvSnippet = (apiKey: string) => `export AMP_URL="${window.location.origin}"\nexport AMP_API_KEY="${apiKey}"`
  const powershellSnippet = (apiKey: string) =>
    `[Environment]::SetEnvironmentVariable("AMP_URL", "${window.location.origin}", "User")\n[Environment]::SetEnvironmentVariable("AMP_API_KEY", "${apiKey}", "User")`

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
        title="API Key 管理"
        description="管理用于 Amp CLI 认证的 API Key。"
        width="4xl"
        actions={<Button onClick={() => setShowCreate(true)}>创建 API Key</Button>}
      >
        <AnimatePresence>
          {newKey && (
            <motion.div
              initial={{ opacity: 0, y: -20, scale: 0.98 }}
              animate={{ opacity: 1, y: 0, scale: 1 }}
              exit={{ opacity: 0, y: -20, scale: 0.98 }}
              transition={{ type: 'spring', bounce: 0.2, duration: 0.45 }}
              className="space-y-3 rounded-lg border border-emerald-500/30 bg-emerald-500/5 px-5 py-4"
            >
              <div className="flex items-center justify-between gap-3">
                <div className="space-y-1">
                  <p className="text-sm font-semibold text-emerald-700 dark:text-emerald-300">API Key 创建成功</p>
                  <p className="text-xs text-muted-foreground">请立即保存，后续仍可在列表中查看。</p>
                </div>
                <Button variant="ghost" size="sm" onClick={() => setNewKey(null)}>
                  关闭
                </Button>
              </div>
              <div className="space-y-3">
                <div className="space-y-2">
                  <Label>API Key</Label>
                  <div className="flex flex-col gap-2 sm:flex-row">
                    <code className="min-w-0 flex-1 rounded-md border bg-background px-3 py-2 text-sm font-mono break-all">
                      {newKey.apiKey}
                    </code>
                    <Button size="sm" onClick={() => copyToClipboard(newKey.apiKey, 'apiKey')}>
                      {copied === 'apiKey' ? '已复制' : '复制'}
                    </Button>
                  </div>
                </div>
                <div className="grid gap-3 md:grid-cols-2">
                  <div className="space-y-2">
                    <Label>Linux/macOS</Label>
                    <pre className="overflow-x-auto rounded-md border bg-slate-950 px-3 py-3 text-xs text-slate-100">
                      <code>{linuxEnvSnippet(newKey.apiKey)}</code>
                    </pre>
                    <Button variant="outline" size="sm" onClick={() => copyToClipboard(linuxEnvSnippet(newKey.apiKey), 'env')}>
                      {copied === 'env' ? '已复制' : '复制环境变量'}
                    </Button>
                  </div>
                  <div className="space-y-2">
                    <Label>PowerShell</Label>
                    <pre className="overflow-x-auto rounded-md border bg-slate-950 px-3 py-3 text-xs text-slate-100">
                      <code>{powershellSnippet(newKey.apiKey)}</code>
                    </pre>
                    <Button variant="outline" size="sm" onClick={() => copyToClipboard(powershellSnippet(newKey.apiKey), 'ps')}>
                      {copied === 'ps' ? '已复制' : '复制 PowerShell 命令'}
                    </Button>
                  </div>
                </div>
              </div>
            </motion.div>
          )}
        </AnimatePresence>

        <motion.div initial={{ opacity: 0, y: 24 }} animate={{ opacity: 1, y: 0 }} transition={{ type: 'spring', bounce: 0.2, duration: 0.55 }}>
          <AdminSurface>
            {error && (
              <div className="admin-surface-body pb-0">
                <Alert variant="destructive">
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              </div>
            )}
            {keys.length === 0 ? (
              <div className="admin-surface-body">
                <div className="rounded-md border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">
                  暂无 API Key
                </div>
              </div>
            ) : (
              <motion.div initial={{ opacity: 0, y: 18 }} animate={{ opacity: 1, y: 0 }} transition={{ type: 'spring', bounce: 0.2, duration: 0.45 }}>
                <div className="admin-surface-header">
                  <div className="space-y-1">
                    <p className="text-sm font-medium text-foreground">已创建 {keys.length} 个 Key</p>
                    <p className="admin-inline-note">支持查看明文与删除操作。</p>
                  </div>
                </div>
                <div className="overflow-hidden rounded-b-lg">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>名称</TableHead>
                        <TableHead>Prefix</TableHead>
                        <TableHead>最后使用</TableHead>
                        <TableHead>创建时间</TableHead>
                        <TableHead className="text-right">操作</TableHead>
                      </TableRow>
                    </TableHeader>
                    <motion.tbody variants={tableStaggerContainer} initial="hidden" animate="visible" key={keys.length}>
                      {keys.map((key) => (
                        <motion.tr key={key.id} variants={tableRowVariants}>
                          <TableCell className="font-medium">{key.name}</TableCell>
                          <TableCell className="font-mono text-muted-foreground">{key.prefix}...</TableCell>
                          <TableCell>{formatDate(key.lastUsedAt)}</TableCell>
                          <TableCell>{formatDate(key.createdAt)}</TableCell>
                          <TableCell className="text-right">
                            <div className="flex items-center justify-end gap-2">
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => handleReveal(key.id)}
                                disabled={revealingId === key.id}
                              >
                                {revealingId === key.id ? '加载中...' : '查看'}
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                className="text-destructive hover:text-destructive"
                                onClick={() => handleDelete(key.id, key.name)}
                                disabled={deletingId === key.id}
                              >
                                {deletingId === key.id ? '删除中...' : '删除'}
                              </Button>
                            </div>
                          </TableCell>
                        </motion.tr>
                      ))}
                    </motion.tbody>
                  </Table>
                </div>
              </motion.div>
            )}
          </AdminSurface>
        </motion.div>

        <Dialog open={showCreate} onOpenChange={setShowCreate}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>创建新 API Key</DialogTitle>
              <DialogDescription>为新设备或应用创建一个 API Key。</DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="keyName">API Key 名称</Label>
                <Input
                  id="keyName"
                  value={createName}
                  onChange={(e) => setCreateName(e.target.value)}
                  placeholder="输入 API Key 名称"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="customKey">自定义 API Key</Label>
                <Input
                  id="customKey"
                  value={customKey}
                  onChange={(e) => setCustomKey(e.target.value.trim())}
                  placeholder="留空自动生成；或输入 16 位以上字母数字"
                  autoComplete="off"
                />
                <p className="text-xs text-muted-foreground">仅创建时可设置。</p>
              </div>
            </div>
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => {
                  setShowCreate(false)
                  setCreateName('')
                  setCustomKey('')
                }}
              >
                取消
              </Button>
              <Button
                onClick={handleCreate}
                disabled={creating || !createName.trim() || !!(customKey && !/^[A-Za-z0-9]{16,}$/.test(customKey))}
              >
                {creating ? '创建中...' : '创建'}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={!!revealKey} onOpenChange={(open) => !open && setRevealKey(null)}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>查看 API Key</DialogTitle>
              <DialogDescription>API Key 明文会显示在此处，请妥善保管。</DialogDescription>
            </DialogHeader>
            {revealKey && (
              <div className="space-y-4 py-4">
                <div className="space-y-2">
                  <Label>API Key</Label>
                  <div className="flex flex-col gap-2 sm:flex-row">
                    <code className="min-w-0 flex-1 rounded-md border bg-background px-3 py-2 text-sm font-mono break-all">
                      {revealKey.apiKey}
                    </code>
                    <Button size="sm" onClick={() => copyToClipboard(revealKey.apiKey, 'revealApiKey')}>
                      {copied === 'revealApiKey' ? '已复制' : '复制'}
                    </Button>
                  </div>
                </div>
                <div className="grid gap-3 md:grid-cols-2">
                  <div className="space-y-2">
                    <Label>Linux/macOS</Label>
                    <pre className="overflow-x-auto rounded-md border bg-slate-950 px-3 py-3 text-xs text-slate-100">
                      <code>{linuxEnvSnippet(revealKey.apiKey)}</code>
                    </pre>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => copyToClipboard(linuxEnvSnippet(revealKey.apiKey), 'revealEnv')}
                    >
                      {copied === 'revealEnv' ? '已复制' : '复制环境变量'}
                    </Button>
                  </div>
                  <div className="space-y-2">
                    <Label>PowerShell</Label>
                    <pre className="overflow-x-auto rounded-md border bg-slate-950 px-3 py-3 text-xs text-slate-100">
                      <code>{powershellSnippet(revealKey.apiKey)}</code>
                    </pre>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => copyToClipboard(powershellSnippet(revealKey.apiKey), 'revealPs')}
                    >
                      {copied === 'revealPs' ? '已复制' : '复制 PowerShell 命令'}
                    </Button>
                  </div>
                </div>
              </div>
            )}
            <DialogFooter>
              <Button onClick={() => setRevealKey(null)}>关闭</Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </AdminPageShell>
    </motion.div>
  )
}
