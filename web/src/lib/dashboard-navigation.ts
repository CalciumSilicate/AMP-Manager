export const DASHBOARD_NAVIGATE_EVENT = 'dashboard:navigate'

export function navigateDashboard(page: string): void {
  window.dispatchEvent(new CustomEvent(DASHBOARD_NAVIGATE_EVENT, { detail: { page } }))
}
