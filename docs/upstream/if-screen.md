# feat(cli): --if-screen guards keys and type against a changed screen

## Problem

A client that answers a dialog reads the pane, decides, then presses keys in a second command. Between the two, the agent can redraw: a permission prompt can replace the one the user saw, or one can open over the prompt box. The key then lands on a screen nobody looked at, and Enter there grants a permission or submits a draft. Agent Desk narrows the gap (a fingerprint of the dialog is checked on a fresh read just before the keys go, under a per-session lock), but the read and the key press are still separate processes.

## Proposal

```
agent-manager read <session-id> --json          # adds "screen_hash"
agent-manager keys <session-id> <key>... --if-screen <hash>
agent-manager type <session-id> <text> --if-screen <hash>
```

- `read --json` adds `screen_hash`: a hash of the pane's visible text (`capture-pane -p`), the same function `--if-screen` uses.
- With `--if-screen`, the command captures the pane while holding the pane send gate, compares the hash, and sends only if it matches. A mismatch exits with a distinct status (4) and changes nothing.
- The capture and the send happen under one lock, so no other `type`, `keys` or queued delivery runs between them.

## Coverage

Tool-neutral: it compares text, so every CLI gets it.

## Tests

- A matching hash sends; a pane changed after the read refuses with status 4 and sends nothing.
- A queued delivery cannot run between the comparison and the send.
- Without `--if-screen`, behavior is unchanged.
