import { useEffect, useMemo, useState } from 'react'
import { conversationTitle, lastPart } from '../model/history'
import { barScale, byProject, formatHours, formatTokens, formatUSD, totalTokens } from '../model/usage'
import type { Conversation, UsageReport } from '../types'

interface Props {
  load: (days: number) => Promise<{ data?: UsageReport; error?: string }>
  conversations: Conversation[] | null
  loadConversations: () => Promise<unknown>
  onOpenConversation: (id: string) => void
}

const ranges: [number, string][] = [
  [7, '7 days'],
  [30, '30 days'],
  [90, '90 days'],
]

// UsageView totals tokens, time and reported cost from transcripts on disk.
export function UsageView({ load, conversations, loadConversations, onOpenConversation }: Props) {
  const [days, setDays] = useState(30)
  const [result, setResult] = useState<{ days: number; data?: UsageReport; error?: string } | null>(null)
  const [table, setTable] = useState(false)

  useEffect(() => {
    let live = true
    void load(days).then((r) => {
      if (live) setResult({ days, ...r })
    })
    return () => {
      live = false
    }
  }, [load, days])

  const needListing = conversations === null
  useEffect(() => {
    if (needListing) void loadConversations()
  }, [needListing, loadConversations])

  const loading = result?.days !== days
  const rep = result?.data
  const listing = useMemo(() => conversations ?? [], [conversations])
  const byId = useMemo(() => new Map(listing.map((c) => [c.id, c])), [listing])
  const projects = useMemo(() => (rep ? byProject(rep.conversations, listing) : []), [rep, listing])

  return (
    <main className="main usage-view" aria-label="Usage">
      <header className="session-head">
        <div className="session-title">
          <h1>Usage</h1>
          <div className="segmented" role="radiogroup" aria-label="Range">
            {ranges.map(([n, label]) => (
              <button
                key={n}
                type="button"
                role="radio"
                aria-checked={n === days}
                className={n === days ? 'active' : undefined}
                onClick={() => {
                  setDays(n)
                }}
              >
                {label}
              </button>
            ))}
          </div>
        </div>
        <div className="session-sub">
          <span className="muted">
            {loading ? 'Reading transcripts…' : rep ? `${rep.from} to ${rep.to} · ${String(rep.files)} transcripts, subagents included` : ''}
          </span>
        </div>
      </header>
      <div className="transcript">
        <div className="usage-body">
          {result?.error && <p className="banner error">{result.error}</p>}
          {rep && (
            <>
              <div className="stat-tiles">
                <Tile label="Tokens" value={formatTokens(totalTokens(rep.totals))} note={tokenNote(rep.totals)} />
                <Tile label="Replies" value={rep.totals.replies.toLocaleString()} note={`${rep.totals.prompts.toLocaleString()} prompts from you`} />
                <Tile label="Agents working" value={formatHours(rep.totals.workingMs)} note="Pauses over 5 minutes left out" />
                <Tile label="Waiting on you" value={formatHours(rep.totals.waitingMs)} note="Each wait capped at 1 hour" />
                <Tile label="Reported cost" value={formatUSD(rep.totals.costUSD)} note="List price, as Claude Code reported it" />
              </div>

              <section className="usage-section">
                <div className="usage-section-head">
                  <h2>Tokens per day</h2>
                  <button
                    type="button"
                    className="link-button"
                    onClick={() => {
                      setTable(!table)
                    }}
                  >
                    {table ? 'Show chart' : 'Show table'}
                  </button>
                </div>
                {table ? <DayTable rep={rep} /> : <DayChart rep={rep} />}
              </section>

              <section className="usage-section">
                <h2>Models</h2>
                <table className="usage-table">
                  <thead>
                    <tr>
                      <th>Model</th>
                      <th className="num">Tokens</th>
                      <th className="num">Share</th>
                      <th className="num">Output</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rep.models.map((m) => (
                      <tr key={m.model}>
                        <td>{m.model}</td>
                        <td className="num">{formatTokens(totalTokens(m))}</td>
                        <td className="num">{share(totalTokens(m), totalTokens(rep.totals))}</td>
                        <td className="num">{formatTokens(m.output)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </section>

              <section className="usage-section">
                <h2>Projects</h2>
                <table className="usage-table">
                  <thead>
                    <tr>
                      <th>Directory</th>
                      <th className="num">Conversations</th>
                      <th className="num">Tokens</th>
                      <th className="num">Working</th>
                      <th className="num">Waiting on you</th>
                      <th className="num">Reported cost</th>
                    </tr>
                  </thead>
                  <tbody>
                    {projects.map((p) => (
                      <tr key={p.directory}>
                        <td title={p.directory}>{p.name}</td>
                        <td className="num">{p.conversations}</td>
                        <td className="num">{formatTokens(totalTokens(p))}</td>
                        <td className="num">{formatHours(p.workingMs)}</td>
                        <td className="num">{formatHours(p.waitingMs)}</td>
                        <td className="num">{formatUSD(p.costUSD)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </section>

              <section className="usage-section">
                <h2>Conversations</h2>
                <table className="usage-table">
                  <thead>
                    <tr>
                      <th>Conversation</th>
                      <th className="num">Tokens</th>
                      <th className="num">Subagents</th>
                      <th className="num">Prompts</th>
                      <th className="num">Working</th>
                      <th className="num">Waiting on you</th>
                      <th className="num">Reported cost</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rep.conversations.slice(0, 50).map((c) => {
                      const info = byId.get(c.id)
                      return (
                        <tr key={c.id}>
                          <td className="usage-conv">
                            <button
                              type="button"
                              className="link-button"
                              onClick={() => {
                                onOpenConversation(c.id)
                              }}
                            >
                              {info ? conversationTitle(info) : c.id}
                            </button>
                            <span className="muted">{lastPart(info?.directory)}</span>
                          </td>
                          <td className="num">{formatTokens(totalTokens(c))}</td>
                          <td className="num">{share(c.subagentTokens, totalTokens(c))}</td>
                          <td className="num">{c.prompts}</td>
                          <td className="num">{formatHours(c.workingMs)}</td>
                          <td className="num">{formatHours(c.waitingMs)}</td>
                          <td className="num">{formatUSD(c.costUSD)}</td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
                {rep.conversations.length > 50 && <p className="muted">The 50 busiest of {rep.conversations.length} conversations.</p>}
              </section>
            </>
          )}
        </div>
      </div>
    </main>
  )
}

function Tile({ label, value, note }: { label: string; value: string; note: string }) {
  return (
    <div className="stat-tile">
      <span className="stat-label">{label}</span>
      <span className="stat-value">{value}</span>
      <span className="muted stat-note">{note}</span>
    </div>
  )
}

function tokenNote(t: UsageReport['totals']): string {
  return `${formatTokens(t.input)} in · ${formatTokens(t.output)} out · ${formatTokens(t.cacheRead)} cache read · ${formatTokens(t.cacheWrite)} cache write`
}

function share(part: number, whole: number): string {
  if (whole === 0) return '0%'
  const p = (part / whole) * 100
  return p > 0 && p < 1 ? '<1%' : `${String(Math.round(p))}%`
}

function DayChart({ rep }: { rep: UsageReport }) {
  const [hover, setHover] = useState<number | null>(null)
  const scale = barScale(rep.days)
  const most = Math.max(0, ...rep.days.map(totalTokens))
  const shown = hover !== null ? rep.days[hover] : undefined
  return (
    <div className="day-chart">
      <div className="day-chart-top">
        <span className="muted">{formatTokens(most)}</span>
        <span className="day-tip" aria-live="polite">
          {shown && (
            <>
              <strong>{shown.day}</strong> {formatTokens(totalTokens(shown))} tokens · {shown.replies.toLocaleString()} replies · {formatHours(shown.workingMs)} working
            </>
          )}
        </span>
      </div>
      <div
        className="day-bars"
        onMouseLeave={() => {
          setHover(null)
        }}
      >
        {rep.days.map((d, i) => (
          <div
            key={d.day}
            className={hover === i ? 'day-col hover' : 'day-col'}
            role="img"
            aria-label={`${d.day}: ${formatTokens(totalTokens(d))} tokens`}
            onMouseEnter={() => {
              setHover(i)
            }}
          >
            <div className="day-bar" style={{ height: `${String(Math.max(scale[i] ?? 0, totalTokens(d) > 0 ? 0.01 : 0) * 100)}%` }} />
          </div>
        ))}
      </div>
      <div className="day-axis muted">
        <span>{rep.from}</span>
        <span>{rep.to}</span>
      </div>
    </div>
  )
}

function DayTable({ rep }: { rep: UsageReport }) {
  return (
    <table className="usage-table">
      <thead>
        <tr>
          <th>Day</th>
          <th className="num">Tokens</th>
          <th className="num">Replies</th>
          <th className="num">Prompts</th>
          <th className="num">Working</th>
          <th className="num">Waiting on you</th>
        </tr>
      </thead>
      <tbody>
        {[...rep.days].reverse().map((d) => (
          <tr key={d.day}>
            <td>{d.day}</td>
            <td className="num">{formatTokens(totalTokens(d))}</td>
            <td className="num">{d.replies.toLocaleString()}</td>
            <td className="num">{d.prompts}</td>
            <td className="num">{formatHours(d.workingMs)}</td>
            <td className="num">{formatHours(d.waitingMs)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
