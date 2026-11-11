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

Out of scope: an attacker who already runs code as your user account. That account can drive agent-manager directly.
