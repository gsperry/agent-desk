import type { DocStatus, Item, Page, ServerMessage, State } from '../types'

export interface Doc {
  status: DocStatus | 'loading'
  path?: string
  items: Item[]
  hasMore: boolean
  loadingOlder: boolean
}

export interface AppData {
  sessions: State | null
  docs: Record<string, Doc>
}

export const emptyData: AppData = { sessions: null, docs: {} }

export const loadingDoc: Doc = { status: 'loading', items: [], hasMore: false, loadingOlder: false }

// applyMessage folds one server message into the client's data.
export function applyMessage(data: AppData, msg: ServerMessage): AppData {
  switch (msg.type) {
    case 'sessions':
      return { ...data, sessions: msg.state }
    case 'transcript.page':
      return withDoc(data, msg.page.sessionId, () => fromPage(msg.page))
    case 'transcript.append':
      return withDoc(data, msg.sessionId, (doc) => {
        const last = doc.items.at(-1)?.seq ?? -1
        const fresh = msg.items.filter((it) => it.seq > last)
        return fresh.length ? { ...doc, items: [...doc.items, ...fresh] } : doc
      })
    default:
      return data
  }
}

// prependOlder merges an older page fetched for scrollback.
export function prependOlder(data: AppData, page: Page): AppData {
  return withDoc(data, page.sessionId, (doc) => {
    const first = doc.items[0]?.seq ?? Infinity
    const older = page.items.filter((it) => it.seq < first)
    return { ...doc, items: [...older, ...doc.items], hasMore: page.hasMore, loadingOlder: false }
  })
}

export function setLoadingOlder(data: AppData, sessionId: string, loading: boolean): AppData {
  return withDoc(data, sessionId, (doc) => ({ ...doc, loadingOlder: loading }))
}

function fromPage(page: Page): Doc {
  return { status: page.status, path: page.path, items: page.items, hasMore: page.hasMore, loadingOlder: false }
}

function withDoc(data: AppData, id: string, f: (d: Doc) => Doc): AppData {
  const doc = data.docs[id] ?? loadingDoc
  const next = f(doc)
  return next === doc ? data : { ...data, docs: { ...data.docs, [id]: next } }
}
