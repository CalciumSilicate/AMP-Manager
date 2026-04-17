import { useCallback, useDeferredValue, useEffect, useMemo, useState } from 'react'

import { AdminPageShell, AdminSurface } from '@/components/admin/AdminPageShell'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
        description="只读排障"
        width="full"
      >
        {listError ? (
          <Alert variant="destructive">
            <AlertDescription>{listError}</AlertDescription>
          </Alert>
        ) : null}

        <AdminSurface className="overflow-hidden">
          <div className="grid xl:grid-cols-[320px_minmax(0,1fr)_280px] xl:divide-x xl:divide-border/70">
            <SessionListColumn
              items={items}
              total={total}
              page={page}
              pageSize={DEFAULT_PAGE_SIZE}
              queryInput={queryInput}
              loading={loading}
              fetching={fetching}
              selectedSessionId={selectedSessionId}
              onQueryInputChange={setQueryInput}
              onSearch={handleSearchSubmit}
              onRefresh={handleRefresh}
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
              onRefresh={loadLeaderboard}
            />
          </div>
        </AdminSurface>
      </AdminPageShell>
    </motion.div>
  )
}
