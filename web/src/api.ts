import type {
  AuthStatus,
  Bookmark,
  BookmarkInput,
  BookmarksResponse,
  DockerBookmarkLinks,
  DockerContainersInfo,
  DockerStatusInfo,
  LayoutResponse,
  NetworkConfig,
  NetworkProbeRun,
  NetworkProbeStatus,
  NetworkProbeTargets,
  NetworkTargetInput,
} from './types'

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

async function sendJSON<T>(path: string, method: 'POST' | 'PUT' | 'PATCH' | 'DELETE', body?: unknown, signal?: AbortSignal): Promise<T> {
  const response = await fetch(path, {
    method,
    credentials: 'same-origin',
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
  })
  if (!response.ok) throw await parseError(response)
  return (await response.json()) as T
}

async function postJSON<T>(path: string, body: unknown, signal?: AbortSignal): Promise<T> {
  return sendJSON<T>(path, 'POST', body, signal)
}

export function loadAuth(signal?: AbortSignal): Promise<AuthStatus> {
  return getJSON<AuthStatus>('/api/auth/status', signal)
}

export function loadBookmarks(signal?: AbortSignal): Promise<BookmarksResponse> {
  return getJSON<BookmarksResponse>('/api/bookmarks', signal)
}

export function loadLayout(signal?: AbortSignal): Promise<LayoutResponse> {
  return getJSON<LayoutResponse>('/api/bookmarks/layout', signal)
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

export function createBookmark(input: BookmarkInput): Promise<Bookmark> {
  return postJSON<Bookmark>('/api/admin/bookmarks', input)
}

export function updateBookmark(id: number, input: BookmarkInput): Promise<Bookmark> {
  return sendJSON<Bookmark>(`/api/admin/bookmarks/${id}`, 'PUT', input)
}

export function deleteBookmark(id: number): Promise<Record<string, unknown>> {
  return sendJSON(`/api/admin/bookmarks/${id}`, 'DELETE')
}

export function saveLayout(layout: LayoutResponse): Promise<LayoutResponse> {
  return sendJSON<LayoutResponse>('/api/admin/bookmarks/layout', 'PUT', layout)
}

export function uploadBookmarkIcon(id: number, file: File): Promise<Bookmark> {
  return fetch(`/api/admin/bookmarks/${id}/icon`, {
    method: 'PUT',
    credentials: 'same-origin',
    headers: { 'Content-Type': file.type || 'application/octet-stream' },
    body: file,
  }).then(async (response) => {
    if (!response.ok) throw await parseError(response)
    return (await response.json()) as Bookmark
  })
}

export function refreshFavicon(id: number): Promise<Bookmark | Record<string, unknown>> {
  return postJSON(`/api/admin/bookmarks/${id}/favicon`, {})
}

export function loadNetworkTargets(signal?: AbortSignal): Promise<NetworkConfig> {
  return getJSON<NetworkConfig>('/api/admin/network/targets', signal)
}

export function saveNetworkTargets(targets: NetworkTargetInput[]): Promise<NetworkConfig> {
  return sendJSON<NetworkConfig>('/api/admin/network/targets', 'PUT', { targets })
}

export function loadDockerStatus(signal?: AbortSignal): Promise<DockerStatusInfo> {
  return getJSON<DockerStatusInfo>('/api/admin/docker/status', signal)
}

export function loadDockerContainers(signal?: AbortSignal): Promise<DockerContainersInfo> {
  return getJSON<DockerContainersInfo>('/api/admin/docker/containers', signal)
}

export function loadDockerBookmarkLinks(signal?: AbortSignal): Promise<DockerBookmarkLinks> {
  return getJSON<DockerBookmarkLinks>('/api/admin/docker/bookmark-links', signal)
}

export function loadNetworkStatus(signal?: AbortSignal): Promise<NetworkProbeStatus> {
  return getJSON<NetworkProbeStatus>('/api/network/status', signal)
}

// P4.3 浏览器自动判断所需的最小启用目标；公开只读，不包含地址之外的管理配置。
export function loadNetworkProbeTargets(signal?: AbortSignal): Promise<NetworkProbeTargets> {
  return getJSON<NetworkProbeTargets>('/api/network/targets', signal)
}

export function loadNetworkResults(signal?: AbortSignal): Promise<NetworkProbeStatus> {
  return getJSON<NetworkProbeStatus>('/api/admin/network/results', signal)
}

export function triggerNetworkProbe(): Promise<NetworkProbeRun> {
  return postJSON<NetworkProbeRun>('/api/admin/network/probe', {})
}

export function loadNetworkProbeRun(id: string, signal?: AbortSignal): Promise<NetworkProbeRun> {
  return getJSON<NetworkProbeRun>(`/api/admin/network/probes/${encodeURIComponent(id)}`, signal)
}
