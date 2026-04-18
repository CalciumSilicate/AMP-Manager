import { useCallback, useDeferredValue, useEffect, useMemo, useState } from 'react'

import { AdminPageShell } from '@/components/admin/AdminPageShell'
import { PageStat, PageStatStrip, PageSurface } from '@/components/layout/PageScaffold'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { motion } from '@/lib/motion'
import {
  getAdminSessionDetail,
  getAdminSessionLeaderboard,
  getAdminSessions,
  type SessionDetail,
  type SessionLeaderboardItem,
  type SessionListItem,
} from '@/api/sessions'
import { SessionDetailColumn } from '@/components/sessions/SessionDetailColumn'
import { SessionLeaderboardColumn } from '@/components/sessions/SessionLeaderboardColumn'
import { SessionListColumn } from '@/components/sessions/SessionListColumn'
import { RefreshCw } from 'lucide-react'

const DEFAULT_PAGE_SIZE = 20

export default function SessionManagement() {
  const [items, setItems] = useState<SessionListItem[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [queryInput, setQueryInput] = useState('')
  const [query, setQuery] = useState('')
  const [loading, setLoading] = useState(true)
  const [fetching, setFetching] = useState(false)
  const [listError, setListError] = useState('')
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null)

  const [detail, setDetail] = useState<SessionDetail | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailError, setDetailError] = useState('')
  const [detailSearchValue, setDetailSearchValue] = useState('')
  const [searchResults, setSearchResults] = useState<SessionListItem[]>([])
  const [searchLoading, setSearchLoading] = useState(false)

  const [leaderboard, setLeaderboard] = useState<SessionLeaderboardItem[]>([])
  const [leaderboardLoading, setLeaderboardLoading] = useState(true)

  const deferredDetailSearch = useDeferredValue(detailSearchValue.trim())

  const loadSessions = useCallback(async (targetPage = page, targetQuery = query) => {
    if (loading) {
      setLoading(true)
    } else {
      setFetching(true)
    }

    setListError('')
    try {
      const result = await getAdminSessions({
        page: targetPage,
        pageSize: DEFAULT_PAGE_SIZE,
        query: targetQuery || undefined,
      })

      setItems(result.items)
      setTotal(result.total)
      setSearchResults((current) => (detailSearchValue.trim() ? current : result.items.slice(0, 6)))
      setSelectedSessionId((current) => current || result.items[0]?.sessionId || null)
    } catch (error) {
      setListError(error instanceof Error ? error.message : '获取 Session 列表失败')
    } finally {
      setLoading(false)
      setFetching(false)
    }
  }, [detailSearchValue, loading, page, query])

  const loadLeaderboard = useCallback(async () => {
    setLeaderboardLoading(true)
    try {
      const result = await getAdminSessionLeaderboard({ windowMinutes: 5, limit: 10 })
      setLeaderboard(result)
    } catch {
      setLeaderboard([])
    } finally {
      setLeaderboardLoading(false)
    }
  }, [])

  useEffect(() => {
    void loadSessions(page, query)
  }, [loadSessions, page, query])

  useEffect(() => {
    void loadLeaderboard()
  }, [loadLeaderboard])

  useEffect(() => {
    if (!selectedSessionId) {
      setDetail(null)
      return
    }

    setDetailLoading(true)
    setDetailError('')

    getAdminSessionDetail(selectedSessionId)
      .then(setDetail)
      .catch((error) => {
        setDetail(null)
        setDetailError(error instanceof Error ? error.message : '获取 Session 详情失败')
      })
      .finally(() => {
        setDetailLoading(false)
      })
  }, [selectedSessionId])

  useEffect(() => {
    if (!deferredDetailSearch) {
      setSearchResults(items.slice(0, 6))
      setSearchLoading(false)
      return
    }

    setSearchLoading(true)
    getAdminSessions({ page: 1, pageSize: 6, query: deferredDetailSearch })
      .then((result) => {
        setSearchResults(result.items)
      })
      .catch(() => {
        setSearchResults([])
      })
      .finally(() => {
        setSearchLoading(false)
      })
  }, [deferredDetailSearch, items])

  const handleSearchSubmit = useCallback(() => {
    setPage(1)
    setQuery(queryInput.trim())
  }, [queryInput])

  const handleClearSearch = useCallback(() => {
    setQueryInput('')
    setPage(1)
    setQuery('')
  }, [])

  const handleRefresh = useCallback(() => {
    void Promise.all([loadSessions(page, query), loadLeaderboard()])
  }, [loadLeaderboard, loadSessions, page, query])

  const searchResultItems = useMemo(() => {
    if (!selectedSessionId || searchResults.some((item) => item.sessionId === selectedSessionId)) {
      return searchResults
    }

    const selectedFromList = items.find((item) => item.sessionId === selectedSessionId)
    return selectedFromList ? [selectedFromList, ...searchResults] : searchResults
  }, [items, searchResults, selectedSessionId])

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
      <AdminPageShell
        title="Session 管理"
        width="full"
        actions={(
          <Button variant="outline" onClick={handleRefresh} disabled={fetching || detailLoading || leaderboardLoading}>
            <RefreshCw className={`mr-2 h-4 w-4 ${(fetching || detailLoading || leaderboardLoading) ? 'animate-spin' : ''}`} />
            刷新
          </Button>
        )}
      >
        {listError ? (
          <Alert variant="destructive">
            <AlertDescription>{listError}</AlertDescription>
          </Alert>
        ) : null}

        <PageStatStrip className="xl:grid-cols-4">
          <PageStat label="Session" value={total} />
          <PageStat
            label="已选"
            value={selectedSessionId ? (
              <span className="block truncate font-mono text-sm">{selectedSessionId}</span>
            ) : '未选'}
          />
          <PageStat label="筛选" value={query || '全部'} />
          <PageStat label="排行" value={leaderboardLoading ? '加载中' : leaderboard.length} />
        </PageStatStrip>

        <PageSurface bodyClassName="space-y-3">
          <div className="flex flex-col gap-3 lg:flex-row lg:items-center">
            <Input
              value={queryInput}
              onChange={(event) => setQueryInput(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  event.preventDefault()
                  handleSearchSubmit()
                }
              }}
              placeholder="搜索 session / 用户"
              className="h-10 flex-1"
            />
            <div className="flex items-center gap-2">
              <Button onClick={handleSearchSubmit}>搜索</Button>
              {query ? (
                <Button variant="ghost" onClick={handleClearSearch}>
                  清除
                </Button>
              ) : null}
            </div>
          </div>
        </PageSurface>

        <div className="grid gap-5 xl:grid-cols-[minmax(300px,340px)_minmax(0,1fr)_minmax(240px,280px)]">
          <SessionListColumn
            items={items}
            total={total}
            page={page}
            pageSize={DEFAULT_PAGE_SIZE}
            loading={loading}
            fetching={fetching}
            selectedSessionId={selectedSessionId}
            onSelect={setSelectedSessionId}
            onPageChange={setPage}
          />
          <SessionDetailColumn
            selectedSessionId={selectedSessionId}
            detail={detail}
            loading={detailLoading}
            error={detailError}
            searchValue={detailSearchValue}
            searchResults={searchResultItems}
            searchLoading={searchLoading}
            onSearchValueChange={setDetailSearchValue}
            onSelectSession={setSelectedSessionId}
          />
          <SessionLeaderboardColumn
            items={leaderboard}
            loading={leaderboardLoading}
          />
        </div>
      </AdminPageShell>
    </motion.div>
  )
}
