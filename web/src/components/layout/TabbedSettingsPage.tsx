import type { ReactNode } from 'react'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { AnimatePresence, motion } from '@/lib/motion'

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
  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="space-y-6">
      <motion.div
        initial={{ opacity: 0, y: -20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ type: 'spring', bounce: 0.2, duration: 0.6 }}
      >
        <h2 className="text-2xl font-bold tracking-tight">{title}</h2>
        <p className="text-muted-foreground">{description}</p>
      </motion.div>

      <motion.div
        initial={{ opacity: 0, y: 10 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ type: 'spring', bounce: 0.2, duration: 0.5, delay: 0.05 }}
        className="flex items-center gap-1 border-b pb-0"
      >
        {tabs.map((tab) => (
          <button
            key={tab.key}
            type="button"
            onClick={() => onTabChange(tab.key)}
            className={`relative rounded-t-md px-4 py-2 text-sm font-medium transition-colors ${
              activeTab === tab.key
                ? 'text-foreground'
                : 'text-muted-foreground hover:text-foreground/80'
            }`}
          >
            {tab.label}
            {activeTab === tab.key && (
              <motion.div
                layoutId={indicatorId}
                className="absolute inset-x-0 -bottom-px h-0.5 bg-primary"
                transition={{ type: 'spring', bounce: 0.2, duration: 0.4 }}
              />
            )}
          </button>
        ))}
      </motion.div>

      <AnimatePresence>
        {message && (
          <motion.div
            initial={{ opacity: 0, y: -20, scale: 0.95 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: -20, scale: 0.95 }}
            transition={{ type: 'spring', bounce: 0.3, duration: 0.5 }}
          >
            <Alert variant={message.type === 'error' ? 'destructive' : 'default'}>
              <AlertDescription>{message.text}</AlertDescription>
            </Alert>
          </motion.div>
        )}
      </AnimatePresence>

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
