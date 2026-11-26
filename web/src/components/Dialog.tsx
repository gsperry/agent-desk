import { useState, type ReactNode } from 'react'
import { renameProblem } from '../model/rename'

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

interface RenameProps {
  current: string
  busy?: boolean
  error?: string | null
  onRename: (name: string) => void
  onClose: () => void
}

// RenameDialog asks for a session's new name. Enter renames; the button
// stays off until the name is usable.
export function RenameDialog({ current, busy, error, onRename, onClose }: RenameProps) {
  const [name, setName] = useState(current)
  const problem = renameProblem(name, current)
  const submit = () => {
    if (!problem && !busy) onRename(name.trim())
  }
  return (
    <Dialog
      title="Rename session"
      onClose={onClose}
      footer={
        <>
          <button type="button" className="secondary" onClick={onClose}>
            Cancel
          </button>
          <button type="button" className="send" disabled={problem !== null || busy} onClick={submit}>
            {busy ? 'Renaming…' : 'Rename'}
          </button>
        </>
      }
    >
      <label className="field">
        <span>Name</span>
        <input
          value={name}
          autoFocus
          onFocus={(e) => {
            e.currentTarget.select()
          }}
          onChange={(e) => {
            setName(e.target.value)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              submit()
            }
          }}
          aria-invalid={problem !== null && name.trim() !== current}
          aria-label="Session name"
        />
      </label>
      <p className="muted setting-note">
        agent-manager renames it, and a worktree session's branch follows the new name. Agent Desk must run outside an agent-manager terminal, and agent-manager must be 0.50 or newer.
      </p>
      {error && <p className="form-error">{error}</p>}
    </Dialog>
  )
}
