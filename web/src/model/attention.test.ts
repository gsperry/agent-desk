import { describe, expect, it } from 'vitest'
import type { SessionView } from '../types'
import {
  attentionCount,
  filterSessions,
  matchScore,
  needsAttention,
  nextAttention,
  shouldNotify,
  statusMap,
  transitions,
  updateAcks,
} from './attention'

function s(id: string, status: string, extra: Partial<SessionView> = {}): SessionView {
  return {
    id,
    name: id,
    tool: 'claude',
    group: '',
    directory: '/x',
    status,
    running: status !== 'dead',
    archived: false,
    project: { key: 'k', name: 'proj', kind: 'dir' },
    ...extra,
  }
}

describe('acks', () => {
  it('acknowledges a finished session while it is viewed, and resets when it works again', () => {
    let acks = updateAcks({}, [s('a', 'finished')], null)
    expect(acks).toEqual({})
    acks = updateAcks(acks, [s('a', 'finished')], 'a')
    expect(acks).toEqual({ a: true })
    acks = updateAcks(acks, [s('a', 'finished')], 'b')
    expect(acks).toEqual({ a: true })
    acks = updateAcks(acks, [s('a', 'working')], null)
    expect(acks).toEqual({})
  })

  it('returns the same object when nothing changes', () => {
    const acks = { a: true as const }
    expect(updateAcks(acks, [s('a', 'finished')], null)).toBe(acks)
  })

  it.each([
    ['waiting', {}, true],
    ['errored', {}, true],
    ['finished', {}, true],
    ['finished', { a: true as const }, false],
    ['working', {}, false],
    ['idle', {}, false],
  ])('%s with acks %j needs attention: %s', (status, acks, want) => {
    expect(needsAttention(s('a', status), acks)).toBe(want)
  })

  it('never flags archived sessions', () => {
    expect(needsAttention(s('a', 'waiting', { archived: true }), {})).toBe(false)
    expect(attentionCount([s('a', 'waiting'), s('b', 'idle'), s('c', 'finished')], {})).toBe(2)
  })
})

describe('transitions and notifications', () => {
  it('yields nothing on the first snapshot', () => {
    expect(transitions(null, [s('a', 'waiting')])).toEqual([])
  })

  it('reports changed statuses only', () => {
    const prev = statusMap([s('a', 'working'), s('b', 'idle')])
    const t = transitions(prev, [s('a', 'waiting'), s('b', 'idle'), s('new', 'starting')])
    expect(t.map((x) => `${x.session.id}:${x.from}>${x.to}`)).toEqual(['a:working>waiting'])
  })

  it.each([
    ['waiting', false, true],
    ['errored', false, true],
    ['finished', false, false],
    ['finished', true, true],
    ['idle', true, false],
  ])('to %s with onFinish=%s notifies: %s', (to, onFinish, want) => {
    expect(shouldNotify({ session: s('a', to), from: 'working', to }, { onFinish })).toBe(want)
  })
})

describe('nextAttention', () => {
  const list = [s('a', 'idle'), s('b', 'waiting'), s('c', 'working'), s('d', 'finished'), s('e', 'errored')]
  it.each([
    [null, 1, 'b'],
    [null, -1, 'e'],
    ['a', 1, 'b'],
    ['b', 1, 'd'],
    ['e', 1, 'b'],
    ['b', -1, 'e'],
    ['c', -1, 'b'],
  ] as const)('from %s dir %i goes to %s', (current, dir, want) => {
    expect(nextAttention(list, {}, current, dir)).toBe(want)
  })

  it('skips acknowledged finished sessions and returns null with nothing to do', () => {
    expect(nextAttention(list, { d: true }, 'b', 1)).toBe('e')
    expect(nextAttention([s('a', 'idle')], {}, null, 1)).toBeNull()
  })
})

describe('switcher matching', () => {
  it.each([
    ['api-orchestrator', '', 0],
    ['api-orchestrator', 'api', 0],
    ['api-orchestrator', 'orch', 0],
    ['api-orchestrator', 'rch', 1],
    ['api-orchestrator', 'aor', 13],
    ['api-orchestrator', 'xyz', null],
  ])('%s ~ %j = %s', (text, q, want) => {
    expect(matchScore(text, q)).toBe(want)
  })

  it('ranks name matches first, then sessions needing attention', () => {
    const sessions = [
      s('web-login', 'idle'),
      s('login-fix', 'idle'),
      s('auth', 'waiting', { project: { key: 'k2', name: 'login-service', kind: 'repo' } }),
    ]
    expect(filterSessions(sessions, 'login', {}).map((x) => x.id)).toEqual(['login-fix', 'web-login', 'auth'])
    expect(filterSessions(sessions, '', {})[0]?.id).toBe('auth')
  })
})
