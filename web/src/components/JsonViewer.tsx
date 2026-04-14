import { useMemo, useState, useCallback } from 'react'

interface JsonViewerProps {
  content: string
  maxHeight?: string
}

// VSCode Dark+ 风格 JSON token 类型
type TokenType = 'key' | 'string' | 'number' | 'boolean' | 'null' | 'punctuation'

interface Token {
  type: TokenType | 'plain'
  value: string
}

// 颜色映射（VSCode Dark+ 主题）
const tokenColors: Record<TokenType, string> = {
  key: 'text-sky-400',          // #9cdcfe — 属性名
  string: 'text-amber-300',     // #ce9178 — 字符串值
  number: 'text-green-400',     // #b5cea8 — 数字
  boolean: 'text-blue-400',     // #569cd6 — 布尔值
  null: 'text-blue-400',        // #569cd6 — null
  punctuation: 'text-zinc-400', // 括号、逗号、冒号
}

function tokenizeLine(line: string): Token[] {
  const tokens: Token[] = []
  let remaining = line

  // 缩进
  const indentMatch = remaining.match(/^(\s+)/)
  if (indentMatch) {
    tokens.push({ type: 'plain', value: indentMatch[1] })
    remaining = remaining.slice(indentMatch[1].length)
  }

  while (remaining.length > 0) {
    // JSON key: "key":
    const keyMatch = remaining.match(/^("(?:[^"\\]|\\.)*")\s*(:)/)
    if (keyMatch) {
      tokens.push({ type: 'key', value: keyMatch[1] })
      tokens.push({ type: 'punctuation', value: keyMatch[2] })
      remaining = remaining.slice(keyMatch[0].length)
      // 冒号后的空格
      const spaceAfter = remaining.match(/^(\s+)/)
      if (spaceAfter) {
        tokens.push({ type: 'plain', value: spaceAfter[1] })
        remaining = remaining.slice(spaceAfter[1].length)
      }
      continue
    }

    // 字符串值
    const strMatch = remaining.match(/^("(?:[^"\\]|\\.)*")(,?)/)
    if (strMatch) {
      tokens.push({ type: 'string', value: strMatch[1] })
      if (strMatch[2]) tokens.push({ type: 'punctuation', value: strMatch[2] })
      remaining = remaining.slice(strMatch[0].length)
      continue
    }

    // 数字
    const numMatch = remaining.match(/^(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)(,?)/)
    if (numMatch) {
      tokens.push({ type: 'number', value: numMatch[1] })
      if (numMatch[2]) tokens.push({ type: 'punctuation', value: numMatch[2] })
      remaining = remaining.slice(numMatch[0].length)
      continue
    }

    // 布尔值
    const boolMatch = remaining.match(/^(true|false)(,?)/)
    if (boolMatch) {
      tokens.push({ type: 'boolean', value: boolMatch[1] })
      if (boolMatch[2]) tokens.push({ type: 'punctuation', value: boolMatch[2] })
      remaining = remaining.slice(boolMatch[0].length)
      continue
    }

    // null
    const nullMatch = remaining.match(/^(null)(,?)/)
    if (nullMatch) {
      tokens.push({ type: 'null', value: nullMatch[1] })
      if (nullMatch[2]) tokens.push({ type: 'punctuation', value: nullMatch[2] })
      remaining = remaining.slice(nullMatch[0].length)
      continue
    }

    // 括号
    const punctMatch = remaining.match(/^([{}[\],])/)
    if (punctMatch) {
      tokens.push({ type: 'punctuation', value: punctMatch[1] })
      remaining = remaining.slice(1)
      continue
    }

    // 其他字符
    tokens.push({ type: 'plain', value: remaining[0] })
    remaining = remaining.slice(1)
  }

  return tokens
}

