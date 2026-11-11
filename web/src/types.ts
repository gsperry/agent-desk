// Mirrors of the backend JSON (internal/hub and internal/transcript).

export type Status = 'starting' | 'working' | 'waiting' | 'finished' | 'idle' | 'errored' | 'dead'

export interface Project {
  key: string
  name: string
  root?: string
  kind: 'group' | 'repo' | 'dir'
}

export interface SessionView {
  id: string
  name: string
  tool: string
  group: string
  directory: string
  status: Status | (string & {})
  running: boolean
  archived: boolean
  branch?: string
  environment?: string
  agentSessionId?: string
  project: Project
}

export interface State {
  managerAwake: boolean
  insideSession: boolean
  error?: { kind: 'not_installed' | 'failed'; message: string }
  sessions: SessionView[]
}

export type ItemKind =
  | 'user'
  | 'peer'
  | 'notice'
  | 'compact_summary'
  | 'text'
  | 'thinking'
  | 'tool_use'
  | 'tool_result'
  | 'image'
  | 'system'
  | 'compact_boundary'
  | 'attachment'
  | 'meta'
  | 'raw'

export interface Item {
  seq: number
  kind: ItemKind
  uuid?: string
  time?: string
  messageId?: string
  sidechain?: boolean
  label?: string
  text?: string
  preamble?: string
  tool?: { id: string; name: string; input?: unknown }
  result?: { toolUseId: string; content: string; isError?: boolean; truncated?: boolean; detail?: unknown }
  meta?: Record<string, unknown>
  raw?: string
}

export type DocStatus = 'ok' | 'pending' | 'gone' | 'unsupported' | 'unknown_session'

export interface Page {
  sessionId: string
  status: DocStatus
  path?: string
  items: Item[]
  hasMore: boolean
}

export type ServerMessage =
  | { type: 'hello'; version: string }
  | { type: 'sessions'; state: State }
  | { type: 'transcript.page'; page: Page }
  | { type: 'transcript.append'; sessionId: string; items: Item[] }
  | { type: 'error'; message: string }

export interface DialogOption {
  index: number
  label: string
  selected: boolean
  free?: boolean
}

export interface Dialog {
  question?: string
  options: DialogOption[]
  footer: string
}

export interface Screen {
  text: string
  dialog?: Dialog
}

export type Answer = { action: 'choose'; index: number; label: string } | { action: 'key'; key: 'Up' | 'Down' | 'Enter' | 'Escape' }

export interface SearchHit {
  sessionId: string
  sessionName: string
  project: string
  archived?: boolean
  uuid: string
  kind: ItemKind
  tool?: string
  time?: string
  snippet: string
}

export interface SearchResult {
  query: string
  hits: SearchHit[]
  truncated: boolean
  searched: number
}
