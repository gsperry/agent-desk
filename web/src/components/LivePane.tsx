import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { Answer, Screen } from '../types'
import { StyledTerminal } from './StyledTerminal'
import { isSendKey } from '../model/compose'

interface Props {
  sessionId: string
  read: (id: string) => Promise<{ screen?: Screen; message?: string }>
  answer: (id: string, a: Answer) => Promise<string | null>
  readStyled: (id: string) => Promise<{ screen?: string; message?: string }>
  onClose?: () => void
}

const colorKey = 'agent-desk:terminal-color'
// screenKey remembers whether a pane that opened by itself, because the
// session waits, shows the screen or only the question and its answers.
const screenKey = 'agent-desk:terminal-screen'

// The live pane refreshes this often while open.
const refreshMs = 1000

// LivePane shows the agent's terminal screen and answers dialogs on it. It
// is the one place Agent Desk shows terminal text. Without onClose it was
// opened by a waiting session, and the screen itself can be hidden.
export function LivePane({ sessionId, read, answer, readStyled, onClose }: Props) {
  const [color, setColor] = useState(() => localStorage.getItem(colorKey) === 'on')
  const [screenOn, setScreenOn] = useState(() => localStorage.getItem(screenKey) !== 'off')
  const showScreen = onClose !== undefined || screenOn
  const [screen, setScreen] = useState<Screen | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  // typing is the text option being answered, and its draft.
  const [typing, setTyping] = useState<{ index: number; label: string; text: string } | null>(null)
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
      if (!err) setTyping(null)
      setTick((n) => n + 1)
    })
  }

  const dialog = screen?.dialog
  return (
    <section className="live-pane" aria-label="Terminal">
      <header className="live-head">
        <span>Terminal</span>
        {showScreen && (
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
        )}
        {!onClose && (
          <button
            type="button"
            className="link-button"
            aria-pressed={!screenOn}
            title="Applies whenever a waiting session opens the terminal"
            onClick={() => {
              localStorage.setItem(screenKey, screenOn ? 'off' : 'on')
              setScreenOn(!screenOn)
            }}
          >
            {screenOn ? 'Hide screen' : 'Show screen'}
          </button>
        )}
        {onClose && (
          <button type="button" className="link-button" onClick={onClose}>
            Hide
          </button>
        )}
      </header>
      {showScreen &&
        (color ? (
          <StyledTerminal sessionId={sessionId} read={readStyled} />
        ) : (
          <pre ref={pre} className="live-screen">
            {screen?.text ?? 'Reading the screen…'}
          </pre>
        ))}
      {!showScreen && !(dialog && dialog.options.length > 0) && (
        <p className="muted live-hidden">Screen hidden. The session is waiting on the terminal.</p>
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
                disabled={busy}
                title={o.free ? 'Answer in your own words' : undefined}
                onClick={() => {
                  if (o.free) setTyping({ index: o.index, label: o.label, text: typing?.index === o.index ? typing.text : '' })
                  else send({ action: 'choose', index: o.index, label: o.label })
                }}
              >
                <span className="live-option-index">{o.index + 1}</span> {o.label}
              </button>
            ))}
          </div>
          {typing && (
            <form
              className="live-typed"
              onSubmit={(e) => {
                e.preventDefault()
                if (typing.text.trim()) send({ action: 'text', index: typing.index, label: typing.label, text: typing.text })
              }}
            >
              <input
                aria-label="Your answer"
                placeholder="Your answer, one line"
                autoFocus
                maxLength={2000}
                value={typing.text}
                onChange={(e) => {
                  setTyping({ ...typing, text: e.target.value.replace(/[\r\n]+/g, ' ') })
                }}
                onKeyDown={(e) => {
                  if (e.key === 'Escape') {
                    e.stopPropagation()
                    setTyping(null)
                  } else if (isSendKey(e.nativeEvent)) {
                    e.preventDefault()
                    e.currentTarget.form?.requestSubmit()
                  }
                }}
              />
              <button type="submit" className="send" disabled={busy || !typing.text.trim()}>
                Send answer
              </button>
              <button
                type="button"
                className="link-button"
                onClick={() => {
                  setTyping(null)
                }}
              >
                Cancel
              </button>
            </form>
          )}
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
