import { describe, expect, it } from 'vitest'
import type { Item } from '../types'
import { emptyNotebook, loadNotebook, pinFrom, pinKey, saveNotebook, togglePin } from './pins'

const item = (over: Partial<Item>): Item => ({ seq: 1, kind: 'text', uuid: 'u1', text: 'Hello', ...over })

function memory() {
  const m = new Map<string, string>()
  return {
    m,
    getItem: (k: string) => m.get(k) ?? null,
    setItem: (k: string, v: string) => void m.set(k, v),
    removeItem: (k: string) => void m.delete(k),
  }
}

describe('pins', () => {
  it.each([
    [item({}), 'u1:text'],
    [item({ kind: 'user' }), 'u1:user'],
    [item({ kind: 'tool_use' }), null],
    [item({ uuid: undefined }), null],
  ])('pinKey %#', (it, want) => {
    expect(pinKey(it)).toBe(want)
  })

  it('makes a one-line excerpt', () => {
    expect(pinFrom(item({ text: 'a\n\n  b' }))?.excerpt).toBe('a b')
    expect(pinFrom(item({ text: 'x'.repeat(200) }))?.excerpt).toHaveLength(161)
  })

  it('toggles and keeps time order', () => {
    let nb = togglePin(emptyNotebook, item({ uuid: 'b', time: '2026-11-02T00:00:00Z' }))
    nb = togglePin(nb, item({ uuid: 'a', time: '2026-11-01T00:00:00Z' }))
    expect(nb.pins.map((p) => p.key)).toEqual(['a:text', 'b:text'])
    nb = togglePin(nb, item({ uuid: 'b', time: '2026-11-02T00:00:00Z' }))
    expect(nb.pins.map((p) => p.key)).toEqual(['a:text'])
    expect(togglePin(nb, item({ kind: 'tool_use' }))).toBe(nb)
  })

  it('stores per conversation and forgets empty notebooks', () => {
    const kv = memory()
    const nb = { note: 'check auth', pins: [] }
    saveNotebook(kv, 'c1', nb)
    expect(loadNotebook(kv, 'c1')).toEqual(nb)
    expect(loadNotebook(kv, 'c2')).toEqual(emptyNotebook)
    saveNotebook(kv, 'c1', emptyNotebook)
    expect(kv.m.size).toBe(0)
    kv.setItem('agent-desk:notebook:c3', JSON.stringify({ note: 4, pins: [{ key: 'k', kind: 'user', excerpt: 'e' }, { bad: 1 }] }))
    expect(loadNotebook(kv, 'c3')).toEqual({ note: '', pins: [{ key: 'k', kind: 'user', excerpt: 'e' }] })
    kv.setItem('agent-desk:notebook:c4', '{')
    expect(loadNotebook(kv, 'c4')).toEqual(emptyNotebook)
  })
})
