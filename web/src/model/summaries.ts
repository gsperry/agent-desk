// SummarySettings turns catch-up summaries on. They run the user's Claude
// Code CLI and spend tokens, so they start off. The server keeps them and
// refuses summaries while they are off.
export interface SummarySettings {
  enabled: boolean
  model: string
}

export const defaultSummaries: SummarySettings = { enabled: false, model: 'haiku' }

// legacyKey is where the page kept the settings before the server did.
const legacyKey = 'agent-desk:summaries'

// takeLegacySummaries returns settings the page kept in this browser
// before the server kept them, and forgets them, or null when there are
// none.
export function takeLegacySummaries(store: Pick<Storage, 'getItem' | 'removeItem'>): SummarySettings | null {
  const raw = store.getItem(legacyKey)
  if (raw === null) return null
  store.removeItem(legacyKey)
  return parseSummaries(raw)
}

// parseSummaries reads settings from JSON, tolerating junk.
export function parseSummaries(raw: string): SummarySettings {
  try {
    const v = JSON.parse(raw) as unknown
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

// costLine says what a summary used: model, tokens and reported cost.
export function costLine(r: { model: string; inputTokens: number; outputTokens: number; costUSD: number; cached?: boolean }): string {
  const tokens = `${r.inputTokens.toLocaleString('en-US')} in, ${r.outputTokens.toLocaleString('en-US')} out`
  const cost = r.costUSD > 0 ? ` · $${r.costUSD.toFixed(r.costUSD < 0.01 ? 4 : 2)}` : ''
  return `${r.model} · ${tokens} tokens${cost}${r.cached ? ' · from cache, no new tokens' : ''}`
}
