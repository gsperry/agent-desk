import type { Item } from '../types'

export type PendingState = 'sending' | 'sent' | 'maybe' | 'unconfirmed' | 'failed'

// Pending is a reply shown before the transcript records it.
export interface Pending {
  clientId: string
  sessionId: string
  text: string
  sentAt: number
  state: PendingState
  // message explains a failure or a refusal; reason is the server's code
  // for a refusal, such as dialog.
  message?: string
  reason?: string
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

// recorded is the text a sent reply leaves in the transcript: a human
// prompt, a slash command's notice ("/review PR 12"), or a shell line's
// notice, which drops the leading "!".
function recorded(it: Item): string | undefined {
  if (it.kind === 'user') return it.text ?? ''
  if (it.kind !== 'notice') return undefined
  if (it.label === 'command') return it.text ?? ''
  if (it.label === 'bash-input') return '!' + (it.text ?? '')
  return undefined
}

// isCommand is a slash command. Claude Code runs some (/model, /clear)
// without writing a record, so an accepted command is not flagged as
// unconfirmed.
export function isCommand(text: string): boolean {
  return /^\/[A-Za-z]/.test(normalize(text))
}

// reconcile removes pending replies that now appear in the transcript as a
// human prompt (typed or queued), a slash command or a shell line. Each record confirms at most one reply,
// oldest reply first, so sending the same text twice needs two records.
// A reply still in flight counts too: the transcript can record the prompt
// before the send request returns.
export function reconcile(pending: Pending[], items: Item[]): Pending[] {
  if (pending.length === 0) return pending
  const used = new Set<number>()
  const prompts = items.flatMap((it) => {
    const text = recorded(it)
    return text === undefined ? [] : [{ it, text: normalize(text) }]
  })
  const remaining = pending.filter((p) => {
    if (p.state === 'failed') return true
    const want = normalize(p.text)
    const match = prompts.find(({ it, text }) => {
      if (used.has(it.seq)) return false
      if (text !== want) return false
      const t = it.time ? Date.parse(it.time) : NaN
      return Number.isNaN(t) || t >= p.sentAt - clockSlackMs
    })
    if (!match) return true
    used.add(match.it.seq)
    return false
  })
  return remaining.length === pending.length ? pending : remaining
}

// expire marks old unconfirmed sends. An accepted slash command that left
// no record is dropped instead: Claude Code ran it without writing one.
export function expire(pending: Pending[], now: number): Pending[] {
  const next = pending.flatMap((p) => {
    if ((p.state !== 'sent' && p.state !== 'maybe') || now - p.sentAt <= confirmWithinMs) return [p]
    if (p.state === 'sent' && isCommand(p.text)) return []
    return [{ ...p, state: 'unconfirmed' as const }]
  })
  return next.length === pending.length && next.every((p, i) => p === pending[i]) ? pending : next
}
