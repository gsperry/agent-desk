# Away from the desk

Status: R0 (groundwork), R1 (Tailscale) and R2 (Cloudflare Tunnel and Access) built. Decided: three remote permission levels, changeable only locally; phone push deferred; reach in order Tailscale, then Cloudflare Tunnel and Access, then a hosted relay as its own decision.

## Goal

Check on sessions and answer a waiting one from a phone or another machine.

## Why today's model does not stretch

Agent Desk listens on 127.0.0.1 with a token that changes on every start, travels in the URL fragment, and lives in one tab's session storage. Host and Origin checks pin it to that loopback address. That is right for one desk and wrong for a phone: there is no route to the server, no lasting credential, and no TLS.

## Reach

Every option keeps the loopback listener and the local token exactly as they are. Remote traffic arrives through a second path that Agent Desk can tell apart from local use, and whose identity it checks itself rather than trusting the network.

| Option | How | Who can read the traffic | Gains | Costs |
| --- | --- | --- | --- | --- |
| A. Tailscale | `tailscale serve` proxies a tailnet HTTPS name to an owner-only Unix socket (built: other accounts cannot connect; tailscaled runs as root). Agent Desk accepts remote requests only with Tailscale's identity header, for allow-listed logins. | Nobody but the two devices (WireGuard end to end) | No open port, no account beyond Tailscale, device identity built in | Tailscale on the phone and the desk |
| B. Cloudflare Tunnel and Access | `cloudflared` opens an outbound tunnel from the desk to a name such as `desk.agent-desk.app`. Cloudflare Access puts a login (email code, GitHub or Google) in front of it. Agent Desk verifies the Access JWT on every remote request. | Cloudflare (TLS ends at its edge) | Any browser, no app on the phone, uses the zone already owned, free tier | Trusts Cloudflare with session content; verifying the JWT means fetching Cloudflare's signing keys |
| C. Hosted end-to-end relay, as Nabu Casa does for Home Assistant | The desk keeps an outbound connection to a relay. The relay routes by name (SNI) without decrypting; TLS ends on the desk, with a certificate the desk obtains through a DNS challenge. A pairing flow links a phone to the desk. | Nobody but the two ends | Any browser, no app, the relay cannot read sessions; the best shape for other users of a public project | Someone runs and pays for the relay, certificate issuance, accounts, abuse handling. A product, not a feature |
| D. Self-hosted relay | Same shape as C on a small VPS the user runs (for example frp with TLS passthrough). | Nobody but the two ends, if TLS passes through | No third party | A server to run and patch |
| E. Peer to peer | WebRTC between phone and desk, with a small signaling service to introduce them. | Nobody but the two ends | No relay for the data itself | Signaling service, NAT traversal, the most code |

Recommendation for now: A for personal use, B as the alternative that needs no app on the phone (it fits the agent-desk.app zone). Design the remote path so it is reach-agnostic: an identity check per option (Tailscale headers, an Access JWT, later a pairing key), then the same permission levels below. C is the right long-term answer if Agent Desk grows other users who want remote access without running anything; treat it as a separate project with its own decision.

## Remote permissions (decided)

A setting with three levels, cumulative:

| Level | A remote client can |
| --- | --- |
| Read and answer | Read sessions, transcripts, changes, history and usage; answer a waiting prompt or dialog |
| Reply | The above, and send replies through the normal reply gate |
| Start and stop | The above, and start, stop, revive, archive and move sessions |

- The setting is changed only from a local client (loopback, local token). A remote client can see the level but never change it, and the server rejects the change from a remote request regardless of what the UI shows.
- Default: Read and answer. Remote access as a whole stays off until turned on locally.
- Enforced in the server per route, not in the UI: every state-changing route declares the level it needs.
- Never remote, at any level: changing settings, rules, catch-up summaries (they spend tokens), opening files in the editor, pasting images.

## Phone push notifications (deferred)

Not built for now: they need an outside service (APNs, ntfy or similar), which breaks "no network calls beyond localhost". The roadmap keeps a row to revisit, likely as a separate opt-in.

## R2 as built

- One proxy at a time (`via`: tailscale or cloudflare); the other's identity counts for nothing, since Cloudflare does not strip Tailscale headers and Tailscale does not strip Cloudflare's.
- `cloudflared` points at the same owner-only socket, with `originRequest.access.required` as a second check. Agent Desk does not rely on it: it verifies the Access token itself (decided 2026-11-11, amending the no-network rule for this one call) and ignores the plain email header.
- Login methods are whatever the Access application allows (one-time email code needs no setup); Agent Desk only sees the verified email.

## Decisions needed

1. Whether to build C, the hosted relay, as its own project.
