import { lazy, Suspense, useState, useEffect } from 'react'
import { motion, AnimatePresence } from '@/lib/motion'
import { FontLoadCoordinator } from '@/components/font-load-coordinator'
import { ChunkLoadBoundary } from '@/components/ChunkLoadBoundary'
import { InlineLoader } from '@/components/PageLoader'
import { GlobalToastProvider } from '@/components/ui/global-toast'
import { listPublicAnnouncements, type Announcement } from '@/api/announcements'
import { getPublicSiteConfig, type AmpProxySettingsPolicy, type SiteContactConfig } from '@/api/system'
import { DEFAULT_SITE_TIME_ZONE, setActiveSiteTimeZone } from '@/lib/site-config'

const Login = lazy(() => import('./pages/Login'))
const Register = lazy(() => import('./pages/Register'))
const CredentialBootstrap = lazy(() => import('./pages/CredentialBootstrap'))
const Dashboard = lazy(() => import('./pages/Dashboard'))

interface UserState {
  username: string
  isAdmin: boolean
  token: string
  mustChangePassword: boolean
  mustChangeUsername: boolean
}

function App() {
  const [page, setPage] = useState<'login' | 'register'>('login')
  const [user, setUser] = useState<UserState | null>(null)
  const [siteName, setSiteName] = useState('AMP Manager')
  const [siteTimeZone, setSiteTimeZone] = useState(DEFAULT_SITE_TIME_ZONE)
  const [ampProxySettingsPolicy, setAmpProxySettingsPolicy] = useState<AmpProxySettingsPolicy>('all')
  const [ampSettingsPolicy, setAmpSettingsPolicy] = useState<AmpProxySettingsPolicy>('all')
  const [siteContact, setSiteContact] = useState<SiteContactConfig>({
    enabled: false,
    title: '',
    description: '',
    link: '',
    qrCodeImageDataUrl: '',
  })
  const [publicAnnouncements, setPublicAnnouncements] = useState<Announcement[]>([])

  useEffect(() => {
    const token = localStorage.getItem('token')
    const username = localStorage.getItem('username')
    const isAdmin = localStorage.getItem('isAdmin') === 'true'
    const mustChangePassword = localStorage.getItem('mustChangePassword') === 'true'
    const mustChangeUsername = localStorage.getItem('mustChangeUsername') === 'true'
    if (token && username) {
      setUser({ username, isAdmin, token, mustChangePassword, mustChangeUsername })
    }
  }, [])

  useEffect(() => {
    let cancelled = false

    getPublicSiteConfig()
      .then((config) => {
        if (!cancelled) {
          if (config.siteName) {
            setSiteName(config.siteName)
          }
          if (config.timeZone) {
            setSiteTimeZone(config.timeZone)
          }
          setAmpProxySettingsPolicy(config.ampProxySettingsPolicy || 'all')
          setAmpSettingsPolicy(config.ampSettingsPolicy || 'all')
          setSiteContact(config.contact || {
            enabled: false,
            title: '',
            description: '',
            link: '',
            qrCodeImageDataUrl: '',
          })
        }
      })
      .catch(() => {
        // fall back to the default site name
      })

    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    let cancelled = false

    listPublicAnnouncements()
      .then((announcements) => {
        if (!cancelled) {
          setPublicAnnouncements(announcements)
        }
      })
      .catch(() => {
        if (!cancelled) {
          setPublicAnnouncements([])
        }
      })

    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    setActiveSiteTimeZone(siteTimeZone)
  }, [siteTimeZone])

  // 监听 Token 过期事件，自动登出
  useEffect(() => {
    const handleExpired = () => {
      setUser(null)
      setPage('login')
    }
    window.addEventListener('auth:expired', handleExpired)
    return () => window.removeEventListener('auth:expired', handleExpired)
  }, [])

  useEffect(() => {
    if (user) return
    document.title = `${page === 'login' ? '登录' : '注册'} - ${siteName}`
  }, [page, siteName, user])

  const handleSuccess = (
    username: string,
    token?: string,
    isAdmin?: boolean,
    mustChangePassword = false,
    mustChangeUsername = false,
  ) => {
    if (token) {
      localStorage.setItem('token', token)
      localStorage.setItem('username', username)
      localStorage.setItem('isAdmin', String(isAdmin || false))
      localStorage.setItem('mustChangePassword', String(mustChangePassword))
      localStorage.setItem('mustChangeUsername', String(mustChangeUsername))
    }
    setUser({
      username,
      isAdmin: isAdmin || false,
      token: token || localStorage.getItem('token') || '',
      mustChangePassword,
      mustChangeUsername,
    })
  }

  const handleLogout = () => {
    localStorage.removeItem('token')
    localStorage.removeItem('username')
    localStorage.removeItem('isAdmin')
    localStorage.removeItem('mustChangePassword')
    localStorage.removeItem('mustChangeUsername')
    setUser(null)
    setPage('login')
  }

  if (user) {
    return (
      <GlobalToastProvider>
        <FontLoadCoordinator />
        <ChunkLoadBoundary scopeLabel="控制台">
          <Suspense fallback={<InlineLoader />}>
            {user.mustChangePassword || user.mustChangeUsername ? (
              <div className="flex min-h-screen items-center justify-center auth-bg px-4">
                <CredentialBootstrap
                  siteName={siteName}
                  token={user.token}
                  username={user.username}
                  mustChangePassword={user.mustChangePassword}
                  mustChangeUsername={user.mustChangeUsername}
                  onSuccess={(next) => handleSuccess(
                    next.username,
                    next.token || user.token,
                    user.isAdmin,
                    next.mustChangePassword || false,
                    next.mustChangeUsername || false,
                  )}
                  onLogout={handleLogout}
                />
              </div>
            ) : (
              <Dashboard
                username={user.username}
                isAdmin={user.isAdmin}
                siteName={siteName}
                siteTimeZone={siteTimeZone}
                ampProxySettingsPolicy={ampProxySettingsPolicy}
                ampSettingsPolicy={ampSettingsPolicy}
                siteContact={siteContact}
                onSiteNameChange={setSiteName}
                onSiteTimeZoneChange={setSiteTimeZone}
                onAmpProxySettingsPolicyChange={setAmpProxySettingsPolicy}
                onAmpSettingsPolicyChange={setAmpSettingsPolicy}
                onSiteContactChange={setSiteContact}
                onLogout={handleLogout}
              />
            )}
          </Suspense>
        </ChunkLoadBoundary>
      </GlobalToastProvider>
    )
  }

  return (
    <GlobalToastProvider>
      <FontLoadCoordinator />
      <div className="flex min-h-screen items-center justify-center auth-bg">
        <AnimatePresence mode="wait">
          <motion.div
            key={page}
            initial={{ opacity: 0, scale: 0.85, y: 40 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.85, y: -40 }}
            transition={{ type: 'spring', bounce: 0.25, duration: 0.6 }}
          >
            <ChunkLoadBoundary scopeLabel={page === 'login' ? '登录页' : '注册页'}>
              <Suspense fallback={<InlineLoader />}>
                {page === 'login' ? (
                  <Login
                    siteName={siteName}
                    siteContact={siteContact}
                    announcements={publicAnnouncements}
                    onSwitch={() => setPage('register')}
                    onSuccess={handleSuccess}
                  />
                ) : (
                  <Register
                    siteName={siteName}
                    siteContact={siteContact}
                    announcements={publicAnnouncements}
                    onSwitch={() => setPage('login')}
                    onSuccess={handleSuccess}
                  />
                )}
              </Suspense>
            </ChunkLoadBoundary>
          </motion.div>
        </AnimatePresence>
      </div>
    </GlobalToastProvider>
  )
}

export default App
