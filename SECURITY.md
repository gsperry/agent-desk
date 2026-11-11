# Security

## Reporting a problem

Report vulnerabilities privately through GitHub: the repository's **Security** tab, then **Report a vulnerability**. Please do not open a public issue for a security problem.

Include what you found, how to reproduce it, and what an attacker gains. You can expect an acknowledgement within a week.

## Supported versions

Only the latest `main` is supported. There are no maintained release branches.

## Security model

Agent Desk can type into agent sessions and start new ones, so access to its API is equivalent to instructing an AI agent with your account's permissions. It defends that API as follows:

- It listens on 127.0.0.1 only.
- Each start generates a random 32-byte token. The browser receives it in the URL fragment, which never reaches the server or logs, and keeps it in session storage. Every API request and WebSocket must present it.
- Every request must name the server's own host, which blocks DNS rebinding.
- State-changing requests and WebSocket upgrades must come from the app's own origin.
- Responses carry a Content Security Policy, `X-Frame-Options: DENY`, and `nosniff`.
- Child processes (`agent-manager`, `tmux`, `code`, `git`) run with argument lists, never a shell, and with timeouts. Values that could look like flags are passed as `--flag=value` or after `--`.
- Transcript content is rendered without raw HTML.

- The browser opens through an owner-only launcher file, so the token never appears in a process's arguments, which other accounts can read.

Remote access, off by default:

- A second listener, also on 127.0.0.1, serves remote devices through a proxy that proves identity (`tailscale serve`, which sets `Tailscale-User-Login`). Only logins listed in the settings get in; the local token is never accepted there, and the identity header is never accepted on the local listener.
- Every remote request must name the configured host; state-changing requests and WebSocket upgrades must come from that host's origin.
- Each route declares the level it needs (read and answer, reply, start and stop). Settings, rules, catch-up summaries, opening files and pasting images are refused remotely at every level.
- The settings live in an owner-only file and change only through the local listener.
- Any local process can reach the remote port and claim an identity, as it could already drive agent-manager directly; that stays out of scope, as below.

Known limits:

- The URL with the token is printed when Agent Desk starts. Do not redirect that output to a file other accounts can read.
- A reply or first prompt is passed to agent-manager as a command argument, so another account on the same machine running `ps` at that moment could read the text. Proposed upstream: [type --at-rest](docs/upstream/type-at-rest.md) asks agent-manager to accept text on stdin.

Out of scope: an attacker who already runs code as your user account. That account can drive agent-manager directly.
