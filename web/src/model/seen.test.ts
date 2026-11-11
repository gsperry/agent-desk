import { describe, expect, it } from 'vitest'
import type { SessionView } from '../types'
import { baseline, dividerSeq, hasNew, loadSeen, markSeen, saveSeen } from './seen'

const MAX = Number.MAX_SAFE_INTEGER

function s(conv: string, lastActivity?: string): SessionView {
  return {
    id: conv,
    name: conv,
    tool: 'claude',
    group: '',
    directory: '/x',
    status: 'idle',
    running: true,
    archived: false,
    agentSessionId: conv,
    lastActivity,
    project: { key: 'k', name: 'p', kind: 'dir' },
  }
}

describe('seen marks', () => {
  it('baselines unknown conversations so the first run flags nothing', () => {
    const seen = baseline({}, [s('a', '2026-11-10T10:00:00Z'), s('b')])
    expect(seen).toEqual({ a: { at: '2026-11-10T10:00:00Z', seq: MAX } })
    expect(hasNew(s('a', '2026-11-10T10:00:00Z'), seen)).toBe(false)
    expect(baseline(seen, [s('a', '2026-11-10T11:00:00Z')])).toBe(seen)
  })

  it('flags activity after the mark', () => {
    const seen = { a: { at: '2026-11-10T10:00:00Z', seq: 5 } }
    expect(hasNew(s('a', '2026-11-10T10:00:01Z'), seen)).toBe(true)
    expect(hasNew(s('a', '2026-11-10T09:00:00Z'), seen)).toBe(false)
    expect(hasNew(s('z', '2026-11-10T10:00:01Z'), seen)).toBe(false)
  })

  it('only moves marks forward', () => {
    let seen = markSeen({}, 'a', '2026-11-10T10:00:00Z', 10)
    seen = markSeen(seen, 'a', '2026-11-10T09:00:00Z', 4)
    expect(seen.a).toEqual({ at: '2026-11-10T10:00:00Z', seq: 10 })
    const same = markSeen(seen, 'a', '2026-11-10T10:00:00Z', 10)
    expect(same).toBe(seen)
    expect(markSeen({ a: { at: 'x', seq: MAX } }, 'a', 'y', 3).a).toEqual({ at: 'y', seq: 3 })
  })
})

describe('dividerSeq', () => {
  it.each([
    [undefined, [1, 2, 3], null],
    [{ seq: MAX }, [1, 2, 3], null],
    [{ seq: 2 }, [1, 2, 3, 4], 3],
    [{ seq: 9 }, [1, 2, 3], null],
  ])('%j over %j is %j', (mark, seqs, want) => {
    expect(dividerSeq(mark, seqs)).toBe(want)
  })
})

describe('storage', () => {
  it('round-trips and caps entries', () => {
    const m = new Map<string, string>()
    const store = { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) }
    const seen = Object.fromEntries(Array.from({ length: 600 }, (_, i) => [`c${String(i)}`, { at: String(i).padStart(4, '0'), seq: i }]))
    saveSeen(store, seen)
    const back = loadSeen(store)
    expect(Object.keys(back)).toHaveLength(500)
    expect(back.c599).toBeDefined()
    expect(back.c0).toBeUndefined()
  })
})
