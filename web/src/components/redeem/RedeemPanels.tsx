import { useEffect, useState } from 'react'

import { listMyRedeemRecords, redeemCode, type RedeemRedemption, type RedeemResult } from '@/api/redeem'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { TablePagination } from '@/components/TablePagination'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { formatDateTime, formatDecimal } from '@/lib/formatters'
import { CheckCircle2, CircleAlert, RefreshCw } from 'lucide-react'

function microsToUsd(value: number): string {
  return `$${formatDecimal(value / 1_000_000, 2)}`
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
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const handleSubmit = async () => {
    if (!code.trim()) return
    setSubmitting(true)
    try {
      const result = await redeemCode(code.trim())
      setCode('')
      setMessage({ type: 'success', text: result.message })
      await onSuccess?.(result)
    } catch (error) {
      setMessage({ type: 'error', text: error instanceof Error ? error.message : '兑换失败' })
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className={`space-y-3 ${className}`}>
      {message && (
        <Alert variant={message.type === 'error' ? 'destructive' : 'default'}>
          {message.type === 'error' ? <CircleAlert className="h-4 w-4" /> : <CheckCircle2 className="h-4 w-4" />}
          <AlertDescription>{message.text}</AlertDescription>
        </Alert>
      )}
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
      <Table>
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
