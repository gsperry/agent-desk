import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { blockedReason, isSendKey } from '../model/compose'
import { loadDraft, saveDraft } from '../model/drafts'
import type { SessionView } from '../types'

const mac = /Mac|iPhone|iPad/.test(navigator.userAgent)

interface Props {
  session: SessionView
  connected: boolean
  insideSession: boolean
  onSend: (text: string) => void
  // restore puts text back for editing, such as a reply that failed.
  restore?: { text: string; nonce: number }
}

// Composer is keyed by session, so each session gets its own draft.
export function Composer({ session, connected, insideSession, onSend, restore }: Props) {
  const [text, setText] = useState(() => loadDraft(localStorage, session.id))
  const area = useRef<HTMLTextAreaElement>(null)
  const blocked = blockedReason(session, connected, insideSession)

  // A restored reply is merged into the draft during render, the pattern
  // React recommends over an effect for state derived from a prop change.
  const [restored, setRestored] = useState(restore?.nonce)
  if (restore && restore.nonce !== restored) {
    setRestored(restore.nonce)
    setText(text.trim() ? `${text}\n\n${restore.text}` : restore.text)
  }
  useEffect(() => {
    if (restore) area.current?.focus()
  }, [restore])

  // Saved on every change: a debounce would drop the last keystrokes when
  // the reader switches sessions right away.
  useEffect(() => {
    saveDraft(localStorage, session.id, text)
  }, [session.id, text])

  // Grow with the text up to the CSS max-height.
  useLayoutEffect(() => {
    const el = area.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${String(el.scrollHeight)}px`
  }, [text])

  const send = () => {
    if (blocked || text.trim() === '') return
    onSend(text)
    setText('')
    saveDraft(localStorage, session.id, '')
  }

  return (
    <div className="composer">
      {blocked && <p className="composer-blocked">{blocked}</p>}
      <div className="composer-box">
        <textarea
          ref={area}
          value={text}
          rows={1}
          placeholder={`Reply to ${session.name}`}
          aria-label={`Reply to ${session.name}`}
          onChange={(e) => {
            setText(e.target.value)
          }}
          onKeyDown={(e) => {
            if (isSendKey(e.nativeEvent)) {
              e.preventDefault()
              send()
            } else if (e.key === 'Escape') {
              // Leaving the composer hands j/k back to session navigation.
              e.currentTarget.blur()
            }
          }}
        />
        <button type="button" className="send" onClick={send} disabled={blocked !== null || text.trim() === ''}>
          Send
        </button>
      </div>
      <p className="composer-hint">
        Enter for a new line · {mac ? '⌘' : 'Ctrl'}+Enter to send
      </p>
    </div>
  )
}
