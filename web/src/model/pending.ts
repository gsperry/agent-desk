import type { Item } from '../types'

export type PendingState = 'sending' | 'sent' | 'maybe' | 'unconfirmed' | 'failed'

// Pending is a reply shown before the transcript records it.
export interface Pending {
  clientId: string
  sessionId: string
  text: string
  sentAt: number
  state: PendingState
  // message explains a failure or a refusal.
  message?: string
}

// After this long without a matching transcript record, a sent reply is
// shown as unconfirmed rather than silently pending forever.
export const confirmWithinMs = 30_000

// Claude Code and the client clocks can disagree a little.
const clockSlackMs = 10_000

// normalize mirrors the backend's cleaning so a reply and its transcript
// record compare equal.
export function normalize(text: string): string {
  return (
    text
      .replace(/\r\n?/g, '\n')
      // eslint-disable-next-line no-control-regex
      .replace(/[\u0000-\u0008\u000b-\u001f\u007f-\u009f]/g, '')
      .trim()
  )
}

function humanPrompts(items: Item[]): Item[] {
  return items.filter((it) => it.kind === 'user')
}

// reconcile removes pending replies that now appear in the transcript as a
// human prompt (typed or queued). Each record confirms at most one reply,
// oldest reply first, so sending the same text twice needs two records.
export function reconcile(pending: Pending[], items: Item[]): Pending[] {
  if (pending.length === 0) return pending
  const used = new Set<number>()
  const prompts = humanPrompts(items)
  const remaining = pending.filter((p) => {
    if (p.state === 'failed' || p.state === 'sending') return true
    const want = normalize(p.text)
    const match = prompts.find((it) => {
      if (used.has(it.seq)) return false
      if (normalize(it.text ?? '') !== want) return false
      const t = it.time ? Date.parse(it.time) : NaN
      return Number.isNaN(t) || t >= p.sentAt - clockSlackMs
    })
    if (!match) return true
    used.add(match.seq)
    return false
  })
  return remaining.length === pending.length ? pending : remaining
}

// expire marks old unconfirmed sends.
export function expire(pending: Pending[], now: number): Pending[] {
  const next = pending.map((p) =>
    (p.state === 'sent' || p.state === 'maybe') && now - p.sentAt > confirmWithinMs ? { ...p, state: 'unconfirmed' as const } : p,
  )
  return next.some((p, i) => p !== pending[i]) ? next : pending
}
