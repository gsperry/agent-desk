# feat(cli): keys sends named keys to an agent session

## Problem

`terminal send --keys` presses tmux key names in a managed terminal. There is no equivalent for agent sessions, so a client that answers a dialog (move the selection, confirm, cancel) has to call `tmux -L agentmgr send-keys -t am_<id>.0` itself. That duplicates the socket and pane conventions AGENTS.md says every caller must get right, and it skips the per-pane send gate that keeps a key press from interleaving with a paste.

## Proposal

```
agent-manager keys <session-id> <key>[,<key>...] [--json]
```

- Accepts the same tmux key names as `terminal send --keys`, limited to navigation and confirmation by default: `Up`, `Down`, `Left`, `Right`, `Enter`, `Escape`, `Tab`, `BTab` and digits.
- Goes through `Driver` and the pane send gate, so it never interleaves with a `type` or a queued delivery.
- Runs with no caller, like `type`, and is refused from inside a session (agents answer their own dialogs).

## Coverage

Key names are tool-neutral, so the command covers every tool.

## Tests

- Keys reach pane 0 of the session's window and nothing else.
- A key outside the allowed set is refused without touching the pane.
- A `keys` call during a `type` waits for the paste to finish.
