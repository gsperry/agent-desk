import { describe, expect, it } from 'vitest'
import type { SpawnRequest } from '../types'
import { applyTemplate, loadTemplates, saveTemplates, templateFrom, upsertTemplate, type Template } from './templates'

const form: SpawnRequest = { name: 'fix-x', tool: 'claude', group: 'web', directory: ' /src/web ', worktree: true, prompt: 'Review it.', model: ' opus ', effort: 'high' }

function memory(): Pick<Storage, 'getItem' | 'setItem'> {
  const m = new Map<string, string>()
  return { getItem: (k) => m.get(k) ?? null, setItem: (k, v) => void m.set(k, v) }
}

describe('templates', () => {
  it('saves a form without its session name', () => {
    const t = templateFrom(' review ', form)
    expect(t).toEqual({ name: 'review', tool: 'claude', group: 'web', directory: '/src/web', worktree: true, prompt: 'Review it.', model: 'opus', effort: 'high' })
    expect(t).not.toHaveProperty('name', 'fix-x')
  })

  it('round-trips through storage and drops malformed entries', () => {
    const kv = memory()
    const t = templateFrom('review', form)
    saveTemplates(kv, [t])
    expect(loadTemplates(kv)).toEqual([t])
    kv.setItem('agent-desk:templates', JSON.stringify([t, { name: 'bad' }, 3]))
    expect(loadTemplates(kv)).toEqual([t])
    kv.setItem('agent-desk:templates', '{not json')
    expect(loadTemplates(kv)).toEqual([])
  })

  it('replaces a template of the same name, or adds in order', () => {
    const a = templateFrom('b-second', form)
    const b = templateFrom('a-first', form)
    let list: Template[] = upsertTemplate([], a)
    list = upsertTemplate(list, b)
    expect(list.map((t) => t.name)).toEqual(['a-first', 'b-second'])
    list = upsertTemplate(list, { ...a, prompt: 'new' })
    expect(list).toHaveLength(2)
    expect(list[1]?.prompt).toBe('new')
  })

  it('keeps the form directory and group when the template has none', () => {
    const generic: Template = { ...templateFrom('g', form), directory: '', group: '', worktree: false, prompt: 'Go' }
    const start: SpawnRequest = { ...form, name: 'kept', directory: '/src/api', group: 'api' }
    expect(applyTemplate(start, generic)).toMatchObject({ name: 'kept', directory: '/src/api', group: 'api', worktree: false, prompt: 'Go' })
    expect(applyTemplate(start, templateFrom('s', form))).toMatchObject({ directory: '/src/web', group: 'web' })
  })
})
