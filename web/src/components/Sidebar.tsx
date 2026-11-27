import { can, levelLabels, useRemote } from '../remote-context'
import { useEffect, useState } from 'react'
import type { ConnectionState } from '../api'
import { needsAttention, type Acks } from '../model/attention'
import { conflictTitle, type Conflicts } from '../model/conflicts'
import { hasNew, type Seen } from '../model/seen'
import { archivedCount, type SidebarGroup } from '../model/sidebar'
import { loadCollapsed, saveCollapsed, toggled, visibleSessions } from '../model/collapse'
import type { Permission } from '../notify'
import type { Prefs } from '../store'
import type { Project, State, SubAgent } from '../types'
import { glyph } from './status'
import { agentType, sidebarAgents } from '../model/agents'
import { SystemStats } from './SystemStats'
import type { SystemStats as Stats } from '../types'

export interface ViewOptions {
  showArchived: boolean
  onlyAttention: boolean
}

interface Props {
  state: State | null
  conn: ConnectionState
  groups: SidebarGroup[]
  acks: Acks
  seen: Seen
  conflicts: Conflicts
  attention: number
  view: ViewOptions
  onView: (v: ViewOptions) => void
  selected: string | null
  onSelect: (id: string) => void
  onStartManager: () => Promise<string | null>
  onSetup: () => void
  // subagents is the selected session's subagents, nested under its row.
  subagents: SubAgent[] | null
  // currentAgent is the subagent open in the main view, if any.
  currentAgent: string | null
  onOpenAgent: (agentId: string) => void
  // stats is the system stats feed; null hides the block.
  stats: { subscribe: (l: () => void) => () => void; get: () => Stats | null } | null
  prefs: Prefs
  permission: Permission
  onPrefs: (p: Partial<Prefs>) => void
  onRequestPermission: () => void
  onOpenSettings: () => void
  search: string
  onSearch: (text: string) => void
  onNewSession: (project: Project | null) => void
  onNewGroup: () => void
  onDeleteGroup: (path: string) => void
  onHome: () => void
  onHistory: () => void
  historyOpen: boolean
  onUsage: () => void
  usageOpen: boolean
}

export function Sidebar(props: Props) {
  const remoteInfo = useRemote()
  const { state, conn, groups, acks, attention, view, onView, selected, onSelect, onStartManager } = props
  const sessions = state?.sessions ?? []
  const archived = archivedCount(sessions)
  // Folded groups are remembered. The Needs you filter shows everything it
  // finds, so it ignores them.
  const [collapsed, setCollapsed] = useState(() => loadCollapsed(localStorage))
  useEffect(() => {
    saveCollapsed(localStorage, collapsed)
  }, [collapsed])

  return (
    <nav className="sidebar" aria-label="Sessions">
      <header className="sidebar-head">
        <button type="button" className="brand" title="Overview" onClick={props.onHome}>
          Agent Desk
        </button>
        <span className="head-right">
          {can(remoteInfo, 'manage') && (
            <button
              type="button"
              className="icon-button"
              aria-label="New session"
              title="New session"
              onClick={() => {
                props.onNewSession(null)
              }}
            >
              ＋
            </button>
          )}
          <button type="button" className="icon-button" aria-label="Settings" title="Settings" onClick={props.onOpenSettings}>
            ⚙
          </button>
          <span className={`conn conn-${conn.kind}`} title={connTitle(conn)} />
        </span>
      </header>
      <div className="search-box">
        <input
          id="search-box"
          type="search"
          value={props.search}
          placeholder="Search transcripts  /"
          aria-label="Search transcripts"
          onChange={(e) => {
            props.onSearch(e.target.value)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Escape') {
              props.onSearch('')
              e.currentTarget.blur()
            }
          }}
        />
      </div>
      <Banners state={state} conn={conn} onStartManager={onStartManager} onSetup={props.onSetup} />
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
        {groups.map((g) => {
          const folded = !view.onlyAttention && collapsed.has(g.project.key)
          const shown = visibleSessions(g.sessions, folded, selected)
          return (
          <section key={g.project.key} className={folded ? 'group folded' : 'group'}>
            <h2 className="group-head" title={g.project.root ?? g.project.name}>
              <button
                type="button"
                className="group-toggle"
                aria-expanded={!folded}
                aria-label={`${folded ? 'Show' : 'Hide'} sessions in ${g.project.name}`}
                title={folded ? 'Show sessions' : 'Hide sessions'}
                onClick={() => {
                  setCollapsed(toggled(collapsed, g.project.key))
                }}
              >
                <span className="group-chevron" aria-hidden>
                  {folded ? '▸' : '▾'}
                </span>
                <span className="group-name">{g.project.name}</span>
              </button>
              {folded && g.sessions.length > 0 && <span className="group-count">{g.sessions.length}</span>}
              {g.attention > 0 && <span className="badge">{g.attention}</span>}
              <span className="group-actions">
                {can(remoteInfo, 'manage') && (
                <button
                  type="button"
                  className="icon-button"
                  aria-label={`New session in ${g.project.name}`}
                  title="New session here"
                  onClick={() => {
                    props.onNewSession(g.project)
                  }}
                >
                  ＋
                </button>
                )}
                {g.project.kind === 'group' && can(remoteInfo, 'manage') && (
                  <button
                    type="button"
                    className="icon-button"
                    aria-label={`Delete group ${g.project.name}`}
                    title="Delete group"
                    onClick={() => {
                      props.onDeleteGroup(g.project.name)
                    }}
                  >
                    ×
                  </button>
                )}
              </span>
            </h2>
            {g.sessions.length === 0 && <p className="muted pad empty-group">No sessions yet.</p>}
            <ul>
              {shown.map((s) => (
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
                    {props.conflicts[s.id] && (
                      <span className="conflict-mark" title={conflictTitle(props.conflicts[s.id] ?? [], sessions)} aria-label="shares files with another session">
                        ⚠
                      </span>
                    )}
                    {s.id !== selected && hasNew(s, props.seen) && (
                      <span className="unread" title="New activity since you last looked" aria-label="new activity" />
                    )}
                    {s.tool !== 'claude' && <span className="row-tool">{s.tool}</span>}
                    <span className="row-status">{s.status}</span>
                  </button>
                  {s.id === selected && props.subagents && props.subagents.length > 0 && (
                    <SubagentRows agents={props.subagents} current={props.currentAgent} onOpen={props.onOpenAgent} />
                  )}
                </li>
              ))}
            </ul>
          </section>
          )
        })}
      </div>
      {props.stats && <SystemStats subscribe={props.stats.subscribe} get={props.stats.get} />}
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
        {can(remoteInfo, 'manage') && (
          <button type="button" className="link-button" onClick={props.onNewGroup}>
            New group
          </button>
        )}
        <button type="button" className="link-button history-link" aria-current={props.historyOpen ? 'page' : undefined} onClick={props.onHistory}>
          All conversations
        </button>
        <button type="button" className="link-button history-link" aria-current={props.usageOpen ? 'page' : undefined} onClick={props.onUsage}>
          Usage
        </button>
        <NotifyControls {...props} />
        {remoteInfo.remote && (
          <p className="remote-badge" title="You are on a remote device. The level is changed on the desk.">
            Remote · {levelLabels[remoteInfo.level]}
            {remoteInfo.user ? ` · ${remoteInfo.user}` : ''}
          </p>
        )}
        <p className="keys">⌘K switch · j/k next that needs you · / search</p>
      </footer>
    </nav>
  )
}

