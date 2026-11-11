import { describe, expect, it } from 'vitest'
import type { SessionView } from '../types'
import { blockedReason, isSendKey } from './compose'

const session = (status: string, running = true): SessionView => ({
  id: 's',
  name: 's',
  tool: 'claude',
  group: '',
  directory: '/x',
  status,
  running,
  archived: false,
  project: { key: 'k', name: 'n', kind: 'dir' },
})

describe('blockedReason', () => {
  it.each([
    ['idle', true, false, null],
    ['working', true, false, null],
    ['finished', true, false, null],
    ['waiting', true, false, 'waiting on a question'],
    ['starting', true, false, 'still starting'],
    ['idle', false, false, 'Not connected'],
    ['idle', true, true, 'inside an agent-manager session'],
  ])('%s connected=%s inside=%s', (status, connected, inside, want) => {
    const got = blockedReason(session(status), connected, inside)
    if (want === null) expect(got).toBeNull()
    else expect(got).toContain(want)
  })

  it('blocks a session that is not running', () => {
    expect(blockedReason(session('idle', false), true, false)).toContain('Revive')
  })
})

describe('isSendKey', () => {
  const k = (key: string, metaKey = false, ctrlKey = false, isComposing = false) => ({ key, metaKey, ctrlKey, isComposing })
  it.each([
    ['Cmd+Enter', k('Enter', true), true],
    ['Ctrl+Enter', k('Enter', false, true), true],
    ['plain Enter', k('Enter'), false],
    ['Cmd+Enter while composing', k('Enter', true, false, true), false],
    ['Cmd+K', k('k', true), false],
  ])('%s', (_name, e, want) => {
    expect(isSendKey(e)).toBe(want)
  })
})
