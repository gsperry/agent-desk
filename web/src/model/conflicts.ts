import type { SessionView } from '../types'

export interface Conflict {
  path: string
  others: string[]
}

export type Conflicts = Record<string, Conflict[]>

// conflictTitle describes a session's shared files for a tooltip, naming
// the other sessions.
export function conflictTitle(list: Conflict[], sessions: SessionView[]): string {
  const name = (id: string) => sessions.find((s) => s.id === id)?.name ?? id
  return list.map((c) => `${c.path}: also written by ${c.others.map(name).join(', ')}`).join('\n')
}
