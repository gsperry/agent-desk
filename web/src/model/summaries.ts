// SummarySettings turns catch-up summaries on. They run the user's Claude
// Code CLI and spend tokens, so they start off.
export interface SummarySettings {
  enabled: boolean
  model: string
}

export const defaultSummaries: SummarySettings = { enabled: false, model: 'haiku' }

const key = 'agent-desk:summaries'

type KV = Pick<Storage, 'getItem' | 'setItem'>

export function loadSummaries(store: KV): SummarySettings {
  try {
    const v = JSON.parse(store.getItem(key) ?? 'null') as unknown
    if (typeof v !== 'object' || v === null) return defaultSummaries
    const o = v as Record<string, unknown>
    return {
      enabled: o.enabled === true,
      model: typeof o.model === 'string' && o.model.trim() ? o.model.trim() : defaultSummaries.model,
    }
  } catch {
    return defaultSummaries
  }
}

export function saveSummaries(store: KV, s: SummarySettings): void {
  store.setItem(key, JSON.stringify(s))
}

// costLine says what a summary used: model, tokens and reported cost.
export function costLine(r: { model: string; inputTokens: number; outputTokens: number; costUSD: number; cached?: boolean }): string {
  const tokens = `${r.inputTokens.toLocaleString('en-US')} in, ${r.outputTokens.toLocaleString('en-US')} out`
  const cost = r.costUSD > 0 ? ` · $${r.costUSD.toFixed(r.costUSD < 0.01 ? 4 : 2)}` : ''
  return `${r.model} · ${tokens} tokens${cost}${r.cached ? ' · from cache, no new tokens' : ''}`
}
