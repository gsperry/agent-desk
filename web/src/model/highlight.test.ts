import { describe, expect, it } from 'vitest'
import { blockSeqs, splitMatches } from './highlight'
import { buildBlocks } from './transcript'

describe('splitMatches', () => {
  it.each([
    ['Flaky test here', 'flaky', [['Flaky', true], [' test here', false]]],
    ['a flaky FLAKY one', 'flaky', [['a ', false], ['flaky', true], [' ', false], ['FLAKY', true], [' one', false]]],
    ['nothing', 'zzz', [['nothing', false]]],
    ['text', '  ', [['text', false]]],
  ])('%s / %s', (text, q, want) => {
    expect(splitMatches(text, q).map((p) => [p.text, p.match])).toEqual(want)
  })
})

describe('blockSeqs', () => {
  it('covers turns with tool results, compactions and single items', () => {
    const blocks = buildBlocks([
      { seq: 1, kind: 'user', text: 'go' },
      { seq: 2, kind: 'tool_use', tool: { id: 't', name: 'Bash' } },
      { seq: 3, kind: 'text', text: 'x' },
      { seq: 4, kind: 'tool_result', result: { toolUseId: 't', content: '' } },
      { seq: 5, kind: 'compact_boundary' },
      { seq: 6, kind: 'compact_summary', text: 's' },
    ])
    expect(blocks.map(blockSeqs)).toEqual([[1], [2, 4, 3], [5, 6]])
  })
})
