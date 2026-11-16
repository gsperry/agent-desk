import { describe, expect, it } from 'vitest'
import type { Item, SubAgent } from '../types'
import { agentCounts, agentDuration, agentFromPath, agentPath, agentType, childAgents, sidebarAgents, sortAgents } from './agents'

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

describe('subagent views', () => {
  it('round-trips the address', () => {
    expect(agentPath('s 1', 'ab/c')).toBe('/s/s%201/agents/ab%2Fc')
    expect(agentFromPath('/s/s%201/agents/ab%2Fc')).toBe('ab/c')
    expect(agentFromPath('/s/abc')).toBeNull()
    expect(agentFromPath('/s/abc/agents/')).toBeNull()
  })

  it('finds the subagents a transcript started, in order', () => {
    const list = [a('one', 'running', '2026-11-10T10:00:00Z'), a('two', 'completed', '2026-11-10T09:00:00Z'), a('other', 'running', '2026-11-10T08:00:00Z')]
    const items: Item[] = [
      { seq: 1, kind: 'tool_use', tool: { id: 'ttwo', name: 'Agent' } },
      { seq: 2, kind: 'tool_use', tool: { id: 'tone', name: 'Agent' } },
      { seq: 3, kind: 'tool_use', tool: { id: 'tzzz', name: 'Bash' } },
      { seq: 4, kind: 'tool_use', tool: { id: 'ttwo', name: 'Agent' } },
    ]
    expect(childAgents(items, list).map((x) => x.id)).toEqual(['two', 'one'])
  })

  it('lists running agents and a couple that ended under a session', () => {
    const list = [
      a('r1', 'running', '2026-11-10T10:00:00Z'),
      a('c1', 'completed', '2026-11-10T09:00:00Z'),
      a('c2', 'completed', '2026-11-10T08:00:00Z'),
      a('c3', 'completed', '2026-11-10T07:00:00Z'),
    ]
    expect(sidebarAgents(list, 5)).toEqual({ shown: [list[0], list[1], list[2]], more: 1 })
    const busy = [1, 2, 3, 4, 5, 6].map((n) => a(`r${String(n)}`, 'running', `2026-11-10T0${String(n)}:00:00Z`))
    expect(sidebarAgents(busy, 5).shown).toHaveLength(6)
    expect(sidebarAgents([], 5)).toEqual({ shown: [], more: 0 })
  })
})
