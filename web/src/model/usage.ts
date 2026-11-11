import type { Conversation, UsageBucket, UsageReport, UsageTokens } from '../types'
import { lastPart } from './history'

export function totalTokens(t: UsageTokens): number {
  return t.input + t.output + t.cacheRead + t.cacheWrite
}

// formatTokens shortens a count: 950, 12.3k, 4.5M, 20.1B.
export function formatTokens(n: number): string {
  const units: [number, string][] = [
    [1e9, 'B'],
    [1e6, 'M'],
    [1e3, 'k'],
  ]
  for (const [size, unit] of units) {
    if (n >= size) {
      const v = n / size
      return `${v >= 100 ? v.toFixed(0) : v.toFixed(1)}${unit}`
    }
  }
  return String(Math.round(n))
}

// formatDuration shows hours and minutes: 0m, 45m, 3h 05m.
export function formatHours(ms: number): string {
  const minutes = Math.round(ms / 60_000)
  if (minutes < 60) return `${String(minutes)}m`
  return `${String(Math.floor(minutes / 60))}h ${String(minutes % 60).padStart(2, '0')}m`
}

export function formatUSD(v: number): string {
  return v.toLocaleString('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: v < 10 ? 2 : 0 })
}

// barScale maps each day's tokens to a share of the busiest day.
export function barScale(days: UsageReport['days']): number[] {
  const most = Math.max(0, ...days.map(totalTokens))
  return days.map((d) => (most === 0 ? 0 : totalTokens(d) / most))
}

export interface ProjectUsage extends UsageBucket {
  directory: string
  name: string
  conversations: number
}

function emptyBucket(): UsageBucket {
  return { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, replies: 0, prompts: 0, workingMs: 0, waitingMs: 0, costUSD: 0 }
}

function addInto(b: UsageBucket, o: UsageBucket) {
  b.input += o.input
  b.output += o.output
  b.cacheRead += o.cacheRead
  b.cacheWrite += o.cacheWrite
  b.replies += o.replies
  b.prompts += o.prompts
  b.workingMs += o.workingMs
  b.waitingMs += o.waitingMs
  b.costUSD += o.costUSD
}

// byProject sums conversations by the directory they ran in, busiest first.
export function byProject(rows: UsageReport['conversations'], listing: Conversation[]): ProjectUsage[] {
  const dirs = new Map(listing.map((c) => [c.id, c.directory ?? '']))
  const out = new Map<string, ProjectUsage>()
  for (const r of rows) {
    const directory = dirs.get(r.id) ?? ''
    let p = out.get(directory)
    if (!p) {
      p = { ...emptyBucket(), directory, name: lastPart(directory) || 'unknown directory', conversations: 0 }
      out.set(directory, p)
    }
    addInto(p, r)
    p.conversations++
  }
  return [...out.values()].sort((a, b) => totalTokens(b) - totalTokens(a))
}
