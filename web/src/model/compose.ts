import type { SessionView } from '../types'

// blockedReason mirrors the server's status gate so the composer can say
// why it is disabled before a send is attempted. The server still checks
// the screen for dialogs and drafts.
// A session reading waiting is blocked only while a question is (or may
// be) on screen: asking false means its screen shows none.
export function blockedReason(session: SessionView, connected: boolean, insideSession: boolean, asking?: boolean): string | null {
  if (!connected) return 'Not connected to Agent Desk.'
  if (insideSession) return 'Agent Desk is running inside an agent-manager session, where sending is refused.'
  if (!session.running || session.status === 'dead') return 'Revive this session in agent-manager to reply.'
  if (session.status === 'starting') return 'The agent is still starting.'
  if (session.status === 'waiting' && asking !== false)
    return 'The agent is waiting on a question. Choose an answer above.'
  return null
}

export interface EnterKey {
  key: string
  metaKey: boolean
  ctrlKey: boolean
  shiftKey: boolean
  altKey: boolean
  isComposing?: boolean
}

// enterAction is what Enter does in a box that sends, as in Claude Code:
// plain Enter sends; Shift, Cmd, Ctrl or Option with Enter adds a line. On
// a touch screen Enter adds a line and the Send button sends. Enter during
// IME composition, and every other key, is left alone (null).
export function enterAction(e: EnterKey, touch = false): 'send' | 'newline' | null {
  if (e.key !== 'Enter' || e.isComposing) return null
  if (touch || e.shiftKey || e.metaKey || e.ctrlKey || e.altKey) return 'newline'
  return 'send'
}

// nativeNewline reports whether the browser inserts the line break itself
// (Shift+Enter, or Enter on a touch screen); for Cmd, Ctrl and Option the
// box inserts it.
export function nativeNewline(e: EnterKey, touch = false): boolean {
  return e.shiftKey || (touch && !e.metaKey && !e.ctrlKey && !e.altKey)
}

// isTouch is true on devices whose main pointer is a finger.
export function isTouch(): boolean {
  return typeof matchMedia === 'function' && matchMedia('(hover: none) and (pointer: coarse)').matches
}

// insertNewline adds a line break at the caret, keeping the box's undo
// history, and lets React see the change.
export function insertNewline(box: HTMLTextAreaElement): void {
  // execCommand is deprecated but is the one way to keep undo working.
  // eslint-disable-next-line @typescript-eslint/no-deprecated
  if (document.execCommand('insertText', false, '\n')) return
  box.setRangeText('\n', box.selectionStart, box.selectionEnd, 'end')
  box.dispatchEvent(new Event('input', { bubbles: true }))
}
