import type { Item } from '../types'

// Pin marks a prompt or reply worth coming back to.
export interface Pin {
  key: string
  kind: 'user' | 'text'
  excerpt: string
  time?: string
}

// Notebook is what the reader keeps for one conversation.
export interface Notebook {
  note: string
  pins: Pin[]
}

export const emptyNotebook: Notebook = { note: '', pins: [] }

type KV = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>

const prefix = 'agent-desk:notebook:'

// pinKey identifies an item across reloads. Claude Code writes one content
// block per record, so a record's uuid and the item kind are enough.
export function pinKey(item: Item): string | null {
  if (!item.uuid || (item.kind !== 'user' && item.kind !== 'text')) return null
  return `${item.uuid}:${item.kind}`
}

export function pinFrom(item: Item): Pin | null {
  const key = pinKey(item)
  if (!key) return null
  const line = (item.text ?? '').replace(/\s+/g, ' ').trim()
  return { key, kind: item.kind === 'user' ? 'user' : 'text', excerpt: line.length > 160 ? line.slice(0, 160) + '…' : line, time: item.time }
}

// togglePin pins an item, or unpins it when already pinned. Pins stay in
// transcript order by time.
export function togglePin(nb: Notebook, item: Item): Notebook {
  const pin = pinFrom(item)
  if (!pin) return nb
  if (nb.pins.some((p) => p.key === pin.key)) return { ...nb, pins: nb.pins.filter((p) => p.key !== pin.key) }
  const pins = [...nb.pins, pin].sort((a, b) => (a.time ?? '').localeCompare(b.time ?? ''))
  return { ...nb, pins }
}

function isPin(v: unknown): v is Pin {
  if (typeof v !== 'object' || v === null) return false
  const p = v as Record<string, unknown>
  return typeof p.key === 'string' && (p.kind === 'user' || p.kind === 'text') && typeof p.excerpt === 'string'
}

export function loadNotebook(store: KV, conversation: string): Notebook {
  try {
    const v = JSON.parse(store.getItem(prefix + conversation) ?? 'null') as unknown
    if (typeof v !== 'object' || v === null) return emptyNotebook
    const o = v as Record<string, unknown>
    return { note: typeof o.note === 'string' ? o.note : '', pins: Array.isArray(o.pins) ? o.pins.filter(isPin) : [] }
  } catch {
    return emptyNotebook
  }
}

// saveNotebook stores a notebook, or forgets it once it is empty.
export function saveNotebook(store: KV, conversation: string, nb: Notebook): void {
  if (nb.note.trim() === '' && nb.pins.length === 0) store.removeItem(prefix + conversation)
  else store.setItem(prefix + conversation, JSON.stringify(nb))
}
