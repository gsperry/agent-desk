import type { Pending } from '../model/pending'

const labels: Record<Pending['state'], string> = {
  sending: 'Sending…',
  sent: 'Sent, waiting for the transcript',
  maybe: 'May have been sent, waiting for the transcript',
  unconfirmed: 'Not seen in the transcript yet',
  failed: 'Not sent',
}

interface Props {
  pending: Pending[]
  onEdit: (p: Pending) => void
  onRetry: (p: Pending) => void
  onDismiss: (p: Pending) => void
}

export function PendingReplies({ pending, onEdit, onRetry, onDismiss }: Props) {
  return (
    <>
      {pending.map((p) => (
        <section key={p.clientId} className={`card user pending pending-${p.state}`} aria-live="polite">
          <header className="card-head">
            <span>You</span>
            <span className="chip">{labels[p.state]}</span>
          </header>
          <div className="prompt-text">{p.text}</div>
          {p.message && <p className="pending-message">{p.message}</p>}
          {(p.state === 'failed' || p.state === 'unconfirmed') && (
            <div className="pending-actions">
              {p.state === 'failed' && (
                <button
                  type="button"
                  className="link-button"
                  onClick={() => {
                    onRetry(p)
                  }}
                >
                  Retry
                </button>
              )}
              <button
                type="button"
                className="link-button"
                onClick={() => {
                  onEdit(p)
                }}
              >
                Edit
              </button>
              <button
                type="button"
                className="link-button"
                onClick={() => {
                  onDismiss(p)
                }}
              >
                Dismiss
              </button>
            </div>
          )}
        </section>
      ))}
    </>
  )
}
