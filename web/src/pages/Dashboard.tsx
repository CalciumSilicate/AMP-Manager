import { lazy, Suspense, useEffect, useMemo, useState } from 'react'
import { listMyAnnouncements, markAnnouncementRead, type Announcement } from '@/api/announcements'
import { AnnouncementCenter, AnnouncementUnreadDialog, ContactCenter } from '@/components/announcements/AnnouncementCenter'
import { PageLoader } from '@/components/PageLoader'
import { motion, AnimatePresence } from '@/lib/motion'
import type { AmpProxySettingsPolicy, SiteContactConfig } from '@/api/system'
import { Button } from '@/components/ui/button'
// Card components available if needed by child pages
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { DASHBOARD_NAVIGATE_EVENT } from '@/lib/dashboard-navigation'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from '@/components/ui/dropdown-menu'
import {
  Settings,
  Key,
  ScrollText,
  Activity,
  BarChart3,
  Layers,
  Database,
  Tag,
  DollarSign,
  Users,
  Wrench,
  UserCircle,
  LogOut,
  ChevronLeft,
  ChevronRight,
  PanelLeft,
  Zap,
  FolderOpen,
  LayoutDashboard,
  CreditCard,
  ShoppingCart,
  TicketPercent,
  Link2,
} from 'lucide-react'

const Overview = lazy(() => import('./Overview'))
const AdminOverview = lazy(() => import('./AdminOverview'))
const AmpSettings = lazy(() => import('./AmpSettings'))
const APIKeys = lazy(() => import('./APIKeys'))
const RequestLogs = lazy(() => import('./RequestLogs'))
const UsageStats = lazy(() => import('./UsageStats'))
const Channels = lazy(() => import('./Channels'))
const Models = lazy(() => import('./Models'))
const ModelMetadata = lazy(() => import('./ModelMetadata'))
const Prices = lazy(() => import('./Prices'))
const SystemSettings = lazy(() => import('./SystemSettings'))
const Groups = lazy(() => import('./Groups'))
const SubscriptionPlans = lazy(() => import('./SubscriptionPlans'))
const UserManagement = lazy(() => import('./UserManagement'))
const AccountSettings = lazy(() => import('./AccountSettings'))
const PurchaseCenter = lazy(() => import('./PurchaseCenter'))
const PaidSubscriptions = lazy(() => import('./PaidSubscriptions'))
const RedeemManagement = lazy(() => import('./RedeemManagement'))
const StatusMonitor = lazy(() => import('./StatusMonitor'))
const SessionManagement = lazy(() => import('./SessionManagement'))

interface Props {
  username: string
  isAdmin: boolean
  siteName: string
  siteTimeZone: string
  ampProxySettingsPolicy: AmpProxySettingsPolicy
  ampSettingsPolicy: AmpProxySettingsPolicy
  siteContact: SiteContactConfig
  onSiteNameChange: (siteName: string) => void
  onSiteTimeZoneChange: (timeZone: string) => void
  onAmpProxySettingsPolicyChange: (policy: AmpProxySettingsPolicy) => void
  onAmpSettingsPolicyChange: (policy: AmpProxySettingsPolicy) => void
  onSiteContactChange: (contact: SiteContactConfig) => void
  onLogout: () => void
}

type Page = 'overview' | 'status-monitor' | 'amp-settings' | 'api-keys' | 'request-logs' | 'usage-stats' | 'channels' | 'models' | 'model-metadata' | 'prices' | 'system-settings' | 'user-management' | 'account-settings' | 'groups' | 'subscription-plans' | 'admin-overview' | 'purchase-center' | 'paid-subscriptions' | 'redeem-management' | 'session-management'

