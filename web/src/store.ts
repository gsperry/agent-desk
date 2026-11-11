import { useSyncExternalStore } from 'react'
import { apiFetch, connect, type Connection, type ConnectionState } from './api'
import { shouldNotify, statusMap, transitions, updateAcks, type Acks, type Transition } from './model/attention'
import { applyMessage, emptyData, prependOlder, setLoadingOlder, type AppData } from './model/docs'
import { expire, reconcile, type Pending } from './model/pending'
import { browserNotifier, type Notifier, type Permission } from './notify'
import type { Answer, Page, Screen, SearchResult, ServerMessage } from './types'

export interface Prefs {
  notify: boolean
  notifyOnFinish: boolean
}

export interface StoreState {
  conn: ConnectionState
  data: AppData
  // pending holds replies per session until the transcript records them.
  pending: Record<string, Pending[]>
  acks: Acks
  prefs: Prefs
  permission: Permission
  // toast is a short message about an action that failed, such as opening
  // a file.
  toast: { message: string; id: number } | null
}

const acksKey = 'agent-desk:acks'
const prefsKey = 'agent-desk:prefs'

function readJSON<T>(key: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(key)
    return raw ? ({ ...fallback, ...(JSON.parse(raw) as object) }) : fallback
  } catch {
    return fallback
  }
}

const glyphs: Record<string, string> = { waiting: '◆', errored: '✕', finished: '●' }
const verbs: Record<string, string> = { waiting: 'is waiting for you', errored: 'hit an error', finished: 'finished its turn' }

export interface SendRefusal {
  reason?: string
  message?: string
  error?: string
}

type Listener = () => void

export class Store {
  private state: StoreState
  private nextId = 0
  private prevStatuses: Record<string, string> | null = null
  private viewing: string | null = null
  private visible = true
  private readonly notifier: Notifier
  private onOpen: (sessionId: string) => void = () => undefined
  private listeners = new Set<Listener>()
  private connection: Connection | null = null
  private followed = new Set<string>()
  private readonly token: string

  constructor(token: string, notifier: Notifier = browserNotifier) {
    this.token = token
    this.notifier = notifier
    this.state = {
      conn: { kind: 'connecting' },
      data: emptyData,
      pending: {},
      acks: readJSON<Acks>(acksKey, {}),
      prefs: readJSON<Prefs>(prefsKey, { notify: true, notifyOnFinish: false }),
      permission: notifier.permission(),
      toast: null,
    }
  }

  // setOnOpen sets what a notification click does.
  setOnOpen(f: (sessionId: string) => void) {
    this.onOpen = f
  }

  // setViewing records which session is on screen and whether the page is
  // visible, which decides what counts as seen and what needs a notification.
  setViewing(sessionId: string | null, visible: boolean) {
    this.viewing = sessionId
    this.visible = visible
    this.refreshAcks()
  }

  private refreshAcks() {
    const sessions = this.state.data.sessions?.sessions ?? []
    const acks = updateAcks(this.state.acks, sessions, this.visible ? this.viewing : null)
    if (acks === this.state.acks) return
    localStorage.setItem(acksKey, JSON.stringify(acks))
    this.set({ ...this.state, acks })
  }

  setPrefs(prefs: Partial<Prefs>) {
    const next = { ...this.state.prefs, ...prefs }
    localStorage.setItem(prefsKey, JSON.stringify(next))
    this.set({ ...this.state, prefs: next })
  }

  async requestPermission() {
    const permission = await this.notifier.request()
    this.set({ ...this.state, permission })
    if (permission === 'granted') this.setPrefs({ notify: true })
  }

