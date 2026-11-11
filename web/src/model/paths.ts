export interface PathRef {
  path: string
  line?: number
}

// Extensions that make a bare file name (no slash) read as a path.
const sourceExtensions = new Set([
  'c', 'cc', 'cpp', 'cs', 'css', 'go', 'h', 'hpp', 'html', 'java', 'js', 'json', 'jsonl', 'jsx', 'kt', 'lock', 'lua', 'md',
  'mjs', 'mod', 'php', 'proto', 'py', 'rb', 'rs', 'scss', 'sh', 'sql', 'sum', 'swift', 'toml', 'ts', 'tsx', 'txt', 'vue', 'xml',
  'yaml', 'yml',
])

const pathPattern = /^((?:~\/|\.{1,2}\/|\/)?(?:[\w.@+-]+\/)*[\w.@+-]+)(?::(\d+))?(?::\d+)?$/

// parsePathRef reads text as a file path with an optional :line or
// :line:col suffix. It accepts paths with a slash, or bare names with a
// common source extension, and rejects URLs and numbers.
export function parsePathRef(text: string): PathRef | null {
  const t = text.trim()
  if (t.length < 3 || t.length > 400 || t.includes('://') || /\s/.test(t)) return null
  const m = pathPattern.exec(t)
  if (!m?.[1]) return null
  const path = m[1]
  const base = path.split('/').pop() ?? ''
  const ext = base.includes('.') ? (base.split('.').pop() ?? '').toLowerCase() : ''
  if (/^[\d.]+$/.test(base)) return null
  const hasSlash = path.includes('/')
  if (!hasSlash && !sourceExtensions.has(ext)) return null
  if (hasSlash && path.endsWith('/')) return null
  // A relative path with a slash still needs a file-like last segment, so
  // "and/or" does not become a link.
  if (hasSlash && !path.startsWith('/') && !path.startsWith('~') && !path.startsWith('.') && !ext) return null
  const line = m[2] ? Number(m[2]) : undefined
  return line ? { path, line } : { path }
}

export interface ToolInputs {
  file_path?: unknown
  notebook_path?: unknown
  offset?: unknown
}

// toolPathRef is the file a tool call works on, with the line it starts at
// when the call says (a Read offset, or the first hunk of an edit).
export function toolPathRef(input: ToolInputs, firstHunkLine?: number): PathRef | null {
  const p = typeof input.file_path === 'string' ? input.file_path : typeof input.notebook_path === 'string' ? input.notebook_path : ''
  if (!p) return null
  const line = firstHunkLine ?? (typeof input.offset === 'number' && input.offset > 0 ? input.offset : undefined)
  return line ? { path: p, line } : { path: p }
}
