import { useEffect, useMemo, useRef, useState } from 'react'
import { blockSeqs, splitMatches } from '../model/highlight'
import type { PathRef } from '../model/paths'
import { buildBlocks } from '../model/transcript'
import { BlockView } from '../render/Blocks'
import { OpenPathContext } from '../session-context'
import type { Page, SearchHit, SearchResult } from '../types'

interface Props {
  query: string
  archived: boolean
  onArchived: (v: boolean) => void
  run: (q: string, archived: boolean) => Promise<{ result?: SearchResult; error?: string }>
  around: (sessionId: string, uuid: string) => Promise<Page | null>
  openPath: (sessionId: string, ref: PathRef) => void
  onOpenSession: (id: string) => void
  onClose: () => void
}

const kindLabels: Record<string, string> = {
  user: 'You',
  text: 'Reply',
  thinking: 'Thinking',
  tool_result: 'Tool output',
  notice: 'Notice',
  peer: 'Peer message',
  compact_summary: 'Summary',
  system: 'System',
}

function label(hit: SearchHit): string {
  return hit.kind === 'tool_use' ? (hit.tool ?? 'Tool') : (kindLabels[hit.kind] ?? hit.kind)
}

function when(time?: string): string {
  if (!time) return ''
  const d = new Date(time)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

export function SearchView(props: Props) {
  const { query, archived, onArchived, run, onClose } = props
  const [state, setState] = useState<{ query: string; archived: boolean; result?: SearchResult; error?: string } | null>(null)
  const [openHit, setOpenHit] = useState<string | null>(null)

  useEffect(() => {
    let live = true
    void run(query, archived).then((r) => {
      if (live) setState({ query, archived, ...r })
    })
    return () => {
      live = false
    }
  }, [query, archived, run])

  const loading = state?.query !== query || state.archived !== archived
  const result = loading ? undefined : state.result

  return (
    <main className="main search-view" aria-label="Search results">
      <header className="session-head search-head">
        <div className="session-title">
          <h1>
            Results for “{query}”
          </h1>
          <button type="button" className="link-button" onClick={onClose}>
            Close
          </button>
        </div>
        <div className="session-sub">
          <span className="muted">
            {loading
              ? 'Searching…'
              : result
                ? `${String(result.hits.length)}${result.truncated ? '+' : ''} matches · searched ${String(result.searched)} sessions`
                : ''}
          </span>
          <label className="muted">
            <input
              type="checkbox"
              checked={archived}
              onChange={(e) => {
                onArchived(e.target.checked)
              }}
            />{' '}
            Include archived
          </label>
        </div>
      </header>
      <div className="transcript">
        <div className="reading">
          {!loading && state.error && <p className="banner error">{state.error}</p>}
          {result?.hits.length === 0 && <p className="muted">No matches.</p>}
          <ul className="hits">
            {result?.hits.map((hit) => {
              const key = `${hit.sessionId}/${hit.uuid}/${hit.kind}`
              const open = openHit === key
              return (
                <li key={key} className={open ? 'hit open' : 'hit'}>
                  <button
                    type="button"
                    className="hit-row"
                    aria-expanded={open}
                    onClick={() => {
                      setOpenHit(open ? null : key)
                    }}
                  >
                    <span className="hit-meta">
                      <strong>{hit.sessionName}</strong>
                      <span className="muted">{hit.project}</span>
                      <span className="chip">{label(hit)}</span>
                      {hit.archived && <span className="chip">archived</span>}
                      <span className="muted hit-time">{when(hit.time)}</span>
                    </span>
                    <span className="hit-snippet">
                      {splitMatches(hit.snippet, query).map((p, i) => (p.match ? <mark key={i}>{p.text}</mark> : <span key={i}>{p.text}</span>))}
                    </span>
                  </button>
                  {open && <Excerpt hit={hit} {...props} />}
                </li>
              )
            })}
          </ul>
        </div>
      </div>
    </main>
  )
}

function Excerpt({ hit, around, openPath, onOpenSession }: Props & { hit: SearchHit }) {
  const [page, setPage] = useState<Page | null | undefined>(undefined)
  const box = useRef<HTMLDivElement>(null)
  const target = useRef<HTMLDivElement>(null)

  useEffect(() => {
    let live = true
    void around(hit.sessionId, hit.uuid).then((p) => {
      if (live) setPage(p)
    })
    return () => {
      live = false
    }
  }, [around, hit.sessionId, hit.uuid])

  const items = useMemo(() => page?.items ?? [], [page])
  const blocks = useMemo(() => buildBlocks(items), [items])
  const targetSeq = items.find((it) => it.uuid === hit.uuid)?.seq

  // Scroll the excerpt, not the page, so the result list stays in place.
  useEffect(() => {
    const b = box.current
    const t = target.current
    if (b && t) b.scrollTop = t.offsetTop - b.offsetTop - 24
  }, [blocks])

  const openHere = useMemo(
    () => (ref: PathRef) => {
      openPath(hit.sessionId, ref)
    },
    [openPath, hit.sessionId],
  )

  return (
    <div className="excerpt">
      <div className="excerpt-scroll" ref={box}>
        {page === undefined && <p className="muted">Loading context…</p>}
        {page === null && <p className="muted">Could not load this part of the transcript.</p>}
        {page && items.length === 0 && <p className="muted">This message is no longer in the transcript.</p>}
        <OpenPathContext.Provider value={openHere}>
          {blocks.map((b) => {
            const isTarget = targetSeq !== undefined && blockSeqs(b).includes(targetSeq)
            return (
              <div key={b.key} ref={isTarget ? target : undefined} className={isTarget ? 'hit-target' : undefined}>
                <BlockView block={b} live={false} />
              </div>
            )
          })}
        </OpenPathContext.Provider>
      </div>
      <button
        type="button"
        className="send excerpt-open"
        onClick={() => {
          onOpenSession(hit.sessionId)
        }}
      >
        Open session
      </button>
    </div>
  )
}
