import { formatCompact, formatExact } from '@/lib/formatters'
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
} from '@/components/ui/tooltip'

interface NumProps {
  value: number | undefined
  className?: string
  compact?: boolean
}

export function Num({ value, className, compact = true }: NumProps) {
  const compactValue = compact ? formatCompact(value) : formatExact(value)
  const exact = formatExact(value)

  if (value === undefined || value === null || compactValue === exact) {
    return <span className={className}>{compactValue}</span>
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className={`cursor-default ${className || ''}`}>{compactValue}</span>
      </TooltipTrigger>
      <TooltipContent>
        <span className="font-mono">{exact}</span>
      </TooltipContent>
    </Tooltip>
  )
}
