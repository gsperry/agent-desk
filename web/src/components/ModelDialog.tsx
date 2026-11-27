import { useEffect, useRef, useState } from 'react'
import { effortFor, pick } from '../model/modelchoice'
import type { ModelOptions, ModelResult } from '../types'
import { Dialog } from './Dialog'

// ModelControl is what a session offers for changing its model and effort.
export interface ModelControl {
  options: () => Promise<{ data?: ModelOptions; error?: string }>
  apply: (model: string, effort: string) => Promise<{ data?: ModelResult; error?: string }>
}

interface Props {
  control: ModelControl
  onClose: () => void
}

// ModelDialog changes the session's model and effort for this session only.
// It reads the choices from Claude Code's own picker when it opens, so the
// list is whatever this Claude Code offers, never a list kept here.
export function ModelDialog({ control, onClose }: Props) {
  const [options, setOptions] = useState<ModelOptions | null>(null)
  const [model, setModel] = useState('')
  const [effort, setEffort] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [pending, setPending] = useState(false)
  // Reading the choices types into the session, so it runs once even where
  // React runs an effect twice.
  const started = useRef(false)
  const alive = useRef(true)

  useEffect(() => {
    alive.current = true
    if (!started.current) {
      started.current = true
      void control.options().then((r) => {
        if (!alive.current) return
        if (r.error || !r.data) {
          setError(r.error ?? 'Claude Code offered no models.')
          return
        }
        setOptions(r.data)
        const start = pick(r.data)
        setModel(start.model)
        setEffort(start.effort)
      })
    }
    return () => {
      alive.current = false
    }
  }, [control])

  const chosen = options?.models.find((m) => m.label === model)
  const apply = () => {
    setBusy(true)
    setError(null)
    void control.apply(model, effort).then((r) => {
      if (!alive.current) return
      setBusy(false)
      if (r.error) {
        setError(r.error)
        return
      }
      if (r.data?.pending) setPending(true)
      else onClose()
    })
  }

  return (
    <Dialog
      title="Model and effort"
      onClose={onClose}
      footer={
        pending ? (
          <button type="button" className="send" onClick={onClose} autoFocus>
            Done
          </button>
        ) : (
          <>
            <button type="button" className="secondary" onClick={onClose}>
              Cancel
            </button>
            <button type="button" className="send" disabled={busy || !chosen} onClick={apply}>
              {busy ? 'Applying…' : 'Apply'}
            </button>
          </>
        )
      }
    >
      {pending ? (
        <p>Claude Code is asking you to confirm the switch, because the conversation is cached for the current model. Answer it in the session, where it is waiting.</p>
      ) : !options && !error ? (
        <p className="muted">Reading the choices from Claude Code…</p>
      ) : (
        options && (
          <>
            <div className="model-choices" role="radiogroup" aria-label="Model">
              {options.models.map((m) => (
                <label key={m.label} className="model-choice">
                  <input
                    type="radio"
                    name="model"
                    checked={m.label === model}
                    disabled={busy}
                    onChange={() => {
                      setModel(m.label)
                      setEffort(effortFor(m, effort))
                    }}
                  />
                  <span className="model-choice-label">
                    {m.label}
                    {m.current && <span className="muted"> · current</span>}
                  </span>
                  {m.description && <span className="muted model-choice-desc">{m.description}</span>}
                </label>
              ))}
            </div>
            {chosen && (
              <div className="setting">
                <span className="setting-label">Effort</span>
                <div className="segmented" role="radiogroup" aria-label="Effort">
                  {chosen.efforts.map((e) => (
                    <button
                      key={e}
                      type="button"
                      role="radio"
                      aria-checked={e === effort}
                      className={e === effort ? 'active' : undefined}
                      disabled={busy}
                      onClick={() => {
                        setEffort(e)
                      }}
                    >
                      {e}
                      {e === chosen.defaultEffort ? ' (default)' : ''}
                    </button>
                  ))}
                </div>
              </div>
            )}
            <p className="muted setting-note">
              This changes the session only; your Claude Code defaults stay as they are. While the conversation is cached for the current model, switching makes the next response slower and use more tokens, and Claude Code asks you to confirm.
            </p>
          </>
        )
      )}
      {error && <p className="form-error">{error}</p>}
    </Dialog>
  )
}
