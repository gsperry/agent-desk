import { Fragment, useEffect, useState } from 'react'
import { patchRows, type DiffRow } from '../model/diff'
import { composeReview, loadComments, saveComments, type ReviewComment } from '../model/review'
import type { ChangedFile, Changes } from '../types'

interface Props {
  sessionId: string
  canSend: string | null
  load: (sessionId: string, mode: Changes['mode']) => Promise<{ data?: Changes; error?: string }>
  onSend: (text: string) => void
  onClose: () => void
}

// ChangesPanel shows a session's git changes and collects line comments
// into one review prompt.
export function ChangesPanel({ sessionId, canSend, load, onSend, onClose }: Props) {
  const [mode, setMode] = useState<Changes['mode']>('uncommitted')
  const [result, setResult] = useState<{ mode: string; tick: number; data?: Changes; error?: string } | null>(null)
  const [tick, setTick] = useState(0)
  const [selected, setSelected] = useState<string | null>(null)
  const [comments, setComments] = useState<ReviewComment[]>(() => loadComments(localStorage, sessionId))
  const [draft, setDraft] = useState<{ file: string; row: DiffRow; text: string } | null>(null)

  useEffect(() => {
    let live = true
    void load(sessionId, mode).then((r) => {
      if (live) setResult({ mode, tick, ...r })
    })
    return () => {
      live = false
    }
  }, [load, sessionId, mode, tick])

  useEffect(() => {
    saveComments(localStorage, sessionId, comments)
  }, [sessionId, comments])

  const loading = result?.mode !== mode || result.tick !== tick
  const data = loading ? undefined : result.data
  const file = data?.files.find((f) => f.path === selected) ?? data?.files[0]

  const addComment = () => {
    if (!draft || !draft.text.trim()) return
    const line = draft.row.type === 'del' ? (draft.row.oldNo ?? 0) : (draft.row.newNo ?? draft.row.oldNo ?? 0)
    setComments((cs) => [
      ...cs,
      { id: `${draft.file}:${String(line)}:${String(Date.now())}`, file: draft.file, line, side: draft.row.type === 'del' ? 'old' : 'new', code: draft.row.text, text: draft.text },
    ])
    setDraft(null)
  }

  const send = () => {
    if (comments.length === 0 || canSend) return
    onSend(composeReview(comments))
    setComments([])
  }

  return (
    <aside className="agents-panel changes-panel" aria-label="Changes">
      <header className="agents-head">
        <strong>Changes</strong>
        <div className="segmented" role="radiogroup" aria-label="Compare">
          {(
            [
              ['uncommitted', 'Uncommitted'],
              ['branch', 'Branch'],
            ] as const
          ).map(([m, label]) => (
            <button
              key={m}
              type="button"
              role="radio"
              aria-checked={mode === m}
              className={mode === m ? 'active' : undefined}
              onClick={() => {
                setMode(m)
                setSelected(null)
              }}
            >
              {label}
            </button>
          ))}
        </div>
        <button
          type="button"
          className="link-button"
          onClick={() => {
            setTick((n) => n + 1)
          }}
        >
          Refresh
        </button>
        <button type="button" className="link-button" onClick={onClose}>
          Close
        </button>
      </header>
      {loading && <p className="muted pad">Reading changes…</p>}
      {!loading && result.error && <p className="form-error pad">{result.error}</p>}
      {data && (
        <div className="changes-body">
          <div className="changes-meta muted">
            {data.branch}
            {data.mode === 'branch' && data.base ? ` since ${data.base.slice(0, 8)}` : ' vs HEAD'} · {data.files.length} file{data.files.length === 1 ? '' : 's'}
            {data.truncated ? ' · diff truncated' : ''}
          </div>
          <ul className="changed-files">
            {data.files.map((f) => (
              <li key={f.path}>
                <button
                  type="button"
                  className={f.path === file?.path ? 'changed-file active' : 'changed-file'}
                  onClick={() => {
                    setSelected(f.path)
                  }}
                >
                  <span className={`file-status status-${f.status}`}>{f.status[0]?.toUpperCase()}</span>
                  <span className="changed-path" title={f.oldPath ? `${f.oldPath} → ${f.path}` : f.path}>
                    {f.path}
                  </span>
                  <span className="tool-stats">
                    <span className="add">+{f.added}</span> <span className="del">−{f.removed}</span>
                  </span>
                </button>
              </li>
            ))}
            {data.files.length === 0 && <li className="muted pad">No changes.</li>}
          </ul>
          {file && (
            <FileDiff
              file={file}
              comments={comments.filter((c) => c.file === file.path)}
              draft={draft?.file === file.path ? draft : null}
              onPick={(row) => {
                setDraft({ file: file.path, row, text: '' })
              }}
              onDraft={(text) => {
                setDraft((d) => (d ? { ...d, text } : d))
              }}
              onAdd={addComment}
              onCancel={() => {
                setDraft(null)
              }}
              onRemove={(id) => {
                setComments((cs) => cs.filter((c) => c.id !== id))
              }}
            />
          )}
        </div>
      )}
      <footer className="changes-foot">
        <span className="muted">{comments.length === 0 ? 'Click a line to comment.' : `${String(comments.length)} comment${comments.length === 1 ? '' : 's'}`}</span>
        {canSend && comments.length > 0 && <span className="muted">{canSend}</span>}
        <button type="button" className="send" disabled={comments.length === 0 || canSend !== null} onClick={send}>
          Send review
        </button>
      </footer>
    </aside>
  )
}

