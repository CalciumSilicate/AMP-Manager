import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'

type Width = '4xl' | '5xl' | '6xl' | '7xl' | 'full'

const WIDTH_CLASS: Record<Width, string> = {
  '4xl': 'max-w-4xl',
  '5xl': 'max-w-5xl',
  '6xl': 'max-w-6xl',
  '7xl': 'max-w-7xl',
  full: 'max-w-none',
}

interface AdminPageShellProps {
  title: string
  description?: string
  actions?: ReactNode
  children: ReactNode
  width?: Width
  className?: string
  headerClassName?: string
}

interface AdminSurfaceProps {
  children: ReactNode
  className?: string
}

interface AdminToolbarRowProps {
  children: ReactNode
  className?: string
}

export function AdminPageShell({
  title,
  description,
  actions,
  children,
  width = '6xl',
  className,
  headerClassName,
}: AdminPageShellProps) {
  return (
    <div className={cn('admin-page-shell', WIDTH_CLASS[width], className)}>
      <div className={cn('admin-page-header', headerClassName)}>
        <div className="min-w-0 space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight text-foreground">{title}</h1>
          {description ? (
            <p className="max-w-3xl text-sm text-muted-foreground">{description}</p>
          ) : null}
        </div>
        {actions ? <div className="flex shrink-0 items-center gap-2">{actions}</div> : null}
      </div>
      {children}
    </div>
  )
}

export function AdminSurface({ children, className }: AdminSurfaceProps) {
  return <section className={cn('admin-surface', className)}>{children}</section>
}

export function AdminToolbarRow({ children, className }: AdminToolbarRowProps) {
  return <div className={cn('admin-toolbar-row', className)}>{children}</div>
}
