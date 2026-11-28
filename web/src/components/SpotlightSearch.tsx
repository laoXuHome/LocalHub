import { useDeferredValue, useEffect, useMemo, useRef, useState } from 'react'
import type { Bookmark } from '../types'
import { bookmarkHref, bookmarkTarget, hueFor, iconKind, iconSource, iconText, initialLetter } from '../lib'
import type { BookmarkAddressMode } from '../network'
import { searchBookmarks } from '../search'
import { BookmarkIcon, CloseIcon, SearchIcon } from './Icons'

type Props = {
  bookmarks: Bookmark[]
  addressMode: BookmarkAddressMode
  onClose: () => void
}

function ResultIcon({ bookmark }: { bookmark: Bookmark }) {
  const [imageFailed, setImageFailed] = useState(false)
  const kind = imageFailed ? 'fallback' : iconKind(bookmark)
  const src = iconSource(bookmark)

  return (
    <span className={`spotlight-result-icon bookmark-icon bookmark-icon--${kind}`} aria-hidden="true">
      {kind === 'image' ? (
        <img src={src} alt="" loading="lazy" decoding="async" draggable={false} referrerPolicy="no-referrer" onError={() => setImageFailed(true)} />
      ) : kind === 'glyph' ? (
        <span className="bookmark-glyph">{iconText(bookmark)}</span>
      ) : (
        <span className="bookmark-initial">{initialLetter(bookmark.name)}</span>
      )}
    </span>
  )
}

function openBookmark(bookmark: Bookmark, addressMode: BookmarkAddressMode, onClose: () => void) {
  const href = bookmarkHref(bookmark, addressMode)
  if (!href) return
  const target = bookmarkTarget(bookmark)
  onClose()
  if (target === '_blank') {
    window.open(href, '_blank', 'noopener,noreferrer')
  } else {
    window.location.assign(href)
  }
}

export function SpotlightSearch({ bookmarks, addressMode, onClose }: Props) {
  const [query, setQuery] = useState('')
  const [activeIndex, setActiveIndex] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const deferredQuery = useDeferredValue(query)
  const results = useMemo(() => searchBookmarks(bookmarks, deferredQuery), [bookmarks, deferredQuery])
  const trimmedQuery = deferredQuery.trim()
  const activeResult = results[activeIndex]

  useEffect(() => {
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    inputRef.current?.focus()
    return () => {
      document.body.style.overflow = previousOverflow
      opener?.focus()
    }
  }, [])

  useEffect(() => {
    setActiveIndex(0)
  }, [deferredQuery])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        onClose()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [onClose])

  useEffect(() => {
    const item = listRef.current?.querySelector<HTMLElement>(`[data-search-index="${activeIndex}"]`)
    item?.scrollIntoView({ block: 'nearest' })
  }, [activeIndex])

  function handleInputKeyDown(event: React.KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      if (results.length > 0) setActiveIndex((index) => Math.min(index + 1, results.length - 1))
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      if (results.length > 0) setActiveIndex((index) => Math.max(index - 1, 0))
    } else if (event.key === 'Enter') {
      event.preventDefault()
      if (activeResult) openBookmark(activeResult, addressMode, onClose)
    }
  }

  return (
    <div className="spotlight-veil" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <div className="spotlight-panel" role="dialog" aria-modal="true" aria-label="搜索书签">
        <div className="spotlight-input-row">
          <SearchIcon size={21} className="spotlight-search-glyph" />
          <input
            ref={inputRef}
            className="spotlight-input"
            type="search"
            value={query}
            placeholder="搜索书签名称、描述或 URL"
            aria-label="搜索书签"
            autoComplete="off"
            autoCorrect="off"
            spellCheck={false}
            inputMode="search"
            enterKeyHint="search"
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={handleInputKeyDown}
          />
          {query ? (
            <button type="button" className="spotlight-clear" aria-label="清空搜索" title="清空搜索" onClick={() => setQuery('')}>
              <CloseIcon size={15} />
            </button>
          ) : null}
          <button type="button" className="spotlight-esc" aria-label="关闭搜索" title="关闭搜索" onClick={onClose}>
            Esc
          </button>
        </div>

        <div className="spotlight-body" ref={listRef}>
          {trimmedQuery === '' ? (
            <div className="spotlight-initial">
              <span className="spotlight-initial-icon">
                <SearchIcon size={23} />
              </span>
              <strong>搜索全部书签</strong>
              <p>输入名称、描述或 URL，支持中文、English 与部分匹配。</p>
            </div>
          ) : results.length === 0 ? (
            <div className="spotlight-empty" role="status">
              <span className="spotlight-empty-icon">
                <BookmarkIcon size={24} />
              </span>
              <strong>没有找到匹配的书签</strong>
              <p>没有名称、描述或 URL 包含「{trimmedQuery}」的书签。</p>
            </div>
          ) : (
            <>
              <div className="spotlight-meta">
                <span>{results.length} 个结果</span>
                <span className="spotlight-hint">↑↓ 选择 · Enter 打开</span>
              </div>
              <div className="spotlight-results" role="listbox" aria-label="搜索结果">
                {results.map((bookmark, index) => {
                  const href = bookmarkHref(bookmark, addressMode)
                  const active = index === activeIndex
                  const style = { '--card-hue': String(hueFor(bookmark.id)) } as React.CSSProperties
                  return (
                    <a
                      key={bookmark.id}
                      className={`spotlight-result${active ? ' is-active' : ''}${href ? '' : ' is-disabled'}`}
                      data-search-index={index}
                      style={style}
                      role="option"
                      aria-selected={active}
                      aria-disabled={!href}
                      href={href || undefined}
                      onClick={(event) => {
                        event.preventDefault()
                        openBookmark(bookmark, addressMode, onClose)
                      }}
                      onMouseEnter={() => setActiveIndex(index)}
                    >
                      <ResultIcon bookmark={bookmark} />
                      <span className="spotlight-result-name">{bookmark.name}</span>
                    </a>
                  )
                })}
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
