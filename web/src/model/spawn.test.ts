import { describe, expect, it } from 'vitest'
import type { SessionView } from '../types'
import { directorySuggestions, formProblem, spawnDefaults } from './spawn'

const s = (dir: string, root: string, kind: 'repo' | 'dir' | 'group' = 'repo', archived = false): SessionView => ({
  id: dir,
  name: dir,
  tool: 'claude',
  group: '',
  directory: dir,
  status: 'idle',
  running: true,
  archived,
  project: { key: kind + root, name: root, root, kind },
})

describe('directorySuggestions', () => {
  it('ranks project roots and frequent directories first, without duplicates', () => {
    const got = directorySuggestions(
      [s('/src/api', '/src/api'), s('/src/api/internal', '/src/api'), s('/src/web', '/src/web'), s('/tmp/x', '/tmp/x', 'dir', true)],
      [{ path: 'ops', directory: '/srv/ops' }],
    )
    expect(got[0]).toBe('/src/api')
    expect(new Set(got).size).toBe(got.length)
    expect(got).toContain('/srv/ops')
    expect(got.at(-1)).toBe('/tmp/x')
  })
})

describe('spawnDefaults', () => {
  it.each([
    [null, '', ''],
    [{ key: 'repo:/src/api', name: 'api', root: '/src/api', kind: 'repo' as const }, '', '/src/api'],
    [{ key: 'group:ops', name: 'ops', root: '/srv/ops', kind: 'group' as const }, 'ops', '/srv/ops'],
    [{ key: 'group:bare', name: 'bare', kind: 'group' as const }, 'bare', ''],
  ])('%j', (project, group, directory) => {
    const r = spawnDefaults(project, 'claude', true)
    expect([r.group, r.directory, r.tool, r.worktree]).toEqual([group, directory, 'claude', true])
  })
})

describe('formProblem', () => {
  const base = spawnDefaults(null, 'claude', false)
  it.each([
    ['', 'Choose a directory.'],
    ['src/api', 'Use a full path'],
    ['/src/api', null],
    ['~/src', null],
  ])('directory %j', (directory, want) => {
    const got = formProblem({ ...base, directory })
    if (want === null) expect(got).toBeNull()
    else expect(got).toContain(want)
  })

  it('rejects a model with spaces', () => {
    expect(formProblem({ ...base, directory: '/x', model: 'opus 4' })).toContain('single words')
  })
})
