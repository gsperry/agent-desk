import { describe, expect, it } from 'vitest'
import { langForPath, resolveLang } from './highlight'

describe('resolveLang', () => {
  it.each([
    ['go', 'go'],
    ['TS', 'typescript'],
    ['sh', 'bash'],
    ['yml', 'yaml'],
    ['python title="x.py"', 'python'],
    ['brainfuck', null],
    ['', null],
    [undefined, null],
  ])('%s -> %s', (input, want) => {
    expect(resolveLang(input)).toBe(want)
  })
})

describe('langForPath', () => {
  it.each([
    ['/src/main.go', 'go'],
    ['web/src/App.tsx', 'tsx'],
    ['Dockerfile', 'dockerfile'],
    ['Makefile', 'makefile'],
    ['notes.txt', null],
    ['LICENSE', null],
  ])('%s -> %s', (path, want) => {
    expect(langForPath(path)).toBe(want)
  })
})
