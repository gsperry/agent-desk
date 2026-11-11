import { describe, expect, it } from 'vitest'
import type { SubAgent } from '../types'
import { agentCounts, agentDuration, agentType, sortAgents } from './agents'

const a = (id: string, status: string, started: string, lastActivity?: string): SubAgent => ({
  id,
  toolUseId: 't' + id,
  type: 'x',
  description: '',
  depth: 1,
  status,
  started,
  lastActivity,
})

describe('agents', () => {
  it('sorts running first, then newest', () => {
    const list = [a('old', 'completed', '2026-11-10T09:00:00Z'), a('run', 'running', '2026-11-10T08:00:00Z'), a('new', 'completed', '2026-11-10T10:00:00Z')]
    expect(sortAgents(list).map((x) => x.id)).toEqual(['run', 'new', 'old'])
    expect(agentCounts(list)).toEqual({ running: 1, total: 3 })
  })

  it.each([
    ['agent-orchestra:executor', { plugin: 'agent-orchestra', name: 'executor' }],
    ['general-purpose', { name: 'general-purpose' }],
    ['', { name: 'agent' }],
  ])('agentType(%j)', (t, want) => {
    expect(agentType(t)).toEqual(want)
  })

  it('measures duration from first record to last write', () => {
    expect(agentDuration(a('x', 'completed', '2026-11-10T10:00:00Z', '2026-11-10T10:02:05Z'))).toBe('2m 5s')
    expect(agentDuration(a('x', 'running', '2026-11-10T10:00:00Z'))).toBe('')
  })
})
