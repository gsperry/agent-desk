import type { Conversation, SessionView, SetupReport } from '../types'

// The welcome screen opens by itself once, on the first run. A browser that
// already holds Agent Desk's settings belongs to someone who has used it
// before it had a welcome screen.
const key = 'agent-desk:welcomed'
const earlier = ['agent-desk:seen', 'agent-desk:view']

export function welcomed(store: Pick<Storage, 'getItem'>): boolean {
  return [key, ...earlier].some((k) => store.getItem(k) !== null)
}

export function markWelcomed(store: Pick<Storage, 'setItem'>): void {
  store.setItem(key, '1')
}

// installHint is how to get a missing program.
export function installHint(name: string): { command?: string; link?: string; note: string } {
  switch (name) {
    case 'agent-manager':
      return { command: 'brew install agent-manager', link: 'https://agent-manager.dev', note: 'Runs the agents in tmux. Agent Desk is a window onto it.' }
    case 'tmux':
      return { command: 'brew install tmux', note: 'agent-manager keeps every agent in its own tmux server.' }
    case 'claude':
      return { link: 'https://claude.com/claude-code', note: 'Claude Code, the CLI whose sessions Agent Desk reads. Install it and sign in once in a terminal.' }
    default:
      return { note: '' }
  }
}

// programLabel is a program's name as the checklist shows it.
export function programLabel(name: string): string {
  return name === 'claude' ? 'Claude Code (claude)' : name
}

export interface CLIChoice {
  shown: string[]
  defaultTool: string
}

// initialChoice is the CLI form as agent-manager has it now. With no
// default stored, agent-manager starts the first CLI it offers.
export function initialChoice(r: SetupReport): CLIChoice {
  const shown = r.shown.length > 0 ? r.shown : r.clis.slice(0, 1)
  const defaultTool = shown.includes(r.defaultTool) ? r.defaultTool : (shown[0] ?? '')
  return { shown, defaultTool }
}

// toggleCLI adds or removes a CLI, keeping agent-manager's order and a
// default among those offered.
export function toggleCLI(c: CLIChoice, all: string[], cli: string, on: boolean): CLIChoice {
  const set = new Set(c.shown)
  if (on) set.add(cli)
  else set.delete(cli)
  const shown = all.filter((t) => set.has(t))
  const defaultTool = shown.includes(c.defaultTool) ? c.defaultTool : (shown[0] ?? '')
  return { shown, defaultTool }
}

export function sameChoice(a: CLIChoice, b: CLIChoice): boolean {
  return a.defaultTool === b.defaultTool && a.shown.length === b.shown.length && a.shown.every((t, i) => b.shown[i] === t)
}

// outsideConversations is the newest conversations no agent-manager
// session holds: those started with a plain claude, which can be resumed
// as sessions.
export function outsideConversations(list: Conversation[], owners: Map<string, SessionView>, limit: number): Conversation[] {
  return list
    .filter((c) => !owners.has(c.id))
    .sort((a, b) => Date.parse(b.modified) - Date.parse(a.modified))
    .slice(0, limit)
}
