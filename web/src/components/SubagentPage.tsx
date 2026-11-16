import { useEffect, useState } from 'react'
import { AgentLinksContext } from '../agents-context'
import { agentDuration, agentType, childAgents } from '../model/agents'
import type { Item, SessionView, SubAgent } from '../types'
import { LiveActivity } from './LiveActivity'
import { Transcript } from './Transcript'

interface Props {
  sessionId: string
  session: SessionView | undefined
  agentId: string
  // agents is the session's subagents, refreshed by the caller; null until
  // first read.
  agents: SubAgent[] | null
  readTranscript: (sessionId: string, agentId: string) => Promise<Item[] | null>
  onBack: () => void
  onOpenAgent: (agentId: string) => void
}

// A running subagent's transcript refreshes this often.
const refreshMs = 3000

// SubagentPage is one subagent of a session, read live: what it was asked,
// every step it takes, the subagents it starts in turn, and a live row
// while it works.
export function SubagentPage({ sessionId, session, agentId, agents, readTranscript, onBack, onOpenAgent }: Props) {
  const agent = agents?.find((a) => a.id === agentId)
  const running = agent?.status === 'running'
  const [items, setItems] = useState<Item[] | null | undefined>(undefined)
  const [tick, setTick] = useState(0)

  useEffect(() => {
    let live = true
    void readTranscript(sessionId, agentId).then((r) => {
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
  }, [sessionId, agentId, readTranscript, running, tick])

  const children = agents && items ? childAgents(items, agents) : []
  const links = {
    agentFor: (toolUseId: string) => agents?.find((a) => a.toolUseId === toolUseId)?.id,
    has: (id: string) => agents?.some((a) => a.id === id) ?? false,
    open: onOpenAgent,
  }
  const t = agentType(agent?.type ?? '')
  const parent = session?.name ?? sessionId

  return (
    <main className="main subagent-view" aria-label="Subagent">
      <header className="session-head">
        <nav className="subagent-crumbs" aria-label="Breadcrumb">
          <button type="button" className="link-button" onClick={onBack}>
            ‹ {parent}
          </button>
          <span className="muted"> / subagent</span>
        </nav>
        <div className="session-title">
          <h1>{agent?.description || (agents ? 'Subagent' : 'Loading…')}</h1>
          {agent && <span className={`chip agent-status agent-${agent.status}`}>{agent.status}</span>}
        </div>
        {agent && (
          <div className="session-sub muted" title={agent.type}>
            {[t.name, t.plugin, agent.model, agent.depth > 1 ? `depth ${String(agent.depth)}` : '', agentDuration(agent)].filter(Boolean).join(' · ')}
          </div>
        )}
      </header>
      {agents && !agent && <p className="banner warn">This session has no subagent {agentId}. It may belong to an earlier conversation.</p>}
      {children.length > 0 && (
        <section className="subagent-children" aria-label="Subagents it started">
          <span className="muted">Started:</span>
          {children.map((c) => (
            <button
              key={c.id}
              type="button"
              className={`chip-button agent-${c.status}`}
              onClick={() => {
                onOpenAgent(c.id)
              }}
            >
              {c.description || agentType(c.type).name} · {c.status}
            </button>
          ))}
        </section>
      )}
      {items === undefined && <div className="empty">Reading the transcript…</div>}
      {items === null && <div className="empty">This subagent's transcript could not be read.</div>}
      {items && (
        <AgentLinksContext.Provider value={links}>
          <Transcript sessionId={`${sessionId}/${agentId}`} items={items} hasMore={false} loadingOlder={false} live={running} onLoadOlder={() => undefined} />
        </AgentLinksContext.Provider>
      )}
      {items && running && (
        <div className="composer-wrap subagent-foot">
          <LiveActivity items={items} />
        </div>
      )}
    </main>
  )
}
