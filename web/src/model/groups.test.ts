import { describe, expect, it } from 'vitest'
import { groupSteps, groupSummary, type Shown } from './groups'
import type { Step } from './transcript'

let seq = 0
const tool = (name: string): Step => ({ kind: 'tool', use: { seq: ++seq, kind: 'tool_use', tool: { id: `t${String(seq)}`, name } } })
const thought = (): Step => ({ kind: 'thinking', item: { seq: ++seq, kind: 'thinking' } })
const text = (): Step => ({ kind: 'text', item: { seq: ++seq, kind: 'text', text: 'Now the composer:' } })

const shape = (shown: Shown[]) => shown.map((s) => (s.kind === 'group' ? `group(${String(s.steps.length)})` : s.kind === 'tool' ? (s.use.tool?.name ?? '') : s.kind))

describe('groupSteps', () => {
  it('folds a run of exploring calls and the thoughts between them', () => {
    expect(shape(groupSteps([text(), tool('Bash'), thought(), tool('Read'), tool('Grep'), text()]))).toEqual(['text', 'group(4)', 'text'])
  })

  it('leaves a lone call, edits, agents and a trailing thought on their own', () => {
    expect(shape(groupSteps([tool('Bash'), text(), tool('Read'), tool('Edit'), tool('Bash'), tool('Read'), thought(), tool('Agent')]))).toEqual([
      'Bash',
      'text',
      'Read',
      'Edit',
      'group(2)',
      'thinking',
      'Agent',
    ])
  })

  it('keeps calls the caller names on their own', () => {
    const steps = [tool('Bash'), tool('Read'), tool('Bash')]
    const second = steps[1]
    expect(shape(groupSteps(steps, (s) => s === second))).toEqual(['Bash', 'Read', 'Bash'])
  })

  it('groups MCP tools with the rest', () => {
    expect(shape(groupSteps([tool('mcp__plugin_x_playwright__browser_click'), tool('Bash')]))).toEqual(['group(2)'])
  })
})

describe('groupSummary', () => {
  it('counts each kind of step', () => {
    expect(groupSummary([tool('Bash'), tool('Bash'), tool('Read'), thought(), tool('Grep'), tool('mcp__a__b')])).toBe('Ran 2 commands, read a file, searched once, used a tool')
    expect(groupSummary([tool('Read'), tool('Read')])).toBe('Read 2 files')
    expect(groupSummary([tool('WebFetch'), tool('WebSearch')])).toBe('Looked up the web 2 times')
  })
})
