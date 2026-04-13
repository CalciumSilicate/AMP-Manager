import { useEffect, useState } from 'react'
import { Slider } from '@/components/ui/slider'
import { cn } from '@/lib/utils'

interface PageSizeSliderProps {
  value: number
  onChange: (value: number) => void
  min?: number
  max?: number
  step?: number
  className?: string
}

const PRESETS = [10, 20, 50, 100]

export function PageSizeSlider({
  value,
  onChange,
  min = 10,
  max = 100,
  step = 5,
  className,
}: PageSizeSliderProps) {
  const [draftValue, setDraftValue] = useState(value)

  useEffect(() => {
    setDraftValue(value)
  }, [value])

  const handlePresetClick = (nextValue: number) => {
    setDraftValue(nextValue)
    onChange(nextValue)
  }

  return (
    <div className={cn('flex items-center gap-3', className)}>
      <span className="text-xs text-muted-foreground whitespace-nowrap">每页</span>
      <div className="flex items-center gap-1">
        {PRESETS.filter(p => p >= min && p <= max).map(p => (
          <button
            key={p}
            onClick={() => handlePresetClick(p)}
            className={cn(
              'h-6 min-w-[2rem] px-1.5 rounded text-xs font-medium transition-colors',
              draftValue === p
                ? 'bg-primary text-primary-foreground'
                : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
            )}
          >
            {p}
          </button>
        ))}
      </div>
      <Slider
        value={[draftValue]}
        onValueChange={([nextValue]) => setDraftValue(nextValue)}
        onValueCommit={([nextValue]) => onChange(nextValue)}
        min={min}
        max={max}
        step={step}
        className="w-24"
      />
      <span className="text-xs font-medium tabular-nums w-6 text-right">{draftValue}</span>
    </div>
  )
}
