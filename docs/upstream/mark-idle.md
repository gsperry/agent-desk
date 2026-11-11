# feat(cli): mark-idle acknowledges a finished session from a shell

## Problem

A `finished` session becomes `idle` only inside the TUI: when the user enters the session or presses `.` (`acknowledgeSelected` in `internal/ui/session_adapter.go`, which queues `focusruntime.AckRequest`). A user who reads the result somewhere else, such as a GUI or a manager on another machine, leaves the session `finished`. Every client then shows it as needing attention again.

## Proposal

```
agent-manager mark-idle <session-id> [--json]
```

It applies the same acknowledgement the `.` key applies: `finished` becomes `idle` until the screen changes, and the same for `waiting` when the user chooses to dismiss it. It runs with no caller, like `kill` and `revive`, and acts on the host's store, so a running manager picks it up on its next poll.

The MCP server gets the matching `acknowledge_session` tool only if maintainers want agents to use it; a human client needs the CLI alone.

## Coverage

Acknowledgement is tool-neutral today, so the command covers every tool.

## Tests

- `mark-idle` on a `finished` session leaves it `idle` after the next poll, and `finished` again after new output.
- `mark-idle` on a `working` session is a no-op with a message, matching the key.
