import { useState } from 'react'

import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { copyText } from '@/lib/clipboard'
import { cn } from '@/lib/utils'

interface OverflowCopyTextProps {
  text: string
  copyValue?: string
  tooltipText?: string
  className?: string
  tooltipClassName?: string
}

export function OverflowCopyText({
  text,
  copyValue,
  tooltipText,
  className,
  tooltipClassName,
}: OverflowCopyTextProps) {
  const [copied, setCopied] = useState(false)

  const displayText = text || '-'
  const fullText = tooltipText || displayText

  const handleCopy = async () => {
    const success = await copyText(copyValue || fullText)
    if (!success) return

    setCopied(true)
    window.setTimeout(() => setCopied(false), 1200)
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          onClick={handleCopy}
          className={cn(
            'block w-full truncate text-left text-inherit decoration-border/80 underline-offset-4 transition hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2',
            className,
          )}
        >
          {displayText}
        </button>
      </TooltipTrigger>
      <TooltipContent className={cn('max-w-[360px] space-y-1', tooltipClassName)}>
        <div className="break-all font-medium">{fullText}</div>
        <div className="text-[11px] opacity-80">{copied ? '已复制' : '点击复制'}</div>
      </TooltipContent>
    </Tooltip>
  )
}
