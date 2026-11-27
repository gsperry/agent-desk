# feat(cli): reorder a session or a group for scripts

## Problem

Agent-manager keeps a manual order: sessions within a group, and the groups themselves, set with the reorder keys or by dragging a row. `snapshot --json` lists them in that order, so a script or a GUI can follow it. Nothing reorders from outside the TUI, so a GUI that follows the order cannot let its user change it.

| TUI action | Effect | CLI today |
| --- | --- | --- |
| reorder up / down on a session | moves it among the sessions of its group | none |
| reorder up / down on a group | moves it among the groups | none |
| drag a row | the same, to any place | none |

## Proposal

```
agent-manager reorder-session <session-id> <up|down|top|bottom> [--json]
agent-manager reorder-group <path> <up|down|top|bottom> [--json]
```

- `up` and `down` move one place, as the reorder keys do. `top` and `bottom` move to the first or last place among the same siblings: a session's group (and, for a terminal nested under an agent, that agent), or the groups.
- A move past an edge changes nothing and says so, exit status 0, as `archive` does for a row already in that state.
- Both run with no caller, like `move` and `rename-session`, and print the session or group with its new position under `--json`.
- Document that `snapshot --json` lists sessions by group and then position, and groups by position, as a stable contract, so a client can show the order without a position field.

## Coverage

Tool-neutral: the order is the manager's own, not any CLI's. It reuses the store function the reorder keys call, so the CLI and the TUI cannot disagree.

## Tests

One end-to-end test per command against a test store: each direction, both edges, a nested terminal, a group, an unknown id, and that `snapshot --json` reflects the move.
