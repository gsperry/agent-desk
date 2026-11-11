import type { Item } from '../types'

export type ActivityFilter = 'all' | 'commands' | 'files' | 'errors'

const editTools = new Set(['Edit', 'MultiEdit', 'Write', 'NotebookEdit'])

function failed(it: Item): boolean {
  if (it.kind === 'tool_result') return it.result?.isError === true
  if (it.kind === 'system') return it.label === 'api_error' || it.meta?.level === 'error'
  return it.kind === 'raw'
}

// filterItems narrows a transcript to one kind of activity. Prompts stay
// as anchors, and a tool call keeps its result (and a failing result its
// call), so every block still reads on its own.
export function filterItems(items: Item[], f: ActivityFilter): Item[] {
  if (f === 'all') return items
  const ids = new Set<string>()
  for (const it of items) {
    if (it.kind === 'tool_use' && it.tool) {
      if ((f === 'commands' && it.tool.name === 'Bash') || (f === 'files' && editTools.has(it.tool.name))) ids.add(it.tool.id)
    }
    if (f === 'errors' && it.kind === 'tool_result' && it.result?.isError) ids.add(it.result.toolUseId)
  }
  return items.filter((it) => {
    if (it.kind === 'user') return true
    if (it.kind === 'tool_use') return it.tool ? ids.has(it.tool.id) : false
    if (it.kind === 'tool_result') return it.result ? ids.has(it.result.toolUseId) : false
    return f === 'errors' && failed(it)
  })
}

const risky: [RegExp, string][] = [
  [/\brm\s+(?:-[a-zA-Z]*r[a-zA-Z]*f|-[a-zA-Z]*f[a-zA-Z]*r|-r\s+-f|-f\s+-r)\b/, 'recursive forced delete'],
  [/\bgit\s+push\b[^\n;&|]*\s(?:--force(?:-with-lease)?|-f)\b/, 'force push'],
  [/\bgit\s+reset\s+--hard\b/, 'hard reset discards work'],
  [/\bgit\s+clean\s+-[a-zA-Z]*f/, 'deletes untracked files'],
  [/\bgit\s+branch\s+-D\b/, 'force-deletes a branch'],
  [/(?:^|[\s;&|])sudo\s/, 'runs as root'],
  [/\bch(?:mod|own)\s+-R\b/, 'recursive permission change'],
  [/\b(?:drop|truncate)\s+(?:table|database|schema)\b/i, 'destroys data'],
  [/\bmkfs\b|\bdd\s+[^\n]*\bof=/, 'writes a device or filesystem'],
  [/\b(?:curl|wget)\b[^\n|]*\|\s*(?:sudo\s+)?(?:ba|z)?sh\b/, 'pipes a download into a shell'],
]

// riskyCommand names why a shell command deserves a second look, or
// returns null.
export function riskyCommand(cmd: string): string | null {
  for (const [re, why] of risky) if (re.test(cmd)) return why
  return null
}
