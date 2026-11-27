import type { Bookmark } from '../types'
import { iconKind, iconSource, iconText, initialLetter, bookmarkHref, bookmarkTarget, hueFor, cardSizeFor } from '../lib'
import { useState } from 'react'

export function BookmarkCard({ bookmark, index }: { bookmark: Bookmark; index: number }) {
  const [imageFailed, setImageFailed] = useState(false)
  const href = bookmarkHref(bookmark)
  const kind = imageFailed ? 'fallback' : iconKind(bookmark)
  const size = cardSizeFor(index)
  const glyph = iconText(bookmark)
  const src = iconSource(bookmark)
  const style = { '--card-hue': String(hueFor(bookmark.id)) } as React.CSSProperties

  const icon = (
    <span className={`bookmark-icon bookmark-icon--${kind}`} aria-hidden="true">
      {kind === 'image' ? (
        <img
          src={src}
          alt=""
          loading="lazy"
          decoding="async"
          draggable={false}
          referrerPolicy="no-referrer"
          onError={() => setImageFailed(true)}
        />
      ) : kind === 'glyph' ? (
        <span className="bookmark-glyph">{glyph}</span>
      ) : (
        <span className="bookmark-initial">{initialLetter(bookmark.name)}</span>
      )}
    </span>
  )

  const content = (
    <>
      {icon}
      <span className="bookmark-name" title={bookmark.name}>
        {bookmark.name}
      </span>
    </>
  )

  if (!href) {
    return (
      <div className="bookmark-card bookmark-card--disabled" data-size={size} style={style}>
        {content}
      </div>
    )
  }

  return (
    <a
      className="bookmark-card"
      data-size={size}
      style={style}
      href={href}
      target={bookmarkTarget(bookmark)}
      rel={bookmarkTarget(bookmark) === '_blank' ? 'noopener noreferrer' : undefined}
      aria-label={bookmark.name}
    >
      {content}
    </a>
  )
}
