import type { Bookmark, CardSize, LayoutDevice, LayoutResponse } from './types'
import { cardSizeFor } from './lib'

export type LayoutEntry = { key: string; size: CardSize }
export type Layouts = Record<LayoutDevice, LayoutEntry[]>

export type DraftBookmark = Bookmark & {
  /** 稳定的前端 key：新书签在保存前没有真实 ID。 */
  key: string
  /** 保存成功后的服务端 ID；仅用于失败重试，不进入正式列表。 */
  serverId?: number
  isNew?: boolean
  /** 仅在最终保存时上传，预览阶段用 object/data URL。 */
  iconBlob?: File
  iconPreview?: string
}

let newBookmarkCounter = 0

export function keyForBookmark(bookmark: Bookmark): string {
  return `b-${bookmark.id}`
}

export function draftKey(): string {
  newBookmarkCounter += 1
  return `n-${Date.now().toString(36)}-${newBookmarkCounter}`
}

export function toDraft(bookmark: Bookmark): DraftBookmark {
  return { ...bookmark, key: keyForBookmark(bookmark) }
}

export function defaultSizeForIndex(index: number): CardSize {
  return cardSizeFor(index)
}

function validSize(size: string): size is CardSize {
  return size === 'large' || size === 'medium' || size === 'small'
}

/**
 * 将服务端布局合并到当前书签集合：
 * - 服务端已存在的项保留其顺序和尺寸；
 * - 新增书签、没有布局行的书签追加到末尾，并使用 P3.1 的默认尺寸；
 * - 删除或不可见的书签不会留在布局里，避免空洞。
 */
export function buildLayouts(bookmarks: DraftBookmark[], remote: LayoutResponse): Layouts {
  const available = new Set(bookmarks.map((bookmark) => bookmark.key))
  const build = (items: LayoutResponse['desktop']): LayoutEntry[] => {
    const seen = new Set<string>()
    const out: LayoutEntry[] = []
    for (const item of items) {
      const key = `b-${item.id}`
      if (!available.has(key) || seen.has(key) || !validSize(item.size)) continue
      seen.add(key)
      out.push({ key, size: item.size })
    }
    for (const bookmark of bookmarks) {
      if (seen.has(bookmark.key)) continue
      seen.add(bookmark.key)
      out.push({ key: bookmark.key, size: defaultSizeForIndex(out.length) })
    }
    return out
  }
  return { desktop: build(remote.desktop), mobile: build(remote.mobile) }
}

export function moveEntry(entries: LayoutEntry[], sourceKey: string, targetKey: string): LayoutEntry[] {
  if (sourceKey === targetKey) return entries
  const sourceIndex = entries.findIndex((entry) => entry.key === sourceKey)
  const targetIndex = entries.findIndex((entry) => entry.key === targetKey)
  if (sourceIndex < 0 || targetIndex < 0) return entries
  const next = entries.slice()
  const [moving] = next.splice(sourceIndex, 1)
  const insertAt = next.findIndex((entry) => entry.key === targetKey)
  if (insertAt < 0) return entries
  next.splice(insertAt, 0, moving)
  return next
}

export function resizeEntry(entries: LayoutEntry[], key: string, size: CardSize): LayoutEntry[] {
  let changed = false
  const next = entries.map((entry) => {
    if (entry.key !== key || entry.size === size) return entry
    changed = true
    return { ...entry, size }
  })
  return changed ? next : entries
}

export function removeEntry(entries: LayoutEntry[], key: string): LayoutEntry[] {
  return entries.filter((entry) => entry.key !== key)
}

export function appendEntry(entries: LayoutEntry[], key: string, size: CardSize = 'medium'): LayoutEntry[] {
  if (entries.some((entry) => entry.key === key)) return entries
  return [...entries, { key, size }]
}

export function layoutPayload(layouts: Layouts, bookmarks: DraftBookmark[]): { desktop: { id: number; size: CardSize }[]; mobile: { id: number; size: CardSize }[] } {
  const ids = new Map<string, number>()
  for (const bookmark of bookmarks) {
    const id = bookmark.serverId ?? (bookmark.isNew ? undefined : bookmark.id)
    if (id && id > 0) ids.set(bookmark.key, id)
  }
  const serialize = (entries: LayoutEntry[]) => {
    const seen = new Set<number>()
    const out: { id: number; size: CardSize }[] = []
    for (const entry of entries) {
      const id = ids.get(entry.key)
      if (!id || seen.has(id)) continue
      seen.add(id)
      out.push({ id, size: entry.size })
    }
    return out
  }
  return { desktop: serialize(layouts.desktop), mobile: serialize(layouts.mobile) }
}

export function bookmarksByLayout(bookmarks: DraftBookmark[], entries: LayoutEntry[]): DraftBookmark[] {
  const byKey = new Map(bookmarks.map((bookmark) => [bookmark.key, bookmark]))
  const out: DraftBookmark[] = []
  for (const entry of entries) {
    const bookmark = byKey.get(entry.key)
    if (bookmark) out.push(bookmark)
  }
  return out
}

export function findEntry(entries: LayoutEntry[], key: string): LayoutEntry | undefined {
  return entries.find((entry) => entry.key === key)
}

/** 是否是“新增书签”的表单草稿：id 为负数，尚未写入服务端。 */
export function isDraftNew(bookmark: DraftBookmark): boolean {
  return Boolean(bookmark.isNew)
}
