import { describe, expect, it, vi } from 'vitest'
import { desktopInvoke, desktopNotifier, setBadge } from './notify'

describe('desktopInvoke', () => {
  it('finds the app bridge', () => {
    const invoke = vi.fn()
    expect(desktopInvoke({ __TAURI__: { core: { invoke } } })).toBe(invoke)
  })

  it('is undefined in a browser', () => {
    expect(desktopInvoke({})).toBeUndefined()
    expect(desktopInvoke({ __TAURI__: {} })).toBeUndefined()
    expect(desktopInvoke({ __TAURI__: { core: { invoke: 'x' } } })).toBeUndefined()
  })
})

describe('desktopNotifier', () => {
  it('sends notifications through the app', async () => {
    const invoke = vi.fn().mockResolvedValue(undefined)
    const n = desktopNotifier(invoke)
    expect(n.permission()).toBe('granted')
    await expect(n.request()).resolves.toBe('granted')
    n.show('build waiting', 'Needs you', 'tag', () => undefined)
    expect(invoke).toHaveBeenCalledWith('notify', { title: 'build waiting', body: 'Needs you', session: 'tag' })
    n.show('rule fired', 'Rule: done', 'rule-a1', () => undefined)
    expect(invoke).toHaveBeenLastCalledWith('notify', { title: 'rule fired', body: 'Rule: done', session: 'a1' })
    n.show('Disk almost full', '3 GB free', 'system:disk:/', () => undefined)
    expect(invoke).toHaveBeenLastCalledWith('notify', { title: 'Disk almost full', body: '3 GB free', session: undefined })
  })

  it('swallows a failed call', async () => {
    const invoke = vi.fn().mockRejectedValue(new Error('denied'))
    desktopNotifier(invoke).show('t', 'b', 'tag', () => undefined)
    await Promise.resolve()
    expect(invoke).toHaveBeenCalledOnce()
  })
})

describe('setBadge', () => {
  it('sends a whole, non-negative count', () => {
    const invoke = vi.fn().mockResolvedValue(undefined)
    setBadge(3, invoke)
    setBadge(-1, invoke)
    setBadge(2.7, invoke)
    expect(invoke.mock.calls).toEqual([
      ['set_badge', { count: 3 }],
      ['set_badge', { count: 0 }],
      ['set_badge', { count: 2 }],
    ])
  })

  it('does nothing in a browser', () => {
    expect(() => {
      setBadge(1, undefined)
    }).not.toThrow()
  })
})
