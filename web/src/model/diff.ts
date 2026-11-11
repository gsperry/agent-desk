export interface Hunk {
  oldStart: number
  oldLines: number
  newStart: number
  newLines: number
  lines: unknown[]
}

export interface DiffRow {
  type: 'add' | 'del' | 'ctx' | 'gap'
  oldNo?: number
  newNo?: number
  text: string
}

function isHunk(v: unknown): v is Hunk {
  if (typeof v !== 'object' || v === null) return false
  const h = v as Record<string, unknown>
  return typeof h.oldStart === 'number' && typeof h.newStart === 'number' && Array.isArray(h.lines)
}

// patchRows turns a structuredPatch into numbered rows, with a gap row
// between hunks. Malformed input yields no rows.
export function patchRows(patch: unknown): DiffRow[] {
  if (!Array.isArray(patch)) return []
  const rows: DiffRow[] = []
  patch.filter(isHunk).forEach((h, i) => {
    if (i > 0) rows.push({ type: 'gap', text: '' })
    let oldNo = h.oldStart
    let newNo = h.newStart
    for (const raw of h.lines) {
      const line = String(raw)
      const sign = line[0]
      const text = line.slice(1)
      if (sign === '+') rows.push({ type: 'add', newNo: newNo++, text })
      else if (sign === '-') rows.push({ type: 'del', oldNo: oldNo++, text })
      else if (sign === '\\') continue
      else rows.push({ type: 'ctx', oldNo: oldNo++, newNo: newNo++, text })
    }
  })
  return rows
}

// replaceRows is the fallback when there is no structured patch yet: the
// old text removed, then the new text added.
export function replaceRows(oldText: string, newText: string): DiffRow[] {
  const split = (s: string) => (s === '' ? [] : s.replace(/\n$/, '').split('\n'))
  return [
    ...split(oldText).map((text): DiffRow => ({ type: 'del', text })),
    ...split(newText).map((text): DiffRow => ({ type: 'add', text })),
  ]
}

export function diffStats(rows: DiffRow[]): { added: number; removed: number } {
  let added = 0
  let removed = 0
  for (const r of rows) {
    if (r.type === 'add') added++
    else if (r.type === 'del') removed++
  }
  return { added, removed }
}
