import { describe, expect, it } from 'vitest'
import { defaultSnippets, expandSnippet, loadSnippets, saveSnippets, slashSnippet, validName } from './snippets'

describe('snippets', () => {
  it('expands known variables and leaves the rest', () => {
    expect(expandSnippet('Open a PR for {branch} in {project} ({unknown}) {session}', { branch: 'am/x', project: 'api', session: '' })).toBe(
      'Open a PR for am/x in api ({unknown}) {session}',
    )
  })

  it.each([
    ['/tests', 'tests'],
    ['  /pr ', 'pr'],
    ['/nope', null],
    ['/tests now', null],
    ['tests', null],
  ])('slashSnippet(%j)', (text, want) => {
    expect(slashSnippet(text, defaultSnippets)?.name ?? null).toBe(want)
  })

  it('round-trips, filters junk, and falls back to defaults', () => {
    const m = new Map<string, string>()
    const store = { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) }
    expect(loadSnippets(store)).toBe(defaultSnippets)
    saveSnippets(store, [{ name: 'a', text: 'b' }])
    expect(loadSnippets(store)).toEqual([{ name: 'a', text: 'b' }])
    m.set('agent-desk:snippets', JSON.stringify([{ name: 'ok', text: 't' }, { name: 3 }, 'x']))
    expect(loadSnippets(store)).toEqual([{ name: 'ok', text: 't' }])
    m.set('agent-desk:snippets', '{bad')
    expect(loadSnippets(store)).toBe(defaultSnippets)
  })

  it.each([
    ['tests', true],
    ['open-pr_2', true],
    ['has space', false],
    ['', false],
  ])('validName(%j) is %s', (name, want) => {
    expect(validName(name)).toBe(want)
  })
})
