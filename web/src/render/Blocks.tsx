import { memo, useState, type ReactNode } from 'react'
import { pinKey } from '../model/pins'
import { riskyCommand } from '../model/filter'
import { bashFiles } from '../model/diff'
import { groupSteps, groupSummary } from '../model/groups'
import { toolCategory } from '../model/viewprefs'
import { formatDuration, type Block, type Step } from '../model/transcript'
import { usePins } from '../pins-context'
import { useViewPrefs } from '../viewprefs-context'
import type { Item } from '../types'
import { Markdown } from './Markdown'
import { ToolCall } from './ToolCall'

function time(item: Item): string {
  if (!item.time) return ''
  const d = new Date(item.time)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

export const BlockView = memo(function BlockView({ block, live }: { block: Block; live: boolean }) {
  switch (block.kind) {
    case 'user':
      return <UserPrompt item={block.item} />
    case 'peer':
      return (
        <section className="card peer">
          <header className="card-head">Message from another agent</header>
          <Markdown text={block.item.text ?? ''} />
        </section>
      )
    case 'notice':
      return <Notice item={block.item} />
    case 'compact':
      return <Compact summary={block.summary} />
    case 'turn':
      return <Turn steps={block.steps} durationMs={block.durationMs} live={live} />
    case 'system':
      return <SystemLine item={block.item} />
    case 'image':
      return <p className="notice">Image attached ({block.item.label ?? 'image'})</p>
    case 'raw':
      return <RawEvent item={block.item} />
  }
})

// PinButton pins a prompt or reply when the view keeps pins.
function PinButton({ item }: { item: Item }) {
  const pins = usePins()
  if (!pins || !pinKey(item)) return null
  const on = pins.isPinned(item)
  return (
    <button
      type="button"
      className={on ? 'pin-button pinned' : 'pin-button'}
      aria-label={on ? 'Unpin' : 'Pin'}
      aria-pressed={on}
      title={on ? 'Unpin' : 'Pin'}
      onClick={() => {
        pins.toggle(item)
      }}
    >
      📌
    </button>
  )
}

// Prompts longer than this are clamped until expanded.
const longPromptLines = 12
const longPromptChars = 1500

function UserPrompt({ item }: { item: Item }) {
  const [showPreamble, setShowPreamble] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const text = item.text ?? ''
  const long = text.length > longPromptChars || text.split('\n').length > longPromptLines
  const queued = item.meta?.queued === true
  return (
    <section className="card user" data-pin={pinKey(item) ?? undefined}>
      <header className="card-head">
        <span>You</span>
        {queued && <span className="chip">queued</span>}
        {item.preamble && (
          <button
            type="button"
            className="chip chip-button"
            onClick={() => {
              setShowPreamble(!showPreamble)
            }}
          >
            agent-manager note
          </button>
        )}
        <span className="card-time">{time(item)}</span>
        <PinButton item={item} />
      </header>
      {showPreamble && <p className="preamble">{item.preamble}</p>}
      <div className={long && !expanded ? 'prompt-text clamped' : 'prompt-text'}>{text}</div>
      {long && (
        <button
          type="button"
          className="link-button"
          onClick={() => {
            setExpanded(!expanded)
          }}
        >
          {expanded ? 'Show less' : `Show all (${String(text.split('\n').length)} lines)`}
        </button>
      )}
    </section>
  )
}

const noticeLabels: Record<string, string> = {
  command: 'Command',
  'command-output': 'Command output',
  'task-notification': 'Background task',
  interrupted: 'Interrupted',
  'bash-input': 'Shell',
  'bash-output': 'Shell output',
}

function Notice({ item }: { item: Item }) {
  const label = noticeLabels[item.label ?? ''] ?? item.label ?? 'Notice'
  const text = item.text ?? ''
  if (item.label === 'command' || item.label === 'interrupted' || item.label === 'bash-input') {
    return (
      <p className="notice">
        <span className="notice-label">{label}</span> <code>{text}</code>
      </p>
    )
  }
  return <Expandable className="notice" label={label} summary={summarize(text)} body={<pre className="notice-body">{text}</pre>} />
}

// summarize pulls a readable line out of tagged notification text.
function summarize(text: string): string {
  const tagged = /<summary>([\s\S]*?)<\/summary>/.exec(text) ?? /<status>([\s\S]*?)<\/status>/.exec(text)
  const line = (tagged?.[1] ?? text.replace(/<[^>]+>/g, ' ')).trim().split('\n', 1)[0] ?? ''
  return line.length > 140 ? line.slice(0, 140) + '…' : line
}

function Compact({ summary }: { summary?: Item }) {
  return (
    <div className="compact">
      <div className="compact-rule">
        <span>Conversation compacted</span>
      </div>
      {summary && <Expandable className="compact-summary" label="Summary of earlier conversation" summary="" body={<Markdown text={summary.text ?? ''} />} />}
    </div>
  )
}

const systemLabels: Record<string, string> = {
  away_summary: 'While you were away',
  local_command: 'Command',
  informational: 'Note',
  scheduled_task_fire: 'Scheduled task',
}

function SystemLine({ item }: { item: Item }) {
  const error = item.label === 'api_error' || item.meta?.level === 'error'
  const detail = item.text ?? (item.meta?.error ? JSON.stringify(item.meta.error) : '')
  return (
    <p className={error ? 'notice notice-error' : 'notice'}>
      <span className="notice-label">{error ? 'API error' : (systemLabels[item.label ?? ''] ?? (item.label ?? 'system').replace(/_/g, ' '))}</span> {detail}
    </p>
  )
}

function RawEvent({ item }: { item: Item }) {
  return <Expandable className="raw" label="Unrecognized event" summary={item.label ?? ''} body={<pre className="notice-body">{item.raw}</pre>} />
}

interface ExpandableProps {
  className: string
  label: string
  summary: string
  body: ReactNode
  defaultOpen?: boolean
}

function Expandable({ className, label, summary, body, defaultOpen = false }: ExpandableProps) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <div className={className}>
      <button
        type="button"
        className="expand-row"
        aria-expanded={open}
        onClick={() => {
          setOpen(!open)
        }}
      >
        <span className="chevron">{open ? '▾' : '▸'}</span>
        <span className="notice-label">{label}</span>
        <span className="expand-summary">{summary}</span>
      </button>
      {open && body}
    </div>
  )
}

