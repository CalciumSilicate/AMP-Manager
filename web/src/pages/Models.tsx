import { useEffect, useMemo, useState } from 'react'

import { motion } from '@/lib/motion'
import { listAvailableModels, fetchAllModels, AvailableModel, FetchModelsResult } from '../api/models'
import { AdminPageShell, AdminSurface, AdminToolbarRow } from '@/components/admin/AdminPageShell'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

interface Props {
  isAdmin: boolean
}

interface AggregatedModel {
  modelId: string
  displayName: string
  channelType: AvailableModel['channelType']
  channelNames: string[]
}

export default function Models({ isAdmin }: Props) {
  const [models, setModels] = useState<AvailableModel[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [fetching, setFetching] = useState(false)
  const [fetchResult, setFetchResult] = useState<FetchModelsResult | null>(null)
  const [filter, setFilter] = useState<'all' | 'openai' | 'claude' | 'gemini'>('all')

  useEffect(() => {
    loadModels()
  }, [])

  const loadModels = async () => {
    try {
      setLoading(true)
      const data = await listAvailableModels()
      setModels(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }

  const handleFetchAll = async () => {
    try {
      setFetching(true)
      setFetchResult(null)
      setError('')
      const result = await fetchAllModels()
      setFetchResult(result)
      loadModels()
    } catch (err) {
      setError(err instanceof Error ? err.message : '获取失败')
    } finally {
      setFetching(false)
    }
  }

  const filteredModels = useMemo(
    () => (filter === 'all' ? models : models.filter((model) => model.channelType === filter)),
    [filter, models],
  )

  const aggregatedModels = useMemo(() => {
    const grouped = new Map<string, AggregatedModel>()

    for (const model of filteredModels) {
      const key = `${model.channelType}:${model.modelId}`
      const existing = grouped.get(key)

      if (existing) {
        if (model.displayName !== model.modelId && existing.displayName === existing.modelId) {
          existing.displayName = model.displayName
        }
        if (!existing.channelNames.includes(model.channelName)) {
          existing.channelNames.push(model.channelName)
        }
        continue
      }

      grouped.set(key, {
        modelId: model.modelId,
        displayName: model.displayName,
        channelType: model.channelType,
        channelNames: [model.channelName],
      })
    }

    return Array.from(grouped.values())
      .map((model) => ({
        ...model,
        channelNames: [...model.channelNames].sort((a, b) => a.localeCompare(b, 'zh-CN')),
      }))
      .sort((a, b) => a.modelId.localeCompare(b.modelId, 'en'))
  }, [filteredModels])

  const groupedModels = useMemo(
    () =>
      aggregatedModels.reduce((acc, model) => {
        const key = model.channelType
        if (!acc[key]) acc[key] = []
        acc[key].push(model)
        return acc
      }, {} as Record<string, AggregatedModel[]>),
    [aggregatedModels],
  )

  const getTypeBadgeVariant = (type: string): 'default' | 'secondary' | 'destructive' | 'outline' => {
    switch (type) {
      case 'gemini':
        return 'default'
      case 'claude':
        return 'secondary'
      case 'openai':
        return 'outline'
      default:
        return 'secondary'
    }
  }

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <div className="text-muted-foreground">加载中...</div>
      </div>
    )
  }

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
      <AdminPageShell
        title="可用模型"
        description="按渠道类型查看当前可用模型"
        actions={
          isAdmin ? (
            <Button onClick={handleFetchAll} disabled={fetching}>
              {fetching ? '获取中...' : '刷新所有模型'}
            </Button>
          ) : undefined
        }
      >
        {error && (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {fetchResult && (
          <Alert>
            <AlertDescription className="space-y-1">
              <p className="font-medium">{fetchResult.message}</p>
              {Object.entries(fetchResult.results).map(([name, count]) => (
                <p key={name} className="text-sm text-muted-foreground">
                  {name}: {count === -1 ? '获取失败' : `${count} 个模型`}
                </p>
              ))}
            </AlertDescription>
          </Alert>
        )}

        <motion.div initial={{ opacity: 0, y: 24 }} animate={{ opacity: 1, y: 0 }} transition={{ type: 'spring', bounce: 0.2, duration: 0.55 }}>
          <AdminSurface>
            <AdminToolbarRow>
              <div className="space-y-1">
                <p className="text-sm font-medium text-foreground">共 {aggregatedModels.length} 个模型</p>
                <p className="admin-inline-note">同一模型 ID 的不同渠道已合并展示，当前列表按渠道类型分组。</p>
              </div>
              <select
                value={filter}
                onChange={(e) => setFilter(e.target.value as typeof filter)}
                className="h-9 rounded-md border border-input bg-background px-3 text-sm ring-offset-background focus:outline-none focus:ring-2 focus:ring-ring"
              >
                <option value="all">全部类型</option>
                <option value="openai">OpenAI</option>
                <option value="claude">Claude</option>
                <option value="gemini">Gemini</option>
              </select>
            </AdminToolbarRow>

            {models.length === 0 ? (
              <div className="admin-surface-body">
                <div className="rounded-xl border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">
                  {isAdmin ? '暂无模型数据，请先添加渠道后刷新。' : '暂无可用模型。'}
                </div>
              </div>
            ) : (
              <div className="admin-list-divider">
                {Object.entries(groupedModels).map(([type, typeModels]) => (
                  <motion.section
                    key={type}
                    initial={{ opacity: 0, y: 16 }}
                    animate={{ opacity: 1, y: 0 }}
                    transition={{ type: 'spring', bounce: 0.18, duration: 0.45 }}
                    className="px-5 py-4"
                  >
                    <div className="mb-3 flex items-center justify-between gap-3">
                      <div className="flex items-center gap-2">
                        <Badge variant={getTypeBadgeVariant(type)}>{type.toUpperCase()}</Badge>
                        <span className="text-sm text-muted-foreground">{typeModels.length} 个模型</span>
                      </div>
                    </div>
                    <div className="overflow-hidden rounded-xl border border-border/70">
                      {typeModels.map((model, index) => (
                        <div
                          key={`${model.channelType}-${model.modelId}`}
                          className={`grid gap-2 px-4 py-3 text-sm md:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_150px] md:items-center ${
                            index === 0 ? '' : 'border-t border-border/70'
                          }`}
                        >
                          <div className="min-w-0 space-y-1">
                            <p className="truncate font-mono text-sm text-foreground">{model.modelId}</p>
                            {model.displayName !== model.modelId ? (
                              <p className="truncate text-xs text-muted-foreground">{model.displayName}</p>
                            ) : null}
                          </div>
                          <div className="min-w-0 space-y-1">
                            <p className="text-sm text-muted-foreground">
                              {model.channelNames.length} 个渠道
                            </p>
                            <p className="truncate text-xs text-muted-foreground" title={model.channelNames.join('、')}>
                              {model.channelNames.join('、')}
                            </p>
                          </div>
                          <p className="text-xs uppercase tracking-[0.14em] text-muted-foreground md:text-right">
                            {model.channelType}
                          </p>
                        </div>
                      ))}
                    </div>
                  </motion.section>
                ))}
              </div>
            )}
          </AdminSurface>
        </motion.div>
      </AdminPageShell>
    </motion.div>
  )
}
