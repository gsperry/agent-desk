import { describe, expect, it } from 'vitest'
import type { Item } from '../types'
import { contextState, formatTokens, learnThresholds, modelLabel, modelState } from './context'

let seq = 0
const turn = (model: string, contextTokens: number): Item => ({ seq: seq++, kind: 'text', text: 'x', meta: { usage: { model, contextTokens, outputTokens: 10 } } })
const compact = (preTokens: number, trigger = 'auto'): Item => ({ seq: seq++, kind: 'compact_boundary', meta: { preTokens, trigger } })

describe('learnThresholds', () => {
  it('learns the largest automatic compaction per model', () => {
    const th = learnThresholds({}, [turn('opus', 900_000), compact(967_000), turn('opus', 10_000), compact(966_000), turn('haiku', 150_000), compact(160_000, 'manual')])
    expect(th).toEqual({ opus: 967_000 })
  })

  it('returns the same object when nothing is learned', () => {
    const th = { opus: 967_000 }
    expect(learnThresholds(th, [turn('opus', 5), compact(900_000)])).toBe(th)
  })
})

describe('contextState', () => {
  it('reports the latest turn against the learned limit', () => {
    expect(contextState([turn('opus', 100_000), turn('opus', 483_500)], { opus: 967_000 })).toEqual({ tokens: 483_500, model: 'opus', limit: 967_000, ratio: 0.5 })
  })

  it('reports tokens only without a learned limit', () => {
    expect(contextState([turn('sonnet', 42_000)], {})).toEqual({ tokens: 42_000, model: 'sonnet', limit: undefined, ratio: undefined })
  })

  it('knows nothing right after a compaction or with no usage', () => {
    expect(contextState([turn('opus', 900_000), compact(960_000)], {})).toBeNull()
    expect(contextState([{ seq: 1, kind: 'user', text: 'hi' }], {})).toBeNull()
  })
})

const withEffort = (model: string, effort?: string): Item => ({ seq: seq++, kind: 'text', text: 'x', meta: { usage: { model, contextTokens: 5, outputTokens: 1, ...(effort ? { effort } : {}) } } })

describe('modelState', () => {
  it('is the model and effort of the latest turn', () => {
    expect(modelState([withEffort('claude-sonnet-5-5', 'medium'), withEffort('claude-opus-5-5', 'high')])).toEqual({ model: 'claude-opus-5-5', effort: 'high' })
  })

  it('has no effort when the version does not record one', () => {
    expect(modelState([withEffort('claude-opus-5-5')])).toEqual({ model: 'claude-opus-5-5' })
  })

  it('skips synthetic records and survives a compaction', () => {
    expect(modelState([withEffort('claude-opus-5-5', 'high'), compact(900_000), withEffort('<synthetic>')])).toEqual({ model: 'claude-opus-5-5', effort: 'high' })
  })

  it('knows nothing without usage', () => {
    expect(modelState([{ seq: 0, kind: 'text', text: 'x' }])).toBeNull()
  })
})

describe('modelLabel', () => {
  it('drops the claude- prefix', () => {
    expect(modelLabel('claude-opus-5-5')).toBe('opus-5-5')
    expect(modelLabel('gpt-x')).toBe('gpt-x')
  })
})

describe('formatTokens', () => {
  it.each([
    [512, '512'],
    [42_400, '42k'],
    [967_000, '967k'],
    [1_200_000, '1.2M'],
    [12_000_000, '12M'],
  ])('%i is %s', (n, want) => {
    expect(formatTokens(n)).toBe(want)
  })
})
