import { describe, expect, it } from 'vitest'
import type { Conversation, SessionView, SetupReport } from '../types'
import { initialChoice, installHint, markWelcomed, outsideConversations, sameChoice, toggleCLI, welcomed } from './welcome'

function memory(init: Record<string, string> = {}) {
  const m = new Map(Object.entries(init))
  return { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) }
}

const report = (p: Partial<SetupReport>): SetupReport => ({ programs: [], clis: ['claude', 'opencode', 'codex'], shown: [], defaultTool: '', ...p })

describe('welcomed', () => {
  it('is false on a first run and true once marked', () => {
    const kv = memory()
    expect(welcomed(kv)).toBe(false)
    markWelcomed(kv)
    expect(welcomed(kv)).toBe(true)
  })
  it('counts earlier use of Agent Desk', () => {
    expect(welcomed(memory({ 'agent-desk:seen': '{}' }))).toBe(true)
    expect(welcomed(memory({ 'agent-desk:view': '{}' }))).toBe(true)
    expect(welcomed(memory({ unrelated: '1' }))).toBe(false)
  })
})

describe('CLI choice', () => {
  it.each([
    ['stored default', report({ shown: ['claude', 'codex'], defaultTool: 'codex' }), { shown: ['claude', 'codex'], defaultTool: 'codex' }],
    ['no default stored', report({ shown: ['opencode', 'codex'] }), { shown: ['opencode', 'codex'], defaultTool: 'opencode' }],
    ['hidden default', report({ shown: ['claude'], defaultTool: 'codex' }), { shown: ['claude'], defaultTool: 'claude' }],
    ['all hidden', report({}), { shown: ['claude'], defaultTool: 'claude' }],
    ['no CLIs known', report({ clis: [] }), { shown: [], defaultTool: '' }],
  ])('%s', (_, r, want) => {
    expect(initialChoice(r)).toEqual(want)
  })

  it('toggles in agent-manager order and keeps a valid default', () => {
    const all = ['claude', 'opencode', 'codex']
    let c = { shown: ['codex'], defaultTool: 'codex' }
    c = toggleCLI(c, all, 'claude', true)
    expect(c).toEqual({ shown: ['claude', 'codex'], defaultTool: 'codex' })
    c = toggleCLI(c, all, 'codex', false)
    expect(c).toEqual({ shown: ['claude'], defaultTool: 'claude' })
    c = toggleCLI(c, all, 'claude', false)
    expect(c).toEqual({ shown: [], defaultTool: '' })
  })

  it('compares choices', () => {
    expect(sameChoice({ shown: ['a', 'b'], defaultTool: 'a' }, { shown: ['a', 'b'], defaultTool: 'a' })).toBe(true)
    expect(sameChoice({ shown: ['a', 'b'], defaultTool: 'a' }, { shown: ['a', 'b'], defaultTool: 'b' })).toBe(false)
    expect(sameChoice({ shown: ['a'], defaultTool: 'a' }, { shown: ['a', 'b'], defaultTool: 'a' })).toBe(false)
  })
})

describe('outsideConversations', () => {
  const conv = (id: string, modified: string): Conversation => ({ id, modified, size: 1 })
  it('keeps the newest ones no session holds', () => {
    const list = [conv('a', '2026-11-01T00:00:00Z'), conv('b', '2026-11-03T00:00:00Z'), conv('c', '2026-11-02T00:00:00Z'), conv('d', '2026-11-04T00:00:00Z')]
    const owners = new Map([['d', { id: 's1' } as SessionView]])
    expect(outsideConversations(list, owners, 2).map((c) => c.id)).toEqual(['b', 'c'])
  })
})

describe('installHint', () => {
  it('names a way to install each program', () => {
    expect(installHint('agent-manager').command).toBe('brew install agent-manager')
    expect(installHint('tmux').command).toBe('brew install tmux')
    expect(installHint('claude').link).toMatch(/^https:/)
  })
})