export function JsonViewer({ content, maxHeight = '55vh' }: JsonViewerProps) {
  const [collapsed, setCollapsed] = useState<Set<number>>(new Set())

  // 格式化并分析 JSON 结构
  const { lines, foldableLines, foldEndMap } = useMemo(() => {
    let formatted: string
    try {
      formatted = JSON.stringify(JSON.parse(content), null, 2)
    } catch {
      formatted = content
    }

    const rawLines = formatted.split('\n')
    const foldable = new Set<number>()
    const endMap = new Map<number, number>()

    // 查找可折叠区域（含 { 或 [ 的行）
    const stack: number[] = []
    rawLines.forEach((line, i) => {
      const trimmed = line.trimEnd().replace(/,\s*$/, '')
      if (trimmed.endsWith('{') || trimmed.endsWith('[')) {
        stack.push(i)
      }
      if (trimmed === '}' || trimmed === ']') {
        const start = stack.pop()
        if (start !== undefined && i - start > 1) {
          foldable.add(start)
          endMap.set(start, i)
        }
      }
    })

    return { lines: rawLines, foldableLines: foldable, foldEndMap: endMap }
  }, [content])

  const toggleFold = useCallback((lineNum: number) => {
    setCollapsed(prev => {
      const next = new Set(prev)
      if (next.has(lineNum)) {
        next.delete(lineNum)
      } else {
        next.add(lineNum)
      }
      return next
    })
  }, [])

  // 计算哪些行被隐藏
  const hiddenLines = useMemo(() => {
    const hidden = new Set<number>()
    collapsed.forEach(start => {
      const end = foldEndMap.get(start)
      if (end !== undefined) {
        for (let i = start + 1; i < end; i++) {
          hidden.add(i)
        }
      }
    })
    return hidden
  }, [collapsed, foldEndMap])

  const lineNumberWidth = String(lines.length).length

  return (
    <div
      className="rounded-lg border border-border bg-[#1e1e1e] overflow-auto font-mono text-[13px] leading-5"
      style={{ maxHeight }}
    >
      <div className="flex">
        {/* 行号栏 */}
        <div className="select-none shrink-0 border-r border-zinc-700/50 bg-[#1e1e1e] sticky left-0 z-10">
          {lines.map((_, i) => {
            if (hiddenLines.has(i)) return null
            return (
              <div
                key={i}
                className="px-3 text-right text-zinc-600 text-xs leading-5 hover:text-zinc-400 relative group"
                style={{ minWidth: `${lineNumberWidth * 0.6 + 1.8}rem` }}
              >
                {foldableLines.has(i) ? (
                  <button
                    onClick={() => toggleFold(i)}
                    className="w-full text-right cursor-pointer hover:text-zinc-300"
                    title={collapsed.has(i) ? '展开' : '折叠'}
                  >
                    <span className="opacity-0 group-hover:opacity-100 absolute left-1 text-zinc-500">
                      {collapsed.has(i) ? '▶' : '▼'}
                    </span>
                    {i + 1}
                  </button>
                ) : (
                  i + 1
                )}
              </div>
            )
          })}
        </div>

        {/* 代码区 */}
        <div className="flex-1 overflow-x-auto">
          <pre className="p-0 m-0">
            {lines.map((line, i) => {
              if (hiddenLines.has(i)) return null

              const tokens = tokenizeLine(line)
              const isFolded = collapsed.has(i)

              return (
                <div
                  key={i}
                  className="px-4 leading-5 hover:bg-zinc-800/50"
                >
                  {tokens.map((token, j) => (
                    <span
                      key={j}
                      className={token.type === 'plain' ? 'text-zinc-300' : tokenColors[token.type]}
                    >
                      {token.value}
                    </span>
                  ))}
                  {isFolded && (
                    <span
                      className="text-zinc-500 cursor-pointer hover:text-zinc-300 bg-zinc-800 rounded px-1.5 py-0.5 mx-1 text-xs border border-zinc-700"
                      onClick={() => toggleFold(i)}
                    >
                      {foldEndMap.get(i)! - i - 1} lines...
                    </span>
                  )}
                </div>
              )
            })}
          </pre>
        </div>
      </div>
    </div>
  )
}
