import type { GroupView, Project, SessionView } from '../types'
import { needsAttention, type Acks } from './attention'

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
  acks?: Acks
  // onlyAttention keeps just the sessions that need the human.
  onlyAttention?: boolean
  // groups are agent-manager groups; empty ones still get a heading so a
  // session can be started in them.
  groups?: GroupView[]
}

// groupSessions files sessions under their project. Groups that need
// attention come first, then groups with live sessions, then by name.
export function groupSessions(sessions: SessionView[], opts: SidebarOptions): SidebarGroup[] {
  const byKey = new Map<string, SidebarGroup>()
  const acks = opts.acks ?? {}
  for (const s of sessions) {
    if (s.archived && !opts.showArchived) continue
    const attention = needsAttention(s, acks)
    if (opts.onlyAttention && !attention) continue
    let g = byKey.get(s.project.key)
    if (!g) {
      g = { project: s.project, sessions: [], attention: 0 }
      byKey.set(s.project.key, g)
    }
    g.sessions.push(s)
    if (attention) g.attention++
  }
  if (!opts.onlyAttention) {
    for (const g of opts.groups ?? []) {
      const key = 'group:' + g.path
      if (!byKey.has(key)) byKey.set(key, { project: { key, name: g.path, root: g.directory, kind: 'group' }, sessions: [], attention: 0 })
    }
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
