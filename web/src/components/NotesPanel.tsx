import { useState } from 'react'
import type { Notebook } from '../model/pins'

interface Props {
  notebook: Notebook
  onChange: (nb: Notebook) => void
  onClose: () => void
}

// NotesPanel holds the reader's note and pins for one conversation. Both
// stay in this browser.
export function NotesPanel({ notebook, onChange, onClose }: Props) {
  const [missing, setMissing] = useState<string | null>(null)
  const show = (key: string) => {
    const el = document.querySelector(`[data-pin="${CSS.escape(key)}"]`)
    if (!el) {
      setMissing(key)
      return
    }
    setMissing(null)
    el.scrollIntoView({ block: 'center' })
    el.classList.add('pin-flash')
    setTimeout(() => {
      el.classList.remove('pin-flash')
    }, 1500)
  }
  return (
    <aside className="agents-panel notes-panel" aria-label="Notes">
      <header className="agents-head">
        <strong>Notes and pins</strong>
        <button type="button" className="link-button" onClick={onClose}>
          Close
        </button>
      </header>
      <div className="notes-body">
        <label className="field">
          <span>Note</span>
          <textarea
            rows={6}
            value={notebook.note}
            placeholder="Anything to remember about this conversation. Kept in this browser."
            onChange={(e) => {
              onChange({ ...notebook, note: e.target.value })
            }}
          />
        </label>
        <h3 className="notes-pins-title">Pins</h3>
        {notebook.pins.length === 0 && <p className="muted">Pin a prompt or reply with the pin button beside it.</p>}
        <ul className="pin-list">
          {notebook.pins.map((p) => (
            <li key={p.key}>
              <button
                type="button"
                className="pin-row"
                onClick={() => {
                  show(p.key)
                }}
              >
                <span className="muted pin-who">{p.kind === 'user' ? 'You' : 'Claude'}</span>
                <span className="pin-excerpt">{p.excerpt || '(empty)'}</span>
              </button>
              {missing === p.key && <p className="muted pin-missing">Not in the loaded part of the transcript. Scroll up to load earlier history.</p>}
              <button
                type="button"
                className="link-button"
                aria-label={`Unpin ${p.excerpt}`}
                onClick={() => {
                  onChange({ ...notebook, pins: notebook.pins.filter((x) => x.key !== p.key) })
                }}
              >
                Unpin
              </button>
            </li>
          ))}
        </ul>
      </div>
    </aside>
  )
}
