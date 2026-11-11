import { useCallback, useEffect, useMemo, useState } from 'react'
import { takeToken } from './auth'
import { SessionView } from './components/SessionView'
import { Sidebar, type ViewOptions } from './components/Sidebar'
import { ConfirmDialog } from './components/Dialog'
import { NewGroup } from './components/NewGroup'
import { NewSession } from './components/NewSession'
import { Overview } from './components/Overview'
import { SearchView } from './components/SearchView'
import { Settings } from './components/Settings'
import { Switcher } from './components/Switcher'
import { attentionCount, nextAttention, ordered } from './model/attention'
import { groupSessions } from './model/sidebar'
import type { PathRef } from './model/paths'
import { directorySuggestions, spawnDefaults } from './model/spawn'
import { loadViewPrefs, saveViewPrefs, widths, type ViewPrefs } from './model/viewprefs'
import { Store, useStore } from './store'
import type { Answer, Project, SessionAction, SpawnRequest } from './types'
import { ViewPrefsContext } from './viewprefs-context'

const token = takeToken()

export default function App() {
  if (!token) return <NoToken />
  return <Desk token={token} />
}

function sessionFromPath(): string | null {
  const m = /^\/s\/([^/]+)/.exec(window.location.pathname)
  return m?.[1] ? decodeURIComponent(m[1]) : null
}

const noPending: never[] = []

function isTyping(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)
}

