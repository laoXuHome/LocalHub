import { describe, expect, it } from 'vitest'
import {
  effectiveNetworkMode,
  isNetworkMode,
  networkModeLabel,
  networkModeStorageKey,
  persistNetworkMode,
  readNetworkMode,
} from './network'

function memoryStorage(initial?: Record<string, string>): Pick<Storage, 'getItem' | 'setItem'> {
  const values = new Map(Object.entries(initial ?? {}))
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => {
      values.set(key, value)
    },
  }
}

describe('network mode preference', () => {
  it('defaults to AUTO when nothing is stored', () => {
    expect(readNetworkMode(memoryStorage())).toBe('auto')
  })

  it('accepts only auto/lan/wan', () => {
    expect(isNetworkMode('auto')).toBe(true)
    expect(isNetworkMode('lan')).toBe(true)
    expect(isNetworkMode('wan')).toBe(true)
    expect(isNetworkMode('WAN')).toBe(false)
    expect(isNetworkMode('')).toBe(false)
    expect(isNetworkMode(null)).toBe(false)
  })

  it('falls back to AUTO for corrupted values', () => {
    expect(readNetworkMode(memoryStorage({ [networkModeStorageKey]: 'invalid' }))).toBe('auto')
  })

  it('persists manual LAN/WAN and reads it back', () => {
    const storage = memoryStorage()
    expect(persistNetworkMode('lan', storage)).toBe(true)
    expect(readNetworkMode(storage)).toBe('lan')
    expect(persistNetworkMode('wan', storage)).toBe(true)
    expect(readNetworkMode(storage)).toBe('wan')
    expect(persistNetworkMode('auto', storage)).toBe(true)
    expect(readNetworkMode(storage)).toBe('auto')
  })

  it('keeps storage independent for different devices', () => {
    const deviceA = memoryStorage()
    const deviceB = memoryStorage()
    persistNetworkMode('lan', deviceA)
    expect(readNetworkMode(deviceA)).toBe('lan')
    expect(readNetworkMode(deviceB)).toBe('auto')
  })

  it('manual mode wins over auto detection; AUTO follows detection', () => {
    expect(effectiveNetworkMode('lan', 'wan')).toBe('lan')
    expect(effectiveNetworkMode('wan', 'lan')).toBe('wan')
    expect(effectiveNetworkMode('auto', 'wan')).toBe('wan')
    expect(effectiveNetworkMode('auto', 'auto')).toBe('auto')
  })

  it('labels the three modes', () => {
    expect(networkModeLabel('auto')).toBe('AUTO')
    expect(networkModeLabel('lan')).toBe('LAN')
    expect(networkModeLabel('wan')).toBe('WAN')
  })
})
