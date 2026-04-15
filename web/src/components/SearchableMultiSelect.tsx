import { useMemo, useState } from 'react'
import { Check, ChevronsUpDown, X } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { cn } from '@/lib/utils'

export interface MultiSelectOption {
  value: string
  label: string
  keywords?: string[]
}

interface SearchableMultiSelectProps {
  values: string[]
  onValuesChange: (values: string[]) => void
  options: MultiSelectOption[]
  searchPlaceholder?: string
  emptyText?: string
  allLabel?: string
  className?: string
}

export function SearchableMultiSelect({
  values,
  onValuesChange,
  options,
  searchPlaceholder = '搜索...',
  emptyText = '无匹配项',
  allLabel = '全部',
  className,
}: SearchableMultiSelectProps) {
  const [open, setOpen] = useState(false)

  const selectedOptions = useMemo(
    () => options.filter((option) => values.includes(option.value)),
    [options, values],
  )

  const triggerLabel = useMemo(() => {
    if (selectedOptions.length === 0) return allLabel
    if (selectedOptions.length <= 2) {
      return selectedOptions.map((option) => option.label).join(', ')
    }
    return `${selectedOptions[0].label}、${selectedOptions[1].label} +${selectedOptions.length - 2}`
  }, [allLabel, selectedOptions])

  const toggleValue = (value: string) => {
    if (values.includes(value)) {
      onValuesChange(values.filter((item) => item !== value))
      return
    }
    onValuesChange([...values, value])
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          className={cn('justify-between font-normal h-9 min-w-[164px]', className)}
        >
          <span className="truncate">{triggerLabel}</span>
          <ChevronsUpDown className="ml-2 h-3.5 w-3.5 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[--radix-popover-trigger-width] p-0" align="start">
        <Command>
          <CommandInput placeholder={searchPlaceholder} />
          <CommandList>
            <CommandEmpty>{emptyText}</CommandEmpty>
            <CommandGroup>
              <CommandItem
                value={allLabel}
                onSelect={() => onValuesChange([])}
              >
                <X className={cn('mr-2 h-4 w-4', values.length === 0 ? 'opacity-100' : 'opacity-0')} />
                {allLabel}
              </CommandItem>
              {options.map((option) => (
                <CommandItem
                  key={option.value}
                  value={option.label}
                  keywords={option.keywords}
                  onSelect={() => toggleValue(option.value)}
                >
                  <Check className={cn('mr-2 h-4 w-4', values.includes(option.value) ? 'opacity-100' : 'opacity-0')} />
                  {option.label}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
          <div className="border-t px-3 py-2 text-xs text-muted-foreground">
            已选 {selectedOptions.length} 项
          </div>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
