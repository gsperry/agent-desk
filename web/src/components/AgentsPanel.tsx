import { useEffect, useState } from 'react'
import { agentDuration, agentType, sortAgents } from '../model/agents'
import type { Item, SubAgent } from '../types'
import { Transcript } from './Transcript'

interface Props {
  sessionId: string
  agents: SubAgent[]
  openAgent: string | null
  onOpen: (id: string | null) => void
  onClose: () => void
  readTranscript: (sessionId: string, agentId: string) => Promise<Item[] | null>
}

// A running subagent's transcript refreshes this often while open.
const refreshMs = 3000

export function AgentsPanel({ sessionId, agents, openAgent, onOpen, onClose, readTranscript }: Props) {
  const agent = openAgent ? agents.find((a) => a.id === openAgent) : undefined
  return (
    <aside className="agents-panel" aria-label="Subagents">
      <header className="agents-head">
        {agent ? (
          <button
            type="button"
            className="link-button"
            onClick={() => {
              onOpen(null)
            }}
          >
            ← All agents
          </button>
        ) : (
          <strong>Subagents</strong>
        )}
        <button type="button" className="link-button" onClick={onClose}>
          Close
        </button>
      </header>
      {agent ? (
        <SubagentView sessionId={sessionId} agent={agent} readTranscript={readTranscript} />
      ) : (
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
      )}
    </aside>
  )
}

function SubagentView({ sessionId, agent, readTranscript }: { sessionId: string; agent: SubAgent; readTranscript: Props['readTranscript'] }) {
  const [items, setItems] = useState<Item[] | null | undefined>(undefined)
  const [tick, setTick] = useState(0)
  const running = agent.status === 'running'

  useEffect(() => {
    let live = true
    void readTranscript(sessionId, agent.id).then((r) => {
      if (live) setItems(r)
    })
    if (!running) return
    const t = setTimeout(() => {
      setTick((n) => n + 1)
    }, refreshMs)
    return () => {
      live = false
      clearTimeout(t)
    }
  }, [sessionId, agent.id, readTranscript, running, tick])

  const t = agentType(agent.type)
  return (
    <div className="subagent">
      <div className="subagent-meta">
        <strong>{t.name}</strong>
        {t.plugin && <span className="muted">{t.plugin}</span>}
        <span className={`agent-status agent-${agent.status}`}>{agent.status}</span>
        {agent.model && <span className="chip">{agent.model}</span>}
        <p className="agent-desc">{agent.description}</p>
      </div>
      {items === undefined && <p className="muted pad">Loading…</p>}
      {items === null && <p className="muted pad">This subagent's transcript could not be read.</p>}
      {items && (
        <Transcript
          sessionId={`${sessionId}/${agent.id}`}
          items={items}
          hasMore={false}
          loadingOlder={false}
          live={running}
          onLoadOlder={() => undefined}
        />
      )}
    </div>
  )
}
