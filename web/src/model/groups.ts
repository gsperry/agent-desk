import type { Step } from './transcript'

type ToolStep = Extract<Step, { kind: 'tool' }>

export type Shown = Step | { kind: 'group'; steps: Step[] }

// Tools that explore rather than change anything. A run of them folds into
// one line, as Claude Code's "Ran 3 shell commands" does; edits, agents and
// questions stay on their own lines.
const exploring = new Set(['Bash', 'BashOutput', 'Read', 'NotebookRead', 'Grep', 'Glob', 'LS', 'WebFetch', 'WebSearch', 'ToolSearch'])

// groupSteps folds each run of two or more exploring tool calls, with the
// thoughts between them, into a group. A thought at the end of a run stays
// outside it. keepAlone names calls that stay on their own line even so:
// failures, commands that changed files, kinds the reader opens by default.
export function groupSteps(steps: Step[], keepAlone: (s: ToolStep) => boolean = () => false): Shown[] {
  const groupable = (s: Step): s is ToolStep => {
    if (s.kind !== 'tool') return false
    const name = s.use.tool?.name ?? ''
    return (exploring.has(name) || name.startsWith('mcp__')) && !keepAlone(s)
  }
  const out: Shown[] = []
  let run: Step[] = []
  const flush = () => {
    const tail: Step[] = []
    while (run.length && run[run.length - 1]?.kind === 'thinking') tail.unshift(run.pop() as Step)
    const tools = run.filter((s) => s.kind === 'tool').length
    if (tools >= 2) out.push({ kind: 'group', steps: run })
    else out.push(...run)
    out.push(...tail)
    run = []
  }
  for (const s of steps) {
    if (groupable(s) || (s.kind === 'thinking' && run.length > 0)) run.push(s)
    else {
      flush()
      out.push(s)
    }
  }
  flush()
  return out
}

const plural = (n: number, one: string, many: string) => (n === 1 ? one : many.replace('#', String(n)))

// groupSummary describes a group in a line: "Ran 3 commands, read 2 files".
export function groupSummary(steps: Step[]): string {
  let commands = 0
  let reads = 0
  let searches = 0
  let web = 0
  let other = 0
  for (const s of steps) {
    if (s.kind !== 'tool') continue
    switch (s.use.tool?.name) {
      case 'Bash':
      case 'BashOutput':
        commands++
        break
      case 'Read':
      case 'NotebookRead':
        reads++
        break
      case 'Grep':
      case 'Glob':
      case 'LS':
      case 'ToolSearch':
        searches++
        break
      case 'WebFetch':
      case 'WebSearch':
        web++
        break
      default:
        other++
    }
  }
  const parts = [
    commands && plural(commands, 'ran a command', 'ran # commands'),
    reads && plural(reads, 'read a file', 'read # files'),
    searches && plural(searches, 'searched once', 'searched # times'),
    web && plural(web, 'looked up the web once', 'looked up the web # times'),
    other && plural(other, 'used a tool', 'used # tools'),
  ].filter((p): p is string => typeof p === 'string')
  const line = parts.join(', ')
  return line.charAt(0).toUpperCase() + line.slice(1)
}
