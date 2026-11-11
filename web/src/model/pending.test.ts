import { describe, expect, it } from 'vitest'
import type { Item } from '../types'
import { confirmWithinMs, expire, normalize, reconcile, type Pending } from './pending'

const t0 = Date.parse('2026-11-10T18:00:00Z')

function pending(text: string, state: Pending['state'] = 'sent', sentAt = t0): Pending {
  return { clientId: text + String(sentAt), sessionId: 's', text, sentAt, state }
}

function prompt(seq: number, text: string, time = '2026-11-10T18:00:02Z', extra: Partial<Item> = {}): Item {
  return { seq, kind: 'user', text, time, ...extra }
}

describe('normalize', () => {
  it.each([
    ['  hi  ', 'hi'],
    ['a\r\nb\rc', 'a\nb\nc'],
    ['x\u001b[201~y', 'x[201~y'],
    ['tab\tkept', 'tab\tkept'],
  ])('%j', (input, want) => {
    expect(normalize(input)).toBe(want)
  })
})

describe('reconcile', () => {
  it('confirms a reply when its prompt appears', () => {
    expect(reconcile([pending('fix it')], [prompt(1, 'fix it\n')])).toEqual([])
  })

  it('confirms against a queued prompt', () => {
    expect(reconcile([pending('also X')], [prompt(1, 'also X', undefined, { meta: { queued: true } })])).toEqual([])
  })

  it('ignores an identical prompt from before the send', () => {
    const p = [pending('again')]
    expect(reconcile(p, [prompt(1, 'again', '2026-11-10T17:50:00Z')])).toBe(p)
  })

  it('needs one record per reply when the same text is sent twice', () => {
    const p = [pending('ok', 'sent', t0), pending('ok', 'sent', t0 + 1000)]
    const left = reconcile(p, [prompt(1, 'ok')])
    expect(left).toHaveLength(1)
    expect(left[0]?.sentAt).toBe(t0 + 1000)
  })

  it('keeps failed and in-flight replies', () => {
    const p = [pending('a', 'failed'), pending('b', 'sending')]
    expect(reconcile(p, [prompt(1, 'a'), prompt(2, 'b')])).toBe(p)
  })

  it('does not match assistant text', () => {
    const p = [pending('hello')]
    expect(reconcile(p, [{ seq: 1, kind: 'text', text: 'hello' }])).toBe(p)
  })
})

describe('expire', () => {
  it('marks old sent and maybe replies unconfirmed', () => {
    const p = [pending('a', 'sent'), pending('b', 'maybe'), pending('c', 'failed'), pending('d', 'sent', t0 + confirmWithinMs)]
    const states = expire(p, t0 + confirmWithinMs + 1).map((x) => x.state)
    expect(states).toEqual(['unconfirmed', 'unconfirmed', 'failed', 'sent'])
  })

  it('returns the same array when nothing changes', () => {
    const p = [pending('a', 'sent')]
    expect(expire(p, t0 + 1)).toBe(p)
  })
})
