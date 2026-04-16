import { useEffect, useState } from 'react'
import { motion, AnimatePresence } from '@/lib/motion'
import Overview from './Overview'
import AdminOverview from './AdminOverview'
import AmpSettings from './AmpSettings'
import APIKeys from './APIKeys'
import RequestLogs from './RequestLogs'
import UsageStats from './UsageStats'
import Channels from './Channels'
import Models from './Models'
import ModelMetadata from './ModelMetadata'
import Prices from './Prices'
import SystemSettings from './SystemSettings'
import Groups from './Groups'
import SubscriptionPlans from './SubscriptionPlans'
import UserManagement from './UserManagement'
import AccountSettings from './AccountSettings'
import PurchaseCenter from './PurchaseCenter'
import PaidSubscriptions from './PaidSubscriptions'
import RedeemManagement from './RedeemManagement'
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
  Zap,
  FolderOpen,
  LayoutDashboard,
  CreditCard,
  ShoppingCart,
  TicketPercent,
} from 'lucide-react'

interface Props {
  username: string
  isAdmin: boolean
  siteName: string
  siteTimeZone: string
  allowAmpProxySettings: boolean
  onSiteNameChange: (siteName: string) => void
  onSiteTimeZoneChange: (timeZone: string) => void
  onAllowAmpProxySettingsChange: (allowed: boolean) => void
  onLogout: () => void
}

type Page = 'overview' | 'amp-settings' | 'api-keys' | 'request-logs' | 'usage-stats' | 'channels' | 'models' | 'model-metadata' | 'prices' | 'system-settings' | 'user-management' | 'account-settings' | 'groups' | 'subscription-plans' | 'admin-overview' | 'purchase-center' | 'paid-subscriptions' | 'redeem-management'

const navIcons: Record<Page, React.ElementType> = {
  'overview': LayoutDashboard,
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
}