interface FileDiffProps {
  file: ChangedFile
  comments: ReviewComment[]
  draft: { row: DiffRow; text: string } | null
  onPick: (row: DiffRow) => void
  onDraft: (text: string) => void
  onAdd: () => void
  onCancel: () => void
  onRemove: (id: string) => void
}

function rowLine(r: DiffRow): { line: number; side: 'old' | 'new' } {
  return r.type === 'del' ? { line: r.oldNo ?? 0, side: 'old' } : { line: r.newNo ?? r.oldNo ?? 0, side: 'new' }
}

function FileDiff({ file, comments, draft, onPick, onDraft, onAdd, onCancel, onRemove }: FileDiffProps) {
  if (file.status === 'untracked') return <p className="muted pad">Untracked file. Its content is not part of the diff.</p>
  if (file.binary) return <p className="muted pad">Binary file.</p>
  const rows = patchRows(file.hunks)
  return (
    <div className="diff review-diff">
      <table>
        <tbody>
          {rows.map((r, i) => {
            if (r.type === 'gap') {
              return (
                <tr key={i} className="diff-gap">
                  <td colSpan={4}>⋯</td>
                </tr>
              )
            }
            const at = rowLine(r)
            const here = comments.filter((c) => c.line === at.line && c.side === at.side)
            const drafting = draft !== null && rowLine(draft.row).line === at.line && rowLine(draft.row).side === at.side
            return (
              <Fragment key={i}>
                <tr
                  className={`diff-${r.type} diff-clickable`}
                  onClick={() => {
                    onPick(r)
                  }}
                >
                  <td className="diff-no">{r.oldNo ?? ''}</td>
                  <td className="diff-no">{r.newNo ?? ''}</td>
                  <td className="diff-sign">{r.type === 'add' ? '+' : r.type === 'del' ? '−' : ' '}</td>
                  <td className="diff-text">{r.text}</td>
                </tr>
                {here.map((c) => (
                  <tr key={c.id} className="review-comment">
                    <td colSpan={4}>
                      <div className="review-note">
                        <span>{c.text}</span>
                        <button
                          type="button"
                          className="link-button"
                          onClick={() => {
                            onRemove(c.id)
                          }}
                        >
                          Remove
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
                {drafting && (
                  <tr className="review-draft">
                    <td colSpan={4}>
                      <textarea
                        autoFocus
                        rows={2}
                        aria-label="Review comment"
                        value={draft.text}
                        onChange={(e) => {
                          onDraft(e.target.value)
                        }}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) onAdd()
                          if (e.key === 'Escape') onCancel()
                        }}
                      />
                      <div className="review-draft-actions">
                        <button type="button" className="link-button" onClick={onCancel}>
                          Cancel
                        </button>
                        <button type="button" className="secondary" onClick={onAdd}>
                          Add comment
                        </button>
                      </div>
                    </td>
                  </tr>
                )}
              </Fragment>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