function Desk({ token }: { token: string }) {
  const store = useMemo(() => new Store(token), [token])
  useEffect(() => store.start(), [store])
  const { conn, data, pending, acks, prefs, permission, toast, seen } = useStore(store)
  const [selected, setSelected] = useState<string | null>(sessionFromPath)
  const [view, setView] = useState<ViewOptions>({ showArchived: false, onlyAttention: false })
  const [switcher, setSwitcher] = useState(false)
  const [visible, setVisible] = useState(document.visibilityState === 'visible')
  const [viewPrefs, setViewPrefs] = useState<ViewPrefs>(() => loadViewPrefs(localStorage))
  const [settingsOpen, setSettingsOpen] = useState(false)
  // searchText is the sidebar box; searchQuery is the search being shown,
  // set once the text settles.
  const [searchText, setSearchText] = useState('')
  const [searchQuery, setSearchQuery] = useState<string | null>(null)
  const [searchArchived, setSearchArchived] = useState(false)
  type Modal = { kind: 'spawn'; initial: SpawnRequest } | { kind: 'group' } | { kind: 'deleteGroup'; path: string; busy: boolean; error: string | null }
  const [modal, setModal] = useState<Modal | null>(null)

  useEffect(() => {
    const q = searchText.trim()
    const t = setTimeout(() => {
      setSearchQuery(q.length >= 2 ? q : null)
    }, 300)
    return () => {
      clearTimeout(t)
    }
  }, [searchText])

  useEffect(() => {
    saveViewPrefs(localStorage, viewPrefs)
    const root = document.documentElement
    if (viewPrefs.theme === 'system') delete root.dataset.theme
    else root.dataset.theme = viewPrefs.theme
    root.style.setProperty('--font-size', `${String(viewPrefs.fontSize)}px`)
    root.style.setProperty('--reading-width', widths[viewPrefs.width])
  }, [viewPrefs])

  const sessions = useMemo(() => data.sessions?.sessions ?? [], [data.sessions])
  const amGroups = useMemo(() => data.sessions?.groups ?? [], [data.sessions])
  const groups = useMemo(() => groupSessions(sessions, { ...view, acks, groups: amGroups }), [sessions, view, acks, amGroups])
  const directories = useMemo(() => directorySuggestions(sessions, amGroups), [sessions, amGroups])
  // j/k walk every session that needs the human, even when filters hide some.
  const walkOrder = useMemo(() => ordered(groupSessions(sessions, { showArchived: false, acks })), [sessions, acks])
  const attention = attentionCount(sessions, acks)

  const select = useCallback((id: string) => {
    history.pushState(null, '', `/s/${encodeURIComponent(id)}`)
    setSelected(id)
    setSearchText('')
    setSearchQuery(null)
  }, [])

  useEffect(() => {
    const onPop = () => {
      setSelected(sessionFromPath())
    }
    const onVisibility = () => {
      setVisible(document.visibilityState === 'visible')
    }
    window.addEventListener('popstate', onPop)
    document.addEventListener('visibilitychange', onVisibility)
    return () => {
      window.removeEventListener('popstate', onPop)
      document.removeEventListener('visibilitychange', onVisibility)
    }
  }, [])

  useEffect(() => {
    store.setOnOpen(select)
    store.setViewing(selected, visible)
  }, [store, select, selected, visible])

  // The open session counts as read while the page is visible.
  const selectedSession = sessions.find((s) => s.id === selected)
  const lastSeq = selected ? data.docs[selected]?.items.at(-1)?.seq : undefined
  useEffect(() => {
    const conv = selectedSession?.agentSessionId
    const at = selectedSession?.lastActivity
    if (!visible || !conv || !at || lastSeq === undefined) return
    store.markViewed(conv, at, lastSeq)
  }, [store, visible, selectedSession?.agentSessionId, selectedSession?.lastActivity, lastSeq])

  useEffect(() => {
    document.title = attention > 0 ? `(${String(attention)}) Agent Desk` : 'Agent Desk'
  }, [attention])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setSwitcher((open) => !open)
        return
      }
      if (switcher || isTyping(e.target) || e.metaKey || e.ctrlKey || e.altKey) return
      if (e.key === '/') {
        e.preventDefault()
        document.getElementById('search-box')?.focus()
        return
      }
      if (e.key === 'j' || e.key === 'k') {
        const id = nextAttention(walkOrder, acks, selected, e.key === 'j' ? 1 : -1)
        if (id) {
          e.preventDefault()
          select(id)
        }
      }
    }
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('keydown', onKey)
    }
  }, [switcher, walkOrder, acks, selected, select])

  const follow = useCallback((id: string) => {
    store.follow(id)
  }, [store])
  const unfollow = useCallback((id: string) => {
    store.unfollow(id)
  }, [store])
  const loadOlder = useCallback((id: string) => {
    void store.loadOlder(id)
  }, [store])
  const send = useCallback((id: string, text: string) => {
    void store.send(id, text)
  }, [store])
  const dismiss = useCallback((id: string, clientId: string) => {
    store.dismiss(id, clientId)
  }, [store])
  const readScreen = useCallback((id: string) => store.screen(id), [store])
  const answer = useCallback((id: string, a: Answer) => store.answer(id, a), [store])
  const loadOptions = useCallback(() => store.spawnOptions(), [store])
  const readAgents = useCallback((id: string) => store.agents(id), [store])
  const loadOverview = useCallback(() => store.overview(), [store])
  const goHome = useCallback(() => {
    history.pushState(null, '', '/')
    setSelected(null)
    setSearchText('')
    setSearchQuery(null)
  }, [])
  const readAgentTranscript = useCallback((id: string, agentId: string) => store.agentTranscript(id, agentId), [store])
  const spawn = useCallback((r: SpawnRequest) => store.spawn(r), [store])
  const act = useCallback((id: string, a: SessionAction) => store.act(id, a), [store])
  const openSpawn = useCallback((project: Project | null) => {
    setModal({ kind: 'spawn', initial: spawnDefaults(project, '', false) })
  }, [])
  const runSearch = useCallback((q: string, archived: boolean) => store.search(q, archived), [store])
  const around = useCallback((id: string, uuid: string) => store.around(id, uuid), [store])
  const openPath = useCallback(
    (id: string, ref: PathRef) => {
      void store.openPath(id, ref)
    },
    [store],
  )
  useEffect(() => {
    if (!toast) return
    const t = setTimeout(() => {
      store.clearToast(toast.id)
    }, 5000)
    return () => {
      clearTimeout(t)
    }
  }, [store, toast])

  if (conn.kind === 'unauthorized') return <NoToken />

  const session = sessions.find((s) => s.id === selected)
  return (
    <ViewPrefsContext.Provider value={viewPrefs}>
      <div className="desk">
        <Sidebar
          state={data.sessions}
          conn={conn}
          groups={groups}
          acks={acks}
          seen={seen}
          attention={attention}
          view={view}
          onView={setView}
          selected={selected}
          onSelect={select}
          onStartManager={() => store.startManager()}
          prefs={prefs}
          permission={permission}
          onPrefs={(p) => {
            store.setPrefs(p)
          }}
          onRequestPermission={() => {
            void store.requestPermission()
          }}
          onOpenSettings={() => {
            setSettingsOpen(true)
          }}
          search={searchText}
          onSearch={setSearchText}
          onNewSession={openSpawn}
          onNewGroup={() => {
            setModal({ kind: 'group' })
          }}
          onDeleteGroup={(path) => {
            setModal({ kind: 'deleteGroup', path, busy: false, error: null })
          }}
          onHome={goHome}
        />
        {searchQuery ? (
          <SearchView
            query={searchQuery}
            archived={searchArchived}
            onArchived={setSearchArchived}
            run={runSearch}
            around={around}
            openPath={openPath}
            onOpenSession={select}
            onClose={() => {
              setSearchText('')
              setSearchQuery(null)
            }}
          />
        ) : selected ? (
          data.sessions ? (
            <SessionView
              key={selected}
              session={session}
              sessionId={selected}
              doc={data.docs[selected]}
              pending={pending[selected] ?? noPending}
              connected={conn.kind === 'open'}
              insideSession={data.sessions.insideSession}
              follow={follow}
              unfollow={unfollow}
              loadOlder={loadOlder}
              send={send}
              dismiss={dismiss}
              readScreen={readScreen}
              answer={answer}
              openPath={openPath}
              groups={amGroups}
              act={act}
              seenMark={session?.agentSessionId ? seen[session.agentSessionId] : undefined}
              readAgents={readAgents}
              readAgentTranscript={readAgentTranscript}
            />
          ) : (
            <main className="main">
              <div className="empty">Loading sessions…</div>
            </main>
          )
        ) : (
          <Overview sessions={sessions} acks={acks} seen={seen} load={loadOverview} onOpen={select} />
        )}
        {modal?.kind === 'spawn' && (
          <NewSession
            initial={modal.initial}
            directories={directories}
            loadOptions={loadOptions}
            spawn={spawn}
            onStarted={(id) => {
              setModal(null)
              select(id)
            }}
            onClose={() => {
              setModal(null)
            }}
          />
        )}
        {modal?.kind === 'group' && (
          <NewGroup
            directories={directories}
            create={(path, directory) => store.createGroup(path, directory)}
            onClose={() => {
              setModal(null)
            }}
          />
        )}
        {modal?.kind === 'deleteGroup' && (
          <ConfirmDialog
            title={`Delete group ${modal.path}?`}
            message="The group is removed. Sessions in it move to the root and keep running."
            confirm="Delete group"
            danger
            busy={modal.busy}
            error={modal.error}
            onConfirm={() => {
              setModal({ ...modal, busy: true })
              void store.deleteGroup(modal.path).then((error) => {
                setModal(error ? { ...modal, busy: false, error } : null)
              })
            }}
            onClose={() => {
              setModal(null)
            }}
          />
        )}
        {toast && (
          <div className="toast" role="status">
            {toast.message}
          </div>
        )}
        {settingsOpen && (
          <Settings
            prefs={viewPrefs}
            onChange={setViewPrefs}
            onClose={() => {
              setSettingsOpen(false)
            }}
          />
        )}
        {switcher && (
          <Switcher
            sessions={sessions}
            acks={acks}
            onPick={select}
            onClose={() => {
              setSwitcher(false)
            }}
          />
        )}
      </div>
    </ViewPrefsContext.Provider>
  )
}

function NoToken() {
  return (
    <main className="shell">
      <h1>Agent Desk</h1>
      <p className="status warn">
        This page needs the link agent-desk printed when it started. Agent Desk makes a new token on every start.
      </p>
    </main>
  )
}
