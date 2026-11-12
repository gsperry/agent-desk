import { useEffect, useMemo, useState } from 'react'
import { formatTokens, liveActivity } from '../model/activity'
import { formatDuration } from '../model/transcript'
import type { Item } from '../types'

interface Props {
  items: Item[]
  // onInterrupt stops the turn, as Esc does; absent when not allowed.
  onInterrupt?: () => Promise<string | null>
}

// LiveActivity is the row above the composer while an agent works: how
// long the turn has run, the tokens written, and the step it is on.
export function LiveActivity({ items, onInterrupt }: Props) {
  const activity = useMemo(() => liveActivity(items), [items])
  const [now, setNow] = useState(() => Date.now())
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState<string | null>(null)

  useEffect(() => {
    const t = setInterval(() => {
      setNow(Date.now())
    }, 1000)
    return () => {
      clearInterval(t)
    }
  }, [])

  const parts = ['Working']
  if (activity.startedAt !== undefined) parts.push(formatDuration(Math.max(0, now - activity.startedAt)))
  if (activity.outputTokens > 0) parts.push(`↓ ${formatTokens(activity.outputTokens)} tokens`)

  return (
    <div className="live-activity" role="status" aria-label="Agent activity">
      <span className="live-activity-spinner" aria-hidden="true" />
      <div className="live-activity-text">
        <span className="live-activity-head">{parts.join(' · ')}</span>
        {activity.step && (
          <span className="live-activity-step" title={activity.step}>
            {activity.step}
          </span>
        )}
        {message && <span className="form-error">{message}</span>}
      </div>
      {onInterrupt && (
        <button
          type="button"
          className="secondary live-interrupt"
          disabled={busy}
          title="Stop the current turn, as Esc does in the terminal"
          onClick={() => {
            setBusy(true)
            setMessage(null)
            void onInterrupt().then((err) => {
              setBusy(false)
              setMessage(err)
            })
          }}
        >
          Interrupt
        </button>
      )}
    </div>
  )
}
