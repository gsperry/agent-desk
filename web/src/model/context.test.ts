import { describe, expect, it } from 'vitest'
import type { Item } from '../types'
import { contextState, formatTokens, learnThresholds } from './context'

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
