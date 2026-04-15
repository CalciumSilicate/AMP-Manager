import type { ReactNode } from 'react'

import { PageShell } from '@/components/layout/PageScaffold'
import { cn } from '@/lib/utils'

type Width = '4xl' | '5xl' | '6xl' | '7xl' | 'full'

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

export function AdminPageShell(props: AdminPageShellProps) {
  return <PageShell {...props} />
}

export function AdminSurface({ children, className }: AdminSurfaceProps) {
  return <section className={cn('ops-surface', className)}>{children}</section>
}

export function AdminToolbarRow({ children, className }: AdminToolbarRowProps) {
  return <div className={cn('ops-toolbar-row', className)}>{children}</div>
}
