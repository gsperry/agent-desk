import type { SessionView } from '../types'

// blockedReason mirrors the server's status gate so the composer can say
// why it is disabled before a send is attempted. The server still checks
// the screen for dialogs and drafts.
export function blockedReason(session: SessionView, connected: boolean, insideSession: boolean): string | null {
  if (!connected) return 'Not connected to Agent Desk.'
  if (insideSession) return 'Agent Desk is running inside an agent-manager session, where sending is refused.'
  if (!session.running || session.status === 'dead') return 'Revive this session in agent-manager to reply.'
  if (session.status === 'starting') return 'The agent is still starting.'
  if (session.status === 'waiting')
    return 'The agent is waiting on a question or permission prompt. Answer it in the terminal panel above.'
  return null
}

// isSendKey accepts Cmd+Enter or Ctrl+Enter on every platform; the hint
// names the platform's usual one. Enter during IME composition is ignored.
export function isSendKey(e: { key: string; metaKey: boolean; ctrlKey: boolean; isComposing?: boolean }): boolean {
  return e.key === 'Enter' && !e.isComposing && (e.metaKey || e.ctrlKey)
}
