import { describe, expect, it } from 'vitest'
import { composeReview, loadComments, saveComments, type ReviewComment } from './review'

const c = (file: string, line: number, text: string, side: 'new' | 'old' = 'new', code = 'x := 1'): ReviewComment => ({ id: file + String(line), file, line, side, code, text })

describe('composeReview', () => {
  it('numbers comments by file and line with their code', () => {
    const text = composeReview([c('b.go', 3, 'rename this'), c('a.go', 20, 'handle the error\nand log it'), c('a.go', 4, 'why removed?', 'old', 'return nil')])
    expect(text).toContain('Address each one')
    expect(text).toContain('1. a.go:4 (removed line)\n   > return nil\n   why removed?')
    expect(text).toContain('2. a.go:20\n   > x := 1\n   handle the error\n   and log it')
    expect(text).toContain('3. b.go:3')
  })

  it('omits empty code', () => {
    expect(composeReview([c('a.go', 1, 'hi', 'new', '  ')])).toContain('1. a.go:1\n   hi')
  })
})

describe('comment storage', () => {
  it('round-trips per session and clears when empty', () => {
    const m = new Map<string, string>()
    const store = { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v), removeItem: (k: string) => void m.delete(k) }
    saveComments(store, 's1', [c('a.go', 1, 'x')])
    expect(loadComments(store, 's1')).toHaveLength(1)
    expect(loadComments(store, 's2')).toEqual([])
    saveComments(store, 's1', [])
    expect(m.size).toBe(0)
  })
})
