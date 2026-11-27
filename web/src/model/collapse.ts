// Collapsed groups are remembered between visits, by project key.
const key = 'agent-desk:collapsed-groups'

type KV = Pick<Storage, 'getItem' | 'setItem'>

export function loadCollapsed(store: KV): Set<string> {
  try {
    const raw: unknown = JSON.parse(store.getItem(key) ?? '[]')
    return new Set(Array.isArray(raw) ? raw.filter((k): k is string => typeof k === 'string') : [])
  } catch {
    return new Set()
  }
}

export function saveCollapsed(store: KV, collapsed: ReadonlySet<string>): void {
  store.setItem(key, JSON.stringify([...collapsed].sort()))
}

// toggled returns the set with key folded or unfolded.
export function toggled(collapsed: ReadonlySet<string>, k: string): Set<string> {
  const next = new Set(collapsed)
  if (!next.delete(k)) next.add(k)
  return next
}

// visibleSessions is what a group shows: everything, or while it is
// folded just the selected session, so the one being read never vanishes.
export function visibleSessions<T extends { id: string }>(sessions: T[], isCollapsed: boolean, selected: string | null): T[] {
  return isCollapsed ? sessions.filter((s) => s.id === selected) : sessions
}
