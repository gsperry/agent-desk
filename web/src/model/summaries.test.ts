import { describe, expect, it } from 'vitest'
import { costLine, defaultSummaries, loadSummaries, saveSummaries } from './summaries'

function memory() {
  const m = new Map<string, string>()
  return { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) }
}

describe('summary settings', () => {
  it('start off', () => {
    expect(loadSummaries(memory())).toEqual({ enabled: false, model: 'haiku' })
    expect(defaultSummaries.enabled).toBe(false)
  })
  it('round-trip and tolerate junk', () => {
    const kv = memory()
    saveSummaries(kv, { enabled: true, model: 'sonnet' })
    expect(loadSummaries(kv)).toEqual({ enabled: true, model: 'sonnet' })
    kv.setItem('agent-desk:summaries', JSON.stringify({ enabled: 'yes', model: '  ' }))
    expect(loadSummaries(kv)).toEqual({ enabled: false, model: 'haiku' })
    kv.setItem('agent-desk:summaries', '{')
    expect(loadSummaries(kv)).toEqual(defaultSummaries)
  })
})

describe('costLine', () => {
  it.each([
    [{ model: 'haiku', inputTokens: 1500, outputTokens: 30, costUSD: 0.0012 }, 'haiku · 1,500 in, 30 out tokens · $0.0012'],
    [{ model: 'sonnet', inputTokens: 9000, outputTokens: 200, costUSD: 0.05 }, 'sonnet · 9,000 in, 200 out tokens · $0.05'],
    [{ model: 'haiku', inputTokens: 10, outputTokens: 2, costUSD: 0, cached: true }, 'haiku · 10 in, 2 out tokens · from cache, no new tokens'],
  ])('%#', (r, want) => {
    expect(costLine(r)).toBe(want)
  })
})
