import { useCallback, useEffect, useMemo, useState } from 'react'
import { takeToken } from './auth'
import { SessionView } from './components/SessionView'
import { Sidebar, type ViewOptions } from './components/Sidebar'
import { Settings } from './components/Settings'
import { Switcher } from './components/Switcher'
import { attentionCount, nextAttention, ordered } from './model/attention'
import { groupSessions } from './model/sidebar'
import { loadViewPrefs, saveViewPrefs, widths, type ViewPrefs } from './model/viewprefs'
import { Store, useStore } from './store'
import type { Answer } from './types'
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
  const { conn, data, pending, acks, prefs, permission } = useStore(store)
  const [selected, setSelected] = useState<string | null>(sessionFromPath)
  const [view, setView] = useState<ViewOptions>({ showArchived: false, onlyAttention: false })
  const [switcher, setSwitcher] = useState(false)
  const [visible, setVisible] = useState(document.visibilityState === 'visible')
  const [viewPrefs, setViewPrefs] = useState<ViewPrefs>(() => loadViewPrefs(localStorage))
  const [settingsOpen, setSettingsOpen] = useState(false)

  useEffect(() => {
    saveViewPrefs(localStorage, viewPrefs)
    const root = document.documentElement
    if (viewPrefs.theme === 'system') delete root.dataset.theme
    else root.dataset.theme = viewPrefs.theme
    root.style.setProperty('--font-size', `${String(viewPrefs.fontSize)}px`)
    root.style.setProperty('--reading-width', widths[viewPrefs.width])
  }, [viewPrefs])

  const sessions = useMemo(() => data.sessions?.sessions ?? [], [data.sessions])
  const groups = useMemo(() => groupSessions(sessions, { ...view, acks }), [sessions, view, acks])
  // j/k walk every session that needs the human, even when filters hide some.
  const walkOrder = useMemo(() => ordered(groupSessions(sessions, { showArchived: false, acks })), [sessions, acks])
  const attention = attentionCount(sessions, acks)

  const select = useCallback((id: string) => {
    history.pushState(null, '', `/s/${encodeURIComponent(id)}`)
    setSelected(id)
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
        />
        {selected ? (
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
            />
          ) : (
            <main className="main">
              <div className="empty">Loading sessions…</div>
            </main>
          )
        ) : (
          <main className="main">
            <div className="empty">
              {attention > 0 ? `${String(attention)} session${attention === 1 ? ' needs' : 's need'} you. Press j to go to the first.` : 'Pick a session on the left, or press ⌘K.'}
            </div>
          </main>
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
