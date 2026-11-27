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

  it('keeps agent-manager order when asked, whatever the statuses', () => {
    const asListed = groupSessions(sessions, { showArchived: false, order: 'manager' })
    // Projects by where their first session stands; sessions as listed.
    expect(asListed.map((g) => g.project.name)).toEqual(['beta', 'alpha', 'gamma', 'delta'])
    expect(asListed[1]?.sessions.map((x) => x.id)).toEqual(['a-working', 'a-finished'])
    // A status change does not move anything.
    const changed = sessions.map((x) => (x.id === 'd-dead' ? { ...x, status: 'waiting', running: true } : x))
    expect(groupSessions(changed, { showArchived: false, order: 'manager' }).map((g) => g.project.name)).toEqual(['beta', 'alpha', 'gamma', 'delta'])
    // Attention still counts per group, so the badge shows without moving it.
    expect(groupSessions(changed, { showArchived: false, order: 'manager' })[3]?.attention).toBe(1)
  })

  it('lists agent-manager groups in its group order before other projects', () => {
    const inGroup = (id: string, path: string, status: string) => s(id, path, status, { group: path, project: { key: 'group:' + path, name: path, kind: 'group' } })
    const mixed = [s('r1', 'repo-one', 'idle'), inGroup('o1', 'ops', 'idle'), inGroup('w1', 'web', 'working'), s('r2', 'repo-two', 'idle')]
    const groups = [{ path: 'web', directory: '' }, { path: 'ops', directory: '' }, { path: 'empty', directory: '' }]
    const names = groupSessions(mixed, { showArchived: false, order: 'manager', groups }).map((g) => g.project.name)
    expect(names).toEqual(['web', 'ops', 'empty', 'repo-one', 'repo-two'])
  })

  it('keeps the old order without an order option', () => {
    expect(groupSessions(sessions, { showArchived: false }).map((g) => g.project.name)).toEqual(groupSessions(sessions, { showArchived: false, order: 'attention' }).map((g) => g.project.name))
  })

  it('counts only unseen finished sessions and can filter to attention', () => {
    const acks = { 'a-finished': true as const }
    const groups = groupSessions(sessions, { showArchived: false, acks, onlyAttention: true })
    expect(groups.map((g) => g.project.name)).toEqual(['gamma'])
    expect(groupSessions(sessions, { showArchived: false, acks })[0]?.project.name).toBe('gamma')
  })

  it('lists empty agent-manager groups, except in the attention filter', () => {
    const groups = [{ path: 'ops', directory: '/srv/ops' }]
    const all = groupSessions(sessions, { showArchived: false, groups })
    const ops = all.find((g) => g.project.key === 'group:ops')
    expect(ops?.sessions).toEqual([])
    expect(ops?.project.root).toBe('/srv/ops')
    expect(all.at(-1)?.project.name).toBe('ops')
    expect(groupSessions(sessions, { showArchived: false, groups, onlyAttention: true }).some((g) => g.project.key === 'group:ops')).toBe(false)
  })

  it('sorts unknown statuses last', () => {
    const g = groupSessions([s('x', 'p', 'mystery'), s('y', 'p', 'idle')], { showArchived: false })[0]
    expect(g?.sessions.map((x) => x.id)).toEqual(['y', 'x'])
  })
})
