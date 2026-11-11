import { useCallback, useEffect, useMemo, useState } from 'react'
import { loadingDoc, type Doc } from '../model/docs'
import type { Pending } from '../model/pending'
import { buildHeader } from '../model/transcript'
import type { SessionView as Session } from '../types'
import type { PathRef } from '../model/paths'
import { OpenPathContext } from '../session-context'
import type { Answer, GroupView, Screen, SessionAction } from '../types'
import { AgentLinksContext } from '../agents-context'
import { agentCounts } from '../model/agents'
import type { Item, SubAgent } from '../types'
import { AgentsPanel } from './AgentsPanel'
import { Composer } from './Composer'
import { ContextMeter } from './ContextMeter'
import { LivePane } from './LivePane'
import { PendingReplies } from './PendingReplies'
import { SessionActions } from './SessionActions'
import { glyph } from './status'
import { Transcript } from './Transcript'

interface Props {
  session: Session | undefined
  sessionId: string
  doc: Doc | undefined
  pending: Pending[]
  connected: boolean
  insideSession: boolean
  follow: (id: string) => void
  unfollow: (id: string) => void
  loadOlder: (id: string) => void
  send: (id: string, text: string) => void
  dismiss: (id: string, clientId: string) => void
  readScreen: (id: string) => Promise<{ screen?: Screen; message?: string }>
  answer: (id: string, a: Answer) => Promise<string | null>
  openPath: (id: string, ref: PathRef) => void
  groups: GroupView[]
  act: (id: string, a: SessionAction) => Promise<string | null>
  // seenMark is how far the reader had looked before opening this session.
  seenMark?: { seq: number }
  readAgents: (id: string) => Promise<SubAgent[] | null>
  readAgentTranscript: (id: string, agentId: string) => Promise<Item[] | null>
}

// The subagent list refreshes this often while a session is open.
const agentsRefreshMs = 5000

