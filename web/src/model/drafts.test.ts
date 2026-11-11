import { describe, expect, it } from 'vitest'
import { loadDraft, saveDraft } from './drafts'

function memory() {
  const m = new Map<string, string>()
  return {
    getItem: (k: string) => m.get(k) ?? null,
    setItem: (k: string, v: string) => m.set(k, v),
    removeItem: (k: string) => m.delete(k),
    size: () => m.size,
  }
}

describe('drafts', () => {
  it('keeps one draft per session', () => {
    const s = memory()
    saveDraft(s, 'a', 'for a')
    saveDraft(s, 'b', 'for b')
    expect(loadDraft(s, 'a')).toBe('for a')
    expect(loadDraft(s, 'b')).toBe('for b')
    expect(loadDraft(s, 'c')).toBe('')
  })

  it('removes a blank draft', () => {
    const s = memory()
    saveDraft(s, 'a', 'text')
    saveDraft(s, 'a', '  \n')
    expect(s.size()).toBe(0)
  })
})
