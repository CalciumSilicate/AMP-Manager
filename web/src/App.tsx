import { useState, useEffect } from 'react'
import { motion, AnimatePresence } from '@/lib/motion'
import { FontLoadCoordinator } from '@/components/font-load-coordinator'
import { listPublicAnnouncements, type Announcement } from '@/api/announcements'
import { getPublicSiteConfig } from '@/api/system'
import { DEFAULT_SITE_TIME_ZONE, setActiveSiteTimeZone } from '@/lib/site-config'
import Login from './pages/Login'
import Register from './pages/Register'
import Dashboard from './pages/Dashboard'

interface UserState {
  username: string
  isAdmin: boolean
}

function App() {
  const [page, setPage] = useState<'login' | 'register'>('login')
  const [user, setUser] = useState<UserState | null>(null)
  const [siteName, setSiteName] = useState('AMP Manager')
  const [siteTimeZone, setSiteTimeZone] = useState(DEFAULT_SITE_TIME_ZONE)
  const [allowAmpProxySettings, setAllowAmpProxySettings] = useState(true)
  const [publicAnnouncements, setPublicAnnouncements] = useState<Announcement[]>([])

  useEffect(() => {
    const token = localStorage.getItem('token')
    const username = localStorage.getItem('username')
    const isAdmin = localStorage.getItem('isAdmin') === 'true'
    if (token && username) {
      setUser({ username, isAdmin })
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
          setAllowAmpProxySettings(config.allowAmpProxySettings !== false)
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

  const handleSuccess = (username: string, token?: string, isAdmin?: boolean) => {
    if (token) {
      localStorage.setItem('token', token)
      localStorage.setItem('username', username)
      localStorage.setItem('isAdmin', String(isAdmin || false))
    }
    setUser({ username, isAdmin: isAdmin || false })
  }

  const handleLogout = () => {
    localStorage.removeItem('token')
    localStorage.removeItem('username')
    localStorage.removeItem('isAdmin')
    setUser(null)
    setPage('login')
  }

  if (user) {
    return (
      <>
        <FontLoadCoordinator />
        <Dashboard
          username={user.username}
          isAdmin={user.isAdmin}
          siteName={siteName}
          siteTimeZone={siteTimeZone}
          allowAmpProxySettings={allowAmpProxySettings}
          onSiteNameChange={setSiteName}
          onSiteTimeZoneChange={setSiteTimeZone}
          onAllowAmpProxySettingsChange={setAllowAmpProxySettings}
          onLogout={handleLogout}
        />
      </>
    )
  }

  return (
    <>
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
            {page === 'login' ? (
              <Login
                siteName={siteName}
                announcements={publicAnnouncements}
                onSwitch={() => setPage('register')}
                onSuccess={handleSuccess}
              />
            ) : (
              <Register
                siteName={siteName}
                announcements={publicAnnouncements}
                onSwitch={() => setPage('login')}
                onSuccess={handleSuccess}
              />
            )}
          </motion.div>
        </AnimatePresence>
      </div>
    </>
  )
}

export default App
