import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import { useEffect, useRef, useState } from 'react'
import { screenSize } from '../model/terminal'

interface Props {
  sessionId: string
  read: (id: string) => Promise<{ screen?: string; message?: string }>
}

const refreshMs = 1000

// StyledTerminal shows the agent's screen in color. It is read-only: input
// is disabled and nothing typed here reaches the session.
export function StyledTerminal({ sessionId, read }: Props) {
  const box = useRef<HTMLDivElement>(null)
  const term = useRef<Terminal | null>(null)
  const [message, setMessage] = useState<string | null>(null)

  useEffect(() => {
    if (!box.current) return
    const style = getComputedStyle(document.documentElement)
    const t = new Terminal({
      disableStdin: true,
      convertEol: true,
      cursorBlink: false,
      cursorInactiveStyle: 'none',
      scrollback: 0,
      fontSize: 12,
      fontFamily: style.getPropertyValue('--font-mono').trim() || 'ui-monospace, Menlo, monospace',
      theme: { background: style.getPropertyValue('--surface').trim() || '#f6f7f9', foreground: style.getPropertyValue('--text').trim() || '#1f2328' },
    })
    t.open(box.current)
    term.current = t
    return () => {
      t.dispose()
      term.current = null
    }
  }, [])

  useEffect(() => {
    let live = true
    let timer: ReturnType<typeof setTimeout> | undefined
    let last = ''
    const tick = () => {
      void read(sessionId).then((r) => {
        if (!live) return
        setMessage(r.message ?? null)
        const t = term.current
        if (r.screen !== undefined && t && r.screen !== last) {
          last = r.screen
          const { cols, rows } = screenSize(r.screen)
          if (t.cols !== cols || t.rows !== rows) t.resize(cols, rows)
          // Home, clear, then the screen without its final newline, so the
          // last line does not scroll the first one away.
          t.write('\x1b[H\x1b[2J\x1b[0m' + r.screen.replace(/\n+$/, ''), () => {
            // Keep the bottom of the screen, where prompts are, in view.
            const b = box.current
            if (b) b.scrollTop = b.scrollHeight
          })
        }
        timer = setTimeout(tick, refreshMs)
      })
    }
    tick()
    return () => {
      live = false
      clearTimeout(timer)
    }
  }, [sessionId, read])

  return (
    <div className="styled-terminal">
      <div ref={box} className="styled-terminal-box" aria-label="Terminal screen in color" role="img" />
      {message && <p className="live-message">{message}</p>}
    </div>
  )
}
