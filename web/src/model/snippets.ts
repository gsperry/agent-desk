export interface Snippet {
  name: string
  text: string
}

export const defaultSnippets: Snippet[] = [
  { name: 'continue', text: 'Continue.' },
  { name: 'tests', text: 'Run the full test suite and fix anything that fails before moving on.' },
  { name: 'pr', text: 'Commit the work on {branch} and open a pull request with a clear description.' },
  { name: 'status', text: 'Give me a short status: what is done, what is left, and anything you need from me.' },
]

const key = 'agent-desk:snippets'

type KV = Pick<Storage, 'getItem' | 'setItem'>

export function loadSnippets(store: KV): Snippet[] {
  try {
    const raw = store.getItem(key)
    if (!raw) return defaultSnippets
    const v: unknown = JSON.parse(raw)
    if (!Array.isArray(v)) return defaultSnippets
    return v.filter((s): s is Snippet => typeof s === 'object' && s !== null && typeof (s as Snippet).name === 'string' && typeof (s as Snippet).text === 'string')
  } catch {
    return defaultSnippets
  }
}

export function saveSnippets(store: KV, list: Snippet[]): void {
  store.setItem(key, JSON.stringify(list))
}

export interface SnippetVars {
  session?: string
  branch?: string
  project?: string
  directory?: string
}

// expandSnippet fills {session}, {branch}, {project} and {directory};
// unknown or empty variables are left as written so nothing silently
// disappears from the message.
export function expandSnippet(text: string, vars: SnippetVars): string {
  return text.replace(/\{(session|branch|project|directory)\}/g, (all, name: keyof SnippetVars) => vars[name] || all)
}

// slashSnippet finds the snippet a composer holding just "/name" refers to.
export function slashSnippet(composer: string, list: Snippet[]): Snippet | null {
  const m = /^\/([\w-]+)$/.exec(composer.trim())
  if (!m) return null
  return list.find((s) => s.name === m[1]) ?? null
}

// validName is a snippet name usable after a slash.
export function validName(name: string): boolean {
  return /^[\w-]{1,40}$/.test(name)
}
