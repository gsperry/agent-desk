import { useCallback, useEffect, useMemo, useState } from 'react'
import { loadingDoc, type Doc } from '../model/docs'
import type { Pending } from '../model/pending'
import { buildHeader } from '../model/transcript'
import type { SessionView as Session } from '../types'
import type { PathRef } from '../model/paths'
import { OpenPathContext, SessionDirContext } from '../session-context'
import type { Answer, GroupView, Screen, SessionAction } from '../types'
import { AgentLinksContext } from '../agents-context'
import { agentCounts } from '../model/agents'
import type { CatchUp, Changes, Item, SlashCommand, SubAgent } from '../types'
import { AgentsPanel } from './AgentsPanel'
import { NotesPanel } from './NotesPanel'
import { togglePin } from '../model/pins'
import type { Suggestion } from '../model/rules'
import { costLine } from '../model/summaries'
import { PinsContext, pinsFor, useNotebook } from '../pins-context'
import { ChangesPanel } from './ChangesPanel'
import { blockedReason } from '../model/compose'
import { conflictTitle, type Conflict } from '../model/conflicts'
import { Composer } from './Composer'
import { can, useRemote } from '../remote-context'
import { ContextMeter } from './ContextMeter'
import { LiveActivity } from './LiveActivity'
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
  readStyled: (id: string) => Promise<{ screen?: string; message?: string }>
  answer: (id: string, a: Answer) => Promise<string | null>
  interrupt: (id: string) => Promise<string | null>
  loadOverview: () => Promise<Record<string, { promptAt?: string }> | null>
  openPath: (id: string, ref: PathRef) => void
  groups: GroupView[]
  act: (id: string, a: SessionAction) => Promise<string | null>
  // seenMark is how far the reader had looked before opening this session.
  seenMark?: { seq: number }
  readAgents: (id: string) => Promise<SubAgent[] | null>
  readAgentTranscript: (id: string, agentId: string) => Promise<Item[] | null>
  readChanges: (id: string, mode: Changes['mode']) => Promise<{ data?: Changes; error?: string }>
  conflicts: Conflict[]
  sessions: Session[]
  pasteImage: (image: Blob) => Promise<{ path?: string; error?: string }>
  exportConversation: (conversationId: string) => void
  suggestion?: Suggestion
  dismissSuggestion: (sessionId: string) => void
  loadCommands: (sessionId: string) => Promise<SlashCommand[]>
  // catchUp is set when catch-up summaries are turned on in Settings.
  catchUp?: (sessionId: string, since: number) => Promise<{ data?: CatchUp; error?: string }>
}

// The subagent list refreshes this often while a session is open.
const agentsRefreshMs = 5000

