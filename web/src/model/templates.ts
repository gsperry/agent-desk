import type { SpawnRequest } from '../types'

// Template is a saved starting point for the new session form.
export interface Template {
  name: string
  tool: string
  group: string
  directory: string
  worktree: boolean
  prompt: string
  model: string
  effort: string
}

const key = 'agent-desk:templates'

type KV = Pick<Storage, 'getItem' | 'setItem'>

function isTemplate(v: unknown): v is Template {
  if (typeof v !== 'object' || v === null) return false
  const t = v as Record<string, unknown>
  return (
    typeof t.name === 'string' &&
    ['tool', 'group', 'directory', 'prompt', 'model', 'effort'].every((k) => typeof t[k] === 'string') &&
    typeof t.worktree === 'boolean'
  )
}

export function loadTemplates(store: KV): Template[] {
  try {
    const v: unknown = JSON.parse(store.getItem(key) ?? '[]')
    return Array.isArray(v) ? v.filter(isTemplate) : []
  } catch {
    return []
  }
}

export function saveTemplates(store: KV, list: Template[]): void {
  store.setItem(key, JSON.stringify(list))
}

// templateFrom saves a form as a template. The session name is left out,
// since two sessions should not share one.
export function templateFrom(name: string, r: SpawnRequest): Template {
  return {
    name: name.trim(),
    tool: r.tool,
    group: r.group,
    directory: r.directory.trim(),
    worktree: r.worktree,
    prompt: r.prompt,
    model: r.model.trim(),
    effort: r.effort.trim(),
  }
}

// upsertTemplate replaces a template of the same name, or adds it.
export function upsertTemplate(list: Template[], t: Template): Template[] {
  const i = list.findIndex((x) => x.name === t.name)
  if (i < 0) return [...list, t].sort((a, b) => a.name.localeCompare(b.name))
  return list.map((x, j) => (j === i ? t : x))
}

// applyTemplate fills the form from a template. A template without a
// directory or group keeps the form's, so a template made generic still
// works from a project's ＋ button.
export function applyTemplate(r: SpawnRequest, t: Template): SpawnRequest {
  return {
    ...r,
    tool: t.tool || r.tool,
    group: t.group || r.group,
    directory: t.directory || r.directory,
    worktree: t.worktree,
    prompt: t.prompt,
    model: t.model,
    effort: t.effort,
  }
}
