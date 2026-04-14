import { useState } from 'react'

import { formatCompact, formatExact } from '@/lib/formatters'
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
} from '@/components/ui/tooltip'
import { copyText } from '@/lib/clipboard'
import { cn } from '@/lib/utils'

interface NumProps {
  value: number | undefined
  className?: string
  compact?: boolean
  interactive?: boolean
  copyable?: boolean
  displayClassName?: string
  fullTextOverride?: string
}

export function Num({
  value,
  className,
  compact = true,
  interactive = false,
  copyable = false,
  displayClassName,
  fullTextOverride,
}: NumProps) {
  const [copied, setCopied] = useState(false)
  const compactValue = compact ? formatCompact(value) : formatExact(value)
  const exact = fullTextOverride || formatExact(value)

  if (value === undefined || value === null || compactValue === exact) {
    return <span className={className}>{compactValue}</span>
  }

  const clickable = interactive || copyable

  const handleCopy = async () => {
    if (!clickable) return
    const success = await copyText(exact)
    if (!success) return

    setCopied(true)
    window.setTimeout(() => setCopied(false), 1200)
  }

  const triggerClassName = cn(
    clickable
      ? 'cursor-pointer decoration-border/80 underline-offset-4 transition hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2'
      : 'cursor-default',
    displayClassName,
    className,
  )

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        {clickable ? (
          <button type="button" onClick={handleCopy} className={triggerClassName}>
            {compactValue}
          </button>
        ) : (
          <span className={triggerClassName}>{compactValue}</span>
        )}
      </TooltipTrigger>
      <TooltipContent>
        <div className="space-y-1">
          <span className="font-mono">{exact}</span>
          {clickable ? <div className="text-[11px] opacity-80">{copied ? '已复制' : '点击复制'}</div> : null}
        </div>
      </TooltipContent>
    </Tooltip>
  )
}
