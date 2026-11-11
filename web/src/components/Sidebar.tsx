import { useState } from 'react'
import type { ConnectionState } from '../api'
import { needsAttention, type Acks } from '../model/attention'
import { archivedCount, type SidebarGroup } from '../model/sidebar'
import type { Permission } from '../notify'
import type { Prefs } from '../store'
import type { State } from '../types'
import { glyph } from './status'

export interface ViewOptions {
  showArchived: boolean
  onlyAttention: boolean
}

interface Props {
  state: State | null
  conn: ConnectionState
  groups: SidebarGroup[]
  acks: Acks
  attention: number
  view: ViewOptions
  onView: (v: ViewOptions) => void
  selected: string | null
  onSelect: (id: string) => void
  onStartManager: () => Promise<string | null>
  prefs: Prefs
  permission: Permission
  onPrefs: (p: Partial<Prefs>) => void
  onRequestPermission: () => void
}

export function Sidebar(props: Props) {
  const { state, conn, groups, acks, attention, view, onView, selected, onSelect, onStartManager } = props
  const sessions = state?.sessions ?? []
  const archived = archivedCount(sessions)

  return (
    <nav className="sidebar" aria-label="Sessions">
      <header className="sidebar-head">
        <span className="brand">Agent Desk</span>
        <span className={`conn conn-${conn.kind}`} title={connTitle(conn)} />
      </header>
      <Banners state={state} conn={conn} onStartManager={onStartManager} />
      <div className="filters">
        <button
          type="button"
          className={view.onlyAttention ? 'filter active' : 'filter'}
          aria-pressed={view.onlyAttention}
          onClick={() => {
            onView({ ...view, onlyAttention: !view.onlyAttention })
          }}
        >
          Needs you
          <span className={attention > 0 ? 'badge' : 'badge badge-zero'}>{attention}</span>
        </button>
      </div>
      <div className="groups">
        {state && groups.length === 0 && (
          <p className="muted pad">{view.onlyAttention ? 'Nothing needs you right now.' : 'No sessions. Start one in agent-manager.'}</p>
        )}
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
                    className={`row status-${s.status}${s.id === selected ? ' selected' : ''}${s.archived ? ' archived' : ''}${needsAttention(s, acks) ? ' attention' : ''}`}
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
      <footer className="sidebar-foot">
        {archived > 0 && (
          <label>
            <input
              type="checkbox"
              checked={view.showArchived}
              onChange={(e) => {
                onView({ ...view, showArchived: e.target.checked })
              }}
            />{' '}
            Show archived ({archived})
          </label>
        )}
        <NotifyControls {...props} />
        <p className="keys">⌘K switch · j/k next that needs you</p>
      </footer>
    </nav>
  )
}

function NotifyControls({ prefs, permission, onPrefs, onRequestPermission }: Props) {
  switch (permission) {
    case 'unsupported':
      return null
    case 'denied':
      return <p className="muted">Notifications are blocked in this browser's site settings.</p>
    case 'default':
      return (
        <button type="button" className="link-button" onClick={onRequestPermission}>
          Turn on notifications
        </button>
      )
    case 'granted':
      return (
        <>
          <label>
            <input
              type="checkbox"
              checked={prefs.notify}
              onChange={(e) => {
                onPrefs({ notify: e.target.checked })
              }}
            />{' '}
            Notify when a session needs me
          </label>
          {prefs.notify && (
            <label>
              <input
                type="checkbox"
                checked={prefs.notifyOnFinish}
                onChange={(e) => {
                  onPrefs({ notifyOnFinish: e.target.checked })
                }}
              />{' '}
              Also when a turn finishes
            </label>
          )}
        </>
      )
  }
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
