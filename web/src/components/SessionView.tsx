import { useEffect, useMemo, useState } from 'react'
import { loadingDoc, type Doc } from '../model/docs'
import type { Pending } from '../model/pending'
import { buildHeader } from '../model/transcript'
import type { SessionView as Session } from '../types'
import type { Answer, Screen } from '../types'
import { Composer } from './Composer'
import { LivePane } from './LivePane'
import { PendingReplies } from './PendingReplies'
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
}

export function SessionView(props: Props) {
  const { session, sessionId, doc, pending, connected, insideSession, follow, unfollow, loadOlder, send, dismiss, readScreen, answer } = props
  const [sends, setSends] = useState(0)
  const [showPane, setShowPane] = useState(false)
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
    <main className="main">
      <header className="session-head">
        <div className="session-title">
          <span className={`glyph status-${session.status}`} aria-hidden>
            {glyph(session.status)}
          </span>
          <h1>{session.name}</h1>
          <span className={`chip status-chip status-${session.status}`}>{session.status}</span>
          {header.permissionMode && <span className="chip">{header.permissionMode}</span>}
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
    </main>
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
