import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { Answer, Screen } from '../types'
import { StyledTerminal } from './StyledTerminal'

interface Props {
  sessionId: string
  read: (id: string) => Promise<{ screen?: Screen; message?: string }>
  answer: (id: string, a: Answer) => Promise<string | null>
  readStyled: (id: string) => Promise<{ screen?: string; message?: string }>
  onClose?: () => void
}

const colorKey = 'agent-desk:terminal-color'

// The live pane refreshes this often while open.
const refreshMs = 1000

// LivePane shows the agent's terminal screen and answers dialogs on it. It
// is the one place Agent Desk shows terminal text.
export function LivePane({ sessionId, read, answer, readStyled, onClose }: Props) {
  const [color, setColor] = useState(() => localStorage.getItem(colorKey) === 'on')
  const [screen, setScreen] = useState<Screen | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [tick, setTick] = useState(0)
  const pre = useRef<HTMLPreElement>(null)

  useEffect(() => {
    let live = true
    void read(sessionId).then((r) => {
      if (!live) return
      if (r.screen) setScreen(r.screen)
      setMessage(r.message ?? null)
    })
    const t = setTimeout(() => {
      setTick((n) => n + 1)
    }, refreshMs)
    return () => {
      live = false
      clearTimeout(t)
    }
  }, [sessionId, read, tick])

  useLayoutEffect(() => {
    if (pre.current) pre.current.scrollTop = pre.current.scrollHeight
  }, [screen?.text])

  const send = (a: Answer) => {
    setBusy(true)
    void answer(sessionId, a).then((err) => {
      setBusy(false)
      setMessage(err)
      setTick((n) => n + 1)
    })
  }

  const dialog = screen?.dialog
  return (
    <section className="live-pane" aria-label="Terminal">
      <header className="live-head">
        <span>Terminal</span>
        <button
          type="button"
          className="link-button color-toggle"
          aria-pressed={color}
          title="Show the screen in color, read-only"
          onClick={() => {
            localStorage.setItem(colorKey, color ? 'off' : 'on')
            setColor(!color)
          }}
        >
          {color ? 'Plain text' : 'Color'}
        </button>
        {onClose && (
          <button type="button" className="link-button" onClick={onClose}>
            Hide
          </button>
        )}
      </header>
      {color ? (
        <StyledTerminal sessionId={sessionId} read={readStyled} />
      ) : (
        <pre ref={pre} className="live-screen">
          {screen?.text ?? 'Reading the screen…'}
        </pre>
      )}
      {dialog && dialog.options.length > 0 && (
        <div className="live-dialog">
          {dialog.question && <p className="live-question">{dialog.question}</p>}
          <div className="live-options">
            {dialog.options.map((o) => (
              <button
                key={o.index}
                type="button"
                className={o.selected ? 'live-option selected' : 'live-option'}
                disabled={busy || o.free === true}
                title={o.free ? 'This option needs typing in the terminal. Answer it in agent-manager.' : undefined}
                onClick={() => {
                  send({ action: 'choose', index: o.index, label: o.label })
                }}
              >
                <span className="live-option-index">{o.index + 1}</span> {o.label}
              </button>
            ))}
          </div>
          <button
            type="button"
            className="live-cancel"
            disabled={busy}
            onClick={() => {
              send({ action: 'key', key: 'Escape' })
            }}
          >
            Cancel (Esc)
          </button>
        </div>
      )}
      {message && <p className="live-message">{message}</p>}
    </section>
  )
}
