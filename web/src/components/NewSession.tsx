import { useEffect, useState } from 'react'
import { formProblem } from '../model/spawn'
import { applyTemplate, templateFrom, type Template } from '../model/templates'
import type { SessionView, SpawnOptions, SpawnRequest } from '../types'
import { Dialog } from './Dialog'
import { isSendKey } from '../model/compose'

interface Props {
  initial: SpawnRequest
  directories: string[]
  loadOptions: () => Promise<{ data?: SpawnOptions; error?: string }>
  spawn: (r: SpawnRequest) => Promise<{ data?: SessionView; error?: string }>
  onStarted: (id: string) => void
  onClose: () => void
  // resumeTitle names the conversation being resumed.
  resumeTitle?: string
  templates: Template[]
  onSaveTemplate: (t: Template) => void
}

export function NewSession({ initial, directories, loadOptions, spawn, onStarted, onClose, resumeTitle, templates, onSaveTemplate }: Props) {
  const [form, setForm] = useState<SpawnRequest>(initial)
  const [options, setOptions] = useState<SpawnOptions | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [advanced, setAdvanced] = useState(false)
  const [templateName, setTemplateName] = useState<string | null>(null)
  const [savedAs, setSavedAs] = useState<string | null>(null)

  useEffect(() => {
    let live = true
    void loadOptions().then((r) => {
      if (!live) return
      if (r.data) {
        const o = r.data
        setOptions(o)
        setForm((f) => (f.resume ? f : { ...f, tool: f.tool || o.defaultTool, worktree: f.worktree || o.worktreeDefault }))
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
  const resuming = Boolean(form.resume)

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
      title={resuming ? 'Resume conversation' : 'New session'}
      onClose={busy ? () => undefined : onClose}
      footer={
        <>
          {!resuming && !busy && templateName === null && (
            <button
              type="button"
              className="link-button template-save"
              onClick={() => {
                setTemplateName('')
                setSavedAs(null)
              }}
            >
              {savedAs ? `Saved “${savedAs}”` : 'Save as template'}
            </button>
          )}
          {templateName !== null && (
            <span className="template-name">
              <input
                aria-label="Template name"
                placeholder="Template name"
                value={templateName}
                autoFocus
                onChange={(e) => {
                  setTemplateName(e.target.value)
                }}
                onKeyDown={(e) => {
                  if (e.key === 'Escape') {
                    e.stopPropagation()
                    setTemplateName(null)
                  }
                }}
              />
              <button
                type="button"
                className="secondary"
                disabled={templateName.trim() === ''}
                onClick={() => {
                  onSaveTemplate(templateFrom(templateName, form))
                  setSavedAs(templateName.trim())
                  setTemplateName(null)
                }}
              >
                Save
              </button>
            </span>
          )}
          {busy && <span className="muted dialog-note">Starting. The first launch of a CLI can take up to two minutes.</span>}
          <button type="button" className="secondary" onClick={onClose} disabled={busy}>
            Cancel
          </button>
          <button type="button" className="send" onClick={submit} disabled={busy || problem !== null || !options}>
            {busy ? 'Starting…' : resuming ? 'Resume' : 'Start'}
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
        {!resuming && templates.length > 0 && (
          <label className="field">
            <span>Template</span>
            <select
              defaultValue=""
              onChange={(e) => {
                const t = templates.find((x) => x.name === e.target.value)
                if (t) {
                  setForm((f) => applyTemplate(f, t))
                  if (t.model || t.effort) setAdvanced(true)
                }
              }}
            >
              <option value="">None</option>
              {templates.map((t) => (
                <option key={t.name} value={t.name}>
                  {t.name}
                </option>
              ))}
            </select>
          </label>
        )}
        {resuming && (
          <p className="muted resume-note">
            Reopens “{resumeTitle ?? form.resume}” as a new agent-manager session, in the directory it was saved in.
          </p>
        )}
        <label className="field">
          <span>Directory</span>
          <input
            list="directories"
            readOnly={resuming && Boolean(initial.directory)}
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
          {!resuming && (
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
          )}
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
        {!resuming && (
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
        )}
        <label className="field">
          <span>First prompt</span>
          <textarea
            rows={4}
            value={form.prompt}
            placeholder={resuming ? 'Optional. Sent once the conversation reopens.' : 'Optional. The agent starts on it right away.'}
            onChange={(e) => {
              set({ prompt: e.target.value })
            }}
            onKeyDown={(e) => {
              if (isSendKey(e.nativeEvent)) {
                e.preventDefault()
                submit()
              }
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
