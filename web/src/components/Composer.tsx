import { useEffect, useLayoutEffect, useRef, useState, type ClipboardEvent } from 'react'
import { blockedReason, isSendKey } from '../model/compose'
import { loadDraft, saveDraft } from '../model/drafts'
import { expandSnippet, slashSnippet, type Snippet } from '../model/snippets'
import { useSnippets } from '../snippets-context'
import type { SessionView } from '../types'

const mac = /Mac|iPhone|iPad/.test(navigator.userAgent)

interface Props {
  session: SessionView
  connected: boolean
  insideSession: boolean
  onSend: (text: string) => void
  // pasteImage stores a pasted image and returns its path.
  pasteImage?: (image: Blob) => Promise<{ path?: string; error?: string }>
  // restore puts text back for editing, such as a reply that failed.
  restore?: { text: string; nonce: number }
}

// Composer is keyed by session, so each session gets its own draft.
export function Composer({ session, connected, insideSession, onSend, pasteImage, restore }: Props) {
  const [text, setText] = useState(() => loadDraft(localStorage, session.id))
  const area = useRef<HTMLTextAreaElement>(null)
  const blocked = blockedReason(session, connected, insideSession)
  const snippets = useSnippets()
  const [menu, setMenu] = useState(false)
  const vars = { session: session.name, branch: session.branch, project: session.project.name, directory: session.directory }

  const [attaching, setAttaching] = useState(false)
  const [pasteError, setPasteError] = useState<string | null>(null)

  // A pasted image is stored by the backend and its path inserted, which
  // Claude Code reads as an image.
  const onPaste = (e: ClipboardEvent<HTMLTextAreaElement>) => {
    const file = [...e.clipboardData.files].find((f) => f.type.startsWith('image/'))
    if (!file || !pasteImage) return
    e.preventDefault()
    const el = e.currentTarget
    const start = el.selectionStart
    const end = el.selectionEnd
    setAttaching(true)
    setPasteError(null)
    void pasteImage(file).then((r) => {
      setAttaching(false)
      if (!r.path) {
        setPasteError(r.error ?? 'The image could not be attached.')
        return
      }
      const path = r.path
      setText((t) => {
        const before = t.slice(0, start)
        const after = t.slice(end)
        const pad = (s: string) => (s && !/\s$/.test(s) ? `${s} ` : s)
        return `${pad(before)}${path}${after && !/^\s/.test(after) ? ' ' : ''}${after}`
      })
    })
  }

  const insert = (sn: Snippet) => {
    const body = expandSnippet(sn.text, vars)
    const el = area.current
    if (!el || !text.trim() || slashSnippet(text, snippets)) {
      setText(body)
    } else {
      const at = el.selectionStart
      setText(text.slice(0, at) + body + text.slice(el.selectionEnd))
    }
    setMenu(false)
    el?.focus()
  }

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
      {attaching && <p className="composer-blocked muted">Attaching image…</p>}
      {pasteError && <p className="composer-blocked">{pasteError}</p>}
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
          onPaste={onPaste}
          onKeyDown={(e) => {
            const slash = e.key === 'Tab' && !e.shiftKey ? slashSnippet(text, snippets) : null
            if (slash) {
              e.preventDefault()
              insert(slash)
            } else if (isSendKey(e.nativeEvent)) {
              e.preventDefault()
              send()
            } else if (e.key === 'Escape') {
              // Leaving the composer hands j/k back to session navigation.
              e.currentTarget.blur()
            }
          }}
        />
        <div className="snippet-menu-wrap">
          <button
            type="button"
            className="secondary snippet-button"
            aria-expanded={menu}
            aria-label="Snippets"
            title="Insert a snippet (or type /name and press Tab)"
            onClick={() => {
              setMenu(!menu)
            }}
          >
            ⋯
          </button>
          {menu && (
            <ul className="snippet-menu" role="menu" aria-label="Snippets">
              {snippets.map((sn) => (
                <li key={sn.name} role="none">
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => {
                      insert(sn)
                    }}
                  >
                    <strong>/{sn.name}</strong>
                    <span className="muted">{sn.text}</span>
                  </button>
                </li>
              ))}
              {snippets.length === 0 && <li className="muted pad">No snippets. Add them in Settings.</li>}
            </ul>
          )}
        </div>
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
