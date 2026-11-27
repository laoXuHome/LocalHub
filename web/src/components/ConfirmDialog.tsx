import { useEffect } from 'react'
import { CloseIcon } from './Icons'

export type ConfirmRequest = {
  title: string
  message: string
  confirmLabel: string
  tone?: 'danger' | 'normal'
  onConfirm: () => void
}

export function ConfirmDialog({ request, onCancel }: { request: ConfirmRequest; onCancel: () => void }) {
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onCancel()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onCancel])

  return (
    <div className="confirm-layer" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onCancel()}>
      <div className="confirm-dialog" role="alertdialog" aria-modal="true" aria-labelledby="confirm-title">
        <header className="confirm-head">
          <h2 id="confirm-title">{request.title}</h2>
          <button type="button" className="sheet-close" aria-label="关闭确认框" onClick={onCancel}>
            <CloseIcon size={17} />
          </button>
        </header>
        <p>{request.message}</p>
        <footer className="confirm-actions">
          <button type="button" className="quiet-button" onClick={onCancel}>取消</button>
          <button
            type="button"
            className={`primary-button${request.tone === 'danger' ? ' danger-button' : ''}`}
            onClick={() => {
              request.onConfirm()
              onCancel()
            }}
          >
            {request.confirmLabel}
          </button>
        </footer>
      </div>
    </div>
  )
}
