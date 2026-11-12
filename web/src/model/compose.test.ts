import { describe, expect, it } from 'vitest'
import type { SessionView } from '../types'
import { blockedReason, enterAction, nativeNewline, type EnterKey } from './compose'

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

  it('lets a waiting session take a reply once its screen shows no question', () => {
    expect(blockedReason(session('waiting'), true, false, true)).toContain('waiting on a question')
    expect(blockedReason(session('waiting'), true, false, undefined)).toContain('waiting on a question')
    expect(blockedReason(session('waiting'), true, false, false)).toBeNull()
  })

  it('blocks a session that is not running', () => {
    expect(blockedReason(session('idle', false), true, false)).toContain('Revive')
  })
})

describe('enterAction', () => {
  const k = (mods: Partial<EnterKey> = {}, key = 'Enter'): EnterKey => ({ key, metaKey: false, ctrlKey: false, shiftKey: false, altKey: false, ...mods })
  it.each([
    ['Enter sends', k(), false, 'send'],
    ['Shift+Enter adds a line', k({ shiftKey: true }), false, 'newline'],
    ['Cmd+Enter adds a line', k({ metaKey: true }), false, 'newline'],
    ['Ctrl+Enter adds a line', k({ ctrlKey: true }), false, 'newline'],
    ['Option+Enter adds a line', k({ altKey: true }), false, 'newline'],
    ['Enter on a touch screen adds a line', k(), true, 'newline'],
    ['Enter while composing is left alone', k({ isComposing: true }), false, null],
    ['other keys are left alone', k({ metaKey: true }, 'k'), false, null],
  ] as const)('%s', (_name, e, touch, want) => {
    expect(enterAction(e, touch)).toBe(want)
  })

  it('lets the browser insert Shift+Enter and touch Enter', () => {
    expect(nativeNewline(k({ shiftKey: true }))).toBe(true)
    expect(nativeNewline(k(), true)).toBe(true)
    expect(nativeNewline(k({ metaKey: true }))).toBe(false)
    expect(nativeNewline(k({ metaKey: true }), true)).toBe(false)
  })
})
