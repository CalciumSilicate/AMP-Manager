import { FormEvent, useState } from 'react'

import { completeBootstrapCredentials } from '@/api/auth'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { KeyRound, UserRound } from 'lucide-react'

interface Props {
  siteName: string
  token: string
  username: string
  mustChangePassword: boolean
  mustChangeUsername: boolean
  onSuccess: (next: { username: string; token?: string; mustChangePassword?: boolean; mustChangeUsername?: boolean }) => void
  onLogout: () => void
}

export default function CredentialBootstrap({
  siteName,
  token,
  username,
  mustChangePassword,
  mustChangeUsername,
  onSuccess,
  onLogout,
}: Props) {
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [newUsername, setNewUsername] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault()
    setError('')

    if (newPassword !== confirmPassword) {
      setError('两次输入的新密码不一致')
      return
    }
    if (mustChangeUsername && !newUsername.trim()) {
      setError('请输入新的用户名')
      return
    }

    setLoading(true)
    try {
      const response = await completeBootstrapCredentials(token, {
        currentPassword,
        newPassword,
        newUsername: mustChangeUsername ? newUsername.trim() : username,
      })
      onSuccess({
        username: response.username,
        token: response.token,
        mustChangePassword: response.mustChangePassword,
        mustChangeUsername: response.mustChangeUsername,
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : '更新失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Card className="w-full max-w-lg border-0 shadow-xl glass-card">
      <CardHeader className="space-y-2">
        <CardTitle>首次登录设置</CardTitle>
        <CardDescription>
          {siteName} 检测到当前账号是导入账号，需先完成改密和改名后才能继续使用。
        </CardDescription>
      </CardHeader>
      <CardContent>
        {error ? (
          <Alert variant="destructive" className="mb-4">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="rounded-lg border border-border/70 px-4 py-3 text-sm">
            <div className="text-muted-foreground">当前登录名</div>
            <div className="mt-1 font-mono text-foreground">{username}</div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="currentPassword">当前密码</Label>
            <Input
              id="currentPassword"
              type="password"
              required
              value={currentPassword}
              onChange={(event) => setCurrentPassword(event.target.value)}
              placeholder="导入默认密码或当前密码"
            />
          </div>

          {mustChangeUsername ? (
            <div className="space-y-2">
              <Label htmlFor="newUsername">新用户名</Label>
              <Input
                id="newUsername"
                value={newUsername}
                onChange={(event) => setNewUsername(event.target.value)}
                minLength={3}
                maxLength={32}
                required
                placeholder="输入新的用户名"
              />
            </div>
          ) : null}

          <div className="space-y-2">
            <Label htmlFor="newPassword">新密码</Label>
            <Input
              id="newPassword"
              type="password"
              required
              minLength={6}
              maxLength={128}
              value={newPassword}
              onChange={(event) => setNewPassword(event.target.value)}
              placeholder="输入新的密码"
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="confirmPassword">确认新密码</Label>
            <Input
              id="confirmPassword"
              type="password"
              required
              minLength={6}
              maxLength={128}
              value={confirmPassword}
              onChange={(event) => setConfirmPassword(event.target.value)}
              placeholder="再次输入新的密码"
            />
          </div>

          <div className="grid gap-2 rounded-lg border border-border/70 px-4 py-3 text-sm text-muted-foreground">
            <div className="flex items-center gap-2">
              <UserRound className="h-4 w-4" />
              {mustChangeUsername ? '本次必须修改用户名' : '用户名已满足要求'}
            </div>
            <div className="flex items-center gap-2">
              <KeyRound className="h-4 w-4" />
              {mustChangePassword ? '本次必须修改密码' : '密码已满足要求'}
            </div>
          </div>

          <div className="flex gap-3">
            <Button type="submit" disabled={loading} className="flex-1">
              {loading ? '保存中...' : '保存并进入控制台'}
            </Button>
            <Button type="button" variant="outline" onClick={onLogout} disabled={loading}>
              退出
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}