export default function Dashboard({
  username: initialUsername,
  isAdmin,
  siteName,
  siteTimeZone,
  allowAmpProxySettings,
  onSiteNameChange,
  onSiteTimeZoneChange,
  onAllowAmpProxySettingsChange,
  onLogout,
}: Props) {
  const [currentPage, setCurrentPage] = useState<Page>('overview')
  const [username, setUsername] = useState(initialUsername)
  const [collapsed, setCollapsed] = useState(false)

  const navItems: { key: Page; label: string; adminOnly?: boolean }[] = [
    { key: 'overview', label: '概览' },
    ...(allowAmpProxySettings ? [{ key: 'amp-settings' as const, label: 'Amp 设置' }] : []),
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
    { key: 'channels', label: '渠道管理', adminOnly: true },
    { key: 'model-metadata', label: '模型元数据', adminOnly: true },
    { key: 'prices', label: '模型价格', adminOnly: true },
    { key: 'user-management', label: '用户管理', adminOnly: true },
    { key: 'system-settings', label: '系统设置', adminOnly: true },
  ]

  const visibleNavItems = navItems.filter(item => !item.adminOnly || isAdmin)
  const userNavItems = visibleNavItems.filter(item => !item.adminOnly)
  const adminNavItems = visibleNavItems.filter(item => item.adminOnly)
  const currentPageLabel = visibleNavItems.find((item) => item.key === currentPage)?.label || '概览'

  useEffect(() => {
    document.title = `${currentPageLabel} - ${siteName}`
  }, [currentPageLabel, siteName])

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
    if (!allowAmpProxySettings && currentPage === 'amp-settings') {
      setCurrentPage('overview')
    }
  }, [allowAmpProxySettings, currentPage])

  const renderNavItem = (item: { key: Page; label: string; adminOnly?: boolean }) => {
    const Icon = navIcons[item.key]
    const isActive = currentPage === item.key

    const button = (
      <div key={item.key}>
        <button
          onClick={() => setCurrentPage(item.key)}
          className={`
            grid w-full items-center justify-items-start rounded-lg px-3 py-2.5 text-left text-sm font-medium transition-all duration-200
            ${isActive
              ? 'bg-primary text-primary-foreground shadow-md shadow-primary/20'
              : 'text-muted-foreground hover:bg-muted hover:text-foreground'
            }
            ${collapsed ? 'grid-cols-[20px_0px_0px] px-2.5' : 'grid-cols-[20px_minmax(0,1fr)_auto]'}
          `}
        >
          <Icon className={`shrink-0 ${collapsed ? 'h-5 w-5' : 'h-4 w-4'}`} />
          <div
            className={`
              min-w-0 overflow-hidden whitespace-nowrap transition-[margin,opacity] duration-200
              ${collapsed ? 'ml-0 opacity-0' : 'ml-3 opacity-100'}
            `}
          >
            {item.label}
          </div>
          <div className={`justify-self-end overflow-hidden transition-opacity duration-200 ${collapsed ? 'opacity-0' : 'opacity-100'}`}>
            {item.adminOnly ? (
              <Badge variant={isActive ? 'secondary' : 'outline'} className="text-[10px] px-1.5 py-0">
                管理
              </Badge>
            ) : null}
          </div>
        </button>
      </div>
    )

    if (collapsed) {
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

  return (
    <TooltipProvider delayDuration={0}>
      <div className="flex h-screen overflow-hidden bg-muted/30">
        {/* Sidebar */}
        <motion.aside
          className="relative flex h-full shrink-0 flex-col border-r bg-background/80 backdrop-blur-sm"
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
            <nav className="flex flex-col gap-1">
              {userNavItems.map(renderNavItem)}

              {adminNavItems.length > 0 && (
                <>
                  {!collapsed ? (
                    <div className="my-3 flex items-center gap-2 px-3">
                      <Separator className="flex-1" />
                      <span className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">管理</span>
                      <Separator className="flex-1" />
                    </div>
                  ) : (
                    <Separator className="my-3" />
                  )}
                  {adminNavItems.map(renderNavItem)}
                </>
              )}
            </nav>
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

        {/* Main content */}
        <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
          <div className="flex-1 overflow-y-auto">
            {/* Header */}
            <header className="sticky top-0 z-10 flex h-16 items-center justify-between border-b bg-background/80 px-6 backdrop-blur-sm">
              <AnimatePresence mode="wait">
                <motion.div
                  key={currentPage}
                  initial={{ opacity: 0, y: 8 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, y: -8 }}
                  transition={{ type: 'spring', bounce: 0.15, duration: 0.35 }}
                  className="flex items-center gap-3"
                >
                  {(() => {
                    const Icon = navIcons[currentPage]
                    return <Icon className="h-5 w-5 text-primary" />
                  })()}
                  <h2 className="text-lg font-semibold">{currentPageLabel}</h2>
                </motion.div>
              </AnimatePresence>
              <div className="flex items-center gap-3">
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="outline" className="gap-2 rounded-full pl-3 pr-4">
                      <div className="flex h-7 w-7 items-center justify-center rounded-full bg-primary/10 text-primary text-xs font-bold">
                        {username[0]?.toUpperCase()}
                      </div>
                      <span className="font-medium">{username}</span>
                      {isAdmin && (
                        <Badge variant="default" className="text-[10px] px-1.5 py-0">
                          管理员
                        </Badge>
                      )}
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
            <main className="p-6">
              <AnimatePresence mode="wait">
                <motion.div
                  key={currentPage}
                  initial={{ opacity: 0, y: 20, scale: 0.99 }}
                  animate={{ opacity: 1, y: 0, scale: 1 }}
                  exit={{ opacity: 0, y: -12, scale: 0.99 }}
                  transition={{ type: 'spring', bounce: 0.15, duration: 0.4 }}
                >
                  {currentPage === 'overview' && <Overview />}
                  {currentPage === 'admin-overview' && isAdmin && <AdminOverview />}
                  {currentPage === 'amp-settings' && allowAmpProxySettings && <AmpSettings />}
                  {currentPage === 'api-keys' && <APIKeys />}
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
                  {currentPage === 'model-metadata' && isAdmin && <ModelMetadata />}
                  {currentPage === 'prices' && isAdmin && <Prices />}
                  {currentPage === 'user-management' && isAdmin && <UserManagement />}
                  {currentPage === 'system-settings' && isAdmin && (
                    <SystemSettings
                      siteName={siteName}
                      siteTimeZone={siteTimeZone}
                      allowAmpProxySettings={allowAmpProxySettings}
                      onSiteNameChange={onSiteNameChange}
                      onSiteTimeZoneChange={onSiteTimeZoneChange}
                      onAllowAmpProxySettingsChange={onAllowAmpProxySettingsChange}
                    />
                  )}
                </motion.div>
              </AnimatePresence>
            </main>
          </div>
        </div>
      </div>
    </TooltipProvider>
  )
}
