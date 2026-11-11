export interface ReviewComment {
  id: string
  file: string
  // line is the new-file line, or the old-file line for a removed line.
  line: number
  side: 'new' | 'old'
  code: string
  text: string
}

// composeReview turns line comments into one prompt the agent can work
// through, numbered, with the code each comment is about.
export function composeReview(comments: ReviewComment[]): string {
  const sorted = [...comments].sort((a, b) => a.file.localeCompare(b.file) || a.line - b.line)
  const items = sorted.map((c, i) => {
    const where = `${c.file}:${String(c.line)}${c.side === 'old' ? ' (removed line)' : ''}`
    const code = c.code.trim() ? `\n   > ${c.code.trim()}` : ''
    const body = c.text
      .trim()
      .split('\n')
      .map((l) => `   ${l}`)
      .join('\n')
    return `${String(i + 1)}. ${where}${code}\n${body}`
  })
  return `Review comments on your changes. Address each one, then reply with what you changed for each number.\n\n${items.join('\n\n')}`
}

const key = (sessionId: string) => `agent-desk:review:${sessionId}`

type KV = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>

export function loadComments(store: KV, sessionId: string): ReviewComment[] {
  try {
    const v: unknown = JSON.parse(store.getItem(key(sessionId)) ?? '[]')
    return Array.isArray(v) ? (v as ReviewComment[]) : []
  } catch {
    return []
  }
}

export function saveComments(store: KV, sessionId: string, comments: ReviewComment[]): void {
  if (comments.length === 0) store.removeItem(key(sessionId))
  else store.setItem(key(sessionId), JSON.stringify(comments))
}
