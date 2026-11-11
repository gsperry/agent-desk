import { describe, expect, it } from 'vitest'
import { parseHashToken } from './auth'

const token = '0123456789abcdef0123456789abcdef'

describe('parseHashToken', () => {
  it.each([
    ['token present', `#t=${token}`, token],
    ['without leading hash', `t=${token}`, token],
    ['among other params', `#x=1&t=${token}`, token],
    ['empty hash', '', null],
    ['missing token', '#x=1', null],
    ['too short', '#t=abc', null],
  ])('%s', (_name, hash, want) => {
    expect(parseHashToken(hash)).toBe(want)
  })
})
