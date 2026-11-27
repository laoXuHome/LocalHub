import type { AuthStatus, BookmarksResponse } from './types'

export class APIError extends Error {
  status: number
  code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'APIError'
    this.status = status
    this.code = code
  }
}

async function parseError(response: Response): Promise<APIError> {
  let payload: { error?: string; message?: string } = {}
  try {
    payload = (await response.json()) as { error?: string; message?: string }
  } catch {
    /* 非 JSON 错误响应 */
  }
  return new APIError(response.status, payload.error || 'request_failed', payload.message || '请求失败')
}

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(path, { credentials: 'same-origin', cache: 'no-store', signal })
  if (!response.ok) throw await parseError(response)
  return (await response.json()) as T
}

async function postJSON<T>(path: string, body: unknown, signal?: AbortSignal): Promise<T> {
  const response = await fetch(path, {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    signal,
  })
  if (!response.ok) throw await parseError(response)
  return (await response.json()) as T
}

export function loadAuth(signal?: AbortSignal): Promise<AuthStatus> {
  return getJSON<AuthStatus>('/api/auth/status', signal)
}

export function loadBookmarks(signal?: AbortSignal): Promise<BookmarksResponse> {
  return getJSON<BookmarksResponse>('/api/bookmarks', signal)
}

export function login(username: string, password: string): Promise<Record<string, unknown>> {
  return postJSON('/api/auth/login', { username, password })
}

export function logout(): Promise<Record<string, unknown>> {
  return postJSON('/api/auth/logout', {})
}

export function changePassword(currentPassword: string, newPassword: string): Promise<Record<string, unknown>> {
  return postJSON('/api/auth/password', {
    current_password: currentPassword,
    new_password: newPassword,
  })
}
