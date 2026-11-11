import type { Project, SessionView } from '../types'

// Lower sorts first: what needs the human, then what is busy, then the rest.
const statusRank: Record<string, number> = {
  waiting: 0,
  errored: 1,
  finished: 2,
  working: 3,
  starting: 4,
  idle: 5,
  dead: 6,
}

export const attentionStatuses = new Set(['waiting', 'errored', 'finished'])

export function rank(status: string): number {
  return statusRank[status] ?? 7
}

export interface SidebarGroup {
  project: Project
  sessions: SessionView[]
  // attention counts sessions in this group that need the human.
  attention: number
}

export interface SidebarOptions {
  showArchived: boolean
}

// groupSessions files sessions under their project. Groups that need
// attention come first, then groups with live sessions, then by name.
export function groupSessions(sessions: SessionView[], opts: SidebarOptions): SidebarGroup[] {
  const byKey = new Map<string, SidebarGroup>()
  for (const s of sessions) {
    if (s.archived && !opts.showArchived) continue
    let g = byKey.get(s.project.key)
    if (!g) {
      g = { project: s.project, sessions: [], attention: 0 }
      byKey.set(s.project.key, g)
    }
    g.sessions.push(s)
    if (attentionStatuses.has(s.status)) g.attention++
  }
  const groups = [...byKey.values()]
  for (const g of groups) {
    g.sessions.sort((a, b) => rank(a.status) - rank(b.status) || a.name.localeCompare(b.name))
  }
  const live = (g: SidebarGroup) => g.sessions.some((s) => s.running)
  groups.sort(
    (a, b) =>
      Number(b.attention > 0) - Number(a.attention > 0) ||
      Number(live(b)) - Number(live(a)) ||
      a.project.name.localeCompare(b.project.name),
  )
  return groups
}

export function archivedCount(sessions: SessionView[]): number {
  return sessions.filter((s) => s.archived).length
}
