// Remote access setup text for the settings screen.

/** cloudflaredIngress is the tunnel ingress for the remote listener. */
export function cloudflaredIngress(host: string, service: string, team: string, aud: string): string {
  return [
    'ingress:',
    `  - hostname: ${host || 'desk.example.com'}`,
    `    service: "${service}"`,
    '    originRequest:',
    '      access:',
    '        required: true',
    `        teamName: ${team || 'your-team'}`,
    `        audTag: [${aud || 'your-aud-tag'}]`,
    '  - service: http_status:404',
  ].join('\n')
}
