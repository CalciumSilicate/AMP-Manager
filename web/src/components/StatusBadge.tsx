import { Badge } from '@/components/ui/badge'

interface StatusBadgeProps {
  status: number
}

export function StatusBadge({ status }: StatusBadgeProps) {
  if (status === 0) {
    return <Badge variant="secondary" className="animate-pulse">请求中</Badge>
  }
  if (status >= 200 && status < 300) {
    return <Badge variant="default" className="bg-green-500">{status}</Badge>
  } else if (status >= 400 && status < 500) {
    return <Badge variant="outline" className="border-amber-300 bg-amber-100 text-amber-800 dark:border-amber-700 dark:bg-amber-950 dark:text-amber-300">{status}</Badge>
  } else if (status >= 500) {
    return <Badge variant="destructive" className="bg-red-700">{status}</Badge>
  }
  return <Badge variant="secondary">{status}</Badge>
}
