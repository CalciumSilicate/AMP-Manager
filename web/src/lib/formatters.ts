export function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleString('zh-CN')
}

export function formatNumber(num: number | undefined): string {
  if (num === undefined || num === null) return '-'
  return num.toLocaleString()
}

export function formatCompact(num: number | undefined): string {
  if (num === undefined || num === null) return '-'
  const abs = Math.abs(num)
  if (abs >= 1_000_000_000) return (num / 1_000_000_000).toFixed(1).replace(/\.0$/, '') + 'B'
  if (abs >= 1_000_000) return (num / 1_000_000).toFixed(1).replace(/\.0$/, '') + 'M'
  if (abs >= 1_000) return (num / 1_000).toFixed(1).replace(/\.0$/, '') + 'K'
  return num.toString()
}

export function formatExact(num: number | undefined): string {
  if (num === undefined || num === null) return '-'
  return num.toLocaleString()
}

export function formatInteger(num: number | undefined): string {
  if (num === undefined || num === null) return '-'
  return Math.trunc(num).toLocaleString('en-US')
}

export function formatDecimal(value: number | undefined, fractionDigits: number): string {
  if (value === undefined || value === null) return '-'
  return value.toLocaleString('en-US', {
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits,
  })
}

export function formatGroupedNumericString(value: string | undefined): string {
  if (!value) return '-'

  const trimmed = value.trim()
  if (!trimmed) return '-'

  const sign = trimmed.startsWith('-') ? '-' : ''
  const unsigned = sign ? trimmed.slice(1) : trimmed
  const [integerPartRaw, fractionPart] = unsigned.split('.')
  const integerValue = Number(integerPartRaw || '0')
  const groupedInteger = Number.isFinite(integerValue)
    ? Math.trunc(integerValue).toLocaleString('en-US')
    : integerPartRaw

  return fractionPart !== undefined && fractionPart !== ''
    ? `${sign}${groupedInteger}.${fractionPart}`
    : `${sign}${groupedInteger}`
}