  private notifyTransitions(list: Transition[]) {
    const { prefs } = this.state
    if (!prefs.notify || this.notifier.permission() !== 'granted') return
    for (const t of list) {
      if (!shouldNotify(t, { onFinish: prefs.notifyOnFinish })) continue
      // The reader is already looking at it.
      if (this.visible && this.viewing === t.session.id) continue
      const s = t.session
      const where = [s.project.name, s.branch].filter(Boolean).join(' · ')
      this.notifier.show(`${glyphs[t.to] ?? ''} ${s.name} ${verbs[t.to] ?? t.to}`.trim(), where, s.id, () => {
        this.onOpen(s.id)
      })
    }
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
    if (data === this.state.data) return
    this.set({ ...this.state, data, pending: this.reconciled(data) })
    if (msg.type === 'sessions') {
      this.notifyTransitions(transitions(this.prevStatuses, msg.state.sessions))
      this.prevStatuses = statusMap(msg.state.sessions)
      this.refreshAcks()
    }
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
    let reason: string | undefined
    try {
      const res = await apiFetch(this.token, `/api/sessions/${encodeURIComponent(sessionId)}/send`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text }),
      })
      const body = (await res.json().catch(() => ({}))) as { status?: string } & SendRefusal
      if (res.ok) state = body.status === 'maybe' ? 'maybe' : 'sent'
      else {
        message = body.message ?? body.error ?? `HTTP ${String(res.status)}`
        reason = body.reason
      }
    } catch (e) {
      message = e instanceof Error ? e.message : String(e)
    }
    this.updatePending(sessionId, clientId, (p) => ({ ...p, state, message, reason, sentAt: Date.now() }))
    // The transcript may already hold the prompt.
    this.set({ ...this.state, pending: this.reconciled(this.state.data) })
  }

  // screen reads a session's live pane. A refusal comes back as message.
  async screen(sessionId: string): Promise<{ screen?: Screen; message?: string }> {
    const res = await apiFetch(this.token, `/api/sessions/${encodeURIComponent(sessionId)}/pane`)
    const body = (await res.json().catch(() => ({}))) as Screen & SendRefusal
    if (res.ok) return { screen: body }
    return { message: body.message ?? body.error ?? `HTTP ${String(res.status)}` }
  }

  // answer replies to a dialog; it resolves to an error message or null.
  async answer(sessionId: string, answer: Answer): Promise<string | null> {
    const res = await apiFetch(this.token, `/api/sessions/${encodeURIComponent(sessionId)}/answer`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(answer),
    })
    if (res.ok) return null
    const body = (await res.json().catch(() => ({}))) as SendRefusal
    return body.message ?? body.error ?? `HTTP ${String(res.status)}`
  }

  async search(query: string, archived: boolean): Promise<{ result?: SearchResult; error?: string }> {
    const params = new URLSearchParams({ q: query })
    if (archived) params.set('archived', '1')
    try {
      const res = await apiFetch(this.token, `/api/search?${params.toString()}`)
      const body = (await res.json().catch(() => ({}))) as SearchResult & { error?: string }
      return res.ok ? { result: body } : { error: body.error ?? `HTTP ${String(res.status)}` }
    } catch (e) {
      return { error: e instanceof Error ? e.message : String(e) }
    }
  }

  async around(sessionId: string, uuid: string): Promise<Page | null> {
    const params = new URLSearchParams({ around: uuid })
    const res = await apiFetch(this.token, `/api/sessions/${encodeURIComponent(sessionId)}/transcript?${params.toString()}`).catch(() => null)
    if (!res?.ok) return null
    return (await res.json()) as Page
  }

  async openPath(sessionId: string, ref: { path: string; line?: number }) {
    const res = await apiFetch(this.token, '/api/open', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ sessionId, path: ref.path, line: ref.line ?? 0 }),
    }).catch(() => null)
    if (res?.ok) return
    const body = res ? ((await res.json().catch(() => ({}))) as SendRefusal) : {}
    this.showToast(body.message ?? body.error ?? 'Could not open the file.')
  }

  showToast(message: string) {
    this.set({ ...this.state, toast: { message, id: ++this.nextId } })
  }

  clearToast(id: number) {
    if (this.state.toast?.id === id) this.set({ ...this.state, toast: null })
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
