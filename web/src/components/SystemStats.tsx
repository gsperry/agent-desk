import { useSyncExternalStore } from 'react'
import { statRows, type Level } from '../model/stats'
import type { SystemStats as Stats } from '../types'

interface Props {
  subscribe: (l: () => void) => () => void
  get: () => Stats | null
}

const levelWords: Record<Level, string> = { ok: '', warn: 'High', critical: 'Critical' }

// SystemStats shows the computer's load, memory, swap and free disk in the
// sidebar, so many agents building at once can be seen running it short.
// A row past its limit turns amber or red and carries a mark, so the
// warning does not rest on color alone.
export function SystemStats({ subscribe, get }: Props) {
  const stats = useSyncExternalStore(subscribe, get)
  if (!stats?.supported) return null
  const rows = statRows(stats)
  if (rows.length === 0) return null
  return (
    <section className="sys-stats" aria-label="System">
      {rows.map((r) => {
        // The row has no box of its own (it shares the block's grid), so
        // each cell carries the details.
        const title = r.level === 'ok' ? r.detail : `${levelWords[r.level]}: ${r.detail}`
        return (
          <div key={r.key} className={`sys-row sys-${r.level}`}>
            <span className="sys-label" title={title}>
              {r.label}
            </span>
            <span className="sys-bar" title={title} aria-hidden>
              <span style={{ width: `${String(Math.round(r.fill * 100))}%` }} />
            </span>
            <span className="sys-value" title={title}>
              {r.level !== 'ok' && (
                <span className="sys-mark" role="img" aria-label={levelWords[r.level]}>
                  ⚠{' '}
                </span>
              )}
              {r.value}
            </span>
          </div>
        )
      })}
    </section>
  )
}
