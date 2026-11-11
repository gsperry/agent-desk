const storageKey = 'agent-desk-token'

// parseHashToken reads the token agent-desk puts in the URL fragment
// (#t=<token>). The fragment never reaches the server or its logs.
export function parseHashToken(hash: string): string | null {
  const params = new URLSearchParams(hash.replace(/^#/, ''))
  const token = params.get('t')
  return token && token.length >= 32 ? token : null
}

// takeToken moves a token from the URL into sessionStorage and clears it from
// the address bar, so it is not left in history or shared by copy and paste.
export function takeToken(): string | null {
  const fromHash = parseHashToken(window.location.hash)
  if (fromHash) {
    sessionStorage.setItem(storageKey, fromHash)
    history.replaceState(null, '', window.location.pathname + window.location.search)
    return fromHash
  }
  return sessionStorage.getItem(storageKey)
}
