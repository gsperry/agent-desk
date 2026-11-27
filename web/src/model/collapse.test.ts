import { describe, expect, it } from 'vitest'
import { loadCollapsed, saveCollapsed, toggled, visibleSessions } from './collapse'

function memory(initial?: string) {
  let value = initial
  return {
    getItem: () => value ?? null,
    setItem: (_k: string, v: string) => {
      value = v
    },
  }
}

describe('collapsed groups', () => {
  it('round-trips through storage in a stable order', () => {
    const store = memory()
    saveCollapsed(store, new Set(['repo:/b', 'group:ops']))
    expect(store.getItem()).toBe('["group:ops","repo:/b"]')
    expect([...loadCollapsed(store)]).toEqual(['group:ops', 'repo:/b'])
  })

  it.each([undefined, 'not json', '{"a":1}', '[1,2]'])('loads %j as nothing collapsed or only valid keys', (raw) => {
    expect(loadCollapsed(memory(raw)).size).toBe(0)
  })

  it('keeps string keys and drops others', () => {
    expect([...loadCollapsed(memory('["group:ops",3,null]'))]).toEqual(['group:ops'])
  })

  it('toggles without changing the original', () => {
    const start = new Set(['a'])
    expect([...toggled(start, 'b')].sort()).toEqual(['a', 'b'])
    expect([...toggled(start, 'a')]).toEqual([])
    expect([...start]).toEqual(['a'])
  })
})

describe('visibleSessions', () => {
  const list = [{ id: 'a' }, { id: 'b' }, { id: 'c' }]
  it('shows all when open, and only the selected one when folded', () => {
    expect(visibleSessions(list, false, 'b')).toEqual(list)
    expect(visibleSessions(list, true, 'b')).toEqual([{ id: 'b' }])
    expect(visibleSessions(list, true, null)).toEqual([])
    expect(visibleSessions(list, true, 'zzz')).toEqual([])
  })
})
