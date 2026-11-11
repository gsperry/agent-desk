import { describe, expect, it } from 'vitest'
import { backoff, socketURL } from './api'

describe('socketURL', () => {
  it.each([
    ['http', { protocol: 'http:', host: '127.0.0.1:7317' }, 'ws://127.0.0.1:7317/api/ws'],
    ['https', { protocol: 'https:', host: 'example.test' }, 'wss://example.test/api/ws'],
  ])('%s', (_name, loc, want) => {
    expect(socketURL(loc)).toBe(want)
  })
})

describe('backoff', () => {
  it.each([
    [0, 500],
    [1, 1000],
    [3, 4000],
    [10, 10_000],
  ])('attempt %i waits %i ms', (attempt, want) => {
    expect(backoff(attempt)).toBe(want)
  })
})
