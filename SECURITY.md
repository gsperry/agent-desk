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

- Remote devices reach a second listener: a Unix socket in an owner-only directory (`remote.sock` beside the settings file), which `tailscale serve`, running as root, forwards to and which no other account on the machine can connect to. Tailscale removes any client-supplied identity headers and sets `Tailscale-User-Login`; only logins listed in the settings get in. For a socket target it sends `Host: localhost` and the tailnet name in `X-Forwarded-Host`, which it always overwrites; Agent Desk trusts that header only on the socket, only with that Host, and only while Tailscale is the configured proxy. The local token is never accepted there, and the identity header is never accepted on the local listener.
- Through Cloudflare instead (one proxy at a time; the other's identity is ignored): `cloudflared` forwards a tunnel to the same socket, behind a Cloudflare Access application. Agent Desk verifies the `Cf-Access-Jwt-Assertion` token on every request itself (RS256 only, the team's issuer, the application's audience tag, expiry and not-before within a minute's leeway), then checks the email against the allowed list. Plain identity headers such as `Cf-Access-Authenticated-User-Email` count for nothing, so a tunnel without Access gets no one in. Under Tailscale, requests carrying Cloudflare's edge headers (`Cf-Ray`, `Cf-Connecting-Ip`, the Access token) are refused, in case a tunnel is left running. `cloudflared` must run as your account (or root) to reach the socket. The public signing keys come from `https://<team>.cloudflareaccess.com`, the only network call Agent Desk makes: only when a token names a key not yet held or the held keys are a day old, at most once a minute and one fetch at a time, without following redirects, keys under 2048 bits ignored. Cloudflare ends TLS at its edge and can read the traffic.
- Every remote request must name the configured host; state-changing requests and WebSocket upgrades must come from that host's HTTPS origin.
- Each route declares the level it needs (read and answer, reply, start and stop), pinned by a test. Read and answer includes typing an answer into a question's "Type something" field; its keys (Up, Down, Enter, Escape) are pressed only while a question is on screen. Interrupt, which presses Escape in a working session, needs the reply level. Settings, rules, catch-up summaries, opening files and pasting images are refused remotely at every level.
- Turning remote access off, removing a login, switching proxy, or changing the Access team or audience tag closes that device's open connections at once. An open connection outlives its Access token's expiry until it reconnects.
- The settings live in an owner-only file and change only through the local listener.
- A process running as your own account (or root) can still connect to the socket and claim an identity; it could already drive agent-manager directly, so that stays out of scope, as below.

Known limits:

- The URL with the token is printed when Agent Desk starts. Do not redirect that output to a file other accounts can read.
- A reply or first prompt is passed to agent-manager as a command argument, so another account on the same machine running `ps` at that moment could read the text. Proposed upstream: [type --at-rest](docs/upstream/type-at-rest.md) asks agent-manager to accept text on stdin.

Out of scope: an attacker who already runs code as your user account. That account can drive agent-manager directly.
