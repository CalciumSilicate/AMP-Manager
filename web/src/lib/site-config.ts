export const DEFAULT_SITE_TIME_ZONE = 'Asia/Shanghai'

export const SITE_TIME_ZONE_OPTIONS = [
  { value: 'Asia/Shanghai', label: 'Asia/Shanghai (UTC+08:00)' },
  { value: 'Asia/Tokyo', label: 'Asia/Tokyo (UTC+09:00)' },
  { value: 'UTC', label: 'UTC (UTC+00:00)' },
  { value: 'America/Los_Angeles', label: 'America/Los_Angeles (UTC-07:00 / -08:00)' },
  { value: 'America/New_York', label: 'America/New_York (UTC-04:00 / -05:00)' },
  { value: 'Europe/London', label: 'Europe/London (UTC+00:00 / +01:00)' },
  { value: 'Europe/Berlin', label: 'Europe/Berlin (UTC+01:00 / +02:00)' },
] as const

export function normalizeSiteTimeZone(value?: string | null): string {
  const trimmed = value?.trim()
  return trimmed || DEFAULT_SITE_TIME_ZONE
}

export function getActiveSiteTimeZone(): string {
  if (typeof document !== 'undefined') {
    const current = document.documentElement.dataset.siteTimeZone
    if (current) {
      return current
    }
  }

  if (typeof window !== 'undefined') {
    const stored = window.localStorage.getItem('siteTimeZone')
    if (stored) {
      return stored
    }
  }

  return DEFAULT_SITE_TIME_ZONE
}

export function setActiveSiteTimeZone(value?: string | null): string {
  const timeZone = normalizeSiteTimeZone(value)

  if (typeof document !== 'undefined') {
    document.documentElement.dataset.siteTimeZone = timeZone
  }

  if (typeof window !== 'undefined') {
    window.localStorage.setItem('siteTimeZone', timeZone)
  }

  return timeZone
}