export function SessionView(props: Props) {
  const { session, sessionId, doc, pending, connected, insideSession, follow, unfollow, loadOlder, send, dismiss, readScreen, answer, openPath } = props
  const openHere = useCallback(
    (ref: PathRef) => {
      openPath(sessionId, ref)
    },
    [openPath, sessionId],
  )
  const [sends, setSends] = useState(0)
  const [showPane, setShowPane] = useState(false)
  // Captured once, so the divider stays put while the session is marked
  // read underneath it.
  const [markAtOpen] = useState(props.seenMark)
  const [agents, setAgents] = useState<SubAgent[]>([])
  const [agentsOpen, setAgentsOpen] = useState(false)
  const [openAgent, setOpenAgent] = useState<string | null>(null)
  const { readAgents } = props
  const isClaude = session?.tool === 'claude'
  useEffect(() => {
    if (!isClaude) return
    let live = true
    const load = () => {
      void readAgents(sessionId).then((list) => {
        if (live && list) setAgents(list)
      })
    }
    load()
    const t = setInterval(load, agentsRefreshMs)
    return () => {
      live = false
      clearInterval(t)
    }
  }, [readAgents, sessionId, isClaude])
  const agentLinks = useMemo(() => {
    const byToolUse = new Map(agents.map((a) => [a.toolUseId, a.id]))
    return {
      agentFor: (toolUseId: string) => byToolUse.get(toolUseId),
      open: (id: string) => {
        setOpenAgent(id)
        setAgentsOpen(true)
      },
    }
  }, [agents])
  const counts = agentCounts(agents)
  const [restore, setRestore] = useState<{ text: string; nonce: number } | undefined>()
  useEffect(() => {
    follow(sessionId)
    return () => {
      unfollow(sessionId)
    }
  }, [sessionId, follow, unfollow])

  const d = doc ?? loadingDoc
  const header = useMemo(() => buildHeader(d.items), [d.items])

  // The pane opens on its own when the agent waits, or when a reply was
  // refused because a dialog is on screen that the status did not show.
  const dialogRefusal = pending.some((p) => p.state === 'failed' && p.reason === 'dialog')
  const paneOpen = session?.running === true && (showPane || session.status === 'waiting' || dialogRefusal)

  if (!session) {
    return (
      <main className="main">
        <div className="empty">This session no longer exists in agent-manager.</div>
      </main>
    )
  }

  return (
    <OpenPathContext.Provider value={openHere}>
    <AgentLinksContext.Provider value={agentLinks}>
        <main className="main">
          <header className="session-head">
            <div className="session-title">
              <span className={`glyph status-${session.status}`} aria-hidden>
                {glyph(session.status)}
              </span>
              <h1>{session.name}</h1>
              <span className={`chip status-chip status-${session.status}`}>{session.status}</span>
              {header.permissionMode && <span className="chip">{header.permissionMode}</span>}
            <ContextMeter items={d.items} />
            {counts.total > 0 && (
              <button
                type="button"
                className="chip chip-button agents-toggle"
                aria-pressed={agentsOpen}
                onClick={() => {
                  setAgentsOpen(!agentsOpen)
                  setOpenAgent(null)
                }}
              >
                Agents {counts.running > 0 ? `${String(counts.running)} running · ` : ''}
                {counts.total}
              </button>
            )}
              {session.running && (
                <button
                  type="button"
                  className="chip chip-button terminal-toggle"
                  aria-pressed={paneOpen}
                  onClick={() => {
                    setShowPane(!showPane)
                  }}
                >
                  Terminal
                </button>
              )}
            </div>
            <div className="session-sub">
              {header.title && <span>{header.title}</span>}
              <span className="muted" title={d.path ?? session.directory}>
                {session.project.name}
                {session.branch ? ` · ${session.branch}` : ''}
              </span>
              <SessionActions session={session} groups={props.groups} act={props.act} />
            {header.prLinks.map((pr) => (
                <a key={pr.url} href={pr.url} target="_blank" rel="noreferrer noopener" className="chip">
                  PR {pr.number !== undefined ? `#${String(pr.number)}` : ''}
                </a>
              ))}
            </div>
          </header>
          <StatusBanner session={session} doc={d} />
          {d.items.length > 0 || pending.length > 0 ? (
            <Transcript
              sessionId={sessionId}
              items={d.items}
              hasMore={d.hasMore}
              loadingOlder={d.loadingOlder}
              live={session.running}
              followKey={sends}
            markAtOpen={markAtOpen}
              onLoadOlder={() => {
                loadOlder(sessionId)
              }}
              trailing={
                <PendingReplies
                  pending={pending}
                  onRetry={(p) => {
                    dismiss(sessionId, p.clientId)
                    send(sessionId, p.text)
                  }}
                  onEdit={(p) => {
                    dismiss(sessionId, p.clientId)
                    setRestore({ text: p.text, nonce: Date.now() })
                  }}
                  onDismiss={(p) => {
                    dismiss(sessionId, p.clientId)
                  }}
                />
              }
            />
          ) : (
            <div className="empty">{emptyText(session, d)}</div>
          )}
          {paneOpen && (
            <LivePane
              sessionId={sessionId}
              read={readScreen}
              answer={answer}
              onClose={
                showPane
                  ? () => {
                      setShowPane(false)
                    }
                  : undefined
              }
            />
          )}
          <Composer
            key={sessionId}
            session={session}
            connected={connected}
            insideSession={insideSession}
            restore={restore}
            onSend={(text) => {
              send(sessionId, text)
              setSends((n) => n + 1)
            }}
          />
          {agentsOpen && (
            <AgentsPanel
              sessionId={sessionId}
              agents={agents}
              openAgent={openAgent}
              onOpen={setOpenAgent}
              onClose={() => {
                setAgentsOpen(false)
                setOpenAgent(null)
              }}
              readTranscript={props.readAgentTranscript}
            />
          )}
        </main>
    </AgentLinksContext.Provider>
    </OpenPathContext.Provider>
  )
}

function StatusBanner({ session, doc }: { session: Session; doc: Doc }) {
  if (!session.running) {
    return <div className="banner info">This session is not running. Revive it in agent-manager with v to continue it.</div>
  }
  if (doc.status === 'gone') {
    return <div className="banner warn">The transcript file was deleted. Showing what was loaded before.</div>
  }
  return null
}

function emptyText(session: Session, doc: Doc): string {
  switch (doc.status) {
    case 'loading':
      return 'Loading transcript…'
    case 'pending':
      return 'Waiting for the first turn. Claude Code writes the transcript after its first reply.'
    case 'unsupported':
      return `Transcripts are available for Claude Code sessions. This session runs ${session.tool}.`
    case 'unknown_session':
      return 'This session no longer exists in agent-manager.'
    case 'gone':
      return 'The transcript file was deleted.'
    case 'ok':
      return 'The transcript has no messages yet.'
  }
}
