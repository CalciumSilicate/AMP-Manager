import { useState, useEffect } from 'react'
import { motion } from '@/lib/motion'
import { listAvailableModels, fetchAllModels, AvailableModel, FetchModelsResult } from '../api/models'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertDescription } from '@/components/ui/alert'

interface Props {
  isAdmin: boolean
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

  const filteredModels = filter === 'all' 
    ? models 
    : models.filter(m => m.channelType === filter)

  const groupedModels = filteredModels.reduce((acc, model) => {
    const key = model.channelType
    if (!acc[key]) acc[key] = []
    acc[key].push(model)
    return acc
  }, {} as Record<string, AvailableModel[]>)

  const getTypeBadgeVariant = (type: string): "default" | "secondary" | "destructive" | "outline" => {
    switch (type) {
      case 'gemini': return 'default'
      case 'claude': return 'secondary'
      case 'openai': return 'outline'
      default: return 'secondary'
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
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="mx-auto max-w-6xl space-y-6">
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {fetchResult && (
        <Alert>
          <AlertDescription>
            <p className="font-medium">{fetchResult.message}</p>
            <div className="mt-2 text-sm">
              {Object.entries(fetchResult.results).map(([name, count]) => (
                <div key={name}>
                  {name}: {count === -1 ? '获取失败' : `${count} 个模型`}
                </div>
              ))}
            </div>
          </AlertDescription>
        </Alert>
      )}

      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>可用模型</CardTitle>
              <CardDescription>
                共 {models.length} 个模型可用
              </CardDescription>
            </div>
            <div className="flex items-center gap-4">
              <select
                value={filter}
                onChange={(e) => setFilter(e.target.value as typeof filter)}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus:outline-none focus:ring-2 focus:ring-ring"
              >
                <option value="all">全部类型</option>
                <option value="openai">OpenAI</option>
                <option value="claude">Claude</option>
                <option value="gemini">Gemini</option>
              </select>
              {isAdmin && (
                <Button onClick={handleFetchAll} disabled={fetching}>
                  {fetching ? '获取中...' : '刷新所有模型'}
                </Button>
              )}
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {models.length === 0 ? (
            <div className="rounded-md border border-dashed p-8 text-center text-muted-foreground">
              {isAdmin ? (
                <div>
                  <p>暂无模型数据</p>
                  <p className="mt-2 text-sm">请先在渠道管理中添加渠道，然后点击"刷新所有模型"获取模型列表</p>
                </div>
              ) : (
                <p>暂无可用模型，请联系管理员配置</p>
              )}
            </div>
          ) : (
            <div className="space-y-6">
              {Object.entries(groupedModels).map(([type, typeModels]) => (
                <motion.div key={type} initial={{ opacity: 0, x: -20 }} animate={{ opacity: 1, x: 0 }} transition={{ type: 'spring', bounce: 0.2, duration: 0.5 }}>
                  <h3 className="mb-3 flex items-center gap-2 text-lg font-medium">
                    <Badge variant={getTypeBadgeVariant(type)}>
                      {type.toUpperCase()}
                    </Badge>
                    <span className="text-sm text-muted-foreground">({typeModels.length} 个模型)</span>
                  </h3>
                  <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                    {typeModels.map((model, index) => (
                      <motion.div
                        key={`${model.channelName}-${model.modelId}`}
                        initial={{ opacity: 0, y: 20, scale: 0.95 }}
                        animate={{ opacity: 1, y: 0, scale: 1 }}
                        transition={{ type: 'spring', bounce: 0.3, duration: 0.5, delay: index * 0.03 }}
                        whileHover={{ scale: 1.04, y: -4 }}
                        whileTap={{ scale: 0.97 }}
                      >
                        <Card className="hover:bg-muted/50 transition-colors h-full">
                        <CardContent className="p-4">
                          <div className="font-mono text-sm font-medium">
                            {model.modelId}
                          </div>
                          {model.displayName !== model.modelId && (
                            <div className="mt-1 text-xs text-muted-foreground">
                              {model.displayName}
                            </div>
                          )}
                          <div className="mt-2 text-xs text-muted-foreground">
                            渠道: {model.channelName}
                          </div>
                        </CardContent>
                        </Card>
                      </motion.div>
                    ))}
                  </div>
                </motion.div>
              ))}
            </div>
          )}

          <Card className="mt-6 bg-muted/50">
            <CardContent className="p-4 text-sm text-muted-foreground">
              <p className="font-medium">说明</p>
              <ul className="mt-2 list-inside list-disc space-y-1">
                <li>模型列表来自各渠道的 API 原始返回结果</li>
                <li>页面中的 OpenAI、Claude、Gemini 分组表示模型来源渠道类型，不再额外筛掉模型名</li>
              </ul>
            </CardContent>
          </Card>
        </CardContent>
      </Card>
    </motion.div>
  )
}
