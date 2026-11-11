import { useEffect, useState } from 'react'
import { levelLabels, type RemoteLevel } from '../remote-context'

export interface RemoteSettings {
  enabled: boolean
  level: RemoteLevel
  host: string
  port: number
  tailscaleLogins: string[]
}

export interface RemoteStatus {
  listening: boolean
  address?: string
  error?: string
}

interface Props {
  load: () => Promise<{ data?: RemoteSettings; status?: RemoteStatus; available: boolean; error?: string }>
  save: (s: RemoteSettings) => Promise<{ data?: RemoteSettings; status?: RemoteStatus; error?: string }>
}

const levels: RemoteLevel[] = ['answer', 'reply', 'manage']

const levelNotes: Record<RemoteLevel, string> = {
  answer: 'Read sessions and answer waiting prompts.',
  reply: 'Also send replies.',
  manage: 'Also start, stop, revive, archive and move sessions.',
}

// RemoteAccess edits the remote access settings. It is shown only on the
// desk; the server refuses changes from a remote device.
export function RemoteAccess({ load, save }: Props) {
  const [form, setForm] = useState<RemoteSettings | null>(null)
  const [logins, setLogins] = useState('')
  const [available, setAvailable] = useState(true)
  const [status, setStatus] = useState<{ error?: string; saved?: boolean } | null>(null)
  const [busy, setBusy] = useState(false)
  const [listener, setListener] = useState<RemoteStatus | null>(null)

  useEffect(() => {
    let live = true
    void load().then((r) => {
      if (!live) return
      setAvailable(r.available)
      if (r.data) {
        setForm(r.data)
        setLogins(r.data.tailscaleLogins.join(', '))
        setListener(r.status ?? null)
      } else if (r.error) setStatus({ error: r.error })
    })
    return () => {
      live = false
    }
  }, [load])

  if (!available) return <p className="muted setting-note">Remote access is not available in this build.</p>
  if (!form) return <p className="muted setting-note">Loading…</p>

  const submit = () => {
    setBusy(true)
    setStatus(null)
    const next = { ...form, tailscaleLogins: logins.split(/[\s,]+/).filter(Boolean) }
    void save(next).then((r) => {
      setBusy(false)
      if (r.data) {
        setForm(r.data)
        setLogins(r.data.tailscaleLogins.join(', '))
        setListener(r.status ?? null)
        setStatus({ saved: true })
      } else setStatus({ error: r.error ?? 'Could not save.' })
    })
  }

  const address = listener?.address ?? ''
  // A Unix socket in an owner-only folder: only this account (and root,
  // which runs tailscaled) can connect. Tests use a TCP port instead.
  const onSocket = address.startsWith('unix:')
  const target = onSocket ? `"${address}"` : `http://${address}`
  return (
    <div className="remote-access">
      <p className="muted setting-note">
        Reach this desk from your phone or another computer through Tailscale. Remote devices never see your local link; Tailscale vouches for who they are, and only the logins you list get in. These settings can only be changed here, on the desk.
      </p>
      <label className="check">
        <input
          type="checkbox"
          checked={form.enabled}
          onChange={(e) => {
            setForm({ ...form, enabled: e.target.checked })
          }}
        />{' '}
        Allow remote access
      </label>
      <fieldset className="remote-levels">
        <legend>What remote devices may do</legend>
        {levels.map((l) => (
          <label key={l} className="check">
            <input
              type="radio"
              name="remote-level"
              checked={form.level === l}
              onChange={() => {
                setForm({ ...form, level: l })
              }}
            />{' '}
            <strong>{levelLabels[l]}</strong> <span className="muted">{levelNotes[l]}</span>
          </label>
        ))}
        <p className="muted setting-note">Settings, rules, catch-up summaries, opening files and pasting images stay on the desk at every level.</p>
      </fieldset>
      <label className="field">
        <span>Allowed Tailscale logins</span>
        <input
          value={logins}
          placeholder="you@example.com"
          onChange={(e) => {
            setLogins(e.target.value)
          }}
        />
      </label>
      <div className="field-row">
        <label className="field">
          <span>Host name remote devices use</span>
          <input
            value={form.host}
            placeholder="your-mac.tail1234.ts.net"
            onChange={(e) => {
              setForm({ ...form, host: e.target.value })
            }}
          />
        </label>
        {!onSocket && (
          <label className="field remote-port">
            <span>Local port</span>
            <input
              type="number"
              value={form.port}
              onChange={(e) => {
                setForm({ ...form, port: Number(e.target.value) })
              }}
            />
          </label>
        )}
      </div>
      <div className="remote-actions">
        <button type="button" className="send" disabled={busy} onClick={submit}>
          {busy ? 'Saving…' : 'Save'}
        </button>
        {status?.saved && <span className="muted">Saved.</span>}
        {status?.error && <span className="form-error">{status.error}</span>}
      </div>
      {form.enabled && listener && (
        <p className={listener.error ? 'form-error' : 'muted setting-note'}>
          {listener.error ? `Not listening: ${listener.error}` : listener.listening ? `Listening on ${listener.address ?? ''}.` : 'Not listening yet. Save to start.'}
        </p>
      )}
      <p className="muted setting-note">
        Then, once, on this Mac: <code>tailscale serve --bg --https=443 {target}</code>. Open{' '}
        <code>https://{form.host || 'your-mac.tail1234.ts.net'}/</code> on a device signed in to your tailnet.
      </p>
    </div>
  )
}
