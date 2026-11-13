import { useEffect, useState } from 'react'
import type { Invoke } from '../notify'

// StartAtLogin turns the desktop app's login item on or off. It shows only
// inside the desktop app, which starts in the background at login so
// remote access and notifications run after a restart.
export function StartAtLogin({ invoke }: { invoke: Invoke }) {
  const [on, setOn] = useState<boolean | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let live = true
    invoke('autostart_get', {})
      .then((v) => {
        if (live) setOn(v === true)
      })
      .catch((e: unknown) => {
        if (live) setError(String(e))
      })
    return () => {
      live = false
    }
  }, [invoke])

  return (
    <div className="setting">
      <label className="check">
        <input
          type="checkbox"
          disabled={on === null}
          checked={on === true}
          onChange={(e) => {
            const want = e.target.checked
            setError(null)
            invoke('autostart_set', { enabled: want })
              .then((v) => {
                setOn(v === true)
              })
              .catch((err: unknown) => {
                setError(String(err))
              })
          }}
        />{' '}
        Start at login, in the background
      </label>
      <p className="muted setting-note">Keeps remote access and notifications running after a restart. The window opens from the dock, the menu bar or Control-Option-A.</p>
      {error && <p className="form-error">{error}</p>}
    </div>
  )
}
