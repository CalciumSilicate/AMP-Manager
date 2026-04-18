import { motion } from '@/lib/motion'
import { cn } from '@/lib/utils'

export interface ScrollableTabBarTab<T extends string> {
  key: T
  label: string
}

interface ScrollableTabBarProps<T extends string> {
  tabs: ScrollableTabBarTab<T>[]
  activeTab: T
  onTabChange: (tab: T) => void
  indicatorId: string
  showBorder?: boolean
  outerClassName?: string
  innerClassName?: string
  buttonClassName?: string
}

export function ScrollableTabBar<T extends string>({
  tabs,
  activeTab,
  onTabChange,
  indicatorId,
  showBorder = true,
  outerClassName,
  innerClassName,
  buttonClassName,
}: ScrollableTabBarProps<T>) {
  return (
    <div
      className={cn(
        'overflow-x-auto pb-0',
        showBorder && 'border-b',
        outerClassName,
      )}
    >
      <div className={cn('flex min-w-max items-center gap-1', innerClassName)}>
        {tabs.map((tab) => (
          <button
            key={tab.key}
            type="button"
            onClick={() => onTabChange(tab.key)}
            className={cn(
              'relative rounded-t-md px-3 py-2 text-sm font-medium whitespace-nowrap transition-colors md:px-4',
              activeTab === tab.key
                ? 'text-foreground'
                : 'text-muted-foreground hover:text-foreground/80',
              buttonClassName,
            )}
          >
            {tab.label}
            {activeTab === tab.key ? (
              <motion.div
                layoutId={indicatorId}
                className="absolute inset-x-0 -bottom-px h-0.5 bg-primary"
                transition={{ type: 'spring', bounce: 0.2, duration: 0.4 }}
              />
            ) : null}
          </button>
        ))}
      </div>
    </div>
  )
}
