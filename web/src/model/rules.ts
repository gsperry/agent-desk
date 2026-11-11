import type { Transition } from './attention'
import { expandSnippet } from './snippets'

export type RuleStatus = 'waiting' | 'finished' | 'errored' | 'idle'

// Rule reacts when a session reaches a status: it can notify, and it can
// suggest a reply. A suggested reply is never sent without the reader
// confirming it.
export interface Rule {
  id: string
  name: string
  status: RuleStatus
  // match narrows the rule to sessions whose name, project or group
  // contains it; empty matches every session.
  match: string
  notify: boolean
  prompt: string
  enabled: boolean
}

export interface Suggestion {
  ruleId: string
  ruleName: string
  sessionId: string
  status: string
  text: string
}

export const ruleStatuses: [RuleStatus, string][] = [
  ['waiting', 'waits for you'],
  ['finished', 'finishes a turn'],
  ['errored', 'hits an error'],
  ['idle', 'goes idle'],
]

const key = 'agent-desk:rules'

type KV = Pick<Storage, 'getItem' | 'setItem'>

function isRule(v: unknown): v is Rule {
  if (typeof v !== 'object' || v === null) return false
  const r = v as Record<string, unknown>
  return (
    typeof r.id === 'string' &&
    typeof r.name === 'string' &&
    ruleStatuses.some(([s]) => s === r.status) &&
    typeof r.match === 'string' &&
    typeof r.notify === 'boolean' &&
    typeof r.prompt === 'string' &&
    typeof r.enabled === 'boolean'
  )
}

export function loadRules(store: KV): Rule[] {
  try {
    const v: unknown = JSON.parse(store.getItem(key) ?? '[]')
    return Array.isArray(v) ? v.filter(isRule) : []
  } catch {
    return []
  }
}

export function saveRules(store: KV, rules: Rule[]): void {
  store.setItem(key, JSON.stringify(rules))
}

export function newRule(id: string): Rule {
  return { id, name: 'New rule', status: 'finished', match: '', notify: true, prompt: '', enabled: true }
}

export function ruleMatches(r: Rule, t: Transition): boolean {
  if (!r.enabled || t.session.archived || t.to !== r.status) return false
  const m = r.match.trim().toLowerCase()
  if (!m) return true
  return [t.session.name, t.session.project.name, t.session.group].some((v) => v.toLowerCase().includes(m))
}

export interface RuleOutcome {
  notify: { rule: Rule; transition: Transition }[]
  suggestions: Suggestion[]
  // cleared lists sessions whose suggestion no longer fits their status.
  cleared: string[]
}

// applyRules decides what the rules do for a batch of status changes. The
// first matching rule with a prompt wins a session's suggestion.
export function applyRules(rules: Rule[], list: Transition[]): RuleOutcome {
  const out: RuleOutcome = { notify: [], suggestions: [], cleared: [] }
  for (const t of list) {
    out.cleared.push(t.session.id)
    let suggested = false
    for (const r of rules) {
      if (!ruleMatches(r, t)) continue
      if (r.notify) out.notify.push({ rule: r, transition: t })
      if (!suggested && r.prompt.trim()) {
        const s = t.session
        out.suggestions.push({
          ruleId: r.id,
          ruleName: r.name,
          sessionId: s.id,
          status: t.to,
          text: expandSnippet(r.prompt.trim(), { session: s.name, branch: s.branch, project: s.project.name, directory: s.directory }),
        })
        suggested = true
      }
    }
  }
  return out
}
