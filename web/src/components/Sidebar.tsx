import { useState } from 'react'
import type { ConnectionState } from '../api'
import { archivedCount, groupSessions } from '../model/sidebar'
import type { State } from '../types'
import { glyph } from './status'

interface Props {
  state: State | null
  conn: ConnectionState
  selected: string | null
  onSelect: (id: string) => void
  onStartManager: () => Promise<string | null>
}

export function Sidebar({ state, conn, selected, onSelect, onStartManager }: Props) {
  const [showArchived, setShowArchived] = useState(false)
  const sessions = state?.sessions ?? []
  const groups = groupSessions(sessions, { showArchived })
  const archived = archivedCount(sessions)

  return (
    <nav className="sidebar" aria-label="Sessions">
      <header className="sidebar-head">
        <span className="brand">Agent Desk</span>
        <span className={`conn conn-${conn.kind}`} title={connTitle(conn)} />
      </header>
      <Banners state={state} conn={conn} onStartManager={onStartManager} />
      <div className="groups">
        {state && groups.length === 0 && <p className="muted pad">No sessions. Start one in agent-manager.</p>}
        {groups.map((g) => (
          <section key={g.project.key} className="group">
            <h2 className="group-head" title={g.project.root ?? g.project.name}>
              <span className="group-name">{g.project.name}</span>
              {g.attention > 0 && <span className="badge">{g.attention}</span>}
            </h2>
            <ul>
              {g.sessions.map((s) => (
                <li key={s.id}>
                  <button
                    type="button"
                    className={`row status-${s.status}${s.id === selected ? ' selected' : ''}${s.archived ? ' archived' : ''}`}
                    onClick={() => {
                      onSelect(s.id)
                    }}
                    aria-current={s.id === selected ? 'page' : undefined}
                  >
                    <span className="glyph" aria-hidden>
                      {glyph(s.status)}
                    </span>
                    <span className="row-name">{s.name}</span>
                    {s.tool !== 'claude' && <span className="row-tool">{s.tool}</span>}
                    <span className="row-status">{s.status}</span>
                  </button>
                </li>
              ))}
            </ul>
          </section>
        ))}
      </div>
      {archived > 0 && (
        <footer className="sidebar-foot">
          <label>
            <input
              type="checkbox"
              checked={showArchived}
              onChange={(e) => {
                setShowArchived(e.target.checked)
              }}
            />{' '}
            Show archived ({archived})
          </label>
        </footer>
      )}
    </nav>
  )
}

function connTitle(c: ConnectionState): string {
  switch (c.kind) {
    case 'open':
      return `Connected (agent-desk ${c.version})`
    case 'connecting':
      return 'Connecting'
    case 'closed':
      return 'Disconnected, retrying'
    case 'unauthorized':
      return 'Token expired'
  }
}

function Banners({ state, conn, onStartManager }: Pick<Props, 'state' | 'conn' | 'onStartManager'>) {
  const [starting, setStarting] = useState(false)
  const [startError, setStartError] = useState<string | null>(null)
  const banners = []
  if (conn.kind === 'closed') {
    banners.push(
      <div key="conn" className="banner warn">
        Lost connection to Agent Desk. Retrying in {Math.ceil(conn.retryInMs / 1000)} s.
      </div>,
    )
  }
  if (state?.error?.kind === 'not_installed') {
    banners.push(
      <div key="am" className="banner error">
        agent-manager was not found on PATH. Install it, then this list fills in on its own.
      </div>,
    )
  } else if (state?.error) {
    banners.push(
      <div key="am" className="banner error">
        Cannot read agent-manager: {state.error.message}. The list below may be out of date.
      </div>,
    )
  } else if (state && !state.managerAwake) {
    const start = () => {
      setStarting(true)
      setStartError(null)
      void onStartManager().then((err) => {
        setStarting(false)
        setStartError(err)
      })
    }
    banners.push(
      <div key="awake" className="banner warn">
        agent-manager is not running, so statuses are frozen.
        <button type="button" className="banner-button" onClick={start} disabled={starting}>
          {starting ? 'Starting…' : 'Start headless manager'}
        </button>
        {startError && <span className="banner-detail">{startError}</span>}
      </div>,
    )
  }
  if (state?.insideSession) {
    banners.push(
      <div key="inside" className="banner warn">
        Agent Desk is running inside an agent-manager session, where sending is refused. Start it from a regular terminal.
      </div>,
    )
  }
  return banners.length ? <div className="banners">{banners}</div> : null
}
