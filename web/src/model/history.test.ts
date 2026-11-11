import { describe, expect, it } from 'vitest'
import type { Conversation, SessionView } from '../types'
import { conversationTitle, filterConversations, humanSize, lastPart, sessionsByConversation } from './history'

const conv = (over: Partial<Conversation>): Conversation => ({ id: 'c1', modified: '2026-11-10T00:00:00Z', size: 10, ...over })

describe('conversationTitle', () => {
  it.each([
    [conv({ title: 'Fix login', firstPrompt: 'please' }), 'Fix login'],
    [conv({ title: '  ', firstPrompt: 'please fix' }), 'please fix'],
    [conv({}), 'c1'],
  ])('%#', (c, want) => {
    expect(conversationTitle(c)).toBe(want)
  })
})

describe('filterConversations', () => {
  const list = [
    conv({ id: 'a', title: 'Fix login bug', directory: '/src/web', branch: 'main' }),
    conv({ id: 'b', firstPrompt: 'Add export', directory: '/src/api', branch: 'feat-export' }),
  ]
  it.each([
    ['', ['a', 'b']],
    ['login', ['a']],
    ['EXPORT api', ['b']],
    ['src', ['a', 'b']],
    ['login api', []],
    ['b', ['a', 'b']],
  ])('%s', (q, want) => {
    expect(filterConversations(list, q).map((c) => c.id)).toEqual(want)
  })
})

describe('sessionsByConversation', () => {
  const s = (id: string, conv: string, archived = false) => ({ id, agentSessionId: conv, archived }) as unknown as SessionView
  it('prefers a live session', () => {
    const m = sessionsByConversation([s('old', 'c1', true), s('new', 'c1'), s('x', '')])
    expect(m.get('c1')?.id).toBe('new')
    expect(m.size).toBe(1)
  })
})

describe('formatting', () => {
  it.each([
    ['/src/agent-desk', 'agent-desk'],
    ['/src/app/', 'app'],
    [undefined, ''],
  ])('lastPart %s', (d, want) => {
    expect(lastPart(d)).toBe(want)
  })
  it.each([
    [512, '512 B'],
    [2048, '2 KB'],
    [5 * 1024 * 1024, '5.0 MB'],
  ])('humanSize %d', (n, want) => {
    expect(humanSize(n)).toBe(want)
  })
})
