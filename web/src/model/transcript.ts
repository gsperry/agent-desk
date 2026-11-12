import type { Item } from '../types'

export type Step =
  | { kind: 'text'; item: Item }
  | { kind: 'thinking'; item: Item }
  | { kind: 'tool'; use: Item; result?: Item }
  | { kind: 'orphan_result'; item: Item }
  | { kind: 'raw'; item: Item }

export type Block =
  | { kind: 'user'; key: string; item: Item }
  | { kind: 'peer'; key: string; item: Item }
  | { kind: 'notice'; key: string; item: Item }
  | { kind: 'compact'; key: string; boundary?: Item; summary?: Item }
  | { kind: 'turn'; key: string; steps: Step[]; durationMs?: number }
  | { kind: 'system'; key: string; item: Item }
  | { kind: 'image'; key: string; item: Item }
  | { kind: 'raw'; key: string; item: Item }

// Notices the harness injects that are noise for a reader.
export const hiddenNotices = new Set([
  'system-reminder',
  'local-command-caveat',
  'injected',
  'ide_opened_file',
  'ide_selection',
  'user-prompt-submit-hook',
  'agent-manager',
])

// System subtypes with nothing to read.
const hiddenSystem = new Set(['stop_hook_summary', 'turn_duration'])

export function buildBlocks(items: Item[]): Block[] {
  const blocks: Block[] = []
  const tools = new Map<string, Extract<Step, { kind: 'tool' }>>()
  let turn: Extract<Block, { kind: 'turn' }> | null = null

  const closeTurn = () => {
    turn = null
  }
  const currentTurn = (item: Item) => {
    if (!turn) {
      turn = { kind: 'turn', key: `t${String(item.seq)}`, steps: [] }
      blocks.push(turn)
    }
    return turn
  }
  const top = (b: Block) => {
    closeTurn()
    blocks.push(b)
  }

  for (const item of items) {
    const key = `i${String(item.seq)}`
    switch (item.kind) {
      case 'text':
        currentTurn(item).steps.push({ kind: 'text', item })
        break
      case 'thinking':
        currentTurn(item).steps.push({ kind: 'thinking', item })
        break
      case 'tool_use': {
        const step = { kind: 'tool' as const, use: item }
        currentTurn(item).steps.push(step)
        if (item.tool?.id) tools.set(item.tool.id, step)
        break
      }
      case 'tool_result': {
        const step = item.result ? tools.get(item.result.toolUseId) : undefined
        if (step) step.result = item
        else currentTurn(item).steps.push({ kind: 'orphan_result', item })
        break
      }
      case 'user':
        top({ kind: 'user', key, item })
        break
      case 'peer':
        top({ kind: 'peer', key, item })
        break
      case 'notice':
        if (!hiddenNotices.has(item.label ?? '')) top({ kind: 'notice', key, item })
        break
      case 'compact_boundary':
        top({ kind: 'compact', key, boundary: item })
        break
      case 'compact_summary': {
        const last = blocks.at(-1)
        if (last?.kind === 'compact' && !last.summary) last.summary = item
        else top({ kind: 'compact', key, summary: item })
        closeTurn()
        break
      }
      case 'system':
        if (item.label === 'turn_duration') {
          const t = turn as Extract<Block, { kind: 'turn' }> | null
          const d = item.meta?.durationMs
          if (t && typeof d === 'number') t.durationMs = d
          closeTurn()
        } else if (!hiddenSystem.has(item.label ?? '')) {
          top({ kind: 'system', key, item })
        }
        break
      case 'image':
        top({ kind: 'image', key, item })
        break
      case 'raw':
        if (item.messageId) currentTurn(item).steps.push({ kind: 'raw', item })
        else top({ kind: 'raw', key, item })
        break
      case 'attachment':
      case 'meta':
        break
    }
  }
  return blocks
}

export interface Header {
  title?: string
  prLinks: { url: string; number?: number; repository?: string }[]
  permissionMode?: string
}