export function NotifyControls({ prefs, permission, onPrefs, onRequestPermission }: Pick<Props, 'prefs' | 'permission' | 'onPrefs' | 'onRequestPermission'>) {
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

function Banners({ state, conn, onStartManager, onSetup }: Pick<Props, 'state' | 'conn' | 'onStartManager' | 'onSetup'>) {
  const [starting, setStarting] = useState(false)
  const [startError, setStartError] = useState<string | null>(null)
  const remoteInfo = useRemote()
  const { remote } = remoteInfo
  const banners = []
  if (conn.kind === 'open' && conn.demo) {
    banners.push(
      <div key="demo" className="banner info">
        <strong>Demo:</strong> made-up sessions. Nothing real is read or changed; answer the waiting one and watch it work.{' '}
        <a href="https://agent-desk.app/#install" target="_blank" rel="noreferrer">
          Install Agent Desk
        </a>
      </div>,
    )
  }
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
        {!remote && (
          <button type="button" className="banner-button" onClick={onSetup}>
            Setup checklist
          </button>
        )}
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
        {can(remoteInfo, 'manage') && (
          <button type="button" className="banner-button" onClick={start} disabled={starting}>
            {starting ? 'Starting…' : 'Start headless manager'}
          </button>
        )}
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

// subagentRowsLimit is how many subagents show under a session before the
// rest fold into a count; running ones always show.
const subagentRowsLimit = 5

// SubagentRows lists a session's subagents under its sidebar row.
function SubagentRows({ agents, current, onOpen }: { agents: SubAgent[]; current: string | null; onOpen: (id: string) => void }) {
  const { shown, more } = sidebarAgents(agents, subagentRowsLimit)
  return (
    <ul className="subagent-rows" aria-label="Subagents">
      {shown.map((a) => (
        <li key={a.id}>
          <button
            type="button"
            className={`row subrow agent-${a.status}${a.id === current ? ' selected' : ''}`}
            aria-current={a.id === current ? 'page' : undefined}
            title={`${agentType(a.type).name}: ${a.description}`}
            onClick={() => {
              onOpen(a.id)
            }}
          >
            <span className="glyph" aria-hidden>
              {a.status === 'running' ? '◐' : a.status === 'failed' ? '✕' : '·'}
            </span>
            <span className="row-name">{a.description || agentType(a.type).name}</span>
            <span className="row-status">{a.status}</span>
          </button>
        </li>
      ))}
      {more > 0 && <li className="muted subrow-more">{String(more)} more in Agents</li>}
    </ul>
  )
}
