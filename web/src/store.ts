import { useSyncExternalStore } from 'react'
import { apiFetch, connect, type Connection, type ConnectionState } from './api'
import { applyMessage, emptyData, prependOlder, setLoadingOlder, type AppData } from './model/docs'
import type { Page, ServerMessage } from './types'

export interface StoreState {
  conn: ConnectionState
  data: AppData
}

type Listener = () => void

export class Store {
  private state: StoreState = { conn: { kind: 'connecting' }, data: emptyData }
  private listeners = new Set<Listener>()
  private connection: Connection | null = null
  private followed = new Set<string>()
  private readonly token: string

  constructor(token: string) {
    this.token = token
  }

  start() {
    this.connection = connect(
      this.token,
      (conn) => {
        this.set({ ...this.state, conn })
      },
      (msg) => {
        this.onMessage(msg)
      },
    )
    return () => this.connection?.close()
  }

  private onMessage(msg: ServerMessage) {
    if (msg.type === 'hello') {
      for (const id of this.followed) this.connection?.send({ type: 'subscribe', sessionId: id })
    }
    const data = applyMessage(this.state.data, msg)
    if (data !== this.state.data) this.set({ ...this.state, data })
  }

  follow(sessionId: string) {
    if (this.followed.has(sessionId)) return
    this.followed.add(sessionId)
    this.connection?.send({ type: 'subscribe', sessionId })
  }

  unfollow(sessionId: string) {
    if (!this.followed.delete(sessionId)) return
    this.connection?.send({ type: 'unsubscribe', sessionId })
  }

  async loadOlder(sessionId: string) {
    const doc = this.state.data.docs[sessionId]
    const first = doc?.items[0]
    if (!doc || !first || !doc.hasMore || doc.loadingOlder) return
    this.set({ ...this.state, data: setLoadingOlder(this.state.data, sessionId, true) })
    try {
      const res = await apiFetch(this.token, `/api/sessions/${encodeURIComponent(sessionId)}/transcript?before=${String(first.seq)}&limit=300`)
      if (!res.ok) throw new Error(String(res.status))
      const page = (await res.json()) as Page
      this.set({ ...this.state, data: prependOlder(this.state.data, page) })
    } catch {
      this.set({ ...this.state, data: setLoadingOlder(this.state.data, sessionId, false) })
    }
  }

  async startManager(): Promise<string | null> {
    const res = await apiFetch(this.token, '/api/manager/start', { method: 'POST' })
    if (res.ok) return null
    const body = (await res.json().catch(() => ({}))) as { error?: string }
    return body.error ?? `HTTP ${String(res.status)}`
  }

  getState = () => this.state

  subscribe = (l: Listener) => {
    this.listeners.add(l)
    return () => this.listeners.delete(l)
  }

  private set(next: StoreState) {
    this.state = next
    for (const l of this.listeners) l()
  }
}

export function useStore(store: Store): StoreState {
  return useSyncExternalStore(store.subscribe, store.getState)
}
