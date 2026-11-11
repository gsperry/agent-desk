import { useEffect, useMemo } from 'react'
import { loadingDoc, type Doc } from '../model/docs'
import { buildHeader } from '../model/transcript'
import type { SessionView as Session } from '../types'
import { glyph } from './status'
import { Transcript } from './Transcript'

interface Props {
  session: Session | undefined
  sessionId: string
  doc: Doc | undefined
  follow: (id: string) => void
  unfollow: (id: string) => void
  loadOlder: (id: string) => void
}

export function SessionView({ session, sessionId, doc, follow, unfollow, loadOlder }: Props) {
  useEffect(() => {
    follow(sessionId)
    return () => {
      unfollow(sessionId)
    }
  }, [sessionId, follow, unfollow])

  const d = doc ?? loadingDoc
  const header = useMemo(() => buildHeader(d.items), [d.items])

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
      {d.items.length > 0 ? (
        <Transcript
          sessionId={sessionId}
          items={d.items}
          hasMore={d.hasMore}
          loadingOlder={d.loadingOlder}
          live={session.running}
          onLoadOlder={() => {
            loadOlder(sessionId)
          }}
        />
      ) : (
        <div className="empty">{emptyText(session, d)}</div>
      )}
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
