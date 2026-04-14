import { Channel, TestChannelResult } from '@/api/channels'

interface TestResultsDisplayProps {
  testResults: Record<string, TestChannelResult>
  channels: Channel[]
}

export function TestResultsDisplay({ testResults, channels }: TestResultsDisplayProps) {
  if (Object.keys(testResults).length === 0) {
    return null
  }

  return (
    <div className="admin-list-divider rounded-md border border-border/70">
      {Object.entries(testResults).map(([id, result]) => {
        const channel = channels.find((item) => item.id === id)
        return (
          <div key={id} className="flex flex-col gap-1 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="space-y-0.5">
              <p className="text-sm font-medium text-foreground">{channel?.name || '未知渠道'}</p>
              <p className="text-xs text-muted-foreground">{result.message}</p>
            </div>
            <div className="flex items-center gap-2 text-xs">
              <span
                className={`rounded-md px-2 py-1 font-medium ${
                  result.success
                    ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
                    : 'bg-destructive/10 text-destructive'
                }`}
              >
                {result.success ? '测试成功' : '测试失败'}
              </span>
              {result.latencyMs ? <span className="text-muted-foreground">{result.latencyMs}ms</span> : null}
            </div>
          </div>
        )
      })}
    </div>
  )
}
