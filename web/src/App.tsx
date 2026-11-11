import { useEffect, useState } from 'react'
import { connect, type ConnectionState } from './api'
import { takeToken } from './auth'

const token = takeToken()

export default function App() {
  const [state, setState] = useState<ConnectionState>({ kind: 'connecting' })

  useEffect(() => {
    if (!token) return
    return connect(token, setState, () => undefined)
  }, [])

  return (
    <main className="shell">
      <h1>Agent Desk</h1>
      {token ? <Status state={state} /> : <NoToken />}
    </main>
  )
}

function Status({ state }: { state: ConnectionState }) {
  switch (state.kind) {
    case 'connecting':
      return <p className="status">Connecting…</p>
    case 'open':
      return <p className="status ok">Connected to agent-desk {state.version}</p>
    case 'closed':
      return <p className="status warn">Disconnected. Retrying in {Math.round(state.retryInMs / 1000)} s.</p>
    case 'unauthorized':
      return <NoToken />
  }
}

function NoToken() {
  return (
    <p className="status warn">
      This page needs the link agent-desk printed when it started. Agent Desk makes a new token on every start.
    </p>
  )
}
