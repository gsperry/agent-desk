import { Fragment, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { filterItems, type ActivityFilter } from '../model/filter'
import { blockSeqs } from '../model/highlight'
import { anchorFor, loadPosition, savePosition } from '../model/scroll'
import { dividerSeq } from '../model/seen'
import { buildBlocks } from '../model/transcript'
import { BlockView } from '../render/Blocks'
import type { Item } from '../types'

interface Props {
  sessionId: string
  items: Item[]
  hasMore: boolean
  loadingOlder: boolean
  live: boolean
  onLoadOlder: () => void
  // trailing renders after the transcript, such as replies not yet recorded.
  trailing?: ReactNode
  // followKey changes when the reader acts (sends a reply), which brings
  // the view back to the latest output.
  followKey?: number
  markAtOpen?: { seq: number }
  // filterable shows the activity filter bar.
  filterable?: boolean
}

// maxRestorePages is how many earlier pages a returning reader's position
// may load before giving up and showing the latest.
const maxRestorePages = 5

// Within this many pixels of the bottom counts as following the latest.
const followSlack = 80

export function Transcript({ sessionId, items: allItems, hasMore, loadingOlder, live, onLoadOlder, trailing, followKey, markAtOpen, filterable }: Props) {
  const [filter, setFilter] = useState<ActivityFilter>('all')
  const items = useMemo(() => filterItems(allItems, filter), [allItems, filter])
  const scroller = useRef<HTMLDivElement>(null)
  const content = useRef<HTMLDivElement>(null)
  // A session opened again returns to where the reader left it, unless
  // they were following the latest output.
  const [restore] = useState(() => loadPosition(sessionStorage, sessionId))
  const restoring = useRef({ done: restore === null, attempts: 0 })
  const following = useRef(restore === null)
  const [atBottom, setAtBottom] = useState(restore === null)
  const [unseen, setUnseen] = useState(0)
  const firstSeq = items[0]?.seq
  const lastSeq = items.at(-1)?.seq
  const prev = useRef({ sessionId, firstSeq, lastSeq, height: 0, count: items.length })

  const blocks = useMemo(() => buildBlocks(items), [items])
  const newFrom = useMemo(
    () =>
      dividerSeq(
        markAtOpen,
        items.map((i) => i.seq),
      ),
    [markAtOpen, items],
  )
  const dividerKey = useMemo(() => {
    if (newFrom === null) return null
    return blocks.find((b) => blockSeqs(b).some((s) => s >= newFrom))?.key ?? null
  }, [blocks, newFrom])

  // pinned is where the view last put itself at the bottom, so a scroll
  // event from content growing below is not mistaken for the reader
  // scrolling up.
  const pinned = useRef(0)
  const scrollToBottom = () => {
    const el = scroller.current
    if (!el) return
    el.scrollTop = el.scrollHeight
    pinned.current = el.scrollTop
  }

  // Keep the reader anchored: follow appends at the bottom, hold position
  // when older items are prepended, and start a new session at the end.
  useLayoutEffect(() => {
    const el = scroller.current
    if (!el) return
    const p = prev.current
    if (p.sessionId !== sessionId) {
      following.current = true
      setUnseen(0)
      scrollToBottom()
    } else if (firstSeq !== p.firstSeq && lastSeq === p.lastSeq) {
      el.scrollTop += el.scrollHeight - p.height
    } else if (lastSeq !== p.lastSeq) {
      if (following.current) scrollToBottom()
      else setUnseen((n) => n + Math.max(items.length - p.count, 1))
    }
    prev.current = { sessionId, firstSeq, lastSeq, height: el.scrollHeight, count: items.length }
  }, [sessionId, firstSeq, lastSeq, items.length])

  // A session the reader was following opens at the latest output.
  useLayoutEffect(() => {
    if (restore === null) scrollToBottom()
  }, [restore])

  // Bring the saved reading position back once its block is loaded,
  // loading earlier pages a few times if it is older than the window.
  useLayoutEffect(() => {
    const r = restoring.current
    const el = scroller.current
    if (r.done || !restore || !el || items.length === 0) return
    const node = content.current?.querySelector(`[data-block="${CSS.escape(restore.key)}"]`)
    if (node) {
      el.scrollTop += node.getBoundingClientRect().top - el.getBoundingClientRect().top - restore.offset
      prev.current.height = el.scrollHeight
      r.done = true
      return
    }
    if (hasMore && r.attempts < maxRestorePages) {
      if (!loadingOlder) {
        r.attempts++
        onLoadOlder()
      }
      return
    }
    r.done = true
    following.current = true
    setAtBottom(true)
    scrollToBottom()
  }, [restore, items.length, firstSeq, hasMore, loadingOlder, onLoadOlder])

  // remember saves the reading position, at most once a frame.
  const frame = useRef(0)
  const remember = () => {
    if (!restoring.current.done || frame.current) return
    frame.current = requestAnimationFrame(() => {
      frame.current = 0
      const el = scroller.current
      const box = content.current
      if (!el || !box) return
      if (following.current) {
        savePosition(sessionStorage, sessionId, null)
        return
      }
      const top = el.getBoundingClientRect().top
      const blocks = [...box.querySelectorAll<HTMLElement>('[data-block]')].map((n) => {
        const r = n.getBoundingClientRect()
        return { key: n.dataset.block ?? '', top: r.top - top, bottom: r.bottom - top }
      })
      savePosition(sessionStorage, sessionId, anchorFor(blocks))
    })
  }
  useEffect(
    () => () => {
      cancelAnimationFrame(frame.current)
    },
    [],
  )

  // Highlighting and expanding change heights after render.
  useEffect(() => {
    const el = content.current
    if (!el) return
    const ro = new ResizeObserver(() => {
      if (following.current) scrollToBottom()
      prev.current.height = scroller.current?.scrollHeight ?? 0
    })
    ro.observe(el)
    // The view itself shrinks when the terminal panel opens below it.
    if (scroller.current) ro.observe(scroller.current)
    return () => {
      ro.disconnect()
    }
  }, [])

  // A send brings the reader back to the latest output.
  const [seenFollowKey, setSeenFollowKey] = useState(followKey)
  if (followKey !== seenFollowKey) {
    setSeenFollowKey(followKey)
    setAtBottom(true)
    setUnseen(0)
  }
  // Only an actual send moves the view, not opening the session.
  const followKeyAtOpen = useRef(followKey)
  useLayoutEffect(() => {
    if (followKey === undefined || followKey === followKeyAtOpen.current) return
    followKeyAtOpen.current = undefined
    following.current = true
    scrollToBottom()
  }, [followKey])

  const onScroll = () => {
    const el = scroller.current
    if (!el) return
    const near = el.scrollHeight - el.scrollTop - el.clientHeight < followSlack
    if (following.current && !near && el.scrollTop >= pinned.current - 2) {
      // The content grew below while following; the reader did not move.
      scrollToBottom()
      return
    }
    following.current = near
    setAtBottom(near)
    if (near) setUnseen(0)
    remember()
    if (el.scrollTop < 400 && hasMore && !loadingOlder && restoring.current.done) onLoadOlder()
  }

  const jump = () => {
    savePosition(sessionStorage, sessionId, null)
    following.current = true
    setAtBottom(true)
    setUnseen(0)
    scrollToBottom()
  }

  return (
    <div className="transcript-wrap">
      <div className="transcript" ref={scroller} onScroll={onScroll}>
        <div className="reading" ref={content}>
          {filterable && (
            <div className="activity-bar">
              <div className="activity-filter segmented" role="radiogroup" aria-label="Show">
                {(
                  [
                    ['all', 'All'],
                    ['commands', 'Commands'],
                    ['files', 'File changes'],
                    ['errors', 'Errors'],
                  ] as const
                ).map(([f, label]) => (
                  <button
                    key={f}
                    type="button"
                    role="radio"
                    aria-checked={filter === f}
                    className={filter === f ? 'active' : undefined}
                    onClick={() => {
                      setFilter(f)
                    }}
                  >
                    {label}
                  </button>
                ))}
              </div>
            </div>
          )}
          {filter !== 'all' && items.length <= allItems.filter((i) => i.kind === 'user').length && <p className="muted">Nothing of this kind in the loaded history.</p>}
          {hasMore && <p className="muted center">{loadingOlder ? 'Loading earlier messages…' : 'Scroll up for earlier messages'}</p>}
          {blocks.map((b) => (
            <Fragment key={b.key}>
              {b.key === dividerKey && (
                <div className="new-divider" role="separator">
                  <span>New since you last looked</span>
                </div>
              )}
              <div data-block={b.key}>
                <BlockView block={b} live={live} />
              </div>
            </Fragment>
          ))}
          {trailing}
        </div>
      </div>
      {!atBottom && (
        <button type="button" className="jump" onClick={jump}>
          {unseen > 0 ? `${String(unseen)} new · Jump to latest` : 'Jump to latest'}
        </button>
      )}
    </div>
  )
}
