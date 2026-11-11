import { describe, expect, it } from 'vitest'
import { parsePathRef, toolPathRef } from './paths'

describe('parsePathRef', () => {
  it.each([
    ['internal/server/server.go', { path: 'internal/server/server.go' }],
    ['internal/server/server.go:42', { path: 'internal/server/server.go', line: 42 }],
    ['web/src/App.tsx:10:5', { path: 'web/src/App.tsx', line: 10 }],
    ['/etc/hosts', { path: '/etc/hosts' }],
    ['~/Projects/x/README.md', { path: '~/Projects/x/README.md' }],
    ['./scripts/dev.sh', { path: './scripts/dev.sh' }],
    ['../go.mod', { path: '../go.mod' }],
    ['main.go', { path: 'main.go' }],
    ['package.json:3', { path: 'package.json', line: 3 }],
    ['.github/workflows/ci.yml', { path: '.github/workflows/ci.yml' }],
  ])('%s is a path', (text, want) => {
    expect(parsePathRef(text)).toEqual(want)
  })

  it.each([
    ['https://example.com/a.js'],
    ['1.2.3'],
    ['v0.48.1'],
    ['and/or'],
    ['src/'],
    ['fmt.Println'],
    ['agent-manager'],
    ['go test ./...'],
    ['x'],
    ['a/b c.go'],
  ])('%s is not a path', (text) => {
    expect(parsePathRef(text)).toBeNull()
  })
})

describe('toolPathRef', () => {
  it.each([
    [{ file_path: '/a.go' }, undefined, { path: '/a.go' }],
    [{ file_path: '/a.go', offset: 120 }, undefined, { path: '/a.go', line: 120 }],
    [{ file_path: '/a.go', offset: 120 }, 7, { path: '/a.go', line: 7 }],
    [{ notebook_path: '/n.ipynb' }, undefined, { path: '/n.ipynb' }],
    [{}, undefined, null],
  ])('%j', (input, hunk, want) => {
    expect(toolPathRef(input, hunk)).toEqual(want)
  })
})
