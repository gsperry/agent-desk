import { describe, expect, it } from 'vitest'
import { screenSize, stripAnsi } from './terminal'

describe('terminal', () => {
  it('strips color and OSC sequences', () => {
    expect(stripAnsi('\x1b[1;31mred\x1b[0m plain \x1b]8;;https://x\x07link\x1b]8;;\x07')).toBe('red plain link')
  })
  it.each([
    ['\x1b[31m' + 'x'.repeat(120) + '\x1b[0m\nshort\n\n', { cols: 120, rows: 5 }],
    ['a\nb\nc\nd\ne\nf\n', { cols: 40, rows: 6 }],
    ['❯ ok', { cols: 40, rows: 5 }],
    ['y'.repeat(900), { cols: 400, rows: 5 }],
  ])('size %#', (screen, want) => {
    expect(screenSize(screen)).toEqual(want)
  })
})
