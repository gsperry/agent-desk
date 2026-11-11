import type { SessionView } from '../types'
import type { SidebarGroup } from './sidebar'

// Acks are finished sessions the reader has looked at since they finished.
// agent-manager clears finished to idle only in its TUI, so Agent Desk keeps
// its own record.
export type Acks = Record<string, true>

export function updateAcks(acks: Acks, sessions: SessionView[], viewing: string | null): Acks {
  const next: Acks = {}
  for (const s of sessions) {
    if (s.status !== 'finished') continue
    if (acks[s.id] || s.id === viewing) next[s.id] = true
  }
  const same = Object.keys(next).length === Object.keys(acks).length && Object.keys(next).every((id) => acks[id])
  return same ? acks : next
}

export function needsAttention(s: SessionView, acks: Acks): boolean {
  if (s.archived) return false
  return s.status === 'waiting' || s.status === 'errored' || (s.status === 'finished' && !acks[s.id])
}

export function attentionCount(sessions: SessionView[], acks: Acks): number {
  return sessions.filter((s) => needsAttention(s, acks)).length
}

export interface Transition {
  session: SessionView
  from: string
  to: string
}

// transitions lists status changes since the previous snapshot. The first
// snapshot has no previous one and yields nothing, so opening the app does
// not replay old alerts.
export function transitions(prev: Record<string, string> | null, sessions: SessionView[]): Transition[] {
  if (!prev) return []
  const out: Transition[] = []
  for (const s of sessions) {
    const from = prev[s.id]
    if (from !== undefined && from !== s.status) out.push({ session: s, from, to: s.status })
  }
  return out
}

export function statusMap(sessions: SessionView[]): Record<string, string> {
  return Object.fromEntries(sessions.map((s) => [s.id, s.status]))
}

export interface NotifyOptions {
  onFinish: boolean
}

export function shouldNotify(t: Transition, opts: NotifyOptions): boolean {
  if (t.session.archived) return false
  if (t.to === 'waiting' || t.to === 'errored') return true
  return t.to === 'finished' && opts.onFinish
}

export function ordered(groups: SidebarGroup[]): SessionView[] {
  return groups.flatMap((g) => g.sessions)
}

// nextAttention steps through sessions that need the reader, in sidebar
// order, wrapping around. With none, it returns null.
export function nextAttention(list: SessionView[], acks: Acks, current: string | null, dir: 1 | -1): string | null {
  const targets = list.filter((s) => needsAttention(s, acks))
  if (targets.length === 0) return null
  const order = list.map((s) => s.id)
  const pos = current ? order.indexOf(current) : -1
  if (pos < 0) return (dir === 1 ? targets[0] : targets.at(-1))?.id ?? null
  const rank = (id: string) => order.indexOf(id)
  const after = targets.filter((t) => (dir === 1 ? rank(t.id) > pos : rank(t.id) < pos))
  const pick = dir === 1 ? (after[0] ?? targets[0]) : (after.at(-1) ?? targets.at(-1))
  return pick?.id ?? null
}

// matchScore ranks a session for the switcher query; null means no match.
// Characters must appear in order; contiguous and word-start hits rank
// higher. Lower is better.
export function matchScore(text: string, query: string): number | null {
  const t = text.toLowerCase()
  const q = query.toLowerCase().trim()
  if (q === '') return 0
  const direct = t.indexOf(q)
  if (direct >= 0) return direct === 0 || /[\s/_.-]/.test(t[direct - 1] ?? '') ? 0 : 1
  let score = 10
  let at = -1
  for (const ch of q) {
    const i = t.indexOf(ch, at + 1)
    if (i < 0) return null
    score += i - at - 1
    at = i
  }
  return score
}

export function filterSessions(sessions: SessionView[], query: string, acks: Acks): SessionView[] {
  const scored = sessions.flatMap((s) => {
    const score = Math.min(matchScore(s.name, query) ?? Infinity, (matchScore(s.project.name, query) ?? Infinity) + 5)
    return score === Infinity ? [] : [{ s, score }]
  })
  scored.sort(
    (a, b) =>
      a.score - b.score ||
      Number(needsAttention(b.s, acks)) - Number(needsAttention(a.s, acks)) ||
      Number(b.s.running) - Number(a.s.running) ||
      a.s.name.localeCompare(b.s.name),
  )
  return scored.map((x) => x.s)
}
