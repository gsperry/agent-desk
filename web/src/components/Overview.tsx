import { useEffect, useState } from 'react'
import type { Acks } from '../model/attention'
import { formatTokens, loadThresholds } from '../model/context'
import { ago, overviewCards, plainText } from '../model/overview'
import { hasNew, type Seen } from '../model/seen'
import type { SessionView, Summary } from '../types'
import { glyph } from './status'

interface Props {
  sessions: SessionView[]
  acks: Acks
  seen: Seen
  load: () => Promise<Record<string, Summary> | null>
  onOpen: (id: string) => void
}

// The overview refreshes this often while it is on screen.
const refreshMs = 4000

// Overview is the home screen: a card per session, what needs the human
// first.
export function Overview({ sessions, acks, seen, load, onOpen }: Props) {
  const [summaries, setSummaries] = useState<Record<string, Summary>>({})
  const [showStopped, setShowStopped] = useState(false)
  const [now, setNow] = useState(() => Date.now())
  const [thresholds] = useState(() => loadThresholds(localStorage))

  useEffect(() => {
    let live = true
    const tick = () => {
      void load().then((s) => {
        if (!live) return
        if (s) setSummaries(s)
        setNow(Date.now())
      })
    }
    tick()
    const t = setInterval(tick, refreshMs)
    return () => {
      live = false
      clearInterval(t)
    }
  }, [load])

  const cards = overviewCards(sessions, acks, showStopped)
  const stopped = sessions.filter((s) => !s.archived && !s.running).length

  return (
    <main className="main overview" aria-label="Overview">
      <header className="session-head">
        <div className="session-title">
          <h1>Overview</h1>
          <span className="muted">{cards.length} sessions</span>
          {stopped > 0 && (
            <label className="muted overview-stopped">
              <input
                type="checkbox"
                checked={showStopped}
                onChange={(e) => {
                  setShowStopped(e.target.checked)
                }}
              />{' '}
              Show stopped ({stopped})
            </label>
          )}
        </div>
      </header>
      <div className="transcript">
        <div className="cards">
          {cards.map((s) => {
            const sum = summaries[s.id]
            const limit = sum?.model ? thresholds[sum.model] : undefined
            return (
              <button
                key={s.id}
                type="button"
                className={`card session-card status-${s.status}`}
                onClick={() => {
                  onOpen(s.id)
                }}
              >
                <span className="card-line">
                  <span className="glyph" aria-hidden>
                    {glyph(s.status)}
                  </span>
                  <strong className="card-name">{s.name}</strong>
                  {hasNew(s, seen) && <span className="unread" aria-label="new activity" />}
                  <span className={`chip status-chip status-${s.status}`}>{s.status}</span>
                </span>
                <span className="muted card-where">
                  {s.project.name}
                  {s.branch ? ` · ${s.branch}` : ''}
                </span>
                {sum?.activity && sum.activityKind !== 'reply' && (
                  <span className={`card-activity activity-${sum.activityKind ?? ''}`}>{sum.activityKind === 'tool' ? `⚙ ${sum.activity}` : plainText(sum.activity)}</span>
                )}
                {sum?.lastReply && <span className="card-reply">{plainText(sum.lastReply)}</span>}
                {sum?.lastPrompt && <span className="muted card-prompt">You: {sum.lastPrompt}</span>}
                <span className="card-foot muted">
                  {sum?.contextTokens ? (
                    <span>
                      Context {formatTokens(sum.contextTokens)}
                      {limit ? ` · ${String(Math.round((sum.contextTokens / limit) * 100))}%` : ''}
                    </span>
                  ) : (
                    <span />
                  )}
                  <span>{ago(s.lastActivity ?? sum?.updated, now)}</span>
                </span>
              </button>
            )
          })}
          {cards.length === 0 && <p className="muted">No running sessions. Start one with ＋ in the sidebar.</p>}
        </div>
      </div>
    </main>
  )
}
