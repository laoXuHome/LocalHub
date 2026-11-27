import { describe, expect, it } from 'vitest'
import {
  appendEntry,
  bookmarksByLayout,
  buildLayouts,
  layoutPayload,
  moveEntry,
  removeEntry,
  resizeEntry,
  toDraft,
  type DraftBookmark,
  type LayoutEntry,
} from './editor'
import type { Bookmark, LayoutResponse } from './types'

function bookmark(id: number, name = `书签 ${id}`): Bookmark {
  return {
    id,
    name,
    lan_url: `http://192.168.1.${id}:8080`,
    wan_url: `https://b${id}.example.com`,
    description: '',
    icon_type: 'none',
    icon_value: '',
    has_icon: false,
    enabled: true,
    visibility: 'public',
    effective_visibility: 'public',
    open_mode: 'new_tab',
    folder_path: [],
    sort_order: id - 1,
  }
}

const remote: LayoutResponse = {
  desktop: [
    { id: 3, size: 'large' },
    { id: 1, size: 'small' },
  ],
  mobile: [{ id: 2, size: 'medium' }],
}

describe('P3.2 布局纯函数', () => {
  it('按服务端布局排序，并为缺失书签补默认位置和尺寸', () => {
    const drafts = [1, 2, 3, 4].map((id) => toDraft(bookmark(id)))
    const layouts = buildLayouts(drafts, remote)
    expect(layouts.desktop.map((entry) => entry.key)).toEqual(['b-3', 'b-1', 'b-2', 'b-4'])
    expect(layouts.desktop.map((entry) => entry.size)).toEqual(['large', 'small', 'medium', 'medium'])
    expect(layouts.mobile.map((entry) => entry.key)).toEqual(['b-2', 'b-1', 'b-3', 'b-4'])
  })

  it('忽略不在当前书签集合中的布局 ID，避免删除后留下空洞', () => {
    const layouts = buildLayouts([toDraft(bookmark(1))], remote)
    expect(layouts.desktop.map((entry) => entry.key)).toEqual(['b-1'])
    expect(layouts.mobile.map((entry) => entry.key)).toEqual(['b-1'])
  })

  it('拖动时把源项插入目标项之前，并保持其余顺序', () => {
    const entries: LayoutEntry[] = [
      { key: 'a', size: 'small' },
      { key: 'b', size: 'medium' },
      { key: 'c', size: 'large' },
    ]
    expect(moveEntry(entries, 'a', 'c').map((entry) => entry.key)).toEqual(['b', 'a', 'c'])
    expect(moveEntry(entries, 'c', 'a').map((entry) => entry.key)).toEqual(['c', 'a', 'b'])
    expect(moveEntry(entries, 'a', 'a')).toBe(entries)
    expect(moveEntry(entries, 'a', 'missing')).toBe(entries)
  })

  it('尺寸调整只改目标项，并支持新增/删除条目', () => {
    const entries: LayoutEntry[] = [{ key: 'a', size: 'medium' }, { key: 'b', size: 'small' }]
    const resized = resizeEntry(entries, 'a', 'large')
    expect(resized[0].size).toBe('large')
    expect(resized[1].size).toBe('small')
    expect(resizeEntry(entries, 'a', 'medium')).toBe(entries)
    expect(appendEntry(entries, 'c').map((entry) => entry.key)).toEqual(['a', 'b', 'c'])
    expect(appendEntry(entries, 'a')).toBe(entries)
    expect(removeEntry(entries, 'b').map((entry) => entry.key)).toEqual(['a'])
  })

  it('布局保存只包含有真实 ID 的书签，并去重', () => {
    const drafts: DraftBookmark[] = [
      { ...toDraft(bookmark(1)), serverId: 1 },
      { ...toDraft(bookmark(2)), serverId: 2 },
    ]
    const layouts = {
      desktop: [
        { key: 'b-2', size: 'large' as const },
        { key: 'b-1', size: 'small' as const },
        { key: 'b-1', size: 'medium' as const },
      ],
      mobile: [{ key: 'b-1', size: 'medium' as const }],
    }
    expect(layoutPayload(layouts, drafts)).toEqual({
      desktop: [{ id: 2, size: 'large' }, { id: 1, size: 'small' }],
      mobile: [{ id: 1, size: 'medium' }],
    })
  })

  it('新书签未保存前不会进入布局 payload', () => {
    const fresh: DraftBookmark = {
      ...toDraft(bookmark(1)),
      id: -10,
      key: 'n-1',
      isNew: true,
    }
    const layouts = { desktop: [{ key: 'n-1', size: 'medium' as const }], mobile: [] }
    expect(layoutPayload(layouts, [fresh]).desktop).toEqual([])
  })

  it('按布局顺序返回书签对象，供网格渲染', () => {
    const drafts = [1, 2, 3].map((id) => toDraft(bookmark(id)))
    const entries: LayoutEntry[] = [
      { key: 'b-2', size: 'large' },
      { key: 'b-3', size: 'small' },
    ]
    expect(bookmarksByLayout(drafts, entries).map((bookmark) => bookmark.id)).toEqual([2, 3])
  })
})
