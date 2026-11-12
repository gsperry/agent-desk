import { useEffect, useState } from 'react'
import { levelLabels, type RemoteLevel } from '../remote-context'
import { cloudflaredIngress } from '../model/remote'

export type RemoteVia = 'tailscale' | 'cloudflare'

export interface RemoteSettings {
  enabled: boolean
  level: RemoteLevel
  host: string
  port: number
  via?: RemoteVia
  tailscaleLogins: string[]
  cloudflareTeam?: string
  cloudflareAud?: string
  cloudflareEmails?: string[]
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

const joinList = (l: string[] | undefined) => (l ?? []).join(', ')
const splitList = (s: string) => s.split(/[\s,]+/).filter(Boolean)

// RemoteAccess edits the remote access settings. It is shown only on the
// desk; the server refuses changes from a remote device.
export function RemoteAccess({ load, save }: Props) {
  const [form, setForm] = useState<RemoteSettings | null>(null)
  const [logins, setLogins] = useState('')
  const [emails, setEmails] = useState('')
  const [available, setAvailable] = useState(true)
  const [status, setStatus] = useState<{ error?: string; saved?: boolean } | null>(null)
  const [busy, setBusy] = useState(false)
  const [listener, setListener] = useState<RemoteStatus | null>(null)

  const show = (d: RemoteSettings, st?: RemoteStatus) => {
    setForm(d)
    setLogins(joinList(d.tailscaleLogins))
    setEmails(joinList(d.cloudflareEmails))
    setListener(st ?? null)
  }

  useEffect(() => {
    let live = true
    void load().then((r) => {
      if (!live) return
      setAvailable(r.available)
      if (r.data) show(r.data, r.status)
      else if (r.error) setStatus({ error: r.error })
    })
    return () => {
      live = false
    }
  }, [load])

  if (!available) return <p className="muted setting-note">Remote access is not available in this build.</p>
  if (!form) return <p className="muted setting-note">Loading…</p>

  const via: RemoteVia = form.via ?? 'tailscale'
  const submit = () => {
    setBusy(true)
    setStatus(null)
    const next = { ...form, via, tailscaleLogins: splitList(logins), cloudflareEmails: splitList(emails) }
    void save(next).then((r) => {
      setBusy(false)
      if (r.data) {
        show(r.data, r.status)
        setStatus({ saved: true })
      } else setStatus({ error: r.error ?? 'Could not save.' })
    })
  }

  const address = listener?.address ?? ''
  // A Unix socket in an owner-only folder: only this account (and root,
  // which runs tailscaled) can connect. Tests use a TCP port instead.
  const onSocket = address.startsWith('unix:')
  const target = onSocket ? `"${address}"` : `http://${address}`
  const team = form.cloudflareTeam ?? ''
  return (
    <div className="remote-access">
      <p className="muted setting-note">
        Reach this desk from your phone or another computer. Remote devices never see your local link; the proxy vouches for who they are, and only the people you list get in. These settings can only be changed here, on the desk.
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
      <fieldset className="remote-levels">
        <legend>Reach through</legend>
        <label className="check">
          <input
            type="radio"
            name="remote-via"
            checked={via === 'tailscale'}
            onChange={() => {
              setForm({ ...form, via: 'tailscale' })
            }}
          />{' '}
          <strong>Tailscale</strong> <span className="muted">Private to your tailnet. Needs Tailscale on each device.</span>
        </label>
        <label className="check">
          <input
            type="radio"
            name="remote-via"
            checked={via === 'cloudflare'}
            onChange={() => {
              setForm({ ...form, via: 'cloudflare' })
            }}
          />{' '}
          <strong>Cloudflare Tunnel</strong> <span className="muted">Any browser, behind a Cloudflare Access login. Cloudflare can read the traffic.</span>
        </label>
      </fieldset>
      {via === 'tailscale' ? (
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
      ) : (
        <>
          <div className="field-row">
            <label className="field">
              <span>Cloudflare team name</span>
              <input
                value={team}
                placeholder="your-team"
                onChange={(e) => {
                  setForm({ ...form, cloudflareTeam: e.target.value })
                }}
              />
            </label>
            <label className="field">
              <span>Access audience (AUD) tag</span>
              <input
                value={form.cloudflareAud ?? ''}
                placeholder="From the Access application's overview"
                onChange={(e) => {
                  setForm({ ...form, cloudflareAud: e.target.value })
                }}
              />
            </label>
          </div>
          <label className="field">
            <span>Allowed emails</span>
            <input
              value={emails}
              placeholder="you@example.com"
              onChange={(e) => {
                setEmails(e.target.value)
              }}
            />
          </label>
        </>
      )}
      <div className="field-row">
        <label className="field">
          <span>Host name remote devices use</span>
          <input
            value={form.host}
            placeholder={via === 'tailscale' ? 'your-mac.tail1234.ts.net' : 'desk.example.com'}
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
      {via === 'tailscale' ? (
        <p className="muted setting-note">
          Then, once, on this Mac: <code>tailscale serve --bg --https=443 {target}</code>. Open{' '}
          <code>https://{form.host || 'your-mac.tail1234.ts.net'}/</code> on a device signed in to your tailnet.
        </p>
      ) : (
        <div className="muted setting-note">
          <p>
            In Cloudflare Zero Trust, add a self-hosted Access application for the host name with a policy for your email, and copy its audience tag here. Create a tunnel with <code>cloudflared</code> and give it this ingress:
          </p>
          <pre className="remote-config">{cloudflaredIngress(form.host, onSocket ? address : `http://${address}`, team, form.cloudflareAud ?? '')}</pre>
          <p>
            Agent Desk checks the token Access signs for every request, using your team&apos;s public signing keys from <code>{team || 'your-team'}.cloudflareaccess.com</code>. That is the only call it makes to Cloudflare.
          </p>
        </div>
      )}
    </div>
  )
}
