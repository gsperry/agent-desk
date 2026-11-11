import { describe, expect, it } from 'vitest'
import type { SessionView } from '../types'
import type { Transition } from './attention'
import { applyRules, loadRules, newRule, ruleMatches, saveRules, type Rule } from './rules'

const session = (over: Partial<SessionView> = {}): SessionView =>
  ({
    id: 's1',
    name: 'fix-login',
    tool: 'claude',
    group: 'web',
    directory: '/src/web',
    branch: 'feat-x',
    status: 'finished',
    running: true,
    archived: false,
    project: { key: 'repo:/src/web', name: 'web', root: '/src/web', kind: 'repo' },
    ...over,
  })

const t = (to: string, over: Partial<SessionView> = {}): Transition => ({ session: session({ status: to, ...over }), from: 'working', to })
const rule = (over: Partial<Rule> = {}): Rule => ({ ...newRule('r1'), ...over })

describe('ruleMatches', () => {
  it.each([
    [rule(), t('finished'), true],
    [rule(), t('waiting'), false],
    [rule({ enabled: false }), t('finished'), false],
    [rule(), t('finished', { archived: true }), false],
    [rule({ match: 'LOGIN' }), t('finished'), true],
    [rule({ match: 'web' }), t('finished', { name: 'other' }), true],
    [rule({ match: 'api' }), t('finished'), false],
  ])('%#', (r, tr, want) => {
    expect(ruleMatches(r, tr)).toBe(want)
  })
})

describe('applyRules', () => {
  it('notifies, suggests one expanded reply per session, and clears stale suggestions', () => {
    const rules = [
      rule({ id: 'a', name: 'tests', prompt: 'Run the tests on {branch}.' }),
      rule({ id: 'b', name: 'second', prompt: 'ignored', notify: false }),
      rule({ id: 'c', name: 'waits', status: 'waiting', prompt: '' }),
    ]
    const out = applyRules(rules, [t('finished'), t('waiting', { id: 's2' }), t('working', { id: 's3' })])
    expect(out.notify.map((n) => n.rule.id)).toEqual(['a', 'c'])
    expect(out.suggestions).toEqual([{ ruleId: 'a', ruleName: 'tests', sessionId: 's1', status: 'finished', text: 'Run the tests on feat-x.' }])
    expect(out.cleared).toEqual(['s1', 's2', 's3'])
  })
})

describe('storage', () => {
  it('round-trips and drops malformed rules', () => {
    const m = new Map<string, string>()
    const kv = { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) }
    saveRules(kv, [rule()])
    expect(loadRules(kv)).toEqual([rule()])
    m.set('agent-desk:rules', JSON.stringify([rule(), { ...rule(), status: 'exploded' }, null]))
    expect(loadRules(kv)).toEqual([rule()])
    m.set('agent-desk:rules', 'nope')
    expect(loadRules(kv)).toEqual([])
  })
})
