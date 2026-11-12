import { describe, expect, it } from 'vitest'
import { cloudflaredIngress } from './remote'

describe('cloudflaredIngress', () => {
  it('points the host at the socket and requires Access', () => {
    const y = cloudflaredIngress('desk.example.com', 'unix:/Users/dev/Library/Application Support/agent-desk/remote.sock', 'team', 'abc123')
    expect(y).toContain('  - hostname: desk.example.com')
    expect(y).toContain('    service: "unix:/Users/dev/Library/Application Support/agent-desk/remote.sock"')
    expect(y).toContain('        required: true')
    expect(y).toContain('        teamName: team')
    expect(y).toContain('        audTag: [abc123]')
    expect(y.trimEnd().endsWith('- service: http_status:404')).toBe(true)
  })

  it('shows placeholders until the fields are filled', () => {
    const y = cloudflaredIngress('', 'unix:/x', '', '')
    expect(y).toContain('desk.example.com')
    expect(y).toContain('teamName: your-team')
    expect(y).toContain('audTag: [your-aud-tag]')
  })
})
