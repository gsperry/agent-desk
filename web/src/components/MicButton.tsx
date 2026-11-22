import type { DictationPhase } from '../dictate'

const labels: Record<DictationPhase, string> = {
  idle: 'Dictate',
  starting: 'Starting dictation',
  listening: 'Stop dictation',
  stopping: 'Finishing dictation',
}

// MicButton starts and stops voice input. While it listens it is lit, and
// what is heard appears in the box.
export function MicButton({ phase, onToggle }: { phase: DictationPhase; onToggle: () => void }) {
  return (
    <button
      type="button"
      className={`secondary mic-button ${phase}`}
      aria-label={labels[phase]}
      aria-pressed={phase !== 'idle'}
      title={phase === 'idle' ? 'Dictate: speech is turned into text on this Mac' : labels[phase]}
      disabled={phase === 'stopping'}
      onMouseDown={(e) => {
        // Keep the caret where it is in the box.
        e.preventDefault()
      }}
      onClick={onToggle}
    >
      <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
        <rect x="9" y="3" width="6" height="11" rx="3" fill="currentColor" />
        <path d="M6 11a6 6 0 0 0 12 0M12 17v4M9 21h6" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
      </svg>
    </button>
  )
}
