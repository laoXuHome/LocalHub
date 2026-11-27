import type { Bookmark, CardSize, ResolvedTheme, ThemeMode } from './types'

export const themeOrder: ThemeMode[] = ['auto', 'light', 'dark']

export const themeCopy: Record<ThemeMode, { label: string; hint: string }> = {
  auto: { label: '自动', hint: '跟随系统外观' },
  light: { label: '浅色', hint: '始终使用浅色' },
  dark: { label: '深色', hint: '始终使用深色' },
}

export function isThemeMode(value: string | null): value is ThemeMode {
  return value === 'auto' || value === 'light' || value === 'dark'
}

export function nextTheme(mode: ThemeMode): ThemeMode {
  const index = themeOrder.indexOf(mode)
  return themeOrder[(index + 1) % themeOrder.length]
}

export function themeFor(mode: ThemeMode, systemDark: boolean): ResolvedTheme {
  if (mode === 'auto') return systemDark ? 'dark' : 'light'
  return mode
}

export function isHTTPURL(raw: string): boolean {
  try {
    const url = new URL(raw)
    return url.protocol === 'http:' || url.protocol === 'https:'
  } catch {
    return false
  }
}

export function bookmarkHref(bookmark: Bookmark): string {
  const wan = bookmark.wan_url.trim()
  if (isHTTPURL(wan)) return wan
  const lan = bookmark.lan_url.trim()
  if (isHTTPURL(lan)) return lan
  return ''
}

export function bookmarkTarget(bookmark: Bookmark): '_self' | '_blank' {
  return bookmark.open_mode === 'self' || bookmark.open_mode === 'current_tab' ? '_self' : '_blank'
}

export type IconKind = 'image' | 'glyph' | 'fallback'

export function iconKind(bookmark: Bookmark): IconKind {
  if (bookmark.has_icon && bookmark.icon_url) return 'image'
  if ((bookmark.icon_type === 'url' || bookmark.icon_type === 'favicon') && isHTTPURL(bookmark.icon_value)) {
    return 'image'
  }
  if ((bookmark.icon_type === 'emoji' || bookmark.icon_type === 'text') && bookmark.icon_value.trim()) {
    return 'glyph'
  }
  return 'fallback'
}

export function iconSource(bookmark: Bookmark): string {
  if (bookmark.has_icon && bookmark.icon_url) return bookmark.icon_url
  if ((bookmark.icon_type === 'url' || bookmark.icon_type === 'favicon') && isHTTPURL(bookmark.icon_value)) {
    return bookmark.icon_value
  }
  return ''
}

export function iconText(bookmark: Bookmark): string {
  if (bookmark.icon_type === 'emoji' || bookmark.icon_type === 'text') return bookmark.icon_value.trim()
  return ''
}

export function initialLetter(name: string): string {
  const chars = Array.from(name.trim())
  return chars[0] ? chars[0].toUpperCase() : '·'
}

export function cardSizeFor(index: number): CardSize {
  if (index === 0) return 'large'
  if (index < 5) return 'medium'
  return 'small'
}

export function hueFor(id: number): number {
  // 稳定的色相，避免同一书签每次刷新换色。
  const hues = [224, 246, 262, 206, 188, 280, 236]
  return hues[Math.abs(id) % hues.length]
}
