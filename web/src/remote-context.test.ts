import { describe, expect, it } from 'vitest'
import { can, localInfo, type Need, type RemoteInfo } from './remote-context'

const needs: Need[] = ['answer', 'reply', 'manage', 'local']

describe('can', () => {
  it.each<[RemoteInfo, boolean[]]>([
    [localInfo, [true, true, true, true]],
    [{ remote: true, level: 'answer' }, [true, false, false, false]],
    [{ remote: true, level: 'reply' }, [true, true, false, false]],
    [{ remote: true, level: 'manage' }, [true, true, true, false]],
  ])('%j', (info, want) => {
    expect(needs.map((n) => can(info, n))).toEqual(want)
  })
})
