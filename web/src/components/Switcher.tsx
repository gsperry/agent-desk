import { useMemo, useState } from 'react'
import { filterSessions, needsAttention, type Acks } from '../model/attention'
import type { SessionView } from '../types'
import { glyph } from './status'

interface Props {
  sessions: SessionView[]
  acks: Acks
  onPick: (id: string) => void
  onClose: () => void
}

// Switcher is the Cmd+K session picker.
export function Switcher({ sessions, acks, onPick, onClose }: Props) {
  const [query, setQuery] = useState('')
  const [index, setIndex] = useState(0)
  const results = useMemo(() => filterSessions(sessions.filter((s) => !s.archived), query, acks).slice(0, 12), [sessions, query, acks])
  const active = Math.min(index, Math.max(results.length - 1, 0))

  const pick = (s: SessionView | undefined) => {
    if (!s) return
    onPick(s.id)
    onClose()
  }

  return (
    <div className="overlay" onMouseDown={onClose}>
      <div
        className="switcher"
        role="dialog"
        aria-label="Switch session"
        onMouseDown={(e) => {
          e.stopPropagation()
        }}
      >
        <input
          autoFocus
          value={query}
          placeholder="Go to session…"
          aria-label="Search sessions"
          onChange={(e) => {
            setQuery(e.target.value)
            setIndex(0)
          }}
          onKeyDown={(e) => {
            if (e.key === 'ArrowDown') {
              e.preventDefault()
              setIndex((active + 1) % Math.max(results.length, 1))
            } else if (e.key === 'ArrowUp') {
              e.preventDefault()
              setIndex((active - 1 + results.length) % Math.max(results.length, 1))
            } else if (e.key === 'Enter') {
              e.preventDefault()
              pick(results[active])
            } else if (e.key === 'Escape') {
              e.preventDefault()
              onClose()
            }
          }}
        />
        <ul role="listbox" aria-label="Sessions">
          {results.map((s, i) => (
            <li key={s.id} role="option" aria-selected={i === active}>
              <button
                type="button"
                className={`switch-row status-${s.status}${i === active ? ' active' : ''}`}
                onMouseMove={() => {
                  setIndex(i)
                }}
                onClick={() => {
                  pick(s)
                }}
              >
                <span className="glyph">{glyph(s.status)}</span>
                <span className="row-name">{s.name}</span>
                <span className="muted">{s.project.name}</span>
                {needsAttention(s, acks) && <span className="chip">{s.status}</span>}
              </button>
            </li>
          ))}
          {results.length === 0 && <li className="muted pad">No matching session.</li>}
        </ul>
      </div>
    </div>
  )
}
