const prefix = 'agent-desk:draft:'

type KV = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>

// Drafts live in localStorage per session, so a half-written reply
// survives switching sessions and reloading the page.
export function loadDraft(store: KV, sessionId: string): string {
  return store.getItem(prefix + sessionId) ?? ''
}

export function saveDraft(store: KV, sessionId: string, text: string): void {
  if (text.trim() === '') store.removeItem(prefix + sessionId)
  else store.setItem(prefix + sessionId, text)
}
