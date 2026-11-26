import { describe, expect, it } from 'vitest'
import { maxNameLength, renameProblem } from './rename'

describe('renameProblem', () => {
  it.each([
    ['shop checkout', 'docs', null],
    ['  padded  ', 'docs', null],
    ['-looks-like-a-flag', 'docs', null],
    ['', 'docs', 'Enter a name.'],
    ['   ', 'docs', 'Enter a name.'],
    ['docs', 'docs', 'That is already its name.'],
    ['  docs ', 'docs', 'That is already its name.'],
    ['two\nlines', 'docs', 'Use no control characters or line breaks.'],
    ['tab\there', 'docs', 'Use no control characters or line breaks.'],
    ['n'.repeat(maxNameLength), 'docs', null],
    ['n'.repeat(maxNameLength + 1), 'docs', `Use at most ${String(maxNameLength)} characters.`],
  ])('%j for %j is %j', (name, current, want) => {
    expect(renameProblem(name, current)).toBe(want)
  })
})
