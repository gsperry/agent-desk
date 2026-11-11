import { useState } from 'react'
import { Dialog } from './Dialog'

interface Props {
  directories: string[]
  create: (path: string, directory: string) => Promise<string | null>
  onClose: () => void
}

export function NewGroup({ directories, create, onClose }: Props) {
  const [path, setPath] = useState('')
  const [directory, setDirectory] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const submit = () => {
    if (!path.trim() || busy) return
    setBusy(true)
    void create(path, directory).then((err) => {
      setBusy(false)
      if (err) setError(err)
      else onClose()
    })
  }
  return (
    <Dialog
      title="New group"
      onClose={onClose}
      footer={
        <>
          <button type="button" className="secondary" onClick={onClose}>
            Cancel
          </button>
          <button type="button" className="send" onClick={submit} disabled={busy || !path.trim()}>
            Create
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
          <span>Name</span>
          <input
            autoFocus
            value={path}
            placeholder="backend/api, with / for nesting"
            onChange={(e) => {
              setPath(e.target.value)
            }}
          />
        </label>
        <label className="field">
          <span>Default directory</span>
          <input
            list="group-directories"
            value={directory}
            placeholder="Optional"
            onChange={(e) => {
              setDirectory(e.target.value)
            }}
          />
          <datalist id="group-directories">
            {directories.map((d) => (
              <option key={d} value={d} />
            ))}
          </datalist>
        </label>
        <button type="submit" hidden />
      </form>
      {error && <p className="form-error">{error}</p>}
    </Dialog>
  )
}
