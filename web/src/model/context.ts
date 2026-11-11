import type { Item } from '../types'

// Thresholds are the context sizes at which each model auto-compacted,
// learned from compaction records. Claude Code does not publish them per
// model, and they differ by model and plan.
export type Thresholds = Record<string, number>

const key = 'agent-desk:compaction'

type KV = Pick<Storage, 'getItem' | 'setItem'>

interface Usage {
  model?: string
  contextTokens?: number
}

function usageOf(it: Item): Usage | null {
  const u = it.meta?.usage
  return typeof u === 'object' && u !== null ? u : null
}

// learnThresholds raises each model's threshold to the largest automatic
// compaction seen after a turn on that model.
export function learnThresholds(th: Thresholds, items: Item[]): Thresholds {
  let model: string | undefined
  let next: Thresholds | null = null
  for (const it of items) {
    const u = usageOf(it)
    if (u?.model) model = u.model
    if (it.kind !== 'compact_boundary' || !model) continue
    const pre = it.meta?.preTokens
    if (it.meta?.trigger !== 'auto' || typeof pre !== 'number') continue
    if (pre > ((next ?? th)[model] ?? 0)) {
      next ??= { ...th }
      next[model] = pre
    }
  }
  return next ?? th
}

export interface ContextState {
  tokens: number
  model?: string
  limit?: number
  ratio?: number
}

// contextState is the size of the latest turn's context, and how close it
// is to the learned compaction point for that model.
export function contextState(items: Item[], th: Thresholds): ContextState | null {
  for (let i = items.length - 1; i >= 0; i--) {
    const it = items[i]
    if (!it) continue
    if (it.kind === 'compact_boundary') return null
    const u = usageOf(it)
    if (typeof u?.contextTokens !== 'number') continue
    const limit = u.model ? th[u.model] : undefined
    return { tokens: u.contextTokens, model: u.model, limit, ratio: limit ? Math.min(u.contextTokens / limit, 1) : undefined }
  }
  return null
}

export function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(n >= 10_000_000 ? 0 : 1)}M`
  if (n >= 1000) return `${String(Math.round(n / 1000))}k`
  return String(Math.round(n))
}

export function loadThresholds(store: KV): Thresholds {
  try {
    const v: unknown = JSON.parse(store.getItem(key) ?? '{}')
    return typeof v === 'object' && v !== null ? (v as Thresholds) : {}
  } catch {
    return {}
  }
}

export function saveThresholds(store: KV, th: Thresholds): void {
  store.setItem(key, JSON.stringify(th))
}
