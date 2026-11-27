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
