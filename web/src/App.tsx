import { useCallback, useEffect, useMemo, useState } from 'react'
import { takeToken } from './auth'
import { Sidebar } from './components/Sidebar'
import { SessionView } from './components/SessionView'
import { Store, useStore } from './store'

const token = takeToken()

export default function App() {
  if (!token) return <NoToken />
  return <Desk token={token} />
}

const noPending: never[] = []

function sessionFromPath(): string | null {
  const m = /^\/s\/([^/]+)/.exec(window.location.pathname)
  return m?.[1] ? decodeURIComponent(m[1]) : null
}

function Desk({ token }: { token: string }) {
  const store = useMemo(() => new Store(token), [token])
  useEffect(() => store.start(), [store])
  const { conn, data, pending } = useStore(store)
  const [selected, setSelected] = useState<string | null>(sessionFromPath)

  useEffect(() => {
    const onPop = () => {
      setSelected(sessionFromPath())
    }
    window.addEventListener('popstate', onPop)
    return () => {
      window.removeEventListener('popstate', onPop)
    }
  }, [])

  const select = useCallback((id: string) => {
    history.pushState(null, '', `/s/${encodeURIComponent(id)}`)
    setSelected(id)
  }, [])
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

  if (conn.kind === 'unauthorized') return <NoToken />

  const session = data.sessions?.sessions.find((s) => s.id === selected)
  return (
    <div className="desk">
      <Sidebar
        state={data.sessions}
        conn={conn}
        selected={selected}
        onSelect={select}
        onStartManager={() => store.startManager()}
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
          />
        ) : (
          <main className="main">
            <div className="empty">Loading sessions…</div>
          </main>
        )
      ) : (
        <main className="main">
          <div className="empty">Pick a session on the left.</div>
        </main>
      )}
    </div>
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
