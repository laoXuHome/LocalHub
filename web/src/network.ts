import type { NetworkMode } from './types'

const STORAGE_KEY = 'localhub.networkMode'
const MODES: NetworkMode[] = ['auto', 'lan', 'wan']

type ModeStorage = Pick<Storage, 'getItem' | 'setItem'>

export function isNetworkMode(value: unknown): value is NetworkMode {
  return value === 'auto' || value === 'lan' || value === 'wan'
}

function defaultStorage(): ModeStorage | null {
  try {
    return window.localStorage
  } catch {
    return null
  }
}

/**
 * 读取当前浏览器的模式偏好。该值只存在 localStorage，不进入账户或服务端配置，
 * 因此不同浏览器/设备互不影响；未设置或损坏时回退 AUTO。
 */
export function readNetworkMode(storage?: ModeStorage | null): NetworkMode {
  const s = storage === undefined ? defaultStorage() : storage
  if (!s) return 'auto'
  try {
    const value = s.getItem(STORAGE_KEY)
    return isNetworkMode(value) ? value : 'auto'
  } catch {
    return 'auto'
  }
}

/** 保存模式偏好；返回是否成功写入。 */
export function persistNetworkMode(mode: NetworkMode, storage?: ModeStorage | null): boolean {
  const s = storage === undefined ? defaultStorage() : storage
  if (!s) return false
  try {
    s.setItem(STORAGE_KEY, mode)
    return true
  } catch {
    return false
  }
}

/**
 * 手动 LAN/WAN 优先于自动判断；只有偏好为 AUTO 时才采用自动检测结果。
 * P4.1 不执行自动检测，detected 固定为 auto；该语义留给后续探测引擎。
 */
export function effectiveNetworkMode(preference: NetworkMode, detected: NetworkMode = 'auto'): NetworkMode {
  if (preference !== 'auto') return preference
  return isNetworkMode(detected) ? detected : 'auto'
}

export function networkModeLabel(mode: NetworkMode): string {
  return mode === 'lan' ? 'LAN' : mode === 'wan' ? 'WAN' : 'AUTO'
}

export function networkModeDescription(mode: NetworkMode): string {
  if (mode === 'lan') return '手动使用局域网地址'
  if (mode === 'wan') return '手动使用广域网地址'
  return '交由后续自动探测判断'
}

export { MODES as networkModes, STORAGE_KEY as networkModeStorageKey }
