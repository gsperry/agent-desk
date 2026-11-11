import type { Conversation, SessionView } from '../types'

// conversationTitle is what a conversation is called in the list.
export function conversationTitle(c: Conversation): string {
  return c.title?.trim() || c.firstPrompt?.trim() || c.id
}

// filterConversations keeps the conversations whose title, first prompt,
// directory, branch or id contain every word of the query.
export function filterConversations(list: Conversation[], query: string): Conversation[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean)
  if (words.length === 0) return list
  return list.filter((c) => {
    const hay = [c.title, c.firstPrompt, c.directory, c.branch, c.id].join('\n').toLowerCase()
    return words.every((w) => hay.includes(w))
  })
}

// sessionsByConversation maps a conversation id to the agent-manager
// session that runs it, preferring a live session over an archived one.
export function sessionsByConversation(sessions: SessionView[]): Map<string, SessionView> {
  const out = new Map<string, SessionView>()
  for (const s of sessions) {
    if (!s.agentSessionId) continue
    const had = out.get(s.agentSessionId)
    if (!had || (had.archived && !s.archived)) out.set(s.agentSessionId, s)
  }
  return out
}

// lastPart is a directory's final component, for compact listings.
export function lastPart(dir: string | undefined): string {
  if (!dir) return ''
  const parts = dir.split('/').filter(Boolean)
  return parts.at(-1) ?? dir
}

// humanSize formats a byte count.
export function humanSize(n: number): string {
  if (n < 1024) return `${String(n)} B`
  if (n < 1024 * 1024) return `${String(Math.round(n / 1024))} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}
