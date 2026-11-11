import { describe, expect, it } from 'vitest'
import { defaultViewPrefs, initiallyOpen, loadViewPrefs, sanitize, saveViewPrefs, toolCategory, type ViewPrefs } from './viewprefs'

describe('sanitize', () => {
  it('defaults everything for junk', () => {
    expect(sanitize(null)).toEqual(defaultViewPrefs)
    expect(sanitize('x')).toEqual(defaultViewPrefs)
  })

  it('keeps valid fields and clamps the font size', () => {
    const p = sanitize({ theme: 'dark', fontSize: 99, width: 'wide', collapse: { bash: 'open', edits: 'bogus' } })
    expect(p.theme).toBe('dark')
    expect(p.fontSize).toBe(20)
    expect(p.width).toBe('wide')
    expect(p.collapse.bash).toBe('open')
    expect(p.collapse.edits).toBe('auto')
    expect(sanitize({ fontSize: 3 }).fontSize).toBe(12)
    expect(sanitize({ fontSize: Number.NaN }).fontSize).toBe(15)
  })
})

describe('load and save', () => {
  it('round-trips and survives bad JSON', () => {
    const m = new Map<string, string>()
    const store = { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) }
    const p: ViewPrefs = { ...defaultViewPrefs, theme: 'light', fontSize: 17 }
    saveViewPrefs(store, p)
    expect(loadViewPrefs(store)).toEqual(p)
    m.set('agent-desk:view', '{nope')
    expect(loadViewPrefs(store)).toEqual(defaultViewPrefs)
  })
})

describe('initiallyOpen', () => {
  const prefs = (collapse: Partial<ViewPrefs['collapse']>): ViewPrefs => ({ ...defaultViewPrefs, collapse: { ...defaultViewPrefs.collapse, ...collapse } })
  it.each([
    ['Edit', {}, 10, false, true],
    ['Edit', {}, 31, false, false],
    ['Edit', {}, 0, false, false],
    ['Edit', { edits: 'open' as const }, 500, false, true],
    ['Edit', { edits: 'closed' as const }, 5, false, false],
    ['Bash', {}, 0, false, false],
    ['Bash', { bash: 'open' as const }, 0, false, true],
    ['Read', { read: 'open' as const }, 0, false, true],
    ['Agent', {}, 0, false, false],
    ['mcp__x__y', { other: 'open' as const }, 0, false, true],
    ['Bash', {}, 0, true, true],
  ])('%s with %j, %i rows, failed=%s opens: %s', (name, collapse, diffRows, failed, want) => {
    expect(initiallyOpen(name, prefs(collapse), { failed, diffRows })).toBe(want)
  })

  it.each([
    ['Write', 'edits'],
    ['Grep', 'read'],
    ['Task', 'agents'],
    ['WebFetch', 'other'],
  ])('%s is in %s', (name, cat) => {
    expect(toolCategory(name)).toBe(cat)
  })
})
