export type Theme = 'system' | 'light' | 'dark'
export type Width = 'narrow' | 'medium' | 'wide' | 'full'
export type Open = 'open' | 'closed'
export type EditsOpen = Open | 'auto'

export type ToolCategory = 'edits' | 'bash' | 'read' | 'agents' | 'other'

export interface ViewPrefs {
  theme: Theme
  fontSize: number
  width: Width
  collapse: {
    edits: EditsOpen
    bash: Open
    read: Open
    agents: Open
    other: Open
    thinking: Open
  }
}

export const defaultViewPrefs: ViewPrefs = {
  theme: 'system',
  fontSize: 15,
  width: 'medium',
  collapse: { edits: 'auto', bash: 'closed', read: 'closed', agents: 'closed', other: 'closed', thinking: 'closed' },
}

export const fontSizes = { min: 12, max: 20 }

export const widths: Record<Width, string> = {
  narrow: '38rem',
  medium: '46rem',
  wide: '60rem',
  full: '100%',
}

const key = 'agent-desk:view'

type KV = Pick<Storage, 'getItem' | 'setItem'>

function oneOf<T extends string>(v: unknown, allowed: readonly T[], fallback: T): T {
  return typeof v === 'string' && (allowed as readonly string[]).includes(v) ? (v as T) : fallback
}

// sanitize accepts stored prefs from any earlier version, keeping valid
// fields and defaulting the rest.
export function sanitize(raw: unknown): ViewPrefs {
  const d = defaultViewPrefs
  const r = (typeof raw === 'object' && raw !== null ? raw : {}) as Record<string, unknown>
  const c = (typeof r.collapse === 'object' && r.collapse !== null ? r.collapse : {}) as Record<string, unknown>
  const open = ['open', 'closed'] as const
  const size = typeof r.fontSize === 'number' && Number.isFinite(r.fontSize) ? Math.round(r.fontSize) : d.fontSize
  return {
    theme: oneOf(r.theme, ['system', 'light', 'dark'] as const, d.theme),
    fontSize: Math.min(fontSizes.max, Math.max(fontSizes.min, size)),
    width: oneOf(r.width, ['narrow', 'medium', 'wide', 'full'] as const, d.width),
    collapse: {
      edits: oneOf(c.edits, ['open', 'closed', 'auto'] as const, d.collapse.edits),
      bash: oneOf(c.bash, open, d.collapse.bash),
      read: oneOf(c.read, open, d.collapse.read),
      agents: oneOf(c.agents, open, d.collapse.agents),
      other: oneOf(c.other, open, d.collapse.other),
      thinking: oneOf(c.thinking, open, d.collapse.thinking),
    },
  }
}

export function loadViewPrefs(store: KV): ViewPrefs {
  try {
    const raw = store.getItem(key)
    return sanitize(raw ? JSON.parse(raw) : {})
  } catch {
    return defaultViewPrefs
  }
}

export function saveViewPrefs(store: KV, prefs: ViewPrefs): void {
  store.setItem(key, JSON.stringify(prefs))
}

export function toolCategory(name: string): ToolCategory {
  switch (name) {
    case 'Edit':
    case 'MultiEdit':
    case 'Write':
    case 'NotebookEdit':
      return 'edits'
    case 'Bash':
      return 'bash'
    case 'Read':
    case 'Grep':
    case 'Glob':
      return 'read'
    case 'Task':
    case 'Agent':
      return 'agents'
    default:
      return 'other'
  }
}

// Edits with up to this many diff rows open in auto mode.
export const autoOpenRows = 30

// initiallyOpen decides whether a tool call starts expanded. A failed call
// always opens, so errors are never hidden behind a click.
export function initiallyOpen(name: string, prefs: ViewPrefs, call: { failed: boolean; diffRows: number }): boolean {
  if (call.failed) return true
  const cat = toolCategory(name)
  if (cat === 'edits') {
    const mode = prefs.collapse.edits
    return mode === 'open' || (mode === 'auto' && call.diffRows > 0 && call.diffRows <= autoOpenRows)
  }
  return prefs.collapse[cat] === 'open'
}
