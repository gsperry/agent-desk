import { describe, expect, it } from 'vitest'
import type { SessionView } from '../types'
import { groupSessions } from './sidebar'

function s(id: string, project: string, status: string, extra: Partial<SessionView> = {}): SessionView {
  return {
    id,
    name: id,
    tool: 'claude',
    group: '',
    directory: '/src/' + project,
    status,
    running: status !== 'dead',
    archived: false,
    project: { key: 'repo:/src/' + project, name: project, kind: 'repo' },
    ...extra,
  }
}

describe('groupSessions', () => {
  const sessions = [
    s('b-idle', 'beta', 'idle'),
    s('a-working', 'alpha', 'working'),
    s('c-waiting', 'gamma', 'waiting'),
    s('a-finished', 'alpha', 'finished'),
    s('d-dead', 'delta', 'dead'),
    s('a-archived', 'alpha', 'dead', { archived: true, running: false }),
  ]

  it('puts attention groups first, then live groups, then by name', () => {
    const names = groupSessions(sessions, { showArchived: false }).map((g) => g.project.name)
    expect(names).toEqual(['alpha', 'gamma', 'beta', 'delta'])
  })

  it('sorts sessions by status rank then name', () => {
    const alpha = groupSessions(sessions, { showArchived: false })[0]
    expect(alpha?.sessions.map((x) => x.id)).toEqual(['a-finished', 'a-working'])
    expect(alpha?.attention).toBe(1)
  })

  it.each([
    [false, 5],
    [true, 6],
  ])('showArchived=%s shows %i sessions', (showArchived, want) => {
    const total = groupSessions(sessions, { showArchived }).reduce((n, g) => n + g.sessions.length, 0)
    expect(total).toBe(want)
  })

  it('counts only unseen finished sessions and can filter to attention', () => {
    const acks = { 'a-finished': true as const }
    const groups = groupSessions(sessions, { showArchived: false, acks, onlyAttention: true })
    expect(groups.map((g) => g.project.name)).toEqual(['gamma'])
    expect(groupSessions(sessions, { showArchived: false, acks })[0]?.project.name).toBe('gamma')
  })

  it('sorts unknown statuses last', () => {
    const g = groupSessions([s('x', 'p', 'mystery'), s('y', 'p', 'idle')], { showArchived: false })[0]
    expect(g?.sessions.map((x) => x.id)).toEqual(['y', 'x'])
  })
})
