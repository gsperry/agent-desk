import { describe, expect, it } from 'vitest'
import { costLine, defaultSummaries, parseSummaries, takeLegacySummaries } from './summaries'

function memory(init: Record<string, string> = {}) {
  const m = new Map<string, string>(Object.entries(init))
  return { m, getItem: (k: string) => m.get(k) ?? null, removeItem: (k: string) => void m.delete(k) }
}

describe('summary settings', () => {
  it('start off', () => {
    expect(defaultSummaries).toEqual({ enabled: false, model: 'haiku' })
  })
  it('tolerate junk', () => {
    expect(parseSummaries(JSON.stringify({ enabled: true, model: ' sonnet ' }))).toEqual({ enabled: true, model: 'sonnet' })
    expect(parseSummaries(JSON.stringify({ enabled: 'yes', model: '  ' }))).toEqual({ enabled: false, model: 'haiku' })
    expect(parseSummaries('{')).toEqual(defaultSummaries)
  })
  it('take the settings this browser kept once, then forget them', () => {
    const kv = memory({ 'agent-desk:summaries': JSON.stringify({ enabled: true, model: 'sonnet' }) })
    expect(takeLegacySummaries(kv)).toEqual({ enabled: true, model: 'sonnet' })
    expect(kv.m.size).toBe(0)
    expect(takeLegacySummaries(kv)).toBeNull()
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
