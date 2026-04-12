const API_BASE = '/api'

export interface RegisterRequest {
  username: string
  password: string
}

export interface LoginRequest {
  username: string
  password: string
}

export interface AuthResponse {
  id: string
  username: string
  token?: string
  isAdmin?: boolean
  message: string
}

export interface ApiError {
  error: string
  details?: string
}

async function readApiPayload<T>(response: Response, fallbackMessage: string): Promise<T> {
  const text = await response.text()
  let data: unknown = null

  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      if (!response.ok) {
        throw new Error(`${fallbackMessage}（服务返回了非 JSON 响应，通常表示后端未启动或代理失败）`)
      }
      throw new Error(`${fallbackMessage}（服务返回了无法解析的响应）`)
    }
  }

  if (!response.ok) {
    const error = data as ApiError | null
    throw new Error(error?.error || fallbackMessage)
  }

  if (!data) {
    throw new Error(`${fallbackMessage}（服务返回空响应，通常表示后端未启动或代理失败）`)
  }

  return data as T
}

export async function register(data: RegisterRequest): Promise<AuthResponse> {
  const response = await fetch(`${API_BASE}/manage/auth/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })

  return readApiPayload<AuthResponse>(response, '注册失败')
}

export async function login(data: LoginRequest): Promise<AuthResponse> {
  const response = await fetch(`${API_BASE}/manage/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })

  return readApiPayload<AuthResponse>(response, '登录失败')
}
