import { useCallback, useEffect, useMemo, useState } from 'react'
import { conversationTitle, filterConversations, humanSize, lastPart } from '../model/history'
import { ago } from '../model/overview'
import type { Conversation, Item, Page, SessionView } from '../types'
import { togglePin } from '../model/pins'
import { PinsContext, pinsFor, useNotebook } from '../pins-context'
import { NotesPanel } from './NotesPanel'
import { Transcript } from './Transcript'

interface ListProps {
  load: () => Promise<{ data?: Conversation[]; error?: string }>
  owners: Map<string, SessionView>
  onOpen: (id: string) => void
}

// HistoryList lists every conversation on disk, newest first.
export function HistoryList({ load, owners, onOpen }: ListProps) {
  const [list, setList] = useState<Conversation[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [now] = useState(() => Date.now())

  useEffect(() => {
    let live = true
    void load().then((r) => {
      if (!live) return
      if (r.data) setList(r.data)
      else setError(r.error ?? 'Could not list conversations.')
    })
    return () => {
      live = false
    }
  }, [load])

  const shown = useMemo(() => filterConversations(list ?? [], query), [list, query])

  return (
    <main className="main history-view" aria-label="All conversations">
      <header className="session-head">
        <div className="session-title">
          <h1>All conversations</h1>
          <span className="muted">{list ? `${String(shown.length)} of ${String(list.length)}` : ''}</span>
        </div>
        <div className="session-sub">
          <input
            className="history-filter"
            type="search"
            placeholder="Filter by title, prompt, directory or branch"
            value={query}
            autoFocus
            onChange={(e) => {
              setQuery(e.target.value)
            }}
          />
        </div>
      </header>
      <div className="transcript">
        <div className="reading">
          {error && <p className="banner error">{error}</p>}
          {!list && !error && <p className="muted">Reading conversations…</p>}
          {list && shown.length === 0 && <p className="muted">No conversations match.</p>}
          <ul className="conversations">
            {shown.map((c) => {
              const owner = owners.get(c.id)
              return (
                <li key={c.id}>
                  <button
                    type="button"
                    className="conversation-row"
                    onClick={() => {
                      onOpen(c.id)
                    }}
                  >
                    <span className="conversation-line">
                      <strong className="conversation-title">{conversationTitle(c)}</strong>
                      {owner && <span className="chip">{owner.archived ? 'archived session' : `session ${owner.name}`}</span>}
                      <span className="muted conversation-time" title={c.modified}>
                        {ago(c.modified, now)}
                      </span>
                    </span>
                    <span className="muted conversation-where" title={c.directory}>
                      {lastPart(c.directory) || 'unknown directory'}
                      {c.branch ? ` · ${c.branch}` : ''} · {humanSize(c.size)}
                    </span>
                    {c.title && c.firstPrompt && <span className="conversation-prompt">{c.firstPrompt}</span>}
                  </button>
                </li>
              )
            })}
          </ul>
        </div>
      </div>
    </main>
  )
}

interface ViewProps {
  id: string
  conversation: Conversation | undefined
  owner: SessionView | undefined
  load: (id: string, before?: number) => Promise<{ data?: Page; error?: string }>
  onBack: () => void
  onOpenSession: (id: string) => void
  onResume: (c: Conversation) => void
  onExport?: (id: string) => void
}

// ConversationView reads one conversation from disk, read-only.
export function ConversationView({ id, conversation, owner, load, onBack, onOpenSession, onResume, onExport }: ViewProps) {
  const [items, setItems] = useState<Item[]>([])
  const [hasMore, setHasMore] = useState(false)
  const [loading, setLoading] = useState(true)
  const [loadingOlder, setLoadingOlder] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notesOpen, setNotesOpen] = useState(false)
  const [notebook, setNotebook] = useNotebook(id)
  const pins = pinsFor(notebook, setNotebook, togglePin)

  useEffect(() => {
    let live = true
    void load(id).then((r) => {
      if (!live) return
      setLoading(false)
      if (r.data) {
        setItems(r.data.items)
        setHasMore(r.data.hasMore)
      } else setError(r.error ?? 'Could not read this conversation.')
    })
    return () => {
      live = false
    }
  }, [load, id])

  const first = items[0]?.seq
  const older = useCallback(() => {
    if (first === undefined || loadingOlder) return
    setLoadingOlder(true)
    void load(id, first).then((r) => {
      setLoadingOlder(false)
      if (r.data) {
        const page = r.data
        setItems((cur) => [...page.items, ...cur])
        setHasMore(page.hasMore)
      }
    })
  }, [load, id, first, loadingOlder])

  const title = conversation ? conversationTitle(conversation) : id
  return (
    <main className="main conversation-view" aria-label={`Conversation ${title}`}>
      <header className="session-head">
        <div className="session-title">
          <button type="button" className="link-button" onClick={onBack}>
            ← All conversations
          </button>
          <h1 className="conversation-heading">{title}</h1>
          <span className="chip">read-only</span>
        </div>
        <div className="session-sub">
          {conversation && (
            <span className="muted" title={conversation.directory}>
              {conversation.directory ?? 'unknown directory'}
              {conversation.branch ? ` · ${conversation.branch}` : ''}
            </span>
          )}
          {owner ? (
            <button
              type="button"
              className="chip chip-button"
              onClick={() => {
                onOpenSession(owner.id)
              }}
            >
              Open session {owner.name}
            </button>
          ) : (
            conversation && (
              <button
                type="button"
                className="chip chip-button resume-button"
                onClick={() => {
                  onResume(conversation)
                }}
              >
                Resume in agent-manager
              </button>
            )
          )}
          <button
            type="button"
            className="chip chip-button notes-toggle"
            aria-pressed={notesOpen}
            onClick={() => {
              setNotesOpen(!notesOpen)
            }}
          >
            Notes{notebook.pins.length > 0 ? ` · ${String(notebook.pins.length)}` : ''}
          </button>
          {onExport && (
            <button
              type="button"
              className="chip chip-button export-button"
              disabled={items.length === 0}
              onClick={() => {
                onExport(id)
              }}
            >
              Export
            </button>
          )}
        </div>
      </header>
      {error ? (
        <div className="empty">{error}</div>
      ) : loading ? (
        <div className="empty">Reading conversation…</div>
      ) : items.length === 0 ? (
        <div className="empty">This conversation has no messages.</div>
      ) : (
        <PinsContext.Provider value={pins}>
          <Transcript sessionId={`history:${id}`} items={items} hasMore={hasMore} loadingOlder={loadingOlder} live={false} onLoadOlder={older} filterable />
        </PinsContext.Provider>
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
  )
}
