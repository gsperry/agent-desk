import type { SlashCommand } from '../types'
import type { Snippet } from './snippets'

// Suggestion is one row of the composer's slash menu.
export interface Suggestion {
  name: string
  kind: 'snippet' | SlashCommand['kind']
  description: string
  hint?: string
  source?: string
}

// slashQuery returns what follows a leading slash while the reader is still
// typing the command name: "/comp" gives "comp". Anything after a space, or
// text not starting with a slash, gives null.
export function slashQuery(text: string, caret: number): string | null {
  if (caret !== text.length && text.slice(caret).trim() !== '') return null
  const m = /^\/([\w.:-]*)$/.exec(text.slice(0, caret))
  return m ? (m[1] ?? '') : null
}

const kindOrder: Record<Suggestion['kind'], number> = { snippet: 0, builtin: 1, command: 2, skill: 3 }

// common are the built-ins people reach for most; they lead their match
// group so "/c" offers /compact before /cd.
const common = ['compact', 'clear', 'context', 'model', 'resume', 'rewind', 'init', 'review', 'config', 'status', 'help', 'memory', 'mcp', 'plugin', 'usage', 'export']

function commonRank(s: Suggestion): number {
  const i = s.kind === 'builtin' ? common.indexOf(s.name) : -1
  return i < 0 ? common.length : i
}

function rank(name: string, q: string): number {
  const n = name.toLowerCase()
  if (n === q) return 0
  if (n.startsWith(q)) return 1
  // A plugin's command matches on its own name too: "insp" finds figma:inspect.
  if (n.split(':').some((part) => part.startsWith(q))) return 2
  if (n.includes(q)) return 3
  return -1
}

interface Candidate extends Suggestion {
  aliases: string[]
}

// suggest ranks snippets and commands for a query: exact, then prefix, then
// a prefix of a namespaced part, then anywhere; ties go snippets first,
// then built-ins, commands and skills, then by name. Built-in aliases match
// as their command.
export function suggest(query: string, snippets: Snippet[], commands: SlashCommand[], limit = 8): Suggestion[] {
  const q = query.toLowerCase()
  const candidates: Candidate[] = [
    ...snippets.map((s): Candidate => ({ name: s.name, kind: 'snippet', description: s.text, aliases: [] })),
    ...commands.map(
      (c): Candidate => ({ name: c.name, kind: c.kind, description: c.description ?? '', hint: c.argumentHint || undefined, source: c.source, aliases: c.aliases ?? [] }),
    ),
  ]
  const ranked: { c: Candidate; r: number }[] = []
  for (const c of candidates) {
    const r = rankWithAliases(c.name, c.aliases, q)
    if (r >= 0) ranked.push({ c, r })
  }
  ranked.sort((a, b) => a.r - b.r || kindOrder[a.c.kind] - kindOrder[b.c.kind] || commonRank(a.c) - commonRank(b.c) || a.c.name.localeCompare(b.c.name))
  return ranked.slice(0, limit).map(({ c }) => ({ name: c.name, kind: c.kind, description: c.description, hint: c.hint, source: c.source }))
}

function rankWithAliases(name: string, aliases: string[], q: string): number {
  const own = rank(name, q)
  let best = own
  for (const a of aliases) {
    const r = rank(a, q)
    if (r >= 0 && (best < 0 || r + 1 < best)) best = r + 1
  }
  return best
}

// completion is the composer text after choosing a suggestion: a snippet's
// expanded text, or the command followed by a space for its arguments.
export function completion(s: Suggestion, expandedSnippet?: string): string {
  return s.kind === 'snippet' ? (expandedSnippet ?? '') : `/${s.name} `
}
