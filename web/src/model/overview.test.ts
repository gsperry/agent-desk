import { describe, expect, it } from 'vitest'
import type { SessionView } from '../types'
import { ago, overviewCards, plainText } from './overview'

function s(id: string, status: string, lastActivity: string, extra: Partial<SessionView> = {}): SessionView {
  return {
    id,
    name: id,
    tool: 'claude',
    group: '',
    directory: '/x',
    status,
    running: status !== 'dead',
    archived: false,
    lastActivity,
    project: { key: 'k', name: 'p', kind: 'dir' },
    ...extra,
  }
}

describe('overviewCards', () => {
  const sessions = [
    s('idle-new', 'idle', '2026-11-10T10:05:00Z'),
    s('working', 'working', '2026-11-10T10:00:00Z'),
    s('waiting', 'waiting', '2026-11-10T09:00:00Z'),
    s('dead', 'dead', '2026-11-10T11:00:00Z'),
    s('archived', 'waiting', '2026-11-10T11:00:00Z', { archived: true }),
    s('idle-old', 'idle', '2026-11-10T08:00:00Z'),
  ]
  it('orders by need, then activity, and hides stopped and archived sessions', () => {
    expect(overviewCards(sessions, {}, false).map((x) => x.id)).toEqual(['waiting', 'working', 'idle-new', 'idle-old'])
  })
  it('shows stopped sessions last on request', () => {
    expect(overviewCards(sessions, {}, true).at(-1)?.id).toBe('dead')
  })
})

describe('ago', () => {
  const now = Date.parse('2026-11-10T12:00:00Z')
  it.each([
    ['2026-11-10T11:59:55Z', 'now'],
    ['2026-11-10T11:59:15Z', '45s ago'],
    ['2026-11-10T11:48:00Z', '12m ago'],
    ['2026-11-10T09:00:00Z', '3h ago'],
    ['2026-11-07T12:00:00Z', '3d ago'],
    [undefined, ''],
    ['garbage', ''],
  ])('%s is %s', (iso, want) => {
    expect(ago(iso, now)).toBe(want)
  })
})

describe('plainText', () => {
  it.each([
    ['Rate limiting is in.\n\n- **Limiter:** token bucket in `api/ratelimit`.', 'Rate limiting is in. Limiter: token bucket in api/ratelimit.'],
    ['## Done\n1. See [the docs](https://x.dev) and _this_.', 'Done See the docs and this.'],
    ['keep snake_case_names and 2*3*4', 'keep snake_case_names and 2*3*4'],
    ['```\ncode\n```\nafter', 'after'],
  ])('%#', (md, want) => {
    expect(plainText(md)).toBe(want)
  })
})
