import { useState, FormEvent } from 'react'
import { register, RegisterRequest } from '../api/auth'
import type { Announcement } from '@/api/announcements'
import type { SiteContactConfig } from '@/api/system'
import { AnnouncementCenter, ContactCenter } from '@/components/announcements/AnnouncementCenter'
import { motion } from '@/lib/motion'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  CardFooter,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { UserPlus } from 'lucide-react'

interface Props {
  siteName: string
  siteContact: SiteContactConfig
  announcements: Announcement[]
  onSwitch: () => void
  onSuccess: (username: string, token?: string, isAdmin?: boolean) => void
}

export default function Register({ siteName, siteContact, announcements, onSwitch, onSuccess }: Props) {
  const [formData, setFormData] = useState<RegisterRequest>({
    username: '',
    password: '',
  })
  const [confirmPassword, setConfirmPassword] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')

    if (formData.password !== confirmPassword) {
      setError('两次输入的密码不一致')
      return
    }

    setLoading(true)
    try {
      const result = await register(formData)
      onSuccess(result.username, result.token)
    } catch (err) {
      setError(err instanceof Error ? err.message : '注册失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <motion.div
      initial={{ opacity: 0, y: 40, scale: 0.9 }}
      animate={{ opacity: 1, y: 0, scale: 1 }}
      transition={{ type: 'spring', bounce: 0.25, duration: 0.7 }}
    >
      <Card className="w-full max-w-md glass-card border-0 shadow-xl">
        <CardHeader className="text-center pb-2">
          <motion.div
            className="mx-auto mb-3 flex h-14 w-14 items-center justify-center rounded-2xl bg-primary/10"
            initial={{ scale: 0, rotate: -180 }}
            animate={{ scale: 1, rotate: 0 }}
            transition={{ type: 'spring', bounce: 0.4, duration: 0.8, delay: 0.2 }}
          >
            <UserPlus className="h-7 w-7 text-primary" />
          </motion.div>
          <CardTitle className="text-2xl font-bold">创建账号</CardTitle>
          <CardDescription>注册 {siteName}</CardDescription>
          <div className="flex justify-center gap-2 pt-2">
            <AnnouncementCenter
              announcements={announcements}
              triggerLabel="公开公告"
              triggerVariant="secondary"
              emptyText="当前没有公开公告"
              description="以下为未登录也可查看的公开公告。"
            />
            <ContactCenter contact={siteContact} triggerVariant="secondary" />
          </div>
        </CardHeader>

        <CardContent>
          {error && (
            <motion.div
              initial={{ opacity: 0, height: 0 }}
              animate={{ opacity: 1, height: 'auto' }}
              transition={{ type: 'spring', bounce: 0.2, duration: 0.4 }}
            >
              <Alert variant="destructive" className="mb-4">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            </motion.div>
          )}

          <form onSubmit={handleSubmit}>
            <motion.div
              className="space-y-4"
              initial="hidden"
              animate="visible"
              variants={{
                hidden: { opacity: 0 },
                visible: { opacity: 1, transition: { staggerChildren: 0.1, delayChildren: 0.25 } },
              }}
            >
              <motion.div
                className="space-y-2"
                variants={{
                  hidden: { opacity: 0, y: 16 },
                  visible: { opacity: 1, y: 0, transition: { type: 'spring', bounce: 0.25, duration: 0.5 } },
                }}
              >
                <Label htmlFor="username">用户名</Label>
                <Input
                  id="username"
                  type="text"
                  required
                  minLength={3}
                  maxLength={32}
                  value={formData.username}
                  onChange={(e) =>
                    setFormData({ ...formData, username: e.target.value })
                  }
                  placeholder="请输入用户名"
                  className="h-11 bg-white/50 dark:bg-white/5"
                />
              </motion.div>

              <motion.div
                className="space-y-2"
                variants={{
                  hidden: { opacity: 0, y: 16 },
                  visible: { opacity: 1, y: 0, transition: { type: 'spring', bounce: 0.25, duration: 0.5 } },
                }}
              >
                <Label htmlFor="password">密码</Label>
                <Input
                  id="password"
                  type="password"
                  required
                  minLength={6}
                  maxLength={128}
                  value={formData.password}
                  onChange={(e) =>
                    setFormData({ ...formData, password: e.target.value })
                  }
                  placeholder="请输入密码（至少6位）"
                  className="h-11 bg-white/50 dark:bg-white/5"
                />
              </motion.div>

              <motion.div
                className="space-y-2"
                variants={{
                  hidden: { opacity: 0, y: 16 },
                  visible: { opacity: 1, y: 0, transition: { type: 'spring', bounce: 0.25, duration: 0.5 } },
                }}
              >
                <Label htmlFor="confirmPassword">确认密码</Label>
                <Input
                  id="confirmPassword"
                  type="password"
                  required
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  placeholder="请再次输入密码"
                  className="h-11 bg-white/50 dark:bg-white/5"
                />
              </motion.div>

              <motion.div
                variants={{
                  hidden: { opacity: 0, y: 16 },
                  visible: { opacity: 1, y: 0, transition: { type: 'spring', bounce: 0.25, duration: 0.5 } },
                }}
              >
                <Button type="submit" disabled={loading} className="w-full h-11 text-base font-medium">
                  {loading ? (
                    <motion.div
                      className="h-5 w-5 rounded-full border-2 border-primary-foreground/30 border-t-primary-foreground"
                      animate={{ rotate: 360 }}
                      transition={{ duration: 0.8, repeat: Infinity, ease: 'linear' }}
                    />
                  ) : '注册'}
                </Button>
              </motion.div>
            </motion.div>
          </form>
        </CardContent>

        <CardFooter className="justify-center pb-6">
          <p className="text-sm text-muted-foreground">
            已有账号？{' '}
            <Button variant="link" className="h-auto p-0 text-primary font-medium" onClick={onSwitch}>
              立即登录
            </Button>
          </p>
        </CardFooter>
      </Card>
    </motion.div>
  )
}
