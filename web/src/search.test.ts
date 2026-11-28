import { describe, expect, it } from 'vitest'
import { searchBookmarks } from './search'
import type { Bookmark } from './types'

function bookmark(input: Partial<Bookmark> & Pick<Bookmark, 'id' | 'name'>): Bookmark {
  const { id, name, ...overrides } = input
  return {
    id,
    name,
    lan_url: '',
    wan_url: '',
    description: '',
    icon_type: 'none',
    icon_value: '',
    has_icon: false,
    enabled: true,
    visibility: 'public',
    effective_visibility: 'public',
    open_mode: 'new_tab',
    folder_path: [],
    sort_order: id,
    ...overrides,
  }
}

const items = [
  bookmark({ id: 1, name: 'NAS 管理', description: '家庭文件与照片', lan_url: 'http://192.168.1.20:5000' }),
  bookmark({ id: 2, name: 'Jellyfin', description: 'Movie & TV streaming', wan_url: 'https://media.example.com/jellyfin' }),
  bookmark({ id: 3, name: 'OpenWrt 路由', description: '软路由控制台', lan_url: 'http://192.168.1.1' }),
  bookmark({ id: 4, name: 'Private Notes', description: '仅管理员可见', visibility: 'private' }),
]

describe('searchBookmarks', () => {
  it('matches bookmark names in Chinese', () => {
    expect(searchBookmarks(items, '路由').map((item) => item.id)).toEqual([3])
  })

  it('matches bookmark names in English case-insensitively', () => {
    expect(searchBookmarks(items, 'jELLY').map((item) => item.id)).toEqual([2])
  })

  it('matches descriptions', () => {
    expect(searchBookmarks(items, '文件').map((item) => item.id)).toEqual([1])
    expect(searchBookmarks(items, 'streaming').map((item) => item.id)).toEqual([2])
  })

  it('matches LAN and WAN URLs and partial host fragments', () => {
    expect(searchBookmarks(items, '192.168.1.20').map((item) => item.id)).toEqual([1])
    expect(searchBookmarks(items, 'example.com').map((item) => item.id)).toEqual([2])
    expect(searchBookmarks(items, 'openwrt').map((item) => item.id)).toEqual([3])
  })

  it('supports multiple terms across fields', () => {
    expect(searchBookmarks(items, '管理 照片').map((item) => item.id)).toEqual([1])
    expect(searchBookmarks(items, '路由 192.168').map((item) => item.id)).toEqual([3])
  })

  it('returns no results for an unmatched query and for empty input', () => {
    expect(searchBookmarks(items, '不存在')).toEqual([])
    expect(searchBookmarks(items, '   ')).toEqual([])
  })

  it('only searches the supplied collection and never invents hidden entries', () => {
    const guestVisible = items.filter((item) => item.visibility === 'public')
    expect(searchBookmarks(guestVisible, 'private')).toEqual([])
    expect(searchBookmarks(guestVisible, '仅管理员')).toEqual([])
  })
})
