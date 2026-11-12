import { describe, expect, it } from 'vitest'
import type { Item } from '../types'
import { buildBlocks, buildHeader, formatDuration, toolLabel } from './transcript'

let seq = 0
function item(kind: Item['kind'], extra: Partial<Item> = {}): Item {
  return { seq: seq++, kind, ...extra }
}

describe('buildBlocks', () => {
  it('groups assistant output into a turn and pairs tool results, even late ones', () => {
    const items = [
      item('user', { text: 'do it' }),
      item('thinking', { text: '' }),
      item('tool_use', { tool: { id: 't1', name: 'Bash', input: { command: 'ls' } } }),
      item('tool_use', { tool: { id: 't2', name: 'Read', input: { file_path: '/a' } } }),
      item('tool_result', { result: { toolUseId: 't2', content: 'file' } }),
      item('text', { text: 'checking' }),
      item('tool_result', { result: { toolUseId: 't1', content: 'a b' } }),
      item('system', { label: 'turn_duration', meta: { durationMs: 4200 } }),
    ]
    const blocks = buildBlocks(items)
    expect(blocks.map((b) => b.kind)).toEqual(['user', 'turn'])
    const turn = blocks[1]
    if (turn?.kind !== 'turn') throw new Error('expected turn')
    expect(turn.steps.map((s) => s.kind)).toEqual(['thinking', 'tool', 'tool', 'text'])
    const [, bash, read] = turn.steps
    expect(bash?.kind === 'tool' && bash.result?.result?.content).toBe('a b')
    expect(read?.kind === 'tool' && read.result?.result?.content).toBe('file')
    expect(turn.durationMs).toBe(4200)
  })

  it('starts a new turn after a prompt and after turn_duration', () => {
    const blocks = buildBlocks([
      item('text', { text: 'one' }),
      item('system', { label: 'turn_duration', meta: { durationMs: 1 } }),
      item('text', { text: 'two' }),
      item('user', { text: 'next' }),
      item('text', { text: 'three' }),
    ])
    expect(blocks.map((b) => b.kind)).toEqual(['turn', 'turn', 'user', 'turn'])
  })

  it('keeps a result whose call is outside the window as an orphan', () => {
    const blocks = buildBlocks([item('tool_result', { result: { toolUseId: 'gone', content: 'x' } })])
    expect(blocks[0]?.kind === 'turn' && blocks[0].steps[0]?.kind).toBe('orphan_result')
  })

  it('merges a compact summary into its boundary', () => {
    const blocks = buildBlocks([item('compact_boundary'), item('compact_summary', { text: 'earlier' })])
    expect(blocks).toHaveLength(1)
    expect(blocks[0]?.kind === 'compact' && blocks[0].summary?.text).toBe('earlier')
  })

  it.each([
    ['hidden reminder', item('notice', { label: 'system-reminder' }), []],
    ['visible command', item('notice', { label: 'command', text: '/review' }), ['notice']],
    ['attachment', item('attachment', { label: 'environment' }), []],
    ['meta', item('meta', { label: 'ai-title' }), []],
    ['hook summary', item('system', { label: 'stop_hook_summary' }), []],
    ['api error', item('system', { label: 'api_error' }), ['system']],
    ['raw record', item('raw', { raw: '{}' }), ['raw']],
    ['peer', item('peer', { text: 'hi' }), ['peer']],
  ])('%s', (_name, it, want) => {
    expect(buildBlocks([it]).map((b) => b.kind)).toEqual(want)
  })

  it('keeps a raw block that belongs to a message inside the turn', () => {
    const blocks = buildBlocks([item('text', { text: 'a', messageId: 'm' }), item('raw', { messageId: 'm', raw: '{}' })])
    expect(blocks[0]?.kind === 'turn' && blocks[0].steps.map((s) => s.kind)).toEqual(['text', 'raw'])
  })
})

describe('buildHeader', () => {
  it('prefers the custom title and collects unique PR links', () => {
    const h = buildHeader([
      item('meta', { label: 'ai-title', meta: { aiTitle: 'Generated' } }),
      item('meta', { label: 'custom-title', meta: { customTitle: 'Mine' } }),
      item('meta', { label: 'pr-link', meta: { prUrl: 'https://x/pr/1', prNumber: 1 } }),
      item('meta', { label: 'pr-link', meta: { prUrl: 'https://x/pr/1', prNumber: 1 } }),
      item('meta', { label: 'permission-mode', meta: { permissionMode: 'auto' } }),
    ])
    expect(h).toEqual({ title: 'Mine', prLinks: [{ url: 'https://x/pr/1', number: 1, repository: undefined }], permissionMode: 'auto' })
  })
})

describe('PR links', () => {
  it.each([
    ['https://github.com/o/r/pull/1', 1],
    ['vscode://file/etc/passwd', 0],
    ['javascript:alert(1)', 0],
    ['file:///etc/hosts', 0],
    ['not a url', 0],
  ])('%s renders %i link', (prUrl, want) => {
    expect(buildHeader([item('meta', { label: 'pr-link', meta: { prUrl } })]).prLinks).toHaveLength(want)
  })
})

describe('toolLabel', () => {
  it.each([
    ['Bash with description', { name: 'Bash', input: { command: 'ls -la', description: 'List files' } }, 'Bash', 'List files'],
    ['Bash multi-line', { name: 'Bash', input: { command: 'cd x\nmake' } }, 'Bash', 'cd x'],
    ['Edit', { name: 'Edit', input: { file_path: '/a.go', old_string: 'x' } }, 'Edit', '/a.go'],
    ['Grep', { name: 'Grep', input: { pattern: 'TODO', path: 'src' } }, 'Grep', 'TODO in src'],
    ['Agent', { name: 'Agent', input: { description: 'Study prior art', prompt: 'long' } }, 'Agent', 'Study prior art'],
    ['TodoWrite', { name: 'TodoWrite', input: { todos: [{}, {}] } }, 'TodoWrite', '2 todos'],
    ['AskUserQuestion', { name: 'AskUserQuestion', input: { questions: [{ question: 'Pick?' }] } }, 'AskUserQuestion', 'Pick?'],
    ['MCP tool', { name: 'mcp__github__create_issue', input: { title: 'Bug' } }, 'github · create issue', 'Bug'],
    ['plugin MCP tool', { name: 'mcp__plugin_mcp-playwright_playwright__browser_run_code_unsafe', input: {} }, 'playwright · browser run code unsafe', ''],
    ['unknown tool', { name: 'Zap', input: { n: 1, target: 'here' } }, 'Zap', 'here'],
    ['no input', { name: 'Zap' }, 'Zap', ''],
  ])('%s', (_name, tool, wantName, wantSummary) => {
    const label = toolLabel({ seq: 0, kind: 'tool_use', tool: { id: 'x', ...tool } })
    expect(label).toEqual({ name: wantName, summary: wantSummary })
  })

  it('shows paths in the session directory relative to it', () => {
    const read = (file_path: string) => toolLabel({ seq: 0, kind: 'tool_use', tool: { id: 'x', name: 'Read', input: { file_path } } }, '/repo/app')
    expect(read('/repo/app/src/main.go').summary).toBe('src/main.go')
    expect(read('/repo/application/x.go').summary).toBe('/repo/application/x.go')
    expect(read('/etc/hosts').summary).toBe('/etc/hosts')
  })
})

describe('formatDuration', () => {
  it.each([
    [900, '1s'],
    [42_000, '42s'],
    [125_000, '2m 5s'],
    [3_780_000, '1h 3m'],
  ])('%i ms is %s', (ms, want) => {
    expect(formatDuration(ms)).toBe(want)
  })
})
