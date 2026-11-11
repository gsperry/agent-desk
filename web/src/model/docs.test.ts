import { describe, expect, it } from 'vitest'
import type { Item, Page } from '../types'
import { applyMessage, emptyData, prependOlder } from './docs'

const it_ = (seq: number): Item => ({ seq, kind: 'text', text: String(seq) })
const page = (seqs: number[], hasMore = false): Page => ({ sessionId: 's', status: 'ok', items: seqs.map(it_), hasMore })
const seqs = (d: typeof emptyData) => d.docs.s?.items.map((i) => i.seq)

describe('docs', () => {
  it('replaces on a page, appends only new items', () => {
    let d = applyMessage(emptyData, { type: 'transcript.page', page: page([1, 2]) })
    d = applyMessage(d, { type: 'transcript.append', sessionId: 's', items: [it_(2), it_(3)] })
    expect(seqs(d)).toEqual([1, 2, 3])
    d = applyMessage(d, { type: 'transcript.page', page: page([10]) })
    expect(seqs(d)).toEqual([10])
  })

  it('returns the same object when an append adds nothing', () => {
    const d = applyMessage(emptyData, { type: 'transcript.page', page: page([1]) })
    expect(applyMessage(d, { type: 'transcript.append', sessionId: 's', items: [it_(1)] })).toBe(d)
  })

  it('prepends older pages without duplicates', () => {
    let d = applyMessage(emptyData, { type: 'transcript.page', page: page([5, 6], true) })
    d = prependOlder(d, page([3, 4, 5], false))
    expect(seqs(d)).toEqual([3, 4, 5, 6])
    expect(d.docs.s?.hasMore).toBe(false)
  })

  it('stores session state', () => {
    const state = { managerAwake: true, insideSession: false, sessions: [] }
    expect(applyMessage(emptyData, { type: 'sessions', state }).sessions).toBe(state)
  })
})
