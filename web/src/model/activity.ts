import type { Item } from '../types'
import { toolLabel } from './transcript'

// Activity is what a working agent is doing now, for the live row under
// the transcript: when the turn began, the tokens it has written, and its
// latest step.
export interface Activity {
  /** Start of the turn, ms since the epoch; undefined without timestamps. */
  startedAt?: number
  outputTokens: number
  /** The latest step, or empty between steps. */
  step: string
}

function outputTokens(it: Item): number | undefined {
  const u = it.meta?.usage
  if (typeof u !== 'object' || u === null) return undefined
  const n = (u as Record<string, unknown>).outputTokens
  return typeof n === 'number' ? n : undefined
}

// liveActivity reads the current turn: everything after the last prompt
// you sent. Usage repeats on each part of one assistant message, so tokens
// count once per message.
export function liveActivity(items: Item[]): Activity {
  let start = items.length
  for (let i = items.length - 1; i >= 0; i--) {
    if (items[i]?.kind === 'user') {
      start = i
      break
    }
  }
  if (start === items.length) start = 0
  const turn = items.slice(start)
  const t = turn[0]?.time ? Date.parse(turn[0].time) : NaN

  const perMessage = new Map<string, number>()
  const results = new Set<string>()
  for (const it of turn) {
    const n = outputTokens(it)
    if (n !== undefined) {
      const id = it.messageId ?? `seq${String(it.seq)}`
      perMessage.set(id, Math.max(n, perMessage.get(id) ?? 0))
    }
    if (it.kind === 'tool_result' && it.result) results.add(it.result.toolUseId)
  }
  let tokens = 0
  for (const n of perMessage.values()) tokens += n

  let step = ''
  for (let i = turn.length - 1; i >= 0; i--) {
    const it = turn[i]
    if (!it) continue
    if (it.kind === 'tool_use') {
      if (it.tool?.id && results.has(it.tool.id)) break
      const l = toolLabel(it)
      step = l.summary ? `${l.name}: ${l.summary}` : l.name
      break
    }
    if (it.kind === 'thinking') {
      step = 'Thinking'
      break
    }
    if (it.kind === 'text') {
      step = 'Writing'
      break
    }
    if (it.kind === 'tool_result') break
  }
  return { startedAt: Number.isNaN(t) ? undefined : t, outputTokens: tokens, step }
}

// formatTokens writes a token count the way Claude Code does: 950, 12.3k.
export function formatTokens(n: number): string {
  if (n < 1000) return String(n)
  return `${(n / 1000).toFixed(1)}k`
}
