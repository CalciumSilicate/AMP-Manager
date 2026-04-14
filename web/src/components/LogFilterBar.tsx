import { useState, useEffect, useRef } from 'react'
import { DistinctAPIKey } from '@/api/amp'
import { UserInfo } from '@/api/users'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { DateTimePicker } from '@/components/ui/datetime-picker'
import { SearchableSelect, SelectOption } from '@/components/SearchableSelect'
import { motion } from '@/lib/motion'

export interface FilterValues {
  userId: string
  apiKeyId: string
  model: string
  from: string
  to: string
}

interface LogFilterBarProps {
  isAdmin: boolean
  users: UserInfo[]
  keys: DistinctAPIKey[]
  models: string[]
  values: FilterValues
  onChange: (values: FilterValues) => void
}

type PresetKey = 'custom' | '1h' | '6h' | '24h' | '7d' | '30d'

const presets: { key: PresetKey; label: string }[] = [
  { key: '1h', label: '最近 1 小时' },
  { key: '6h', label: '最近 6 小时' },
  { key: '24h', label: '最近 24 小时' },
  { key: '7d', label: '最近 7 天' },
  { key: '30d', label: '最近 30 天' },
]

function toLocalDatetimeString(date: Date): string {
  const pad = (n: number) => n.toString().padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

function getPresetRange(key: PresetKey): { from: string; to: string } {
  const now = new Date()
  const to = toLocalDatetimeString(now)
  let fromDate: Date

  switch (key) {
    case '1h':
      fromDate = new Date(now.getTime() - 60 * 60 * 1000)
      break
    case '6h':
      fromDate = new Date(now.getTime() - 6 * 60 * 60 * 1000)
      break
    case '24h':
      fromDate = new Date(now.getTime() - 24 * 60 * 60 * 1000)
      break
    case '7d':
      fromDate = new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000)
      break
    case '30d':
      fromDate = new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000)
      break
    default:
      return { from: '', to: '' }
  }

  return { from: toLocalDatetimeString(fromDate), to }
}

function localToISO(localStr: string): string {
  if (!localStr) return ''
  return new Date(localStr).toISOString()
}

export function LogFilterBar({ isAdmin, users, keys, models, values, onChange }: LogFilterBarProps) {
  const [activePreset, setActivePreset] = useState<PresetKey>('custom')
  const preservePresetOnSyncRef = useRef(false)

  useEffect(() => {
    if (preservePresetOnSyncRef.current) {
      preservePresetOnSyncRef.current = false
      return
    }
    if (values.from || values.to) {
      setActivePreset('custom')
    }
  }, [values.from, values.to])

  const update = (partial: Partial<FilterValues>) => {
    onChange({ ...values, ...partial })
  }

  const handlePreset = (key: PresetKey) => {
    setActivePreset(key)
    if (key === 'custom') {
      update({ from: '', to: '' })
    } else {
      preservePresetOnSyncRef.current = true
      const range = getPresetRange(key)
      update(range)
    }
  }

  const handleFromChange = (from: string) => {
    setActivePreset('custom')
    update({ from })
  }

  const handleToChange = (to: string) => {
    setActivePreset('custom')
    update({ to })
  }

  const handleClearAll = () => {
    setActivePreset('custom')
    onChange({ userId: '', apiKeyId: '', model: '', from: '', to: '' })
  }

  const handleUserChange = (userId: string) => {
    update({ userId, apiKeyId: '' })
  }

  const userOptions: SelectOption[] = users.map(u => ({ value: u.id, label: u.username }))
  const keyOptions: SelectOption[] = keys.map(k => ({ value: k.id, label: `${k.name} (${k.prefix})`, keywords: [k.prefix] }))
  const modelOptions: SelectOption[] = models.map(m => ({ value: m, label: m }))

  const hasAnyFilter = values.userId || values.apiKeyId || values.model || values.from || values.to

  return (
    <motion.div
      initial={{ opacity: 0, y: 20 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ type: 'spring', bounce: 0.2, duration: 0.5, delay: 0.05 }}
    >
      <Card>
        <CardContent className="pt-4 pb-4 space-y-3">
          <div className="flex items-center gap-3 flex-wrap">
            {isAdmin && (
              <div className="flex items-center gap-2">
                <Label className="text-sm text-muted-foreground whitespace-nowrap">用户</Label>
                <SearchableSelect
                  value={values.userId}
                  onValueChange={handleUserChange}
                  options={userOptions}
                  searchPlaceholder="搜索用户..."
                  allLabel="所有用户"
                  className="w-36"
                />
              </div>
            )}

            {keys.length > 0 && (
              <div className="flex items-center gap-2">
                <Label className="text-sm text-muted-foreground whitespace-nowrap">Key</Label>
                <SearchableSelect
                  value={values.apiKeyId}
                  onValueChange={(v) => update({ apiKeyId: v })}
                  options={keyOptions}
                  searchPlaceholder="搜索 Key..."
                  allLabel="所有 Key"
                  className="w-48"
                />
              </div>
            )}

            <div className="flex items-center gap-2">
              <Label className="text-sm text-muted-foreground whitespace-nowrap">模型</Label>
              <SearchableSelect
                value={values.model}
                onValueChange={(v) => update({ model: v })}
                options={modelOptions}
                searchPlaceholder="搜索模型..."
                allLabel="所有模型"
                className="w-48"
              />
            </div>

            <div className="h-6 w-px bg-border mx-1" />

            <div className="flex items-center gap-2">
              <Label className="text-sm text-muted-foreground whitespace-nowrap">从</Label>
              <DateTimePicker
                value={values.from}
                onChange={handleFromChange}
                placeholder="开始时间"
              />
            </div>

            <div className="flex items-center gap-2">
              <Label className="text-sm text-muted-foreground whitespace-nowrap">到</Label>
              <DateTimePicker
                value={values.to}
                onChange={handleToChange}
                placeholder="结束时间"
              />
            </div>

            {hasAnyFilter && (
              <Button variant="ghost" size="sm" onClick={handleClearAll} className="text-muted-foreground">
                清除筛选
              </Button>
            )}
          </div>

          <div className="flex items-center gap-1.5 flex-wrap">
            <span className="text-xs text-muted-foreground mr-1">快捷时间:</span>
            {presets.map(p => (
              <Button
                key={p.key}
                variant={activePreset === p.key ? 'default' : 'outline'}
                size="sm"
                className="h-6 text-xs px-2"
                onClick={() => handlePreset(p.key)}
              >
                {p.label}
              </Button>
            ))}
          </div>
        </CardContent>
      </Card>
    </motion.div>
  )
}

export { localToISO }
