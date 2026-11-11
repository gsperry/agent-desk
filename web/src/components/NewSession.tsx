import { useEffect, useState } from 'react'
import { formProblem } from '../model/spawn'
import type { SessionView, SpawnOptions, SpawnRequest } from '../types'
import { Dialog } from './Dialog'

interface Props {
  initial: SpawnRequest
  directories: string[]
  loadOptions: () => Promise<{ data?: SpawnOptions; error?: string }>
  spawn: (r: SpawnRequest) => Promise<{ data?: SessionView; error?: string }>
  onStarted: (id: string) => void
  onClose: () => void
}

export function NewSession({ initial, directories, loadOptions, spawn, onStarted, onClose }: Props) {
  const [form, setForm] = useState<SpawnRequest>(initial)
  const [options, setOptions] = useState<SpawnOptions | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [advanced, setAdvanced] = useState(false)

  useEffect(() => {
    let live = true
    void loadOptions().then((r) => {
      if (!live) return
      if (r.data) {
        const o = r.data
        setOptions(o)
        setForm((f) => ({ ...f, tool: f.tool || o.defaultTool, worktree: f.worktree || o.worktreeDefault }))
      } else setError(r.error ?? 'Could not read agent-manager settings.')
    })
    return () => {
      live = false
    }
  }, [loadOptions])

  const set = (p: Partial<SpawnRequest>) => {
    setForm((f) => ({ ...f, ...p }))
  }
  const problem = formProblem(form)

  const submit = () => {
    if (problem || busy) return
    setBusy(true)
    setError(null)
    void spawn(form).then((r) => {
      setBusy(false)
      if (r.data) onStarted(r.data.id)
      else setError(r.error ?? 'The session did not start.')
    })
  }

  return (
    <Dialog
      title="New session"
      onClose={busy ? () => undefined : onClose}
      footer={
        <>
          {busy && <span className="muted dialog-note">Starting. The first launch of a CLI can take up to two minutes.</span>}
          <button type="button" className="secondary" onClick={onClose} disabled={busy}>
            Cancel
          </button>
          <button type="button" className="send" onClick={submit} disabled={busy || problem !== null || !options}>
            {busy ? 'Starting…' : 'Start'}
          </button>
        </>
      }
    >
      <form
        className="form"
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <label className="field">
          <span>Directory</span>
          <input
            list="directories"
            value={form.directory}
            placeholder="/path/to/project"
            autoFocus={!form.directory}
            onChange={(e) => {
              set({ directory: e.target.value })
            }}
          />
          <datalist id="directories">
            {directories.map((d) => (
              <option key={d} value={d} />
            ))}
          </datalist>
        </label>
        <label className="field">
          <span>Name</span>
          <input
            value={form.name}
            placeholder="Left empty, the agent names it"
            autoFocus={Boolean(form.directory)}
            onChange={(e) => {
              set({ name: e.target.value })
            }}
          />
        </label>
        <div className="field-row">
          <label className="field">
            <span>CLI</span>
            <select
              value={form.tool}
              onChange={(e) => {
                set({ tool: e.target.value })
              }}
            >
              {(options?.tools ?? [form.tool]).map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
          </label>
          <label className="field">
            <span>Group</span>
            <select
              value={form.group}
              onChange={(e) => {
                set({ group: e.target.value })
              }}
            >
              <option value="">None</option>
              {(options?.groups ?? []).map((g) => (
                <option key={g.path} value={g.path}>
                  {g.path}
                </option>
              ))}
            </select>
          </label>
        </div>
        <label className="check">
          <input
            type="checkbox"
            checked={form.worktree}
            onChange={(e) => {
              set({ worktree: e.target.checked })
            }}
          />{' '}
          Own git worktree and branch
        </label>
        <label className="field">
          <span>First prompt</span>
          <textarea
            rows={4}
            value={form.prompt}
            placeholder="Optional. The agent starts on it right away."
            onChange={(e) => {
              set({ prompt: e.target.value })
            }}
          />
        </label>
        <button
          type="button"
          className="link-button"
          onClick={() => {
            setAdvanced(!advanced)
          }}
        >
          {advanced ? '▾' : '▸'} Model and effort
        </button>
        {advanced && (
          <div className="field-row">
            <label className="field">
              <span>Model</span>
              <input
                value={form.model}
                placeholder="CLI default"
                onChange={(e) => {
                  set({ model: e.target.value })
                }}
              />
            </label>
            <label className="field">
              <span>Effort</span>
              <input
                value={form.effort}
                placeholder="CLI default"
                onChange={(e) => {
                  set({ effort: e.target.value })
                }}
              />
            </label>
          </div>
        )}
        <button type="submit" hidden />
      </form>
      {problem && form.directory && <p className="form-error">{problem}</p>}
      {error && <p className="form-error">{error}</p>}
    </Dialog>
  )
}
