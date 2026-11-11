// A reading position is the block at the top of the view and how far
// below the view's top edge it starts. Null means following the latest.
export interface ReadingPosition {
  key: string
  offset: number
}

type KV = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>

const prefix = 'agent-desk:scroll:'

// savePosition remembers where the reader was in a session, for this tab.
export function savePosition(store: KV, sessionId: string, pos: ReadingPosition | null): void {
  if (pos) store.setItem(prefix + sessionId, JSON.stringify(pos))
  else store.removeItem(prefix + sessionId)
}

export function loadPosition(store: KV, sessionId: string): ReadingPosition | null {
  try {
    const v = JSON.parse(store.getItem(prefix + sessionId) ?? 'null') as unknown
    if (typeof v !== 'object' || v === null) return null
    const o = v as Record<string, unknown>
    return typeof o.key === 'string' && typeof o.offset === 'number' ? { key: o.key, offset: o.offset } : null
  } catch {
    return null
  }
}

// anchorFor picks the first block still visible at the top of the view.
// Tops and bottoms are relative to the view's top edge.
export function anchorFor(blocks: { key: string; top: number; bottom: number }[]): ReadingPosition | null {
  const b = blocks.find((x) => x.bottom > 0)
  return b ? { key: b.key, offset: b.top } : null
}
