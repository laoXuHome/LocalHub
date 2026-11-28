export type Role = 'guest' | 'admin'

export type AuthStatus = {
  authenticated: boolean
  role: Role
  needs_setup: boolean
  username?: string
  session_expires_at?: string
  session_ttl_seconds?: number
  error?: string
}

export type Bookmark = {
  id: number
  name: string
  lan_url: string
  wan_url: string
  description: string
  icon_type: string
  icon_value: string
  icon_url?: string
  icon_mime?: string
  has_icon: boolean
  enabled: boolean
  visibility: string
  effective_visibility: string
  open_mode: string
  folder_path: string[]
  sort_order: number
}

export type BookmarksResponse = {
  items: Bookmark[]
  total: number
  role: Role
}

export type ThemeMode = 'auto' | 'light' | 'dark'
export type ResolvedTheme = 'light' | 'dark'

export type CardSize = 'large' | 'medium' | 'small'
export type LayoutDevice = 'desktop' | 'mobile'

export type LayoutItem = {
  id: number
  position?: number
  size: CardSize
}

export type LayoutResponse = {
  desktop: LayoutItem[]
  mobile: LayoutItem[]
}

export type BookmarkInput = {
  name?: string
  lan_url?: string
  wan_url?: string
  description?: string
  icon_type?: string
  icon_value?: string
  enabled?: boolean
  visibility?: string
  open_mode?: string
  folder_path?: string[]
  sort_order?: number
}

export type NetworkMode = 'auto' | 'lan' | 'wan'
export type NetworkTargetKey = 'router' | 'bypass_router' | 'nas'

export type NetworkTarget = {
  key: NetworkTargetKey
  label: string
  address: string
  enabled: boolean
}

export type NetworkConfig = {
  targets: NetworkTarget[]
  updated_at?: string
}

// 浏览器自动判断只获取启用目标的最小信息，不包含标签、禁用项或任何失败详情。
export type NetworkProbeTarget = Pick<NetworkTarget, 'key' | 'address'>

export type NetworkProbeTargets = {
  targets: NetworkProbeTarget[]
}

export type AutoNetworkStatus = 'lan' | 'wan' | 'unknown'

export type NetworkTargetInput = Pick<NetworkTarget, 'key' | 'address' | 'enabled'>

export type NetworkProbeState =
  | 'success'
  | 'http_error'
  | 'timeout'
  | 'connection_failed'
  | 'probe_error'
  | 'not_probed'
  | 'disabled'

export type NetworkProbeResult = {
  key: NetworkTargetKey
  label: string
  address?: string
  enabled: boolean
  status: NetworkProbeState
  http_status: number
  latency_ms: number
  checked_at?: string
  failure_reason?: string
}

export type NetworkProbeStatus = {
  targets: NetworkProbeResult[]
  updated_at?: string
}

export type NetworkProbeRun = {
  id: string
  status: 'running' | 'completed'
  started_at: string
  finished_at?: string
  results: NetworkProbeResult[]
}
