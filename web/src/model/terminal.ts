// stripAnsi removes CSI escape sequences, leaving the visible text.
export function stripAnsi(s: string): string {
  // eslint-disable-next-line no-control-regex
  return s.replace(/\x1b\[[0-9;?]*[ -/]*[@-~]/g, '').replace(/\x1b\][^\x07\x1b]*(\x07|\x1b\\)/g, '')
}

// screenSize is the terminal size that fits a screen: its widest line and
// its line count, within sane bounds.
export function screenSize(screen: string): { cols: number; rows: number } {
  const lines = stripAnsi(screen).replace(/\n+$/, '').split('\n')
  const widest = Math.max(0, ...lines.map((l) => Array.from(l).length))
  return { cols: Math.min(Math.max(widest, 40), 400), rows: Math.min(Math.max(lines.length, 5), 200) }
}
