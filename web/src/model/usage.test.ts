import { describe, expect, it } from 'vitest'
import type { Conversation, UsageReport } from '../types'
import { barScale, byProject, formatHours, formatTokens, formatUSD, totalTokens } from './usage'

const bucket = (input: number, extra: Partial<UsageReport['totals']> = {}) => ({
  input,
  output: 0,
  cacheRead: 0,
  cacheWrite: 0,
  replies: 1,
  prompts: 1,
  workingMs: 1000,
  waitingMs: 0,
  costUSD: 0.5,
  ...extra,
})

describe('formatting', () => {
  it.each([
    [0, '0'],
    [950, '950'],
    [12_300, '12.3k'],
    [4_500_000, '4.5M'],
    [20_149_300_000, '20.1B'],
    [250_000_000, '250M'],
  ])('tokens %d', (n, want) => {
    expect(formatTokens(n)).toBe(want)
  })
  it.each([
    [0, '0m'],
    [45 * 60_000, '45m'],
    [185 * 60_000, '3h 05m'],
  ])('hours %d', (ms, want) => {
    expect(formatHours(ms)).toBe(want)
  })
  it('formats dollars', () => {
    expect(formatUSD(1.234)).toBe('$1.23')
    expect(formatUSD(13268.93)).toBe('$13,269')
  })
})

describe('barScale', () => {
  it('scales to the busiest day', () => {
    const days = [
      { day: 'a', ...bucket(0) },
      { day: 'b', ...bucket(50) },
      { day: 'c', ...bucket(100, { cacheRead: 100 }) },
    ]
    expect(barScale(days)).toEqual([0, 0.25, 1])
    expect(barScale([{ day: 'a', ...bucket(0) }])).toEqual([0])
  })
})

describe('byProject', () => {
  it('sums conversations by directory', () => {
    const rows = [
      { id: 'c1', subagentTokens: 0, ...bucket(10) },
      { id: 'c2', subagentTokens: 0, ...bucket(30) },
      { id: 'c3', subagentTokens: 0, ...bucket(5) },
      { id: 'gone', subagentTokens: 0, ...bucket(1) },
    ]
    const listing: Conversation[] = [
      { id: 'c1', directory: '/src/web', modified: '', size: 0 },
      { id: 'c2', directory: '/src/web', modified: '', size: 0 },
      { id: 'c3', directory: '/src/api', modified: '', size: 0 },
    ]
    const got = byProject(rows, listing)
    expect(got.map((p) => [p.name, totalTokens(p), p.conversations])).toEqual([
      ['web', 40, 2],
      ['api', 5, 1],
      ['unknown directory', 1, 1],
    ])
    expect(got[0]?.costUSD).toBe(1)
  })
})
