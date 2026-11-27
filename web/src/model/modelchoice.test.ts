import { describe, expect, it } from 'vitest'
import type { ModelChoice, ModelOptions } from '../types'
import { effortFor, pick, startEffort } from './modelchoice'

const all = ['Low', 'Medium', 'High', 'xHigh', 'Max']
const sonnet: ModelChoice = { label: 'Sonnet 5.5', current: true, efforts: all, defaultEffort: 'Medium' }
const opus: ModelChoice = { label: 'Opus 4.6', efforts: ['Low', 'Medium', 'High', 'Max'], defaultEffort: 'High' }
const bare: ModelChoice = { label: 'Odd', efforts: ['Low', 'High'] }

describe('pick', () => {
  it('starts on the current model at its default effort', () => {
    expect(pick({ models: [opus, sonnet] })).toEqual({ model: 'Sonnet 5.5', effort: 'Medium' })
  })

  it('starts on the first model when none is marked current', () => {
    expect(pick({ models: [opus, bare] })).toEqual({ model: 'Opus 4.6', effort: 'High' })
  })

  it('has nothing to start on without models', () => {
    expect(pick({ models: [] } satisfies ModelOptions)).toEqual({ model: '', effort: '' })
  })
})

describe('startEffort', () => {
  it('is the default, else the first level', () => {
    expect(startEffort(sonnet)).toBe('Medium')
    expect(startEffort(bare)).toBe('Low')
    expect(startEffort({ label: 'None', efforts: [] })).toBe('')
  })
})

describe('effortFor', () => {
  it('keeps an effort the model takes', () => {
    expect(effortFor(sonnet, 'xHigh')).toBe('xHigh')
  })

  it('falls back to the model start for one it lacks', () => {
    expect(effortFor(opus, 'xHigh')).toBe('High')
  })
})
