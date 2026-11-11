import { describe, expect, it } from 'vitest'
import { bashFiles, diffStats, patchRows, replaceRows } from './diff'

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

describe('bashFiles', () => {
  it('reads files, created flags and counts from bashEditDiff', () => {
    const detail = {
      stdout: 'ok',
      bashEditDiff: {
        files: [
          { filePath: 'site/_headers', created: true, hunks: [{ oldStart: 0, oldLines: 0, newStart: 1, newLines: 2, lines: ['+a', '+b'] }] },
          { filePath: 'site/index.html', hunks: [{ oldStart: 3, oldLines: 2, newStart: 3, newLines: 1, lines: [' keep', '-old', '-older', '+new'] }] },
          { hunks: [] },
          'junk',
        ],
        moreFiles: 2,
        changedFiles: ['site/_headers', 'site/index.html'],
      },
    }
    const { files, more } = bashFiles(detail)
    expect(files.map((f) => [f.path, f.created, f.added, f.removed])).toEqual([
      ['site/_headers', true, 2, 0],
      ['site/index.html', false, 1, 2],
    ])
    expect(files[1]?.rows[0]).toEqual({ type: 'ctx', oldNo: 3, newNo: 3, text: 'keep' })
    expect(more).toBe(2)
  })

  it.each([undefined, null, {}, { bashEditDiff: 'x' }, { bashEditDiff: { files: 'x', moreFiles: -1 } }])('ignores %j', (d) => {
    expect(bashFiles(d)).toEqual({ files: [], more: 0 })
  })
})
