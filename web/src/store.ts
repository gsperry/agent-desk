import { useSyncExternalStore } from 'react'
import { apiFetch, connect, type Connection, type ConnectionState } from './api'
import { applyMessage, emptyData, prependOlder, setLoadingOlder, type AppData } from './model/docs'
import { expire, reconcile, type Pending } from './model/pending'
import type { Page, ServerMessage } from './types'

export interface StoreState {
  conn: ConnectionState
  data: AppData
  // pending holds replies per session until the transcript records them.
  pending: Record<string, Pending[]>
}

export interface SendRefusal {
  reason?: string
  message?: string
  error?: string
}

type Listener = () => void

export class Store {
  private state: StoreState = { conn: { kind: 'connecting' }, data: emptyData, pending: {} }
  private nextId = 0
  private listeners = new Set<Listener>()
  private connection: Connection | null = null
  private followed = new Set<string>()
  private readonly token: string

  constructor(token: string) {
    this.token = token
  }

  start() {
    const timer = setInterval(() => {
      this.expirePending()
    }, 5000)
    this.connection = connect(
      this.token,
      (conn) => {
        this.set({ ...this.state, conn })
      },
      (msg) => {
        this.onMessage(msg)
      },
    )
    return () => {
      clearInterval(timer)
      this.connection?.close()
    }
  }

  private onMessage(msg: ServerMessage) {
    if (msg.type === 'hello') {
      for (const id of this.followed) this.connection?.send({ type: 'subscribe', sessionId: id })
    }
    const data = applyMessage(this.state.data, msg)
    if (data !== this.state.data) this.set({ ...this.state, data, pending: this.reconciled(data) })
  }

  private reconciled(data: AppData): Record<string, Pending[]> {
    let changed = false
    const next: Record<string, Pending[]> = {}
    for (const [id, list] of Object.entries(this.state.pending)) {
      const items = data.docs[id]?.items ?? []
      const r = reconcile(list, items)
      if (r !== list) changed = true
      if (r.length) next[id] = r
      else changed = true
    }
    return changed ? next : this.state.pending
  }

  private expirePending() {
    let changed = false
    const next: Record<string, Pending[]> = {}
    for (const [id, list] of Object.entries(this.state.pending)) {
      const e = expire(list, Date.now())
      if (e !== list) changed = true
      next[id] = e
    }
    if (changed) this.set({ ...this.state, pending: next })
  }

  private updatePending(sessionId: string, clientId: string, f: (p: Pending) => Pending | null) {
    const list = this.state.pending[sessionId] ?? []
    const next = list.flatMap((p) => {
      if (p.clientId !== clientId) return [p]
      const r = f(p)
      return r ? [r] : []
    })
    const others = Object.entries(this.state.pending).filter(([id]) => id !== sessionId)
    const pending = Object.fromEntries(next.length ? [...others, [sessionId, next]] : others)
    this.set({ ...this.state, pending })
  }

  // send shows the reply at once, then reports how delivery went. The
  // transcript confirms it later through reconcile.
  async send(sessionId: string, text: string): Promise<void> {
    const clientId = `c${String(++this.nextId)}`
    const entry: Pending = { clientId, sessionId, text, sentAt: Date.now(), state: 'sending' }
    this.set({ ...this.state, pending: { ...this.state.pending, [sessionId]: [...(this.state.pending[sessionId] ?? []), entry] } })
    let state: Pending['state'] = 'failed'
    let message: string | undefined
    try {
      const res = await apiFetch(this.token, `/api/sessions/${encodeURIComponent(sessionId)}/send`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text }),
      })
      const body = (await res.json().catch(() => ({}))) as { status?: string } & SendRefusal
      if (res.ok) state = body.status === 'maybe' ? 'maybe' : 'sent'
      else message = body.message ?? body.error ?? `HTTP ${String(res.status)}`
    } catch (e) {
      message = e instanceof Error ? e.message : String(e)
    }
    this.updatePending(sessionId, clientId, (p) => ({ ...p, state, message, sentAt: Date.now() }))
    // The transcript may already hold the prompt.
    this.set({ ...this.state, pending: this.reconciled(this.state.data) })
  }

  dismiss(sessionId: string, clientId: string) {
    this.updatePending(sessionId, clientId, () => null)
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
