import { useState, FormEvent } from 'react'
import { login, LoginRequest } from '../api/auth'
import type { Announcement } from '@/api/announcements'
import { AnnouncementCenter } from '@/components/announcements/AnnouncementCenter'
import { motion } from '@/lib/motion'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Separator } from '@/components/ui/separator'
import { LogIn, EyeIcon, EyeOffIcon, ShieldCheck, Zap } from 'lucide-react'

interface Props {
  siteName: string
  announcements: Announcement[]
  onSwitch: () => void
  onSuccess: (username: string, token?: string, isAdmin?: boolean) => void
}

export default function Login({ siteName, announcements, onSwitch, onSuccess }: Props) {
  const [formData, setFormData] = useState<LoginRequest>({
    username: '',
    password: '',
  })
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [showPassword, setShowPassword] = useState(false)

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')

    setLoading(true)
    try {
      const result = await login(formData)
      onSuccess(result.username, result.token, result.isAdmin)
    } catch (err) {
      setError(err instanceof Error ? err.message : '登录失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <motion.div
      initial={{ opacity: 0, y: 40, scale: 0.9 }}
      animate={{ opacity: 1, y: 0, scale: 1 }}
      transition={{ type: 'spring', bounce: 0.25, duration: 0.7 }}
      className="relative w-[33.6rem] max-w-[90vw]"
    >
      {/* Decorative background shape */}
      <div className="pointer-events-none absolute -inset-20 -z-10 opacity-30">
        <svg viewBox="0 0 686 671" fill="none" xmlns="http://www.w3.org/2000/svg" className="h-full w-full">
          <path
            d="M442.137 102.265C482.171 64.0112 545.636 65.454 583.89 105.488C622.145 145.522 620.702 208.987 580.668 247.242L242.62 570.26C202.586 608.514 139.121 607.071 100.867 567.037C62.6126 527.003 64.0555 463.538 104.09 425.283L442.137 102.265Z"
            fill="hsl(var(--primary))"
            fillOpacity="0.1"
          />
          <path
            d="M583.529 105.834C621.592 145.668 620.157 208.817 580.322 246.88L242.275 569.898C202.44 607.962 139.292 606.526 101.228 566.691C63.1649 526.857 64.6005 463.708 104.435 425.645L442.482 102.627C482.317 64.5634 545.465 65.9991 583.529 105.834Z"
            stroke="hsl(var(--primary))"
            strokeOpacity="0.15"
          />
        </svg>
      </div>

      <Card className="w-full max-w-3xl border-0 shadow-xl glass-card">
        <CardHeader className="gap-4 pb-2">
          {/* Logo */}
          <motion.div
            className="flex items-center gap-3"
            initial={{ opacity: 0, x: -20 }}
            animate={{ opacity: 1, x: 0 }}
            transition={{ type: 'spring', bounce: 0.3, duration: 0.6, delay: 0.1 }}
          >
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-primary text-primary-foreground">
              <ShieldCheck className="h-5 w-5" />
            </div>
            <span className="text-lg font-semibold">{siteName}</span>
          </motion.div>

          <div>
            <CardTitle className="mb-1.5 text-2xl font-bold">欢迎回来</CardTitle>
            <CardDescription className="text-base">登录到 {siteName}</CardDescription>
          </div>
          <div className="flex justify-start">
            <AnnouncementCenter
              announcements={announcements}
              triggerLabel="查看公告"
              triggerVariant="secondary"
              emptyText="当前没有公开公告"
              description="以下为未登录也可查看的公开公告。"
            />
          </div>
        </CardHeader>

        <CardContent className="pt-2">
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

          {/* Feature badges */}
          <motion.div
            className="mb-5 flex flex-wrap gap-2"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ delay: 0.3, duration: 0.4 }}
          >
            <div className="flex items-center gap-1.5 rounded-full bg-primary/10 px-3 py-1 text-xs font-medium text-primary">
              <Zap className="h-3 w-3" />
              快速部署
            </div>
            <div className="flex items-center gap-1.5 rounded-full bg-primary/10 px-3 py-1 text-xs font-medium text-primary">
              <ShieldCheck className="h-3 w-3" />
              安全管理
            </div>
          </motion.div>

          <form onSubmit={handleSubmit}>
            <motion.div
              className="space-y-4"
              initial="hidden"
              animate="visible"
              variants={{
                hidden: { opacity: 0 },
                visible: { opacity: 1, transition: { staggerChildren: 0.1, delayChildren: 0.2 } },
              }}
            >
              {/* Username */}
              <motion.div
                className="space-y-1.5"
                variants={{
                  hidden: { opacity: 0, y: 16 },
                  visible: { opacity: 1, y: 0, transition: { type: 'spring', bounce: 0.25, duration: 0.5 } },
                }}
              >
                <Label htmlFor="username" className="leading-5">用户名</Label>
                <Input
                  id="username"
                  type="text"
                  required
                  value={formData.username}
                  onChange={(e) =>
                    setFormData({ ...formData, username: e.target.value })
                  }
                  placeholder="请输入用户名"
                  className="h-11 bg-white/50 dark:bg-white/5"
                />
              </motion.div>

              {/* Password with toggle */}
              <motion.div
                className="space-y-1.5"
                variants={{
                  hidden: { opacity: 0, y: 16 },
                  visible: { opacity: 1, y: 0, transition: { type: 'spring', bounce: 0.25, duration: 0.5 } },
                }}
              >
                <Label htmlFor="password" className="leading-5">密码</Label>
                <div className="relative">
                  <Input
                    id="password"
                    type={showPassword ? 'text' : 'password'}
                    required
                    value={formData.password}
                    onChange={(e) =>
                      setFormData({ ...formData, password: e.target.value })
                    }
                    placeholder="请输入密码"
                    className="h-11 pr-10 bg-white/50 dark:bg-white/5"
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    onClick={() => setShowPassword(prev => !prev)}
                    className="absolute inset-y-0 right-0 rounded-l-none text-muted-foreground hover:bg-transparent"
                  >
                    {showPassword ? <EyeOffIcon className="h-4 w-4" /> : <EyeIcon className="h-4 w-4" />}
                    <span className="sr-only">{showPassword ? '隐藏密码' : '显示密码'}</span>
                  </Button>
                </div>
              </motion.div>

              {/* Submit */}
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
                  ) : (
                    <>
                      <LogIn className="mr-2 h-4 w-4" />
                      登录
                    </>
                  )}
                </Button>
              </motion.div>

              {/* Divider */}
              <motion.div
                className="flex items-center gap-4"
                variants={{
                  hidden: { opacity: 0 },
                  visible: { opacity: 1, transition: { duration: 0.4 } },
                }}
              >
                <Separator className="flex-1" />
                <span className="text-sm text-muted-foreground">或</span>
                <Separator className="flex-1" />
              </motion.div>

              {/* Register link */}
              <motion.p
                className="text-center text-sm text-muted-foreground"
                variants={{
                  hidden: { opacity: 0 },
                  visible: { opacity: 1, transition: { duration: 0.4 } },
                }}
              >
                没有账号？{' '}
                <Button variant="link" className="h-auto p-0 text-primary font-medium" onClick={onSwitch}>
                  立即注册
                </Button>
              </motion.p>
            </motion.div>
          </form>
        </CardContent>
      </Card>
    </motion.div>
  )
}
