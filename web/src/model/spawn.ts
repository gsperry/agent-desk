import type { GroupView, Project, SessionView, SpawnRequest } from '../types'

// directorySuggestions lists directories the new session form offers:
// project roots, group directories and session directories, most used first.
export function directorySuggestions(sessions: SessionView[], groups: GroupView[]): string[] {
  const count = new Map<string, number>()
  const add = (d: string | undefined, weight: number) => {
    if (!d) return
    count.set(d, (count.get(d) ?? 0) + weight)
  }
  for (const s of sessions) {
    if (s.project.kind !== 'group') add(s.project.root, s.archived ? 0.5 : 2)
    add(s.directory, s.archived ? 0.5 : 1)
  }
  for (const g of groups) add(g.directory, 1)
  return [...count.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).map(([d]) => d)
}

// spawnDefaults prefills the form from where it was opened: a project group
// gives its directory, an agent-manager group its path and directory.
export function spawnDefaults(project: Project | null, tool: string, worktree: boolean): SpawnRequest {
  const base: SpawnRequest = { name: '', tool, group: '', directory: '', worktree, prompt: '', model: '', effort: '' }
  if (!project) return base
  if (project.kind === 'group') return { ...base, group: project.name, directory: project.root ?? '' }
  return { ...base, directory: project.root ?? '' }
}

// formProblem is the first thing the form must fix before it can submit.
export function formProblem(r: SpawnRequest): string | null {
  const d = r.directory.trim()
  if (!d) return 'Choose a directory.'
  if (!d.startsWith('/') && !d.startsWith('~/')) return 'Use a full path, starting with / or ~/.'
  if (/\s/.test(r.model.trim()) || /\s/.test(r.effort.trim())) return 'Model and effort are single words.'
  return null
}
