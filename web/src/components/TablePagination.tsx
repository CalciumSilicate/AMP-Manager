import { PageSizeSlider } from '@/components/PageSizeSlider'
import { Button } from '@/components/ui/button'

interface TablePaginationProps {
  page: number
  pageSize: number
  total: number
  onPageChange: (page: number) => void
  onPageSizeChange: (pageSize: number) => void
  totalLabel?: string
}

export function TablePagination({
  page,
  pageSize,
  total,
  onPageChange,
  onPageSizeChange,
  totalLabel = '条',
}: TablePaginationProps) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  return (
    <div className="mt-4 flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
      <div className="flex flex-col gap-3 md:flex-row md:items-center md:gap-4">
        <p className="text-sm text-muted-foreground">
          第 {page} 页，共 {totalPages} 页（{total} {totalLabel}）
        </p>
        <PageSizeSlider value={pageSize} onChange={onPageSizeChange} />
      </div>
      <div className="flex gap-2 self-end md:self-auto">
        <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => onPageChange(page - 1)}>
          上一页
        </Button>
        <Button variant="outline" size="sm" disabled={page >= totalPages} onClick={() => onPageChange(page + 1)}>
          下一页
        </Button>
      </div>
    </div>
  )
}
