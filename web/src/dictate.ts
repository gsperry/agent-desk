import { createContext, useCallback, useContext, useEffect, useRef, useState, type RefObject } from 'react'
import { anchorAt, parseEvent, withHeard, type Anchor, type DictationEvent } from './model/dictation'

// Dictate is voice input through the desktop app: speech is recognized on
// this Mac by its helper, and only text reaches the page.
export interface Dictate {
  start: (on: (e: DictationEvent) => void) => Promise<unknown>
  stop: () => Promise<unknown>
}

interface TauriChannel {
  onmessage: (m: unknown) => void
}

interface TauriWindow {
  __TAURI__?: {
    core?: {
      invoke?: (cmd: string, args: Record<string, unknown>) => Promise<unknown>
      Channel?: new () => TauriChannel
    }
  }
}

// desktopDictate returns the app's dictation bridge, or null in a browser.
export function desktopDictate(w: unknown = globalThis): Dictate | null {
  const core = (w as TauriWindow).__TAURI__?.core
  const invoke = core?.invoke
  const Channel = core?.Channel
  if (typeof invoke !== 'function' || typeof Channel !== 'function') return null
  return {
    start: (on) => {
      const events = new Channel()
      events.onmessage = (m) => {
        const e = parseEvent(m)
        if (e) on(e)
      }
      return invoke('dictate_start', { events })
    },
    stop: () => invoke('dictate_stop', {}),
  }
}

// DictateContext carries the bridge where voice input is offered; null
// hides every mic button.
export const DictateContext = createContext<Dictate | null>(null)

export type DictationPhase = 'idle' | 'starting' | 'listening' | 'stopping'

// active is the one dictation running, so starting another (in a second
// box) stops the first.
let active: { stop: () => void } | null = null

// useDictation types what is heard into a text box at its caret. Typing
// while it listens stops it and keeps what was typed.
export function useDictation(text: string, setText: (t: string) => void, area: RefObject<HTMLTextAreaElement | null>) {
  const dictate = useContext(DictateContext)
  const [phase, setPhase] = useState<DictationPhase>('idle')
  const [error, setError] = useState<string | null>(null)
  // run counts starts and stops, so words from an ended run are dropped.
  const run = useRef(0)
  const running = useRef(false)
  const anchor = useRef<Anchor>({ before: '', after: '' })
  const caret = useRef<number | null>(null)

  // halt stops listening at once and ignores the words still to come.
  const halt = useCallback(() => {
    run.current++
    running.current = false
    setPhase('idle')
    void dictate?.stop().catch(() => undefined)
  }, [dictate])

  const start = () => {
    if (!dictate) return
    active?.stop()
    const mine = ++run.current
    running.current = true
    const el = area.current
    anchor.current = anchorAt(text, el?.selectionStart ?? text.length, el?.selectionEnd ?? text.length)
    setError(null)
    setPhase('starting')
    const handle = {
      stop: () => {
        if (run.current === mine) halt()
      },
    }
    active = handle
    const end = () => {
      run.current++
      running.current = false
      if (active === handle) active = null
      setPhase('idle')
    }
    const on = (e: DictationEvent) => {
      if (run.current !== mine) return
      switch (e.type) {
        case 'ready':
          setPhase((p) => (p === 'starting' ? 'listening' : p))
          break
        case 'text': {
          const r = withHeard(anchor.current, e.text)
          caret.current = r.caret
          setText(r.text)
          break
        }
        case 'error':
          setError(e.message)
          break
        case 'done':
          end()
          break
      }
    }
    dictate.start(on).catch((err: unknown) => {
      if (run.current !== mine) return
      end()
      setError(`Dictation could not start: ${String(err)}`)
    })
  }

  // The caret follows the dictated words.
  useEffect(() => {
    const el = area.current
    if (caret.current === null || !el) return
    el.setSelectionRange(caret.current, caret.current)
    caret.current = null
  }, [text, area])

  // Leaving the box stops listening.
  useEffect(
    () => () => {
      if (running.current) halt()
    },
    [halt],
  )

  const toggle = () => {
    if (phase === 'idle') {
      start()
      return
    }
    setPhase('stopping')
    void dictate?.stop().catch(() => undefined)
  }

  // interrupt is for typing during dictation: the typed text stays.
  const interrupt = () => {
    if (running.current) halt()
  }

  return {
    available: dictate !== null,
    phase,
    error,
    toggle,
    interrupt,
    clearError: () => {
      setError(null)
    },
  }
}
