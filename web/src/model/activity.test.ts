import { describe, expect, it } from 'vitest'
import type { Item } from '../types'
import { formatTokens, liveActivity } from './activity'

let seq = 0
const item = (kind: Item['kind'], extra: Partial<Item> = {}): Item => ({ seq: ++seq, kind, ...extra })

describe('liveActivity', () => {
  it('starts at the last prompt and counts tokens once per message', () => {
    const items = [
      item('user', { time: '2026-11-12T10:00:00Z', text: 'old' }),
      item('text', { messageId: 'm0', meta: { usage: { outputTokens: 999 } } }),
      item('user', { time: '2026-11-12T11:00:00Z', text: 'now' }),
      item('thinking', { messageId: 'm1', meta: { usage: { outputTokens: 300 } } }),
      item('text', { messageId: 'm1', meta: { usage: { outputTokens: 400 } } }),
      item('text', { messageId: 'm2', meta: { usage: { outputTokens: 1200 } } }),
    ]
    const a = liveActivity(items)
    expect(a.startedAt).toBe(Date.parse('2026-11-12T11:00:00Z'))
    expect(a.outputTokens).toBe(1600)
    expect(a.step).toBe('Writing')
  })

  it('names a tool call still running', () => {
    const a = liveActivity([
      item('user', { time: '2026-11-12T11:00:00Z' }),
      item('tool_use', { tool: { id: 't1', name: 'Bash', input: { command: 'ls', description: 'List files' } } }),
      item('tool_result', { result: { toolUseId: 't1', content: 'a' } }),
      item('tool_use', { tool: { id: 't2', name: 'Read', input: { file_path: '/repo/main.go' } } }),
    ])
    expect(a.step).toBe('Read: /repo/main.go')
  })

  it('has no step between steps, and thinks while it thinks', () => {
    expect(
      liveActivity([item('user'), item('tool_use', { tool: { id: 't1', name: 'Bash', input: {} } }), item('tool_result', { result: { toolUseId: 't1', content: '' } })]).step,
    ).toBe('')
    expect(liveActivity([item('user'), item('thinking')]).step).toBe('Thinking')
  })

  it('copes with no prompt and no timestamps', () => {
    const a = liveActivity([item('text')])
    expect(a.startedAt).toBeUndefined()
    expect(a.outputTokens).toBe(0)
    expect(liveActivity([]).step).toBe('')
  })
})

describe('formatTokens', () => {
  it.each([
    [950, '950'],
    [12_345, '12.3k'],
  ])('%d', (n, want) => {
    expect(formatTokens(n)).toBe(want)
  })
})
