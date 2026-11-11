import type { ReactNode } from 'react'

interface Props {
  title: string
  onClose: () => void
  children: ReactNode
  footer: ReactNode
}

// Dialog is a modal panel. Escape and a click outside close it.
export function Dialog({ title, onClose, children, footer }: Props) {
  return (
    <div className="overlay" onMouseDown={onClose}>
      <div
        className="settings dialog"
        role="dialog"
        aria-label={title}
        onMouseDown={(e) => {
          e.stopPropagation()
        }}
        onKeyDown={(e) => {
          if (e.key === 'Escape') onClose()
        }}
      >
        <header className="settings-head">
          <h2>{title}</h2>
        </header>
        <div className="dialog-body">{children}</div>
        <footer className="dialog-foot">{footer}</footer>
      </div>
    </div>
  )
}

interface ConfirmProps {
  title: string
  message: string
  confirm: string
  danger?: boolean
  busy?: boolean
  error?: string | null
  onConfirm: () => void
  onClose: () => void
}

export function ConfirmDialog({ title, message, confirm, danger, busy, error, onConfirm, onClose }: ConfirmProps) {
  return (
    <Dialog
      title={title}
      onClose={onClose}
      footer={
        <>
          <button type="button" className="secondary" onClick={onClose}>
            Cancel
          </button>
          <button type="button" className={danger ? 'send danger' : 'send'} disabled={busy} onClick={onConfirm} autoFocus>
            {busy ? 'Working…' : confirm}
          </button>
        </>
      }
    >
      <p>{message}</p>
      {error && <p className="form-error">{error}</p>}
    </Dialog>
  )
}