export function SessionView(props: Props) {
  const { session, sessionId, doc, pending, connected, insideSession, follow, unfollow, loadOlder, send, dismiss, readScreen, answer, openPath } = props
  const remoteInfo = useRemote()
  const mayReply = can(remoteInfo, 'reply')
  const openHere = useCallback(
    (ref: PathRef) => {
      openPath(sessionId, ref)
    },
    [openPath, sessionId],
  )
  const { loadOverview } = props
  const promptAt = useCallback(() => loadOverview().then((o) => o?.[sessionId]?.promptAt), [loadOverview, sessionId])
  const [sends, setSends] = useState(0)
  const [showPane, setShowPane] = useState(false)
  // Captured once, so the divider stays put while the session is marked
  // read underneath it.
  const [markAtOpen] = useState(props.seenMark)
  const [agents, setAgents] = useState<SubAgent[]>([])
  const [agentsOpen, setAgentsOpen] = useState(false)
  const [changesOpen, setChangesOpen] = useState(false)
  const [notesOpen, setNotesOpen] = useState(false)
  const [catchUpState, setCatchUpState] = useState<{ busy: boolean; data?: CatchUp; error?: string } | null>(null)
  const [notebook, setNotebook] = useNotebook(props.session?.agentSessionId)
  const pins = pinsFor(notebook, setNotebook, togglePin)
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
    <OpenPathContext.Provider value={can(remoteInfo, 'local') ? openHere : null}>
    <SessionDirContext.Provider value={session.directory}>
    <AgentLinksContext.Provider value={agentLinks}>
    <PinsContext.Provider value={session.agentSessionId ? pins : null}>
        <main className="main">
          <header className="session-head">
            <div className="session-title">
              <span className={`glyph status-${session.status}`} aria-hidden>
                {glyph(session.status)}
              </span>
              <h1>{session.name}</h1>
              <span className={`chip status-chip status-${session.status}`}>
                {session.status}
                {counts.running > 0 && session.status !== 'working' ? ` · ${String(counts.running)} agent${counts.running === 1 ? '' : 's'} running` : ''}
              </span>
              <span className="session-info">
                {header.permissionMode && <span>{header.permissionMode}</span>}
                <ContextMeter items={d.items} />
              </span>
            {props.conflicts.length > 0 && (
            <span className="chip conflict-chip" title={conflictTitle(props.conflicts, props.sessions)}>
              ⚠ {props.conflicts.length} shared file{props.conflicts.length === 1 ? '' : 's'}
            </span>
          )}
          <div className="session-views" role="group" aria-label="Views">
          <button
            type="button"
            className="chip chip-button changes-toggle"
            aria-pressed={changesOpen}
            onClick={() => {
              setChangesOpen(!changesOpen)
              setAgentsOpen(false)
              setNotesOpen(false)
            }}
          >
            Changes
          </button>
          {counts.total > 0 && (
              <button
                type="button"
                className="chip chip-button agents-toggle"
                aria-pressed={agentsOpen}
                onClick={() => {
                  setAgentsOpen(!agentsOpen)
                  setOpenAgent(null)
                  setNotesOpen(false)
                }}
              >
                Agents {counts.running > 0 ? `${String(counts.running)} running · ` : ''}
                {counts.total}
              </button>
            )}
              {props.catchUp && can(remoteInfo, 'local') && isClaude && session.agentSessionId && (
                <button
                  type="button"
                  className="chip chip-button catchup-toggle"
                  disabled={catchUpState?.busy}
                  title="Summarize what happened since you last looked. Costs tokens on your Claude Code provider."
                  onClick={() => {
                    const run = props.catchUp
                    if (!run) return
                    setCatchUpState({ busy: true })
                    // Since the last look when something is new, else the most
                    // recent part of the transcript.
                    const latest = d.items.at(-1)?.seq
                    const since = markAtOpen && latest !== undefined && markAtOpen.seq < latest ? markAtOpen.seq : -1
                    void run(sessionId, since).then((r) => {
                      setCatchUpState({ busy: false, ...r })
                    })
                  }}
                >
                  {catchUpState?.busy ? 'Summarizing…' : 'Catch me up'}
                </button>
              )}
              {session.agentSessionId && (
                <button
                  type="button"
                  className="chip chip-button notes-toggle"
                  aria-pressed={notesOpen}
                  onClick={() => {
                    setNotesOpen(!notesOpen)
                    setAgentsOpen(false)
                    setChangesOpen(false)
                  }}
                >
                  Notes{notebook.pins.length > 0 ? ` · ${String(notebook.pins.length)}` : ''}
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
              <SessionActions
                session={session}
                groups={props.groups}
                act={props.act}
                manage={can(remoteInfo, 'manage')}
                onExport={
                  isClaude && session.agentSessionId && d.items.length > 0
                    ? () => {
                        if (session.agentSessionId) props.exportConversation(session.agentSessionId)
                      }
                    : undefined
                }
              />
            </div>
            <div className="session-sub">
              {header.title && <span>{header.title}</span>}
              <span className="muted" title={d.path ?? session.directory}>
                {session.project.name}
                {session.branch ? ` · ${session.branch}` : ''}
              </span>
            {header.prLinks.map((pr) => (
                <a key={pr.url} href={pr.url} target="_blank" rel="noreferrer noopener" className="pr-link">
                  PR {pr.number !== undefined ? `#${String(pr.number)}` : ''}
                </a>
              ))}
            </div>
          </header>
          <StatusBanner session={session} doc={d} />
          {catchUpState && !catchUpState.busy && (
            <section className="catchup" aria-label="Catch-up summary">
              {catchUpState.data ? (
                <div>
                  <p className="catchup-text">{catchUpState.data.text}</p>
                  <p className="muted catchup-cost">{costLine(catchUpState.data)}</p>
                </div>
              ) : (
                <p className="catchup-error">{catchUpState.error ?? 'The summary failed.'}</p>
              )}
              <button
                type="button"
                className="link-button"
                onClick={() => {
                  setCatchUpState(null)
                }}
              >
                Close
              </button>
            </section>
          )}
          {d.items.length > 0 || pending.length > 0 ? (
            <Transcript
              sessionId={sessionId}
              items={d.items}
              hasMore={d.hasMore}
              loadingOlder={d.loadingOlder}
              live={session.running}
              followKey={sends}
            markAtOpen={markAtOpen}
          filterable
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
              readStyled={props.readStyled}
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
          {props.suggestion && mayReply && (
            <div className="suggestion" role="status" aria-label="Suggested reply">
              <div className="suggestion-text">
                <span className="muted">Rule “{props.suggestion.ruleName}” suggests replying:</span>
                <p>{props.suggestion.text}</p>
              </div>
              <div className="suggestion-actions">
                <button
                  type="button"
                  className="send"
                  disabled={blockedReason(session, connected, insideSession) !== null}
                  title={blockedReason(session, connected, insideSession) ?? undefined}
                  onClick={() => {
                    if (!props.suggestion) return
                    send(sessionId, props.suggestion.text)
                    setSends((n) => n + 1)
                    props.dismissSuggestion(sessionId)
                  }}
                >
                  Send
                </button>
                <button
                  type="button"
                  className="secondary"
                  onClick={() => {
                    if (!props.suggestion) return
                    setRestore({ text: props.suggestion.text, nonce: Date.now() })
                    props.dismissSuggestion(sessionId)
                  }}
                >
                  Edit
                </button>
                <button
                  type="button"
                  className="link-button"
                  onClick={() => {
                    props.dismissSuggestion(sessionId)
                  }}
                >
                  Dismiss
                </button>
              </div>
            </div>
          )}
          {session.running && session.status === 'working' && (
            <LiveActivity
              items={d.items}
              dir={session.directory}
              promptAt={promptAt}
              onInterrupt={
                mayReply
                  ? () => props.interrupt(sessionId)
                  : undefined
              }
            />
          )}
          {mayReply ? (
            <Composer
              key={sessionId}
              session={session}
              connected={connected}
              insideSession={insideSession}
              restore={restore}
              pasteImage={can(remoteInfo, 'local') ? props.pasteImage : undefined}
              loadCommands={props.loadCommands}
              onSend={(text) => {
                send(sessionId, text)
                setSends((n) => n + 1)
              }}
            />
          ) : (
            <p className="muted remote-no-reply">Replies are off for remote devices at this level. Waiting prompts can still be answered in the Terminal panel.</p>
          )}
          {changesOpen && (
          <ChangesPanel
            sessionId={sessionId}
            canSend={mayReply ? blockedReason(session, connected, insideSession) : 'Replies are off for remote devices at this level.'}
            load={props.readChanges}
            onSend={(text) => {
              send(sessionId, text)
              setSends((n) => n + 1)
              setChangesOpen(false)
            }}
            onClose={() => {
              setChangesOpen(false)
            }}
          />
        )}
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
        {notesOpen && (
          <NotesPanel
            notebook={notebook}
            onChange={setNotebook}
            onClose={() => {
              setNotesOpen(false)
            }}
          />
        )}
        </main>
    </PinsContext.Provider>
    </AgentLinksContext.Provider>
    </SessionDirContext.Provider>
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
