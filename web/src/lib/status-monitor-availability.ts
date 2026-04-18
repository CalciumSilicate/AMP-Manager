export const STATUS_MONITOR_AVAILABILITY_EVENT = 'status-monitor:availability'

export function dispatchStatusMonitorAvailability(available: boolean): void {
  if (typeof window === 'undefined') return

  window.dispatchEvent(new CustomEvent(STATUS_MONITOR_AVAILABILITY_EVENT, {
    detail: { available },
  }))
}
