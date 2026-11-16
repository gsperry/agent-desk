import type { Item, SubAgent } from '../types'
import { formatDuration } from './transcript'

// sortAgents puts running agents first, then the newest.
export function sortAgents(list: SubAgent[]): SubAgent[] {
  return [...list].sort((a, b) => Number(b.status === 'running') - Number(a.status === 'running') || (b.started ?? '').localeCompare(a.started ?? ''))
}

// agentType splits a plugin-qualified type like "agent-orchestra:executor"
// into its plugin and name.
export function agentType(t: string): { plugin?: string; name: string } {
  const i = t.lastIndexOf(':')
  return i > 0 ? { plugin: t.slice(0, i), name: t.slice(i + 1) } : { name: t || 'agent' }
}

// agentDuration is how long an agent ran, or has been running, from its
// first record to its last write.
export function agentDuration(a: SubAgent): string {
  if (!a.started || !a.lastActivity) return ''
  const ms = Date.parse(a.lastActivity) - Date.parse(a.started)
  return Number.isFinite(ms) && ms >= 0 ? formatDuration(ms) : ''
}

export function agentCounts(list: SubAgent[]): { running: number; total: number } {
  return { running: list.filter((a) => a.status === 'running').length, total: list.length }
}

// agentPath is the address of a subagent's own view.
export function agentPath(sessionId: string, agentId: string): string {
  return `/s/${encodeURIComponent(sessionId)}/agents/${encodeURIComponent(agentId)}`
}

// agentFromPath reads the subagent from /s/<session>/agents/<agent>.
export function agentFromPath(pathname: string): string | null {
  const m = /^\/s\/[^/]+\/agents\/([^/]+)\/?$/.exec(pathname)
  return m?.[1] ? decodeURIComponent(m[1]) : null
}

// childAgents are the subagents a transcript's Agent tool calls started,
// in the order they were started.
export function childAgents(items: Item[], agents: SubAgent[]): SubAgent[] {
  const byToolUse = new Map(agents.map((a) => [a.toolUseId, a]))
  const out: SubAgent[] = []
  for (const it of items) {
    const a = it.tool ? byToolUse.get(it.tool.id) : undefined
    if (a && !out.includes(a)) out.push(a)
  }
  return out
}

// sidebarAgents is the subagents listed under a session in the sidebar:
// running ones, then the latest few that ended, at most limit in all.
export function sidebarAgents(list: SubAgent[], limit: number): { shown: SubAgent[]; more: number } {
  const sorted = sortAgents(list)
  const running = sorted.filter((a) => a.status === 'running')
  const shown = running.length >= limit ? running : [...running, ...sorted.filter((a) => a.status !== 'running').slice(0, Math.max(0, Math.min(2, limit - running.length)))]
  return { shown, more: list.length - shown.length }
}
