# Away from the desk

Status: design only. Nothing is built without the owner's decisions.

## Goal

Check on sessions and answer a waiting one from a phone or another machine.

## Why today's model does not stretch

Agent Desk listens on 127.0.0.1 with a token that changes on every start, travels in the URL fragment, and lives in one tab's session storage. Host and Origin checks pin it to that loopback address. That is right for one desk and wrong for a phone: there is no route to the server, no lasting credential, and no TLS.

## Options

| Option | How | Gains | Costs |
| --- | --- | --- | --- |
| A. Tailscale | `tailscale serve` proxies a tailnet HTTPS name to the loopback port; Agent Desk trusts Tailscale's identity headers only from the local proxy and only for an allow-listed login | TLS and device identity come from the tailnet; no open port | Needs Tailscale on both ends; a second auth path to test |
| B. Device pairing | A "Pair a device" screen shows a QR code with a one-time code; the device gets a long-lived key (or registers a passkey); the server listens on a LAN or tailnet address with its own TLS certificate | Works without Tailscale | Certificates, key storage, revocation, a LAN listener |
| C. Read-only remote | Either A or B, but remote clients may only read and answer dialogs; sending replies stays local | Smaller blast radius | Less useful |

Push notifications to a phone would need a network service (APNs, ntfy or similar), which breaks "no network calls beyond localhost". Leave them out, or make them a separate opt-in.

## Recommendation

A first, with C's limits at the start: remote clients can read, see attention, and answer dialogs, and replies come later. Keep the loopback listener and token exactly as they are for local use.

## Decisions needed

1. Build this at all, and on which option.
2. What remote clients may do: read, answer dialogs, send replies, manage sessions.
3. Whether phone push is wanted, given it needs an outside service.
