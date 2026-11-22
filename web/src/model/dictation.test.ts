import { describe, expect, it } from 'vitest'
import { anchorAt, parseEvent, withHeard } from './dictation'

describe('withHeard', () => {
  it.each([
    ['', 0, 0, 'hello there', 'hello there', 11],
    ['Fix', 3, 3, 'the tests', 'Fix the tests', 13],
    ['Fix ', 4, 4, 'the tests', 'Fix the tests', 13],
    ['Fix now', 3, 3, 'the tests', 'Fix the tests now', 13],
    ['Fix X now', 4, 5, 'the tests', 'Fix the tests now', 13],
    ['Fix', 3, 3, '  ', 'Fix', 3],
  ])('%j at %i-%i hearing %j', (text, start, end, heard, want, caret) => {
    expect(withHeard(anchorAt(text, start, end), heard)).toEqual({ text: want, caret })
  })

  it('replaces a revised guess rather than adding to it', () => {
    const a = anchorAt('Note:', 5, 5)
    withHeard(a, 'wreck a nice')
    expect(withHeard(a, 'recognize speech').text).toBe('Note: recognize speech')
  })

  it('clamps a stale caret', () => {
    expect(anchorAt('abc', 10, 2)).toEqual({ before: 'abc', after: '' })
  })
})

describe('parseEvent', () => {
  it('accepts known events', () => {
    expect(parseEvent({ type: 'ready' })).toEqual({ type: 'ready' })
    expect(parseEvent({ type: 'text', text: 'hi', final: true })).toEqual({ type: 'text', text: 'hi', final: true })
    expect(parseEvent({ type: 'error', message: '' })).toEqual({ type: 'error', message: 'Dictation stopped.' })
  })
  it('drops anything else', () => {
    expect(parseEvent(null)).toBeNull()
    expect(parseEvent({ type: 'text' })).toBeNull()
    expect(parseEvent({ type: 'eval', text: 'x' })).toBeNull()
  })
})
