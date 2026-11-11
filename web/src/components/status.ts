// Glyphs follow agent-manager's Advanced marks so both tools read alike.
export const statusGlyph: Record<string, string> = {
  working: '◐',
  waiting: '◆',
  finished: '●',
  errored: '✕',
  idle: '○',
  dead: '✕',
  starting: '◌',
}

export function glyph(status: string): string {
  return statusGlyph[status] ?? '?'
}
