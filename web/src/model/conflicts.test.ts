import { describe, expect, it } from 'vitest'
import type { SessionView } from '../types'
import { conflictTitle } from './conflicts'

const s = (id: string, name: string) => ({ id, name }) as SessionView

describe('conflictTitle', () => {
  it('names the other sessions per file', () => {
    expect(conflictTitle([{ path: '/a.go', others: ['b', 'zz'] }], [s('b', 'web-login')])).toBe('/a.go: also written by web-login, zz')
  })
})
