import type { SubAgent } from '../types'
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
