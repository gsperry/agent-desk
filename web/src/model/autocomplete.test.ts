import { describe, expect, it } from 'vitest'
import type { SlashCommand } from '../types'
import { completion, slashQuery, suggest } from './autocomplete'

describe('slashQuery', () => {
  it.each([
    ['/', 1, ''],
    ['/comp', 5, 'comp'],
    ['/figma:ins', 10, 'figma:ins'],
    ['/compact now', 12, null],
    ['/comp', 3, null],
    ['hello /comp', 11, null],
    ['', 0, null],
    ['/comp\n', 6, null],
  ])('%j at %d', (text, caret, want) => {
    expect(slashQuery(text, caret)).toBe(want)
  })
})

const cmds: SlashCommand[] = [
  { name: 'compact', kind: 'builtin', description: 'Free up context' },
  { name: 'context', kind: 'builtin', description: 'Context grid' },
  { name: 'clear', kind: 'builtin', description: 'New session', aliases: ['reset', 'new'] },
  { name: 'figma:inspect', kind: 'command', source: 'plugin:figma', description: 'Inspect' },
  { name: 'triage', kind: 'skill', source: 'user', description: 'Sort issues' },
  { name: 'aws-compute', kind: 'skill', source: 'user', description: 'EC2' },
]
const snippets = [{ name: 'continue', text: 'Continue.' }]

describe('suggest', () => {
  it('ranks prefixes first, snippets before commands', () => {
    expect(suggest('co', snippets, cmds).map((s) => s.name)).toEqual(['continue', 'compact', 'context', 'aws-compute'])
  })
  it('puts everyday built-ins first within a match group', () => {
    const more: SlashCommand[] = [...cmds, { name: 'cd', kind: 'builtin' }, { name: 'chrome', kind: 'builtin' }]
    expect(suggest('c', [], more).map((s) => s.name).slice(0, 4)).toEqual(['compact', 'clear', 'context', 'cd'])
  })
  it('matches a namespaced part and anywhere', () => {
    expect(suggest('insp', snippets, cmds).map((s) => s.name)).toEqual(['figma:inspect'])
    expect(suggest('riag', snippets, cmds).map((s) => s.name)).toEqual(['triage'])
  })
  it('matches aliases', () => {
    expect(suggest('reset', snippets, cmds).map((s) => s.name)).toEqual(['clear'])
  })
  it('lists everything for a bare slash, capped', () => {
    expect(suggest('', snippets, cmds, 3)).toHaveLength(3)
  })
  it('returns nothing without a match', () => {
    expect(suggest('zzz', snippets, cmds)).toEqual([])
  })
})

describe('completion', () => {
  it('inserts a command with a space, or a snippet expanded', () => {
    expect(completion({ name: 'compact', kind: 'builtin', description: '' })).toBe('/compact ')
    expect(completion({ name: 'continue', kind: 'snippet', description: '' }, 'Continue.')).toBe('Continue.')
  })
})
