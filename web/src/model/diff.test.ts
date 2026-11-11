import { describe, expect, it } from 'vitest'
import { diffStats, patchRows, replaceRows } from './diff'

describe('patchRows', () => {
  it('numbers lines across hunks and marks gaps', () => {
    const rows = patchRows([
      { oldStart: 10, oldLines: 2, newStart: 10, newLines: 2, lines: [' keep', '-old', '+new'] },
      { oldStart: 40, oldLines: 1, newStart: 40, newLines: 2, lines: [' x', '+y', '\\ No newline at end of file'] },
    ])
    expect(rows).toEqual([
      { type: 'ctx', oldNo: 10, newNo: 10, text: 'keep' },
      { type: 'del', oldNo: 11, text: 'old' },
      { type: 'add', newNo: 11, text: 'new' },
      { type: 'gap', text: '' },
      { type: 'ctx', oldNo: 40, newNo: 40, text: 'x' },
      { type: 'add', newNo: 41, text: 'y' },
    ])
    expect(diffStats(rows)).toEqual({ added: 2, removed: 1 })
  })

  it.each([[null], ['text'], [[{ nope: 1 }]]])('ignores malformed input %#', (input) => {
    expect(patchRows(input)).toEqual([])
  })
})

describe('replaceRows', () => {
  it('removes old lines then adds new ones', () => {
    expect(replaceRows('a\nb\n', 'c')).toEqual([
      { type: 'del', text: 'a' },
      { type: 'del', text: 'b' },
      { type: 'add', text: 'c' },
    ])
  })
})
