import { describe, expect, it } from 'vitest'
import { anchorFor, loadPosition, savePosition } from './scroll'

function memory() {
  const m = new Map<string, string>()
  return { m, getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v), removeItem: (k: string) => void m.delete(k) }
}

describe('reading position', () => {
  it('round-trips per session and forgets when following', () => {
    const kv = memory()
    savePosition(kv, 's1', { key: 'b12', offset: -40 })
    expect(loadPosition(kv, 's1')).toEqual({ key: 'b12', offset: -40 })
    expect(loadPosition(kv, 's2')).toBeNull()
    savePosition(kv, 's1', null)
    expect(kv.m.size).toBe(0)
    kv.setItem('agent-desk:scroll:s3', '{"key":1}')
    expect(loadPosition(kv, 's3')).toBeNull()
    kv.setItem('agent-desk:scroll:s4', 'nope')
    expect(loadPosition(kv, 's4')).toBeNull()
  })

  it.each([
    [[{ key: 'a', top: -300, bottom: -10 }, { key: 'b', top: -10, bottom: 200 }, { key: 'c', top: 200, bottom: 500 }], { key: 'b', offset: -10 }],
    [[{ key: 'a', top: 20, bottom: 100 }], { key: 'a', offset: 20 }],
    [[{ key: 'a', top: -300, bottom: -10 }], null],
    [[], null],
  ])('anchor %#', (blocks, want) => {
    expect(anchorFor(blocks)).toEqual(want)
  })
})
