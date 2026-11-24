// Dictation puts the words heard so far where the caret was when it began,
// replacing them as the recognizer revises its guess.

export interface Anchor {
  before: string
  after: string
}

// anchorAt splits text at the selection; a selection is replaced.
export function anchorAt(text: string, start: number, end: number): Anchor {
  const s = Math.max(0, Math.min(start, text.length))
  const e = Math.max(s, Math.min(end, text.length))
  return { before: text.slice(0, s), after: text.slice(e) }
}

// withHeard returns the text with heard in place, spaced from its
// neighbours, and the caret just after it.
export function withHeard(a: Anchor, heard: string): { text: string; caret: number } {
  const words = heard.trim()
  if (!words) return { text: a.before + a.after, caret: a.before.length }
  const lead = a.before && !/\s$/.test(a.before) ? ' ' : ''
  const trail = a.after && !/^\s/.test(a.after) ? ' ' : ''
  const head = a.before + lead + words
  return { text: head + trail + a.after, caret: head.length }
}

export type DictationEvent =
  | { type: 'ready' }
  | { type: 'text'; text: string; final: boolean }
  | { type: 'error'; message: string }
  | { type: 'done' }

// parseEvent checks a message from the desktop app's dictation helper.
export function parseEvent(raw: unknown): DictationEvent | null {
  if (typeof raw !== 'object' || raw === null) return null
  const r = raw as Record<string, unknown>
  switch (r.type) {
    case 'ready':
    case 'done':
      return { type: r.type }
    case 'text':
      return typeof r.text === 'string' ? { type: 'text', text: r.text, final: r.final === true } : null
    case 'error':
      return { type: 'error', message: typeof r.message === 'string' && r.message ? r.message : 'Dictation stopped.' }
  }
  return null
}
