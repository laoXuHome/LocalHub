import type { ResolvedTheme, ThemeMode } from './types'
import { isThemeMode, themeFor } from './lib'

const STORAGE_KEY = 'localhub.theme'

export function readTheme(): ThemeMode {
  try {
    const value = localStorage.getItem(STORAGE_KEY)
    return isThemeMode(value) ? value : 'auto'
  } catch {
    return 'auto'
  }
}

export function persistTheme(mode: ThemeMode): void {
  try {
    localStorage.setItem(STORAGE_KEY, mode)
  } catch {
    /* 隐私模式下可能不可写，主题仍可在本次会话使用 */
  }
}

export function systemPrefersDark(): boolean {
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ?? false
}

export function resolveTheme(mode: ThemeMode): ResolvedTheme {
  return themeFor(mode, systemPrefersDark())
}

export function applyTheme(mode: ThemeMode): ResolvedTheme {
  const resolved = resolveTheme(mode)
  const root = document.documentElement
  root.dataset.theme = resolved
  root.dataset.themeMode = mode
  root.style.colorScheme = resolved
  const meta = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')
  if (meta) meta.content = resolved === 'dark' ? '#0c0d12' : '#f4f5f9'
  return resolved
}

export function watchSystemTheme(callback: () => void): () => void {
  const query = window.matchMedia?.('(prefers-color-scheme: dark)')
  if (!query) return () => undefined
  const listener = () => callback()
  query.addEventListener('change', listener)
  return () => query.removeEventListener('change', listener)
}
