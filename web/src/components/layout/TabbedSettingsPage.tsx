import { useEffect, useRef, type ReactNode } from 'react'

import { ScrollableTabBar } from '@/components/layout/ScrollableTabBar'
import { AnimatePresence, motion } from '@/lib/motion'
import { useGlobalToast } from '@/components/ui/use-global-toast'

export interface TabbedSettingsPageTab<T extends string> {
  key: T
  label: string
}

interface TabbedSettingsPageProps<T extends string> {
  title: string
  description: string
  tabs: TabbedSettingsPageTab<T>[]
  activeTab: T
  onTabChange: (tab: T) => void
  indicatorId: string
  children: ReactNode
  message?: { type: 'success' | 'error'; text: string } | null
  extraContent?: ReactNode
}

export function TabbedSettingsPage<T extends string>({
  title,
  description,
  tabs,
  activeTab,
  onTabChange,
  indicatorId,
  children,
  message,
  extraContent,
}: TabbedSettingsPageProps<T>) {
  const { showToast } = useGlobalToast()
  const lastMessageKeyRef = useRef('')

  useEffect(() => {
    if (!message) return
    const nextKey = `${message.type}:${message.text}`
    if (lastMessageKeyRef.current === nextKey) return
    lastMessageKeyRef.current = nextKey
    showToast(message.type, message.text)
  }, [message, showToast])

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="space-y-6">
      <motion.div
        initial={{ opacity: 0, y: -20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ type: 'spring', bounce: 0.2, duration: 0.6 }}
      >
        <h2 className="text-xl font-bold tracking-tight md:text-2xl">{title}</h2>
        <p className="text-sm text-muted-foreground md:text-base">{description}</p>
      </motion.div>

      <motion.div
        initial={{ opacity: 0, y: 10 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ type: 'spring', bounce: 0.2, duration: 0.5, delay: 0.05 }}
      >
        <ScrollableTabBar
          tabs={tabs}
          activeTab={activeTab}
          onTabChange={onTabChange}
          indicatorId={indicatorId}
          outerClassName="[-ms-overflow-style:none] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
        />
      </motion.div>

      {extraContent}

      <AnimatePresence mode="wait">
        <motion.div
          key={activeTab}
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -10 }}
          transition={{ type: 'spring', bounce: 0.2, duration: 0.4 }}
          className="space-y-6"
        >
          {children}
        </motion.div>
      </AnimatePresence>
    </motion.div>
  )
}