export function buildHeader(items: Item[]): Header {
  let custom: string | undefined
  let ai: string | undefined
  let permissionMode: string | undefined
  const prs = new Map<string, Header['prLinks'][number]>()
  for (const it of items) {
    if (it.kind !== 'meta') continue
    const m = it.meta ?? {}
    switch (it.label) {
      case 'custom-title':
        if (typeof m.customTitle === 'string') custom = m.customTitle
        break
      case 'ai-title':
        if (typeof m.aiTitle === 'string') ai = m.aiTitle
        break
      case 'permission-mode':
        if (typeof m.permissionMode === 'string') permissionMode = m.permissionMode
        break
      case 'pr-link':
        // Transcript data decides the href, so only https links render.
        if (typeof m.prUrl === 'string' && isHttps(m.prUrl)) {
          prs.set(m.prUrl, {
            url: m.prUrl,
            number: typeof m.prNumber === 'number' ? m.prNumber : undefined,
            repository: typeof m.prRepository === 'string' ? m.prRepository : undefined,
          })
        }
        break
    }
  }
  return { title: custom ?? ai, prLinks: [...prs.values()], permissionMode }
}

function isHttps(u: string): boolean {
  try {
    return new URL(u).protocol === 'https:'
  } catch {
    return false
  }
}

function str(v: unknown): string | undefined {
  return typeof v === 'string' && v !== '' ? v : undefined
}

function firstLine(s: string): string {
  const line = s.split('\n', 1)[0] ?? ''
  return line.length > 160 ? line.slice(0, 160) + '…' : line
}

export interface ToolLabel {
  name: string
  summary: string
}

// toolLabel is the one-line description a collapsed tool call shows.
// mcpName shortens an MCP tool name for display: "plugin_x_playwright" is
// the playwright server, and underscores in the tool read as spaces.
function mcpName(server: string, tool: string): string {
  const short = server.replace(/^plugin_/, '').split('_').at(-1) ?? server
  return `${short} · ${tool.replace(/_/g, ' ')}`
}

// relative shows a path inside the session's directory relative to it.
export function relativePath(path: string, dir?: string): string {
  if (!dir) return path
  const base = dir.endsWith('/') ? dir : dir + '/'
  return path.startsWith(base) ? path.slice(base.length) : path
}

export function toolLabel(use: Item, dir?: string): ToolLabel {
  const name = use.tool?.name ?? 'tool'
  const input = (typeof use.tool?.input === 'object' && use.tool.input !== null ? use.tool.input : {}) as Record<string, unknown>
  const mcp = /^mcp__(.+?)__(.+)$/.exec(name)
  const display = mcp ? mcpName(mcp[1] ?? '', mcp[2] ?? '') : name
  let summary: string | undefined
  switch (name) {
    case 'Bash':
      summary = str(input.description) ?? str(input.command)
      break
    case 'Read':
    case 'Edit':
    case 'MultiEdit':
    case 'Write':
    case 'NotebookEdit':
      summary = relativePath(str(input.file_path) ?? str(input.notebook_path) ?? '', dir) || undefined
      break
    case 'Grep':
      summary = [str(input.pattern), str(input.path)].filter(Boolean).join(' in ')
      break
    case 'Glob':
      summary = str(input.pattern)
      break
    case 'Task':
    case 'Agent':
      summary = str(input.description) ?? str(input.subagent_type)
      break
    case 'WebFetch':
      summary = str(input.url)
      break
    case 'WebSearch':
      summary = str(input.query)
      break
    case 'TodoWrite':
      summary = Array.isArray(input.todos) ? `${String(input.todos.length)} todos` : undefined
      break
    case 'AskUserQuestion': {
      const qs = Array.isArray(input.questions) ? (input.questions as Record<string, unknown>[]) : []
      summary = str(qs[0]?.question)
      break
    }
  }
  summary ??= Object.values(input).map(str).find(Boolean)
  return { name: display, summary: firstLine(summary ?? '') }
}

export function formatDuration(ms: number): string {
  const s = Math.round(ms / 1000)
  if (s < 60) return `${String(s)}s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${String(m)}m ${String(s % 60)}s`
  return `${String(Math.floor(m / 60))}h ${String(m % 60)}m`
}
