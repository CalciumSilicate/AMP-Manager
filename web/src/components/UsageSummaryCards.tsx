import { Card, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Num } from '@/components/Num'
import { UsageSummary } from '@/api/amp'
import { motion } from '@/lib/motion'
import { formatGroupedNumericString } from '@/lib/formatters'

interface UsageSummaryCardsProps {
  summary: UsageSummary[]
}

export function UsageSummaryCards({ summary }: UsageSummaryCardsProps) {
  const totalRequests = summary.reduce((acc, s) => acc + s.requestCount, 0)
  const totalInputTokens = summary.reduce((acc, s) => acc + s.inputTokensSum, 0)
  const totalOutputTokens = summary.reduce((acc, s) => acc + s.outputTokensSum, 0)
  const totalCacheRead = summary.reduce((acc, s) => acc + s.cacheReadInputTokensSum, 0)
  const totalCacheWrite = summary.reduce((acc, s) => acc + s.cacheCreationInputTokensSum, 0)
  const totalCostMicros = summary.reduce((acc, s) => acc + (s.costMicrosSum || 0), 0)
  const totalCostUsd = (totalCostMicros / 1_000_000).toFixed(6)

  return (
    <motion.div variants={{ hidden: { opacity: 0 }, visible: { opacity: 1, transition: { staggerChildren: 0.12, delayChildren: 0.1 } } }} initial="hidden" animate="visible" className="grid gap-4 md:grid-cols-6">
      <motion.div variants={{ hidden: { opacity: 0, y: 30, scale: 0.9 }, visible: { opacity: 1, y: 0, scale: 1, transition: { type: 'spring', bounce: 0.35, duration: 0.6 } } }} whileHover={{ scale: 1.05, y: -6 }} whileTap={{ scale: 0.97 }}>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>总请求数</CardDescription>
            <CardTitle className="text-2xl"><Num value={totalRequests} compact={false} /></CardTitle>
          </CardHeader>
        </Card>
      </motion.div>
      <motion.div variants={{ hidden: { opacity: 0, y: 30, scale: 0.9 }, visible: { opacity: 1, y: 0, scale: 1, transition: { type: 'spring', bounce: 0.35, duration: 0.6 } } }} whileHover={{ scale: 1.05, y: -6 }} whileTap={{ scale: 0.97 }}>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>输入 Tokens(去缓存)</CardDescription>
            <CardTitle className="text-2xl"><Num value={totalInputTokens} compact={false} /></CardTitle>
          </CardHeader>
        </Card>
      </motion.div>
      <motion.div variants={{ hidden: { opacity: 0, y: 30, scale: 0.9 }, visible: { opacity: 1, y: 0, scale: 1, transition: { type: 'spring', bounce: 0.35, duration: 0.6 } } }} whileHover={{ scale: 1.05, y: -6 }} whileTap={{ scale: 0.97 }}>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>输出 Tokens</CardDescription>
            <CardTitle className="text-2xl"><Num value={totalOutputTokens} compact={false} /></CardTitle>
          </CardHeader>
        </Card>
      </motion.div>
      <motion.div variants={{ hidden: { opacity: 0, y: 30, scale: 0.9 }, visible: { opacity: 1, y: 0, scale: 1, transition: { type: 'spring', bounce: 0.35, duration: 0.6 } } }} whileHover={{ scale: 1.05, y: -6 }} whileTap={{ scale: 0.97 }}>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>缓存读取</CardDescription>
            <CardTitle className="text-2xl"><Num value={totalCacheRead} compact={false} /></CardTitle>
          </CardHeader>
        </Card>
      </motion.div>
      <motion.div variants={{ hidden: { opacity: 0, y: 30, scale: 0.9 }, visible: { opacity: 1, y: 0, scale: 1, transition: { type: 'spring', bounce: 0.35, duration: 0.6 } } }} whileHover={{ scale: 1.05, y: -6 }} whileTap={{ scale: 0.97 }}>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>缓存写入</CardDescription>
            <CardTitle className="text-2xl"><Num value={totalCacheWrite} compact={false} /></CardTitle>
          </CardHeader>
        </Card>
      </motion.div>
      <motion.div variants={{ hidden: { opacity: 0, y: 30, scale: 0.9 }, visible: { opacity: 1, y: 0, scale: 1, transition: { type: 'spring', bounce: 0.35, duration: 0.6 } } }} whileHover={{ scale: 1.05, y: -6 }} whileTap={{ scale: 0.97 }}>
        <Card className="bg-gradient-to-br from-green-500/10 to-emerald-500/10 border-green-500/20">
          <CardHeader className="pb-2">
            <CardDescription>总成本</CardDescription>
            <CardTitle className="text-2xl text-green-600 dark:text-green-400">
              ${formatGroupedNumericString(totalCostUsd)}
            </CardTitle>
          </CardHeader>
        </Card>
      </motion.div>
    </motion.div>
  )
}
