import type { Bookmark } from './types'

function normalizeText(value: string): string {
  return value.normalize('NFKC').toLocaleLowerCase()
}

function normalizeQuery(query: string): string[] {
  const normalized = normalizeText(query).trim()
  if (!normalized) return []
  return normalized.split(/\s+/).filter(Boolean)
}

/**
 * 搜索当前调用者已经拿到的书签集合。
 * 后端 `/api/bookmarks` 已按角色、可见性和 enabled 过滤，前端不再补数据或放宽范围。
 */
export function searchBookmarks(bookmarks: Bookmark[], query: string): Bookmark[] {
  const terms = normalizeQuery(query)
  if (terms.length === 0) return []

  return bookmarks.filter((bookmark) => {
    const fields = [
      normalizeText(bookmark.name),
      normalizeText(bookmark.description),
      normalizeText(bookmark.lan_url),
      normalizeText(bookmark.wan_url),
    ]
    return terms.every((term) => fields.some((field) => field.includes(term)))
  })
}
