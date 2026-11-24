import { describe, expect, it } from 'vitest'
import { desktopDictate } from './dictate'
import type { DictationEvent } from './model/dictation'

class FakeChannel {
  onmessage: (m: unknown) => void = () => undefined
}

describe('desktopDictate', () => {
  it('is absent in a browser', () => {
    expect(desktopDictate({})).toBeNull()
    expect(desktopDictate({ __TAURI__: { core: { invoke: () => Promise.resolve() } } })).toBeNull()
  })

  it('starts and stops through the app, passing only known events', async () => {
    const calls: [string, Record<string, unknown>][] = []
    const invoke = (cmd: string, args: Record<string, unknown>) => {
      calls.push([cmd, args])
      return Promise.resolve()
    }
    const d = desktopDictate({ __TAURI__: { core: { invoke, Channel: FakeChannel } } })
    expect(d).not.toBeNull()
    const got: DictationEvent[] = []
    await d?.start((e) => got.push(e))
    await d?.stop()
    expect(calls.map((c) => c[0])).toEqual(['dictate_start', 'dictate_stop'])
    const channel = calls[0]?.[1].events as FakeChannel
    channel.onmessage({ type: 'text', text: 'hi', final: false })
    channel.onmessage({ type: 'script', text: 'x' })
    expect(got).toEqual([{ type: 'text', text: 'hi', final: false }])
  })
})
