import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { conversationTitle, lastPart } from '../model/history'
import { ago } from '../model/overview'
import { initialChoice, installHint, outsideConversations, programLabel, sameChoice, toggleCLI, type CLIChoice } from '../model/welcome'
import type { Conversation, SessionView, SetupReport, State } from '../types'

interface Props {
  state: State | null
  load: () => Promise<{ data?: SetupReport; error?: string }>
  apply: (c: CLIChoice) => Promise<{ data?: SetupReport; error?: string }>
  onStartManager: () => Promise<string | null>
  loadConversations: () => Promise<{ data?: Conversation[]; error?: string }>
  owners: Map<string, SessionView>
  onResume: (c: Conversation) => void
  onAllConversations: () => void
  onNewSession: () => void
  onOpenSettings: () => void
  // extras are the optional settings shown last: notifications, and start
  // at login inside the desktop app.
  extras: ReactNode
  onDone: () => void
}

// Welcome is the setup checklist: the programs Agent Desk needs, the
// manager, the CLIs new sessions offer, and how to bring in conversations
// started outside agent-manager. It opens by itself on the first run and
// from Settings after that.
export function Welcome(props: Props) {
  const { state, load } = props
  const [report, setReport] = useState<SetupReport | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [checking, setChecking] = useState(false)

  const take = useCallback((r: { data?: SetupReport; error?: string }) => {
    if (r.data) {
      setReport(r.data)
      setError(null)
    } else setError(r.error ?? 'Could not check the setup.')
  }, [])
  useEffect(() => {
    void load().then(take)
  }, [load, take])
  const check = () => {
    setChecking(true)
    void load().then((r) => {
      setChecking(false)
      take(r)
    })
  }

  const missing = report?.programs.filter((p) => !p.found) ?? []
  const managerFound = report?.programs.find((p) => p.name === 'agent-manager')?.found !== false

  return (
    <main className="main welcome-view" aria-label="Welcome">
      <header className="session-head">
        <div className="session-title">
          <h1>Welcome to Agent Desk</h1>
        </div>
        <div className="session-sub muted">A window onto the agents agent-manager runs. Everything below goes through agent-manager's own commands.</div>
      </header>
      <div className="transcript">
        <div className="welcome">
          {error && <p className="banner error">{error}</p>}
          <Step n={1} title="Programs" done={report ? missing.length === 0 : null}>
            {!report && !error && <p className="muted">Checking…</p>}
            {report && (
              <ul className="welcome-programs">
                {report.programs.map((p) => {
                  const hint = installHint(p.name)
                  return (
                    <li key={p.name}>
                      <span className={p.found ? 'welcome-ok' : 'welcome-missing'} aria-hidden>
                        {p.found ? '✓' : '✗'}
                      </span>{' '}
                      <strong>{programLabel(p.name)}</strong> <span className="muted">{p.found ? 'found' : 'not found on PATH'}</span>
                      {!p.found && (
                        <div className="welcome-hint">
                          <span className="muted">{hint.note}</span>
                          {hint.command && <code className="welcome-command">{hint.command}</code>}
                          {hint.link && (
                            <a href={hint.link} target="_blank" rel="noreferrer">
                              {hint.link.replace(/^https:\/\//, '')}
                            </a>
                          )}
                        </div>
                      )}
                    </li>
                  )
                })}
              </ul>
            )}
            {missing.length > 0 && (
              <p>
                <button type="button" className="secondary" onClick={check} disabled={checking}>
                  {checking ? 'Checking…' : 'Check again'}
                </button>
              </p>
            )}
          </Step>
          <ManagerStep state={state} found={managerFound} onStart={props.onStartManager} />
          <CLIStep report={report} apply={props.apply} onReport={setReport} />
          <SessionsStep
            sessions={state?.sessions ?? []}
            loadConversations={props.loadConversations}
            owners={props.owners}
            onResume={props.onResume}
            onAllConversations={props.onAllConversations}
            onNewSession={props.onNewSession}
          />
          <Step n={5} title="Optional" done={null}>
            {props.extras}
            <p>
              Remote access lets your phone read and answer sessions through Tailscale or Cloudflare. It is off until you turn it on in{' '}
              <button type="button" className="link-button" onClick={props.onOpenSettings}>
                Settings
              </button>
              .
            </p>
          </Step>
          <footer className="welcome-foot">
            <span className="muted">This checklist stays in Settings.</span>
            <button type="button" className="send" onClick={props.onDone}>
              Go to the overview
            </button>
          </footer>
        </div>
      </div>
    </main>
  )
}

// done is true for a step that is set, false for one that needs work, and
// null for a step with nothing to check.
function Step({ n, title, done, children }: { n: number; title: string; done: boolean | null; children: ReactNode }) {
  const mark = done === null ? String(n) : done ? '✓' : '!'
  const cls = done === null ? 'welcome-num' : done ? 'welcome-num done' : 'welcome-num todo'
  return (
    <section className="welcome-step" aria-label={title}>
      <h2>
        {done === null ? (
          <span className={cls} aria-hidden>
            {mark}
          </span>
        ) : (
          <span className={cls} role="img" aria-label={done ? 'Done:' : 'Needs attention:'}>
            {mark}
          </span>
        )}
        {title}
      </h2>
      <div className="welcome-body">{children}</div>
    </section>
  )
}

function ManagerStep({ state, found, onStart }: { state: State | null; found: boolean; onStart: () => Promise<string | null> }) {
  const [starting, setStarting] = useState(false)
  const [startError, setStartError] = useState<string | null>(null)
  const awake = Boolean(state && !state.error && state.managerAwake)
  const start = () => {
    setStarting(true)
    setStartError(null)
    void onStart().then((err) => {
      setStarting(false)
      setStartError(err)
    })
  }
  return (
    <Step n={2} title="agent-manager running" done={state ? awake && !state.insideSession : null}>
      {!found ? (
        <p className="muted">Install agent-manager first.</p>
      ) : state?.error ? (
        <p>Agent Desk cannot read agent-manager: {state.error.message}</p>
      ) : awake ? (
        <p>Running, so statuses update as agents work. Closing Agent Desk leaves every agent running.</p>
      ) : state ? (
        <>
          <p>agent-manager is not running, so session statuses are frozen. Open agent-manager in a terminal, or start it in the background:</p>
          <p>
            <button type="button" className="send" onClick={start} disabled={starting}>
              {starting ? 'Starting…' : 'Start headless manager'}
            </button>
          </p>
          {startError && <p className="form-error">{startError}</p>}
        </>
      ) : (
        <p className="muted">Checking…</p>
      )}
      {state?.insideSession && <p className="banner warn">Agent Desk is running inside an agent-manager session, where sending is refused. Start it from a regular terminal or the desktop app.</p>}
    </Step>
  )
}

function CLIStep({ report, apply, onReport }: { report: SetupReport | null; apply: Props['apply']; onReport: (r: SetupReport) => void }) {
  const saved = useMemo(() => (report ? initialChoice(report) : null), [report])
  // edit is the form's unsaved change, if any.
  const [edit, setChoice] = useState<CLIChoice | null>(null)
  const choice = edit ?? saved
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState<{ error: boolean; text: string } | null>(null)

  if (!report || report.settingsError || report.clis.length === 0 || !choice || !saved) {
    return (
      <Step n={3} title="CLIs for new sessions" done={null}>
        <p className="muted">{report?.settingsError ? `agent-manager's settings could not be read: ${report.settingsError}` : 'Available once agent-manager is installed.'}</p>
      </Step>
    )
  }
  const changed = !sameChoice(choice, saved)
  const save = () => {
    setBusy(true)
    setMessage(null)
    void apply(choice).then((r) => {
      setBusy(false)
      if (r.data) {
        onReport(r.data)
        setChoice(null)
        setMessage({ error: false, text: 'Saved to agent-manager.' })
      } else setMessage({ error: true, text: r.error ?? 'Could not save.' })
    })
  }
  return (
    <Step n={3} title="CLIs for new sessions" done={null}>
      <p>Choose the CLIs you use. New sessions offer only these, here and in agent-manager, and start the default unless you pick another.</p>
      <div className="welcome-clis" role="group" aria-label="CLIs offered">
        {report.clis.map((t) => (
          <label key={t} className="check">
            <input
              type="checkbox"
              checked={choice.shown.includes(t)}
              onChange={(e) => {
                setChoice(toggleCLI(choice, report.clis, t, e.target.checked))
                setMessage(null)
              }}
            />{' '}
            {t}
          </label>
        ))}
      </div>
      <div className="welcome-default">
        <label className="field">
          <span>Default</span>
          <select
            value={choice.defaultTool}
            disabled={choice.shown.length === 0}
            onChange={(e) => {
              setChoice({ ...choice, defaultTool: e.target.value })
              setMessage(null)
            }}
          >
            {choice.shown.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
        </label>
        <button type="button" className="send" onClick={save} disabled={busy || !changed || choice.shown.length === 0}>
          {busy ? 'Saving…' : 'Save'}
        </button>
      </div>
      <p className="muted setting-note">Reading transcripts, answering questions and the live activity row are built for Claude Code. Sessions of other CLIs are listed and managed.</p>
      {message && <p className={message.error ? 'form-error' : 'welcome-saved'}>{message.text}</p>}
    </Step>
  )
}

function SessionsStep({
  sessions,
  loadConversations,
  owners,
  onResume,
  onAllConversations,
  onNewSession,
}: {
  sessions: SessionView[]
  loadConversations: Props['loadConversations']
  owners: Map<string, SessionView>
  onResume: (c: Conversation) => void
  onAllConversations: () => void
  onNewSession: () => void
}) {
  const [list, setList] = useState<Conversation[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [now] = useState(() => Date.now())
  useEffect(() => {
    let live = true
    void loadConversations().then((r) => {
      if (!live) return
      if (r.data) setList(r.data)
      else setError(r.error ?? 'Could not list conversations.')
    })
    return () => {
      live = false
    }
  }, [loadConversations])
  const active = sessions.filter((s) => !s.archived).length
  const archived = sessions.length - active
  const outside = useMemo(() => (list ? outsideConversations(list, owners, 5) : []), [list, owners])

  return (
    <Step n={4} title="Your sessions" done={null}>
      {active > 0 ? (
        <p>
          The {active} session{active === 1 ? '' : 's'} agent-manager runs {active === 1 ? 'is' : 'are'} in the sidebar
          {archived > 0 ? `, and ${String(archived)} archived behind Show archived` : ''}. There is nothing to import.
        </p>
      ) : (
        <p>
          agent-manager has no sessions yet.{' '}
          <button type="button" className="link-button" onClick={onNewSession}>
            Start one
          </button>{' '}
          and it appears in the sidebar.
        </p>
      )}
      <p>
        Conversations you started with a plain <code>claude</code>, outside agent-manager, can be read and searched in{' '}
        <button type="button" className="link-button" onClick={onAllConversations}>
          All conversations
        </button>
        . Resume one to continue it as an agent-manager session in its own directory. Quit the original <code>claude</code> first, so two processes do not write to one conversation.
      </p>
      {error && <p className="form-error">{error}</p>}
      {!list && !error && <p className="muted">Reading conversations…</p>}
      {list && outside.length === 0 && <p className="muted">Every conversation on disk already belongs to a session.</p>}
      {outside.length > 0 && (
        <ul className="welcome-conversations" aria-label="Recent conversations outside agent-manager">
          {outside.map((c) => (
            <li key={c.id}>
              <span className="welcome-conv-text">
                <strong>{conversationTitle(c)}</strong>
                <span className="muted">
                  {lastPart(c.directory) || 'unknown directory'}
                  {c.branch ? ` · ${c.branch}` : ''} · {ago(c.modified, now)}
                </span>
              </span>
              <button
                type="button"
                className="secondary"
                disabled={!c.directory}
                title={c.directory ? undefined : 'Its directory is not recorded'}
                onClick={() => {
                  onResume(c)
                }}
              >
                Resume…
              </button>
            </li>
          ))}
        </ul>
      )}
    </Step>
  )
}
