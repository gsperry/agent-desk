import { useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react'
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
}

// Within this many pixels of the bottom counts as following the latest.
const followSlack = 80

export function Transcript({ sessionId, items, hasMore, loadingOlder, live, onLoadOlder, trailing, followKey }: Props) {
  const scroller = useRef<HTMLDivElement>(null)
  const content = useRef<HTMLDivElement>(null)
  const following = useRef(true)
  const [atBottom, setAtBottom] = useState(true)
  const [unseen, setUnseen] = useState(0)
  const firstSeq = items[0]?.seq
  const lastSeq = items.at(-1)?.seq
  const prev = useRef({ sessionId, firstSeq, lastSeq, height: 0, count: items.length })

  const blocks = useMemo(() => buildBlocks(items), [items])

  const scrollToBottom = () => {
    const el = scroller.current
    if (el) el.scrollTop = el.scrollHeight
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

  // Highlighting and expanding change heights after render.
  useEffect(() => {
    const el = content.current
    if (!el) return
    const ro = new ResizeObserver(() => {
      if (following.current) scrollToBottom()
      prev.current.height = scroller.current?.scrollHeight ?? 0
    })
    ro.observe(el)
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
  useLayoutEffect(() => {
    if (followKey === undefined) return
    following.current = true
    scrollToBottom()
  }, [followKey])

  const onScroll = () => {
    const el = scroller.current
    if (!el) return
    const near = el.scrollHeight - el.scrollTop - el.clientHeight < followSlack
    following.current = near
    setAtBottom(near)
    if (near) setUnseen(0)
    if (el.scrollTop < 400 && hasMore && !loadingOlder) onLoadOlder()
  }

  const jump = () => {
    following.current = true
    setAtBottom(true)
    setUnseen(0)
    scrollToBottom()
  }

  return (
    <div className="transcript-wrap">
      <div className="transcript" ref={scroller} onScroll={onScroll}>
        <div className="reading" ref={content}>
          {hasMore && <p className="muted center">{loadingOlder ? 'Loading earlier messages…' : 'Scroll up for earlier messages'}</p>}
          {blocks.map((b) => (
            <BlockView key={b.key} block={b} live={live} />
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
