import { describe, expect, it } from 'vitest'
import {
  bookmarkAddressHint,
  bookmarkAddressMode,
  bookmarkAddressShortLabel,
  effectiveNetworkMode,
  isNetworkMode,
  networkEntryState,
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

  it('maps bookmark address selection without treating UNKNOWN as LAN/WAN', () => {
    expect(bookmarkAddressMode('lan', 'wan', false)).toBe('lan')
    expect(bookmarkAddressMode('wan', 'lan', false)).toBe('wan')
    expect(bookmarkAddressMode('lan', 'lan', true)).toBe('lan')
    expect(bookmarkAddressMode('wan', 'wan', true)).toBe('wan')
    expect(bookmarkAddressMode('auto', 'lan', false)).toBe('lan')
    expect(bookmarkAddressMode('auto', 'wan', false)).toBe('wan')
    expect(bookmarkAddressMode('auto', 'unknown', false)).toBe('default')
    expect(bookmarkAddressMode('auto', null, false)).toBe('default')
    expect(bookmarkAddressMode('auto', 'lan', true)).toBe('default')
    expect(bookmarkAddressMode('auto', 'wan', true)).toBe('default')
  })

  it('describes the effective bookmark address mode', () => {
    expect(bookmarkAddressShortLabel('lan')).toBe('LAN 地址')
    expect(bookmarkAddressShortLabel('wan')).toBe('WAN 地址')
    expect(bookmarkAddressShortLabel('default')).toBe('默认地址')
    expect(bookmarkAddressHint('lan')).toContain('LAN 地址')
    expect(bookmarkAddressHint('wan')).toContain('WAN 地址')
    expect(bookmarkAddressHint('default')).toContain('默认地址')
  })

  it('labels the three modes', () => {
    expect(networkModeLabel('auto')).toBe('AUTO')
    expect(networkModeLabel('lan')).toBe('LAN')
    expect(networkModeLabel('wan')).toBe('WAN')
  })

  it('shows AUTO detection states while keeping manual LAN/WAN highest priority', () => {
    expect(networkEntryState('auto', 'lan', false)).toMatchObject({ label: 'LAN', tone: 'lan', preference: 'auto' })
    expect(networkEntryState('auto', 'wan', false)).toMatchObject({ label: 'WAN', tone: 'wan', preference: 'auto' })
    expect(networkEntryState('auto', 'unknown', false)).toMatchObject({ label: 'UNKNOWN', tone: 'unknown' })
    expect(networkEntryState('auto', null, true)).toMatchObject({ label: '判断中', tone: 'probing' })
    expect(networkEntryState('auto', null, false)).toMatchObject({ label: 'AUTO', tone: 'unknown' })
    expect(networkEntryState('lan', 'wan', false)).toMatchObject({ label: 'LAN', tone: 'lan', detected: null })
    expect(networkEntryState('wan', 'lan', false)).toMatchObject({ label: 'WAN', tone: 'wan', detected: null })
  })
})
