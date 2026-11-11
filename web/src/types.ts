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
  lastActivity?: string
}

export interface GroupView {
  path: string
  directory?: string
}

export interface State {
  managerAwake: boolean
  insideSession: boolean
  error?: { kind: 'not_installed' | 'failed'; message: string }
  sessions: SessionView[]
  groups?: GroupView[]
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

export interface SpawnOptions {
  tools: string[]
  defaultTool: string
  worktreeDefault: boolean
  groups: GroupView[]
}

export interface SpawnRequest {
  name: string
  tool: string
  group: string
  directory: string
  worktree: boolean
  prompt: string
  model: string
  effort: string
  // resume is a saved Claude Code conversation to reopen.
  resume?: string
}

export type SessionAction = { action: 'kill' | 'revive' | 'archive' | 'restore' } | { action: 'move'; group: string }

export interface SubAgent {
  id: string
  toolUseId: string
  type: string
  description: string
  depth: number
  model?: string
  status: 'running' | 'completed' | 'failed' | 'stopped' | (string & {})
  started?: string
  lastActivity?: string
}

export interface Summary {
  activity?: string
  activityKind?: 'tool' | 'reply' | 'prompt' | (string & {})
  lastReply?: string
  lastPrompt?: string
  promptAt?: string
  contextTokens?: number
  model?: string
  updated?: string
}

export interface ChangedFile {
  path: string
  oldPath?: string
  status: 'modified' | 'added' | 'deleted' | 'renamed' | 'untracked' | (string & {})
  added: number
  removed: number
  binary?: boolean
  hunks: { oldStart: number; oldLines: number; newStart: number; newLines: number; lines: string[] }[]
}

export interface Changes {
  repo: string
  branch: string
  mode: 'uncommitted' | 'branch'
  base?: string
  files: ChangedFile[]
  truncated?: boolean
}

// Conversation is a Claude Code transcript on disk, whether or not
// agent-manager runs it.
export interface Conversation {
  id: string
  directory?: string
  branch?: string
  title?: string
  firstPrompt?: string
  started?: string
  modified: string
  size: number
}

export interface UsageTokens {
  input: number
  output: number
  cacheRead: number
  cacheWrite: number
}

export interface UsageBucket extends UsageTokens {
  replies: number
  prompts: number
  workingMs: number
  waitingMs: number
  costUSD: number
}

export interface UsageReport {
  from: string
  to: string
  totals: UsageBucket
  days: (UsageBucket & { day: string })[]
  models: (UsageTokens & { model: string })[]
  conversations: (UsageBucket & { id: string; subagentTokens: number; last?: string })[]
  files: number
}

// CatchUp is a catch-up summary written by the user's Claude Code CLI.
export interface CatchUp {
  text: string
  model: string
  inputTokens: number
  outputTokens: number
  costUSD: number
  items: number
  cached?: boolean
}
