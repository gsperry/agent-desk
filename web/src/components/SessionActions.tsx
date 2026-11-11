import { useState } from 'react'
import type { GroupView, SessionAction, SessionView } from '../types'
import { ConfirmDialog } from './Dialog'

interface Props {
  session: SessionView
  groups: GroupView[]
  act: (id: string, a: SessionAction) => Promise<string | null>
}

// SessionActions are the lifecycle buttons in the session header. Stopping
// asks first, since it interrupts whatever the agent is doing.
export function SessionActions({ session, groups, act }: Props) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [confirmStop, setConfirmStop] = useState(false)

  const run = (a: SessionAction) => {
    setBusy(true)
    setError(null)
    void act(session.id, a).then((err) => {
      setBusy(false)
      setError(err)
      if (!err) setConfirmStop(false)
    })
  }

  return (
    <div className="session-actions">
      {session.running ? (
        <button
          type="button"
          className="chip chip-button"
          disabled={busy}
          onClick={() => {
            setConfirmStop(true)
          }}
        >
          Stop
        </button>
      ) : (
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
        className="chip chip-button"
        disabled={busy}
        onClick={() => {
          run({ action: session.archived ? 'restore' : 'archive' })
        }}
      >
        {session.archived ? 'Restore' : 'Archive'}
      </button>
      <select
        className="chip move-select"
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
      <span
        className="muted actions-note"
        title="agent-manager has no command for these yet; use its TUI (r, d, R, f)."
      >
        Rename, delete, restart, fork: in agent-manager
      </span>
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
