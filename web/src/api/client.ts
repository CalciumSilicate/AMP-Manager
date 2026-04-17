/**
 * 统一的认证 HTTP 客户端
 * - 自动注入 Authorization 头
 * - 401 响应自动触发登出
 * - 自动接收刷新的 Token（滑动过期）
 */
export async function authFetch(url: string, options: RequestInit = {}): Promise<Response> {
  const token = localStorage.getItem('token')
  const headers = new Headers(options.headers)

  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }

  // FormData 不设置 Content-Type（浏览器自动设置 boundary）
  if (!headers.has('Content-Type') && !(options.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json')
  }

  const response = await fetch(url, { ...options, headers })

  // 自动接收刷新的 Token
  const newToken = response.headers.get('X-New-Token')
  if (newToken) {
    localStorage.setItem('token', newToken)
  }

  // 401 自动登出
  if (response.status === 401) {
    localStorage.removeItem('token')
    localStorage.removeItem('username')
    localStorage.removeItem('isAdmin')
    localStorage.removeItem('mustChangePassword')
    localStorage.removeItem('mustChangeUsername')
    window.dispatchEvent(new CustomEvent('auth:expired'))
  }

  return response
}