const navIcons: Record<Page, React.ElementType> = {
  'overview': LayoutDashboard,
  'status-monitor': Activity,
  'admin-overview': BarChart3,
  'amp-settings': Settings,
  'api-keys': Key,
  'request-logs': ScrollText,
  'usage-stats': BarChart3,
  'models': Layers,
  'account-settings': UserCircle,
  'channels': Database,
  'model-metadata': Tag,
  'prices': DollarSign,
  'user-management': Users,
  'system-settings': Wrench,
  'groups': FolderOpen,
  'subscription-plans': CreditCard,
  'purchase-center': ShoppingCart,
  'paid-subscriptions': CreditCard,
  'redeem-management': TicketPercent,
  'session-management': Link2,
}

const DASHBOARD_PAGES = new Set<Page>(Object.keys(navIcons) as Page[])
const DASHBOARD_PAGE_STORAGE_KEY = 'dashboard:last-page'

function isDashboardPage(value: string | null): value is Page {
  return value !== null && DASHBOARD_PAGES.has(value as Page)
}

function getDashboardPageStorageKey(username: string, isAdmin: boolean) {
  return `${DASHBOARD_PAGE_STORAGE_KEY}:${isAdmin ? 'admin' : 'user'}:${username}`
}

function readStoredDashboardPage(username: string, isAdmin: boolean): Page | null {
  if (typeof window === 'undefined') return null

  const value = window.localStorage.getItem(getDashboardPageStorageKey(username, isAdmin))
  return isDashboardPage(value) ? value : null
}

