import { useEffect, useRef, useState } from 'react'
import type { GroupView, SessionAction, SessionView } from '../types'
import { ConfirmDialog } from './Dialog'

interface Props {
  session: SessionView
  groups: GroupView[]
  act: (id: string, a: SessionAction) => Promise<string | null>
  // manage allows the lifecycle actions; without it only Export shows.
  manage: boolean
  onExport?: () => void
}

// SessionActions is the session header's actions menu. Revive stays out of
// it as a button, since a stopped session has nothing else to do; stopping
// asks first, since it interrupts whatever the agent is doing.
export function SessionActions({ session, groups, act, manage, onExport }: Props) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [confirmStop, setConfirmStop] = useState(false)
  const [open, setOpen] = useState(false)
  const menu = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const close = (e: MouseEvent | KeyboardEvent) => {
      if (e instanceof KeyboardEvent ? e.key === 'Escape' : !menu.current?.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', close)
    document.addEventListener('keydown', close)
    return () => {
      document.removeEventListener('mousedown', close)
      document.removeEventListener('keydown', close)
    }
  }, [open])

  const run = (a: SessionAction) => {
    setBusy(true)
    setError(null)
    setOpen(false)
    void act(session.id, a).then((err) => {
      setBusy(false)
      setError(err)
      if (!err) setConfirmStop(false)
    })
  }

  if (!manage && !onExport) return null
  return (
    <div className="session-actions" ref={menu}>
      {manage && !session.running && (
        <button
          type="button"
          className="chip chip-button"
          disabled={busy}
          onClick={() => {
            run({ action: 'revive' })
          }}
        >
          Revive
        </button>
      )}
      <button
        type="button"
        className="chip chip-button actions-toggle"
        aria-label="Session actions"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => {
          setOpen(!open)
        }}
      >
        ⋯
      </button>
      {open && (
        <div className="actions-menu" role="menu" aria-label="Session actions">
          {manage && session.running && (
            <button
              type="button"
              role="menuitem"
              disabled={busy}
              onClick={() => {
                setOpen(false)
                setConfirmStop(true)
              }}
            >
              Stop
            </button>
          )}
          {manage && (
            <button
              type="button"
              role="menuitem"
              disabled={busy}
              onClick={() => {
                run({ action: session.archived ? 'restore' : 'archive' })
              }}
            >
              {session.archived ? 'Restore' : 'Archive'}
            </button>
          )}
          {manage && (
            <label className="actions-move">
              <span>Group</span>
              <select
                aria-label="Move to group"
                disabled={busy}
                value={session.group}
                onChange={(e) => {
                  run({ action: 'move', group: e.target.value })
                }}
              >
                <option value="">No group</option>
                {groups.map((g) => (
                  <option key={g.path} value={g.path}>
                    {g.path}
                  </option>
                ))}
              </select>
            </label>
          )}
          {onExport && (
            <button
              type="button"
              role="menuitem"
              onClick={() => {
                setOpen(false)
                onExport()
              }}
            >
              Export
            </button>
          )}
          {manage && (
            <p className="muted actions-note" title="Rename, delete, restart and fork have no agent-manager command yet; use its TUI (r, d, R, f).">
              More in agent-manager
            </p>
          )}
        </div>
      )}
      {error && !confirmStop && <span className="form-error">{error}</span>}
      {confirmStop && (
        <ConfirmDialog
          title={`Stop ${session.name}?`}
          message="The agent's process ends, interrupting anything it is doing. The session stays in the list, and Revive resumes the same conversation."
          confirm="Stop session"
          danger
          busy={busy}
          error={error}
          onConfirm={() => {
            run({ action: 'kill' })
          }}
          onClose={() => {
            setConfirmStop(false)
            setError(null)
          }}
        />
      )}
    </div>
  )
}
