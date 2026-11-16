import { agentDuration, agentType, sortAgents } from '../model/agents'
import type { SubAgent } from '../types'

interface Props {
  agents: SubAgent[]
  // onOpen opens a subagent's own view.
  onOpen: (id: string) => void
  onClose: () => void
}

// AgentsPanel lists a session's subagents, running first; each opens its
// own view.
export function AgentsPanel({ agents, onOpen, onClose }: Props) {
  return (
    <aside className="agents-panel" aria-label="Subagents">
      <header className="agents-head">
        <strong>Subagents</strong>
        <button type="button" className="link-button" onClick={onClose}>
          Close
        </button>
      </header>
      <ul className="agent-list">
        {sortAgents(agents).map((a) => {
          const t = agentType(a.type)
          return (
            <li key={a.id}>
              <button
                type="button"
                className={`agent-row agent-${a.status}`}
                onClick={() => {
                  onOpen(a.id)
                }}
              >
                <span className="agent-top">
                  <span className="agent-status">{a.status}</span>
                  <span className="agent-type" title={a.type}>
                    {t.name}
                  </span>
                  {a.depth > 1 && <span className="chip">depth {a.depth}</span>}
                  <span className="muted agent-time">{agentDuration(a)}</span>
                </span>
                <span className="agent-desc">{a.description || '(no description)'}</span>
              </button>
            </li>
          )
        })}
        {agents.length === 0 && <li className="muted pad">This session has not spawned subagents.</li>}
      </ul>
    </aside>
  )
}
