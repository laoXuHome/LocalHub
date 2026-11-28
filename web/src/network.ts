import type { AutoNetworkStatus, NetworkMode } from './types'

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
  return '根据当前浏览器自动判断'
}

export type NetworkTone = 'lan' | 'wan' | 'unknown' | 'probing'

export type NetworkEntryState = {
  /** 顶部入口要显示的状态文字。 */
  label: string
  /** 状态颜色；判断中沿用灰色并配合动画。 */
  tone: NetworkTone
  /** 用户选择的模式，用于辅助说明与 aria。 */
  preference: NetworkMode
  /** AUTO 时的原始判断结果，手动模式下为 null。 */
  detected: AutoNetworkStatus | null
}

/**
 * 顶部入口展示“当前状态”，而不是只显示偏好：
 * 手动 LAN/WAN 立即覆盖；AUTO 下展示自动判断 LAN/WAN/UNKNOWN，判断中单独表示。
 */
export function networkEntryState(
  preference: NetworkMode,
  detected: AutoNetworkStatus | null,
  probing = false,
): NetworkEntryState {
  if (preference !== 'auto') {
    return { label: networkModeLabel(preference), tone: preference, preference, detected: null }
  }
  if (probing) return { label: '判断中', tone: 'probing', preference, detected }
  if (detected === 'lan') return { label: 'LAN', tone: 'lan', preference, detected }
  if (detected === 'wan') return { label: 'WAN', tone: 'wan', preference, detected }
  if (detected === 'unknown') return { label: 'UNKNOWN', tone: 'unknown', preference, detected }
  return { label: 'AUTO', tone: 'unknown', preference, detected: null }
}

export function autoStatusText(status: AutoNetworkStatus): string {
  if (status === 'lan') return '当前为 LAN'
  if (status === 'wan') return '当前为 WAN'
  return '无法确定（UNKNOWN）'
}

export function autoReasonText(reason: string): string {
  switch (reason) {
    case 'no_targets':
      return '没有启用的探测目标'
    case 'offline':
      return '浏览器当前离线'
    case 'browser_restricted':
      return '浏览器安全策略阻止了判断'
    case 'targets_unavailable':
      return '无法读取探测目标'
    case 'all_failed':
      return '两轮探测均未成功'
    default:
      return '自动判断已完成'
  }
}

export { MODES as networkModes, STORAGE_KEY as networkModeStorageKey }
