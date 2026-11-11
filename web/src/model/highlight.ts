import type { Block } from './transcript'

export interface Part {
  text: string
  match: boolean
}

// splitMatches cuts text into parts, marking case-insensitive occurrences
// of query.
export function splitMatches(text: string, query: string): Part[] {
  const q = query.trim().toLowerCase()
  if (!q) return [{ text, match: false }]
  const lower = text.toLowerCase()
  const parts: Part[] = []
  let at = 0
  for (let i = lower.indexOf(q); i >= 0; i = lower.indexOf(q, at)) {
    if (i > at) parts.push({ text: text.slice(at, i), match: false })
    parts.push({ text: text.slice(i, i + q.length), match: true })
    at = i + q.length
  }
  if (at < text.length) parts.push({ text: text.slice(at), match: false })
  return parts
}

// blockSeqs lists the item sequence numbers a block renders.
export function blockSeqs(b: Block): number[] {
  switch (b.kind) {
    case 'turn':
      return b.steps.flatMap((s) => (s.kind === 'tool' ? [s.use.seq, ...(s.result ? [s.result.seq] : [])] : [s.item.seq]))
    case 'compact':
      return [b.boundary?.seq, b.summary?.seq].filter((n): n is number => n !== undefined)
    default:
      return [b.item.seq]
  }
}
