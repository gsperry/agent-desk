import { useEffect, useRef, useState } from 'react'
import { AgentLinksContext } from '../agents-context'
import { agentDuration, agentType, childAgents } from '../model/agents'
import { enterAction, insertNewline, isTouch, nativeNewline } from '../model/compose'
import { can, useRemote } from '../remote-context'
import type { Item, SessionView, SubAgent } from '../types'
import { LiveActivity } from './LiveActivity'
import { Transcript } from './Transcript'
import { ConfirmDialog } from './Dialog'
import { MicButton } from './MicButton'
import { useDictation } from '../dictate'

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
  // sendMessage messages the subagent; it resolves to a refusal, or null.
  sendMessage: (agentId: string, text: string) => Promise<string | null>
  // stopAgent stops the subagent; it resolves to a refusal, or null.
  stopAgent: (agentId: string) => Promise<string | null>
}

// A running subagent's transcript refreshes this often.
const refreshMs = 3000

// SubagentPage is one subagent of a session, read live: what it was asked,
// every step it takes, the subagents it starts in turn, and a live row
// while it works.
export function SubagentPage({ sessionId, session, agentId, agents, readTranscript, onBack, onOpenAgent, sendMessage, stopAgent }: Props) {
  const [stopping, setStopping] = useState<{ busy: boolean; error: string | null } | null>(null)
  const remote = useRemote()
  const agent = agents?.find((a) => a.id === agentId)
  const running = agent?.status === 'running'
  const [items, setItems] = useState<Item[] | null | undefined>(undefined)
  const [tick, setTick] = useState(0)

  useEffect(() => {
    let live = true
    void readTranscript(sessionId, agentId).then((r) => {
      if (live) setItems(r)
    })
    const t = running
      ? setTimeout(() => {
          setTick((n) => n + 1)
        }, refreshMs)
      : undefined
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
          {running && can(remote, 'reply') && session?.running && (
            <button
              type="button"
              className="secondary subagent-stop"
              onClick={() => {
                setStopping({ busy: false, error: null })
              }}
            >
              Stop
            </button>
          )}
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
        <div className="subagent-foot">
          <LiveActivity items={items} />
          {can(remote, 'reply') && session?.running && (
            <MessageBox
              type={agent.type}
              send={(text) => sendMessage(agentId, text)}
              onSent={() => {
                setTick((n) => n + 1)
              }}
            />
          )}
        </div>
      )}
      {stopping && (
        <ConfirmDialog
          title="Stop this subagent?"
          message="It stops at once, as x does in Claude Code's agent list. Its work so far stays, and the main agent is told it was stopped."
          confirm="Stop subagent"
          danger
          busy={stopping.busy}
          error={stopping.error}
          onConfirm={() => {
            setStopping({ busy: true, error: null })
            void stopAgent(agentId).then((err) => {
              setStopping(err ? { busy: false, error: err } : null)
              if (!err) setTick((n) => n + 1)
            })
          }}
          onClose={() => {
            setStopping(null)
          }}
        />
      )}
    </main>
  )
}

// MessageBox sends a message to the subagent. Enter sends, as in Claude
// Code; Shift, Cmd or Ctrl with Enter adds a line.
function MessageBox({ type, send, onSent }: { type: string; send: (text: string) => Promise<string | null>; onSent: () => void }) {
  const [text, setText] = useState('')
  const [busy, setBusy] = useState(false)
  const [note, setNote] = useState<{ error: boolean; text: string } | null>(null)
  const box = useRef<HTMLTextAreaElement>(null)
  const [touch] = useState(isTouch)
  const dictation = useDictation(text, setText, box)
  const submit = () => {
    const t = text.trim()
    if (!t || busy) return
    dictation.interrupt()
    setBusy(true)
    setNote(null)
    void send(t).then((err) => {
      setBusy(false)
      if (err) {
        setNote({ error: true, text: err })
        return
      }
      setText('')
      setNote({ error: false, text: 'Sent. Claude Code queues it, and the subagent reads it at its next step.' })
      onSent()
    })
  }
  return (
    <form
      className="subagent-message"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <textarea
        ref={box}
        rows={2}
        aria-label={`Message the subagent`}
        placeholder={dictation.phase === 'listening' ? 'Listening…' : `Message @${agentType(type).name}…`}
        value={text}
        disabled={busy}
        onChange={(e) => {
          dictation.interrupt()
          setText(e.target.value)
        }}
        onKeyDown={(e) => {
          const enter = enterAction(e.nativeEvent, touch)
          if (enter === 'send') {
            e.preventDefault()
            submit()
          } else if (enter === 'newline' && !nativeNewline(e.nativeEvent, touch)) {
            e.preventDefault()
            insertNewline(e.currentTarget)
          }
        }}
      />
      {dictation.available && <MicButton phase={dictation.phase} onToggle={dictation.toggle} />}
      <button type="submit" className="send" disabled={busy || !text.trim()}>
        {busy ? 'Sending…' : 'Send'}
      </button>
      {note && <p className={note.error ? 'form-error' : 'muted subagent-sent'}>{note.text}</p>}
      {dictation.error && <p className="form-error">{dictation.error}</p>}
    </form>
  )
}