export default function Dashboard({
  username: initialUsername,
  isAdmin,
  siteName,
  siteTimeZone,
  ampProxySettingsPolicy,
  ampSettingsPolicy,
  siteContact,
  onSiteNameChange,
  onSiteTimeZoneChange,
  onAmpProxySettingsPolicyChange,
  onAmpSettingsPolicyChange,
  onSiteContactChange,
  onLogout,
}: Props) {
  const [currentPage, setCurrentPage] = useState<Page>(() => readStoredDashboardPage(initialUsername, isAdmin) ?? 'overview')
  const [username, setUsername] = useState(initialUsername)
  const [collapsed, setCollapsed] = useState(false)
  const [mobileNavOpen, setMobileNavOpen] = useState(false)
  const [isMobileViewport, setIsMobileViewport] = useState(false)
  const [announcements, setAnnouncements] = useState<Announcement[]>([])
  const [announcementBusyId, setAnnouncementBusyId] = useState<string | null>(null)
  const [announcementBusyAll, setAnnouncementBusyAll] = useState(false)
  const [showUnreadDialog, setShowUnreadDialog] = useState(false)

  const canAccessAmpRouteSettings = ampProxySettingsPolicy === 'all' || (ampProxySettingsPolicy === 'admin_only' && isAdmin)
  const canAccessAmpUpstreamSettings = ampSettingsPolicy === 'all' || (ampSettingsPolicy === 'admin_only' && isAdmin)
  const canAccessAmpSettings = canAccessAmpRouteSettings || canAccessAmpUpstreamSettings

  const navItems: { key: Page; label: string; adminOnly?: boolean }[] = [
    { key: 'overview', label: '概览' },
    { key: 'status-monitor', label: '状态监控' },
    ...(canAccessAmpSettings ? [{ key: 'amp-settings' as const, label: '路由设置' }] : []),
    { key: 'api-keys', label: 'API Key 管理' },
    { key: 'request-logs', label: '请求日志' },
    { key: 'usage-stats', label: '使用量统计' },
    { key: 'models', label: '可用模型' },
    { key: 'account-settings', label: '账户设置' },
    { key: 'purchase-center', label: '购买订阅' },
    { key: 'admin-overview', label: '管理概览', adminOnly: true },
    { key: 'groups', label: '分组管理', adminOnly: true },
    { key: 'subscription-plans', label: '订阅套餐', adminOnly: true },
    { key: 'paid-subscriptions', label: '付费订阅', adminOnly: true },
    { key: 'redeem-management', label: '兑换码管理', adminOnly: true },
    { key: 'session-management', label: 'Session 管理', adminOnly: true },
    { key: 'channels', label: '渠道管理', adminOnly: true },
    { key: 'model-metadata', label: '模型元数据', adminOnly: true },
    { key: 'prices', label: '模型价格', adminOnly: true },
    { key: 'user-management', label: '用户管理', adminOnly: true },
    { key: 'system-settings', label: '系统设置', adminOnly: true },
  ]

  const visibleNavItems = navItems.filter(item => !item.adminOnly || isAdmin)
  const userNavItems = visibleNavItems.filter(item => !item.adminOnly)
  const adminNavItems = visibleNavItems.filter(item => item.adminOnly)
  const currentPageLabel = visibleNavItems.find((item) => item.key === currentPage)?.label || visibleNavItems[0]?.label || '概览'

  useEffect(() => {
    if (visibleNavItems.some((item) => item.key === currentPage)) return
    setCurrentPage(visibleNavItems[0]?.key || 'overview')
  }, [currentPage, visibleNavItems])

  useEffect(() => {
    if (typeof window === 'undefined') return
    if (!visibleNavItems.some((item) => item.key === currentPage)) return

    window.localStorage.setItem(getDashboardPageStorageKey(initialUsername, isAdmin), currentPage)
  }, [currentPage, initialUsername, isAdmin, visibleNavItems])

  useEffect(() => {
    document.title = `${currentPageLabel} - ${siteName}`
  }, [currentPageLabel, siteName])

  useEffect(() => {
    if (typeof window === 'undefined') return

    const media = window.matchMedia('(max-width: 767px)')
    const syncViewport = () => {
      setIsMobileViewport(media.matches)
      if (!media.matches) {
        setMobileNavOpen(false)
      }
    }

    syncViewport()
    media.addEventListener('change', syncViewport)
    return () => media.removeEventListener('change', syncViewport)
  }, [])

  useEffect(() => {
    const handleNavigate = (event: Event) => {
      const detail = (event as CustomEvent<{ page?: string }>).detail
      const nextPage = detail?.page
      if (!nextPage) return

      const target = visibleNavItems.find((item) => item.key === nextPage)
      if (target) {
        setCurrentPage(target.key)
      }
    }

    window.addEventListener(DASHBOARD_NAVIGATE_EVENT, handleNavigate)
    return () => window.removeEventListener(DASHBOARD_NAVIGATE_EVENT, handleNavigate)
  }, [visibleNavItems])

  useEffect(() => {
    if (!canAccessAmpSettings && currentPage === 'amp-settings') {
      setCurrentPage('overview')
    }
  }, [canAccessAmpSettings, currentPage])

  useEffect(() => {
    let cancelled = false

    listMyAnnouncements()
      .then((items) => {
        if (!cancelled) {
          setAnnouncements(items)
        }
      })
      .catch((error) => {
        console.error('获取公告失败:', error)
      })

    return () => {
      cancelled = true
    }
  }, [])

  const unreadAnnouncements = useMemo(
    () => announcements.filter((announcement) => !announcement.isRead),
    [announcements],
  )

  useEffect(() => {
    if (unreadAnnouncements.length > 0) {
      setShowUnreadDialog(true)
    }
  }, [unreadAnnouncements.length])

  const handleMarkRead = async (announcement: Announcement) => {
    setAnnouncementBusyId(announcement.id)
    try {
      await markAnnouncementRead(announcement.id)
      setAnnouncements((prev) =>
        prev.map((item) =>
          item.id === announcement.id
            ? { ...item, isRead: true, readAt: new Date().toISOString() }
            : item,
        ),
      )
    } catch (error) {
      console.error('标记公告已读失败:', error)
    } finally {
      setAnnouncementBusyId(null)
    }
  }

  const handleMarkAllRead = async () => {
    if (unreadAnnouncements.length === 0) return

    setAnnouncementBusyAll(true)
    try {
      await Promise.all(unreadAnnouncements.map((announcement) => markAnnouncementRead(announcement.id)))
      const readAt = new Date().toISOString()
      setAnnouncements((prev) => prev.map((item) => (item.isRead ? item : { ...item, isRead: true, readAt })))
      setShowUnreadDialog(false)
    } catch (error) {
      console.error('批量标记公告已读失败:', error)
    } finally {
      setAnnouncementBusyAll(false)
    }
  }

  const renderNavItem = (
    item: { key: Page; label: string; adminOnly?: boolean },
    opts?: { collapsed?: boolean; onSelect?: (page: Page) => void },
  ) => {
    const Icon = navIcons[item.key]
    const isActive = currentPage === item.key
    const collapsedState = opts?.collapsed ?? collapsed

    const button = (
      <button
        key={item.key}
        onClick={() => {
          setCurrentPage(item.key)
          opts?.onSelect?.(item.key)
        }}
        className={`
          flex w-full items-center rounded-lg py-2.5 text-left text-sm font-medium transition-all duration-200
          ${isActive
            ? 'bg-primary text-primary-foreground shadow-md shadow-primary/20'
            : 'text-muted-foreground hover:bg-muted hover:text-foreground'
          }
          ${collapsedState ? 'justify-center px-2.5' : 'gap-3 px-3'}
        `}
      >
        <Icon className="h-4 w-4 shrink-0" />
        {!collapsedState ? (
          <>
            <span className="min-w-0 flex-1 truncate">{item.label}</span>
            {item.adminOnly ? (
              <Badge variant={isActive ? 'secondary' : 'outline'} className="px-1.5 py-0 text-[10px]">
                管理
              </Badge>
            ) : null}
          </>
        ) : null}
      </button>
    )

    if (collapsedState) {
      return (
        <Tooltip key={item.key}>
          <TooltipTrigger asChild>
            {button}
          </TooltipTrigger>
          <TooltipContent side="right" sideOffset={8}>
            <p>{item.label}</p>
          </TooltipContent>
        </Tooltip>
      )
    }

    return button
  }

  const renderSidebarNav = (opts?: { collapsed?: boolean; onSelect?: (page: Page) => void }) => {
    const collapsedState = opts?.collapsed ?? collapsed
    return (
      <nav className="flex flex-col gap-1">
        {userNavItems.map((item) => renderNavItem(item, opts))}

        {adminNavItems.length > 0 && (
          <>
            {!collapsedState ? (
              <div className="my-3 flex items-center gap-2 px-3">
                <Separator className="flex-1" />
                <span className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">管理</span>
                <Separator className="flex-1" />
              </div>
            ) : (
              <Separator className="my-3" />
            )}
            {adminNavItems.map((item) => renderNavItem(item, opts))}
          </>
        )}
      </nav>
    )
  }

  return (
    <TooltipProvider delayDuration={0}>
      <div className="flex h-screen overflow-hidden bg-muted/30">
        {/* Sidebar */}
        <motion.aside
          className="relative hidden h-full shrink-0 flex-col border-r bg-background/80 backdrop-blur-sm md:flex"
          animate={{ width: collapsed ? 68 : 256 }}
          transition={{ type: 'spring', bounce: 0.15, duration: 0.4 }}
        >
          {/* Logo */}
          <div className="flex h-16 items-center border-b px-4">
            <motion.div
              className={`grid items-center gap-3 overflow-hidden ${collapsed ? 'grid-cols-[36px_0px]' : 'grid-cols-[36px_minmax(0,1fr)]'}`}
              initial={{ opacity: 0, x: -20 }}
              animate={{ opacity: 1, x: 0 }}
              transition={{ type: 'spring', bounce: 0.3, duration: 0.6 }}
            >
              <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-md shadow-primary/25">
                <Zap className="h-5 w-5" />
              </div>
              <div
                className={`
                  min-w-0 overflow-hidden whitespace-nowrap text-lg font-bold tracking-tight
                  transition-opacity duration-200
                  ${collapsed ? 'opacity-0' : 'opacity-100'}
                `}
              >
                {siteName}
              </div>
            </motion.div>
          </div>

          {/* Nav */}
          <div className="flex-1 overflow-y-auto px-3 py-4">
            {renderSidebarNav()}
          </div>

          {/* Collapse toggle */}
          <div className="border-t p-3">
            <Button
              variant="ghost"
              size="sm"
              className="w-full justify-center"
              onClick={() => setCollapsed(!collapsed)}
            >
              {collapsed ? <ChevronRight className="h-4 w-4" /> : <ChevronLeft className="h-4 w-4" />}
            </Button>
          </div>
        </motion.aside>

        <AnimatePresence>
          {isMobileViewport && mobileNavOpen ? (
            <>
              <motion.button
                type="button"
                className="fixed inset-0 z-40 bg-black/30 md:hidden"
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                exit={{ opacity: 0 }}
                onClick={() => setMobileNavOpen(false)}
                aria-label="关闭导航"
              />
              <motion.aside
                className="fixed inset-y-0 left-0 z-50 flex w-[288px] max-w-[86vw] flex-col border-r bg-background/95 backdrop-blur-sm md:hidden"
                initial={{ x: -320 }}
                animate={{ x: 0 }}
                exit={{ x: -320 }}
                transition={{ type: 'spring', bounce: 0.1, duration: 0.35 }}
              >
                <div className="flex h-16 items-center border-b px-4">
                  <div className="flex items-center gap-3">
                    <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-md shadow-primary/25">
                      <Zap className="h-5 w-5" />
                    </div>
                    <div className="min-w-0 overflow-hidden whitespace-nowrap text-lg font-bold tracking-tight">
                      {siteName}
                    </div>
                  </div>
                </div>
                <div className="flex-1 overflow-y-auto px-3 py-4">
                  {renderSidebarNav({ collapsed: false, onSelect: () => setMobileNavOpen(false) })}
                </div>
              </motion.aside>
            </>
          ) : null}
        </AnimatePresence>

        {/* Main content */}
        <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
          <div className="flex-1 overflow-y-auto">
            {/* Header */}
            <header className="sticky top-0 z-10 flex h-16 items-center justify-between border-b bg-background/80 px-4 backdrop-blur-sm md:px-6">
              <AnimatePresence mode="wait">
                <motion.div
                  key={currentPage}
                  initial={{ opacity: 0, y: 8 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, y: -8 }}
                  transition={{ type: 'spring', bounce: 0.15, duration: 0.35 }}
                  className="flex min-w-0 items-center gap-3"
                >
                  {isMobileViewport ? (
                    <Button variant="outline" size="icon" className="h-9 w-9 md:hidden" onClick={() => setMobileNavOpen(true)}>
                      <PanelLeft className="h-4 w-4" />
                    </Button>
                  ) : null}
                  {(() => {
                    const Icon = navIcons[currentPage]
                    return <Icon className="h-5 w-5 text-primary" />
                  })()}
                  <h2 className="truncate text-base font-semibold md:text-lg">{currentPageLabel}</h2>
                </motion.div>
              </AnimatePresence>
              <div className="flex items-center gap-2 md:gap-3">
                <AnnouncementCenter
                  announcements={announcements}
                  compact={isMobileViewport}
                  unreadCount={unreadAnnouncements.length}
                  triggerLabel="公告"
                  description="查看当前账号适用的公告，并可手动标记为已读。"
                  onMarkRead={handleMarkRead}
                  onMarkAllRead={handleMarkAllRead}
                  busyId={announcementBusyId}
                  busyAll={announcementBusyAll}
                />
                <ContactCenter contact={siteContact} compact={isMobileViewport} />
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button
                      variant="outline"
                      className={isMobileViewport ? 'h-9 w-9 rounded-full p-0' : 'gap-2 rounded-full pl-3 pr-4'}
                    >
                      <div className="flex h-7 w-7 items-center justify-center rounded-full bg-primary/10 text-primary text-xs font-bold">
                        {username[0]?.toUpperCase()}
                      </div>
                      {!isMobileViewport ? <span className="font-medium">{username}</span> : null}
                      {!isMobileViewport && isAdmin ? (
                        <Badge variant="default" className="text-[10px] px-1.5 py-0">
                          管理员
                        </Badge>
                      ) : null}
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="w-48">
                    <DropdownMenuItem onClick={() => setCurrentPage('account-settings')}>
                      <UserCircle className="mr-2 h-4 w-4" />
                      账户设置
                    </DropdownMenuItem>
                    <Separator className="my-1" />
                    <DropdownMenuItem onClick={onLogout} className="text-destructive focus:text-destructive">
                      <LogOut className="mr-2 h-4 w-4" />
                      退出登录
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            </header>

            {/* Page content */}
            <main className="p-4 md:p-6">
              <Suspense fallback={<PageLoader />}>
                <AnimatePresence mode="wait">
                  <motion.div
                    key={currentPage}
                    initial={{ opacity: 0, y: 20, scale: 0.99 }}
                    animate={{ opacity: 1, y: 0, scale: 1 }}
                    exit={{ opacity: 0, y: -12, scale: 0.99 }}
                    transition={{ type: 'spring', bounce: 0.15, duration: 0.4 }}
                  >
                    {currentPage === 'overview' && <Overview />}
                    {currentPage === 'status-monitor' && <StatusMonitor />}
                    {currentPage === 'admin-overview' && isAdmin && <AdminOverview />}
                    {currentPage === 'amp-settings' && canAccessAmpSettings && (
                      <AmpSettings
                        canAccessRouteSettings={canAccessAmpRouteSettings}
                        canAccessAmpUpstreamSettings={canAccessAmpUpstreamSettings}
                      />
                    )}
                    {currentPage === 'api-keys' && <APIKeys siteName={siteName} />}
                    {currentPage === 'request-logs' && <RequestLogs isAdmin={isAdmin} />}
                    {currentPage === 'usage-stats' && <UsageStats isAdmin={isAdmin} />}
                    {currentPage === 'models' && <Models isAdmin={isAdmin} />}
                    {currentPage === 'account-settings' && <AccountSettings username={username} onUsernameChange={setUsername} />}
                    {currentPage === 'purchase-center' && <PurchaseCenter />}
                    {currentPage === 'channels' && isAdmin && <Channels />}
                    {currentPage === 'groups' && isAdmin && <Groups />}
                    {currentPage === 'subscription-plans' && isAdmin && <SubscriptionPlans />}
                    {currentPage === 'paid-subscriptions' && isAdmin && <PaidSubscriptions />}
                    {currentPage === 'redeem-management' && isAdmin && <RedeemManagement />}
                    {currentPage === 'session-management' && isAdmin && <SessionManagement />}
                    {currentPage === 'model-metadata' && isAdmin && <ModelMetadata />}
                    {currentPage === 'prices' && isAdmin && <Prices />}
                    {currentPage === 'user-management' && isAdmin && <UserManagement />}
                    {currentPage === 'system-settings' && isAdmin && (
                      <SystemSettings
                        siteName={siteName}
                        siteTimeZone={siteTimeZone}
                        ampProxySettingsPolicy={ampProxySettingsPolicy}
                        ampSettingsPolicy={ampSettingsPolicy}
                        siteContact={siteContact}
                        onSiteNameChange={onSiteNameChange}
                        onSiteTimeZoneChange={onSiteTimeZoneChange}
                        onAmpProxySettingsPolicyChange={onAmpProxySettingsPolicyChange}
                        onAmpSettingsPolicyChange={onAmpSettingsPolicyChange}
                        onSiteContactChange={onSiteContactChange}
                      />
                    )}
                  </motion.div>
                </AnimatePresence>
              </Suspense>
            </main>
          </div>
        </div>
      </div>
      <AnnouncementUnreadDialog
        announcements={announcements}
        open={showUnreadDialog}
        onOpenChange={setShowUnreadDialog}
        onMarkAllRead={handleMarkAllRead}
        busy={announcementBusyAll}
      />
    </TooltipProvider>
  )
}
