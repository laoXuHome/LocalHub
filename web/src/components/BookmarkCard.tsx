import type { Bookmark, CardSize } from '../types'
import { iconKind, iconSource, iconText, initialLetter, bookmarkHref, bookmarkTarget, hueFor, cardSizeFor } from '../lib'
import { useState } from 'react'
import type { DraftBookmark } from '../editor'

type Props = {
  bookmark: Bookmark | DraftBookmark
  index?: number
  size?: CardSize
  editing?: boolean
  dragOver?: boolean
  onEdit?: () => void
  onSizeChange?: (size: CardSize) => void
  onDragStart?: (event: React.PointerEvent<HTMLButtonElement>) => void
  dataKey?: string
}

const sizes: { value: CardSize; label: string; title: string }[] = [
  { value: 'large', label: '大', title: '大卡片' },
  { value: 'medium', label: '中', title: '中卡片' },
  { value: 'small', label: '小', title: '小卡片' },
]

export function BookmarkCard({
  bookmark,
  index = 0,
  size,
  editing = false,
  dragOver = false,
  onEdit,
  onSizeChange,
  onDragStart,
  dataKey,
}: Props) {
  const [imageFailed, setImageFailed] = useState(false)
  const href = bookmarkHref(bookmark)
  const previewSrc = 'iconPreview' in bookmark ? bookmark.iconPreview : undefined
  const kind = imageFailed ? 'fallback' : previewSrc ? 'image' : iconKind(bookmark)
  const cardSize = size ?? cardSizeFor(index)
  const glyph = iconText(bookmark)
  const src = previewSrc || iconSource(bookmark)
  const style = { '--card-hue': String(hueFor(bookmark.id || 0)) } as React.CSSProperties

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

  if (editing) {
    return (
      <div
        className={`bookmark-card bookmark-card--editing${dragOver ? ' is-drag-over' : ''}`}
        data-size={cardSize}
        data-key={dataKey}
        style={style}
        role="button"
        tabIndex={0}
        title="点击编辑书签"
        onClick={() => onEdit?.()}
        onKeyDown={(event) => {
          if (event.target !== event.currentTarget) return
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault()
            onEdit?.()
          }
        }}
      >
        {content}
        <div className="card-edit-controls" onClick={(event) => event.stopPropagation()}>
          <button
            type="button"
            className="drag-handle"
            aria-label="拖动调整书签顺序"
            title="拖动调整顺序"
            onPointerDown={(event) => onDragStart?.(event)}
            onClick={(event) => event.stopPropagation()}
          >
            <span className="drag-dots" aria-hidden="true">⠿</span>
          </button>
          <div className="card-size-picker" role="group" aria-label="卡片尺寸">
            {sizes.map((item) => (
              <button
                key={item.value}
                type="button"
                className={`size-button${cardSize === item.value ? ' is-active' : ''}`}
                aria-pressed={cardSize === item.value}
                title={item.title}
                onClick={() => onSizeChange?.(item.value)}
              >
                {item.label}
              </button>
            ))}
          </div>
        </div>
      </div>
    )
  }

  if (!href) {
    return (
      <div className="bookmark-card bookmark-card--disabled" data-size={cardSize} style={style}>
        {content}
      </div>
    )
  }

  return (
    <a
      className="bookmark-card"
      data-size={cardSize}
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
