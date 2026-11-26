// The server's limit on a session name (internal/manage maxName).
export const maxNameLength = 80

// renameProblem says why a name cannot be used for a session called
// current, or null when it can. The server checks again; this is for
// answering before the request.
export function renameProblem(name: string, current: string): string | null {
  const t = name.trim()
  if (t === '') return 'Enter a name.'
  if (t.length > maxNameLength) return `Use at most ${String(maxNameLength)} characters.`
  // eslint-disable-next-line no-control-regex
  if (/[\u0000-\u001f\u007f-\u009f]/.test(t)) return 'Use no control characters or line breaks.'
  if (t === current) return 'That is already its name.'
  return null
}
