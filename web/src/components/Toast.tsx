import { CloseIcon } from './Icons'

export type ToastTone = 'info' | 'success' | 'error'

export type ToastMessage = {
  id: number
  text: string
  tone: ToastTone
}

export function Toast({ toast, onClose }: { toast: ToastMessage; onClose: () => void }) {
  return (
    <div className={`toast toast--${toast.tone}`} role="status" aria-live="polite">
      <span>{toast.text}</span>
      <button type="button" className="toast-close" onClick={onClose} aria-label="关闭提示">
        <CloseIcon size={15} />
      </button>
    </div>
  )
}
