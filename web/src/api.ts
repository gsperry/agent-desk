export const wsProtocol = 'agentdesk.v1'
const tokenProtocolPrefix = 'agentdesk.token.'

export type ServerMessage = { type: 'hello'; version: string } | { type: string; [key: string]: unknown }

export type ConnectionState =
  | { kind: 'connecting' }
  | { kind: 'open'; version: string }
  | { kind: 'closed'; retryInMs: number }
  | { kind: 'unauthorized' }

export function apiFetch(token: string, path: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers)
  headers.set('Authorization', `Bearer ${token}`)
  return fetch(path, { ...init, headers })
}

export function socketURL(loc: Pick<Location, 'protocol' | 'host'>): string {
  return `${loc.protocol === 'https:' ? 'wss' : 'ws'}://${loc.host}/api/ws`
}

// backoff is the reconnect delay after the given number of failed attempts.
export function backoff(attempt: number): number {
  return Math.min(500 * 2 ** attempt, 10_000)
}

// connect keeps one WebSocket open, reconnecting with backoff, until the
// returned function is called.
export function connect(
  token: string,
  onState: (s: ConnectionState) => void,
  onMessage: (m: ServerMessage) => void,
): () => void {
  let attempt = 0
  let socket: WebSocket | null = null
  let timer: ReturnType<typeof setTimeout> | undefined
  let stopped = false

  const open = () => {
    onState({ kind: 'connecting' })
    socket = new WebSocket(socketURL(window.location), [wsProtocol, tokenProtocolPrefix + token])
    socket.onmessage = (ev) => {
      const msg = JSON.parse(String(ev.data)) as ServerMessage
      if (msg.type === 'hello') {
        attempt = 0
        onState({ kind: 'open', version: String(msg.version) })
      }
      onMessage(msg)
    }
    socket.onclose = () => {
      if (stopped) return
      void checkAuth().then((authorized) => {
        if (stopped) return
        if (!authorized) {
          onState({ kind: 'unauthorized' })
          return
        }
        const delay = backoff(attempt++)
        onState({ kind: 'closed', retryInMs: delay })
        timer = setTimeout(open, delay)
      })
    }
  }

  // A refused upgrade looks like any other close, so ask the API whether the
  // token is still good before retrying.
  const checkAuth = async () => {
    try {
      const res = await apiFetch(token, '/api/health')
      return res.status !== 401
    } catch {
      return true
    }
  }

  open()
  return () => {
    stopped = true
    clearTimeout(timer)
    socket?.close()
  }
}
