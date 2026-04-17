import { type ReactNode, useEffect, useState } from 'react'

import { listMyRedeemRecords, redeemCode, type RedeemRedemption, type RedeemResult } from '@/api/redeem'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { TablePagination } from '@/components/TablePagination'
import { useGlobalToast } from '@/components/ui/use-global-toast'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { formatDateTime, formatDecimal } from '@/lib/formatters'
import { RefreshCw } from 'lucide-react'

function microsToUsd(value: number): string {
  return `$${formatDecimal(value / 1_000_000, 2)}`
}

function MobileInfoRow({
  label,
  value,
}: {
  label: string
  value: ReactNode
}) {
  return (
    <div className="flex items-start justify-between gap-3">
      <span className="text-muted-foreground">{label}</span>
      <div className="min-w-0 text-right text-foreground">{value}</div>
    </div>
  )
}

export function RedeemQuickEntry({
  onSuccess,
  className = '',
}: {
  onSuccess?: (result: RedeemResult) => void | Promise<void>
  className?: string
}) {
  const [code, setCode] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const { showToast } = useGlobalToast()

  const handleSubmit = async () => {
    if (!code.trim()) return
    setSubmitting(true)
    try {
      const result = await redeemCode(code.trim())
      setCode('')
      showToast('success', result.message)
      await onSuccess?.(result)
    } catch (error) {
      showToast('error', error instanceof Error ? error.message : '兑换失败')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className={`space-y-3 ${className}`}>
      <div className="flex flex-col gap-3 sm:flex-row">
        <Input
          value={code}
          onChange={(event) => setCode(event.target.value.toUpperCase())}
          placeholder="输入兑换码"
          className="font-mono"
        />
        <Button onClick={handleSubmit} disabled={submitting || !code.trim()} className="sm:w-[132px]">
          {submitting ? <RefreshCw className="mr-2 h-4 w-4 animate-spin" /> : null}
          立即兑换
        </Button>
      </div>
    </div>
  )
}

export function RedeemRecordTable({
  compact = false,
  refreshKey = 0,
}: {
  compact?: boolean
  refreshKey?: number
}) {
  const [items, setItems] = useState<RedeemRedemption[]>([])
  const [loading, setLoading] = useState(true)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    listMyRedeemRecords()
      .then((records) => {
        if (!cancelled) {
          setItems(records)
        }
      })
      .catch(() => {
        if (!cancelled) {
          setItems([])
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false)
        }
      })

    return () => {
      cancelled = true
    }
  }, [refreshKey])

  useEffect(() => {
    setPage(1)
  }, [refreshKey])

  if (loading) {
    return (
      <div className="flex h-24 items-center justify-center">
        <RefreshCw className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (items.length === 0) {
    return <div className="py-8 text-center text-sm text-muted-foreground">暂无兑换记录。</div>
  }

  const totalPages = Math.max(1, Math.ceil(items.length / pageSize))
  const currentPage = Math.min(page, totalPages)
  const visibleItems = items.slice((currentPage - 1) * pageSize, currentPage * pageSize)

  return (
    <div>
      <div className="space-y-3 md:hidden">
        {visibleItems.map((item) => (
          <div key={item.id} className="rounded-xl border border-border/70 px-4 py-4">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <p className="font-medium">{item.campaignName || '未关联活动'}</p>
                <p className="mt-1 font-mono text-xs text-muted-foreground">{item.codeMask || '-'}</p>
              </div>
              <Badge variant={item.status === 'success' ? 'default' : 'secondary'}>
                {item.status === 'success' ? '成功' : '拒绝'}
              </Badge>
            </div>

            <div className="mt-3 grid gap-2 text-sm">
              <MobileInfoRow label="时间" value={formatDateTime(item.createdAt)} />
              {!compact ? (
                <MobileInfoRow
                  label="奖励"
                  value={(
                    <div className="text-right">
                      <div>{item.subscriptionPlanName ? `${item.subscriptionPlanName} ${item.subscriptionDurationDays}天` : '-'}</div>
                      {item.balanceMicros > 0 ? <div>{microsToUsd(item.balanceMicros)}</div> : null}
                    </div>
                  )}
                />
              ) : null}
              <MobileInfoRow
                label="说明"
                value={
                  item.status === 'success'
                    ? item.grantedExpiresAt
                      ? `到期 ${formatDateTime(item.grantedExpiresAt)}`
                      : item.balanceAfterMicros > 0
                        ? `余额 ${microsToUsd(item.balanceAfterMicros)}`
                        : '-'
                    : item.failureReason || '-'
                }
              />
            </div>
          </div>
        ))}
      </div>

      <Table className="hidden md:table">
        <TableHeader>
          <TableRow>
            <TableHead>时间</TableHead>
            <TableHead>活动</TableHead>
            <TableHead>兑换码</TableHead>
            {!compact ? <TableHead>奖励</TableHead> : null}
            <TableHead>结果</TableHead>
            <TableHead>说明</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {visibleItems.map((item) => (
            <TableRow key={item.id}>
              <TableCell className="text-muted-foreground">{formatDateTime(item.createdAt)}</TableCell>
              <TableCell>{item.campaignName || '-'}</TableCell>
              <TableCell className="font-mono text-xs">{item.codeMask || '-'}</TableCell>
              {!compact ? (
                <TableCell className="text-sm text-muted-foreground">
                  <div>{item.subscriptionPlanName ? `${item.subscriptionPlanName} ${item.subscriptionDurationDays}天` : '-'}</div>
                  {item.balanceMicros > 0 ? <div>{microsToUsd(item.balanceMicros)}</div> : null}
                </TableCell>
              ) : null}
              <TableCell>
                <Badge variant={item.status === 'success' ? 'default' : 'secondary'}>
                  {item.status === 'success' ? '成功' : '拒绝'}
                </Badge>
              </TableCell>
              <TableCell className="text-sm text-muted-foreground">
                {item.status === 'success'
                  ? item.grantedExpiresAt
                    ? `到期 ${formatDateTime(item.grantedExpiresAt)}`
                    : item.balanceAfterMicros > 0
                      ? `余额 ${microsToUsd(item.balanceAfterMicros)}`
                      : '-'
                  : item.failureReason || '-'}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <TablePagination
        page={currentPage}
        pageSize={pageSize}
        total={items.length}
        onPageChange={setPage}
        onPageSizeChange={(nextPageSize) => {
          setPageSize(nextPageSize)
          setPage(1)
        }}
      />
    </div>
  )
}