function Turn({ steps, durationMs, live }: { steps: Step[]; durationMs?: number; live: boolean }) {
  const prefs = useViewPrefs()
  const keepAlone = (s: Extract<Step, { kind: 'tool' }>) => {
    const name = s.use.tool?.name ?? ''
    if (s.result?.result?.isError) return true
    if (name === 'Bash' && bashFiles(s.result?.result?.detail).files.length > 0) return true
    const cat = toolCategory(name)
    return cat !== 'edits' && prefs.collapse[cat] === 'open'
  }
  return (
    <section className="turn">
      {groupSteps(steps, keepAlone).map((s, i) =>
        s.kind === 'group' ? (
          <ToolGroup key={stepKey(s.steps[0] as Step, i)} steps={s.steps} live={live} />
        ) : (
          <StepView key={stepKey(s, i)} step={s} live={live} />
        ),
      )}
      {durationMs !== undefined && <p className="turn-foot">Worked for {formatDuration(durationMs)}</p>}
    </section>
  )
}

// ToolGroup is a run of exploring steps folded into one line, with any
// risky command, failure or running step shown on the line.
function ToolGroup({ steps, live }: { steps: Step[]; live: boolean }) {
  const [open, setOpen] = useState(false)
  let risky: string | null = null
  let failed = 0
  let running = false
  for (const s of steps) {
    if (s.kind !== 'tool') continue
    if (s.result?.result?.isError) failed++
    if (!s.result && live) running = true
    const input = s.use.tool?.input
    if (!risky && s.use.tool?.name === 'Bash' && typeof input === 'object' && input !== null) {
      const cmd = (input as Record<string, unknown>).command
      risky = typeof cmd === 'string' ? riskyCommand(cmd) : null
    }
  }
  return (
    <div className={`tool-group${risky ? ' tool-risky' : ''}`}>
      <button
        type="button"
        className="tool-group-row"
        aria-expanded={open}
        onClick={() => {
          setOpen(!open)
        }}
      >
        <span className="chevron">{open ? '▾' : '▸'}</span>
        <span className="tool-group-summary">{groupSummary(steps)}</span>
        {risky && (
          <span className="risk-badge" title={`Risky: ${risky}`}>
            ⚠ {risky}
          </span>
        )}
        {failed > 0 && <span className="tool-group-failed">{failed} failed</span>}
        {running && <span className="tool-state">…</span>}
      </button>
      {open && (
        <div className="tool-group-steps">
          {steps.map((s, i) => (
            <StepView key={stepKey(s, i)} step={s} live={live} />
          ))}
        </div>
      )}
    </div>
  )
}

function stepKey(s: Step, i: number): string {
  return s.kind === 'tool' ? `u${String(s.use.seq)}` : `s${String(s.item.seq)}-${String(i)}`
}

function StepView({ step, live }: { step: Step; live: boolean }) {
  const prefs = useViewPrefs()
  switch (step.kind) {
    case 'text':
      return (
        <div className="pinnable" data-pin={pinKey(step.item) ?? undefined}>
          <PinButton item={step.item} />
          <Markdown text={step.item.text ?? ''} />
        </div>
      )
    case 'thinking': {
      const ms = step.item.meta?.durationMs
      // An empty thought under a second has nothing to tell the reader.
      if (!step.item.text && (typeof ms !== 'number' || ms < 1000)) return null
      const label = typeof ms === 'number' && ms >= 1000 ? `Thought for ${formatDuration(ms)}` : 'Thought'
      if (!step.item.text) return <p className="thinking">{label}</p>
      return (
        <Expandable
          className="thinking"
          label={label}
          summary=""
          body={<Markdown text={step.item.text} />}
          defaultOpen={prefs.collapse.thinking === 'open'}
        />
      )
    }
    case 'tool':
      return <ToolCall use={step.use} result={step.result} live={live} />
    case 'orphan_result':
      return (
        <Expandable
          className="notice"
          label="Tool result"
          summary="for a call outside the loaded history"
          body={<pre className="notice-body">{step.item.result?.content}</pre>}
        />
      )
    case 'raw':
      return <RawEvent item={step.item} />
  }
}
