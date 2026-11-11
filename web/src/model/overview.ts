import type { SessionView } from '../types'
import { needsAttention, type Acks } from './attention'

// Card order: what needs the human, then busy agents, then the rest; most
// recently active first within each.
function band(s: SessionView, acks: Acks): number {
  if (needsAttention(s, acks)) return 0
  if (s.status === 'working' || s.status === 'starting') return 1
  if (s.running) return 2
  return 3
}

export function overviewCards(sessions: SessionView[], acks: Acks, showStopped: boolean): SessionView[] {
  return sessions
    .filter((s) => !s.archived && (showStopped || s.running))
    .sort((a, b) => band(a, acks) - band(b, acks) || (b.lastActivity ?? '').localeCompare(a.lastActivity ?? '') || a.name.localeCompare(b.name))
}

// ago is a short relative time: "now", "45s", "12m", "3h", "2d".
export function ago(iso: string | undefined, now: number): string {
  if (!iso) return ''
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return ''
  const s = Math.max(0, Math.round((now - t) / 1000))
  if (s < 10) return 'now'
  if (s < 60) return `${String(s)}s ago`
  const m = Math.floor(s / 60)
  if (m < 60) return `${String(m)}m ago`
  const h = Math.floor(m / 60)
  if (h < 48) return `${String(h)}h ago`
  return `${String(Math.floor(h / 24))}d ago`
}

// plainText turns Markdown into a one-line preview: emphasis, code marks,
// headings, list markers and link targets go; their text stays.
export function plainText(md: string): string {
  return md
    .replace(/```[\s\S]*?```/g, ' ')
    .replace(/!?\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/^\s{0,3}(#{1,6}\s+|[-*+]\s+|\d+\.\s+|>\s?)/gm, '')
    .replace(/(\*\*|__)(.+?)\1/g, '$2')
    .replace(/(^|[\s(])[*_](\S(?:.*?\S)?)[*_](?=[\s).,:;!?]|$)/g, '$1$2')
    .replace(/`([^`]+)`/g, '$1')
    .replace(/\s+/g, ' ')
    .trim()
}
