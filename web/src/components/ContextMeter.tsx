import { useEffect, useMemo, useState } from 'react'
import { contextState, formatTokens, learnThresholds, loadThresholds, saveThresholds } from '../model/context'
import type { Item } from '../types'

// ContextMeter shows how full the session's context is. The limit is the
// size at which this model last auto-compacted, learned from transcripts.
export function ContextMeter({ items }: { items: Item[] }) {
  const [stored] = useState(() => loadThresholds(localStorage))
  const th = useMemo(() => learnThresholds(stored, items), [stored, items])

  useEffect(() => {
    if (th !== stored) saveThresholds(localStorage, { ...loadThresholds(localStorage), ...th })
  }, [th, stored])

  const state = useMemo(() => contextState(items, th), [items, th])
  if (!state) return null
  const pct = state.ratio === undefined ? null : Math.round(state.ratio * 100)
  const title =
    state.limit === undefined
      ? `${state.tokens.toLocaleString()} tokens in context. No auto-compaction seen yet for ${state.model ?? 'this model'}, so there is no limit to compare with.`
      : `${state.tokens.toLocaleString()} of about ${state.limit.toLocaleString()} tokens before ${state.model ?? 'this model'} auto-compacts (learned from earlier compactions).`
  return (
    <span className={pct !== null && pct >= 80 ? 'context-meter high' : 'context-meter'} title={title} aria-label="Context used">
      <span className="context-label">Context {formatTokens(state.tokens)}</span>
      {pct !== null && (
        <>
          <span className="context-bar" aria-hidden>
            <span style={{ width: `${String(pct)}%` }} />
          </span>
          <span className="context-pct">{pct}%</span>
        </>
      )}
    </span>
  )
}
