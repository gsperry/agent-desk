import type { SessionView } from '../types'

// Seen records, per conversation, how far the reader has looked: the
// session's lastActivity at the time and the last item seq on screen.
export type Seen = Record<string, { at: string; seq: number }>

const key = 'agent-desk:seen'
const maxEntries = 500

type KV = Pick<Storage, 'getItem' | 'setItem'>

export function loadSeen(store: KV): Seen {
  try {
    const raw = store.getItem(key)
    const v: unknown = raw ? JSON.parse(raw) : {}
    return typeof v === 'object' && v !== null ? (v as Seen) : {}
  } catch {
    return {}
  }
}

export function saveSeen(store: KV, seen: Seen): void {
  const entries = Object.entries(seen)
  const kept = entries.length > maxEntries ? entries.sort((a, b) => b[1].at.localeCompare(a[1].at)).slice(0, maxEntries) : entries
  store.setItem(key, JSON.stringify(Object.fromEntries(kept)))
}

// baseline marks conversations the reader has never opened as seen up to
// now, so the first run does not flag every session.
export function baseline(seen: Seen, sessions: SessionView[]): Seen {
  let next: Seen | null = null
  for (const s of sessions) {
    if (!s.agentSessionId || !s.lastActivity || seen[s.agentSessionId]) continue
    next ??= { ...seen }
    next[s.agentSessionId] = { at: s.lastActivity, seq: Number.MAX_SAFE_INTEGER }
  }
  return next ?? seen
}

export function hasNew(s: SessionView, seen: Seen): boolean {
  if (!s.agentSessionId || !s.lastActivity) return false
  const mark = seen[s.agentSessionId]
  return mark !== undefined && s.lastActivity > mark.at
}

// markSeen moves a conversation's mark forward, never back.
export function markSeen(seen: Seen, conv: string, at: string, seq: number): Seen {
  const cur = seen[conv]
  const nextAt = cur && cur.at > at ? cur.at : at
  const nextSeq = cur && cur.seq !== Number.MAX_SAFE_INTEGER && cur.seq > seq ? cur.seq : seq
  if (cur?.at === nextAt && cur.seq === nextSeq) return seen
  return { ...seen, [conv]: { at: nextAt, seq: nextSeq } }
}

// dividerSeq is the seq the "new since you last looked" divider sits
// before, or null when there is nothing new or no position to mark.
export function dividerSeq(markAtOpen: { seq: number } | undefined, itemSeqs: number[]): number | null {
  if (!markAtOpen || markAtOpen.seq === Number.MAX_SAFE_INTEGER) return null
  const first = itemSeqs.find((s) => s > markAtOpen.seq)
  return first ?? null
}
