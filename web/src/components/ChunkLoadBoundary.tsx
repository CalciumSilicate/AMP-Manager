import { Component, type ErrorInfo, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'

interface ChunkLoadBoundaryProps {
  children: ReactNode
  scopeLabel?: string
  secondaryActionLabel?: string
  onSecondaryAction?: () => void
}

interface ChunkLoadBoundaryState {
  error: Error | null
}

function isChunkLoadError(error: Error) {
  const message = error.message.toLowerCase()
  return (
    message.includes('failed to fetch dynamically imported module') ||
    message.includes('importing a module script failed') ||
    message.includes('dynamically imported module')
  )
}

export class ChunkLoadBoundary extends Component<ChunkLoadBoundaryProps, ChunkLoadBoundaryState> {
  state: ChunkLoadBoundaryState = {
    error: null,
  }

  static getDerivedStateFromError(error: Error): ChunkLoadBoundaryState {
    return { error }
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error('chunk load failed', {
      scope: this.props.scopeLabel,
      error,
      componentStack: errorInfo.componentStack,
    })
  }

  render() {
    const { error } = this.state
    if (!error) {
      return this.props.children
    }

    const chunkLoadFailed = isChunkLoadError(error)
    const title = chunkLoadFailed ? '页面资源加载失败' : '页面渲染失败'
    const description = chunkLoadFailed
      ? `${this.props.scopeLabel || '当前页面'} 依赖的静态资源未成功加载，通常和发布切换、缓存错位或静态资源异常有关。`
      : `${this.props.scopeLabel || '当前页面'} 渲染时发生错误。`

    return (
      <div className="mx-auto max-w-3xl rounded-2xl border bg-background px-6 py-8 shadow-sm">
        <div className="space-y-3">
          <h2 className="text-lg font-semibold text-foreground">{title}</h2>
          <p className="text-sm leading-6 text-muted-foreground">{description}</p>
          <div className="rounded-lg bg-muted/60 px-3 py-2 text-xs text-muted-foreground">
            {error.message}
          </div>
        </div>
        <div className="mt-5 flex flex-wrap gap-3">
          {this.props.onSecondaryAction ? (
            <Button variant="outline" onClick={this.props.onSecondaryAction}>
              {this.props.secondaryActionLabel || '返回上一页'}
            </Button>
          ) : null}
          <Button
            onClick={() => {
              window.location.reload()
            }}
          >
            刷新页面
          </Button>
        </div>
      </div>
    )
  }
}
