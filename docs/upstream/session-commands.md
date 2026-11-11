# feat(cli): rename, delete, restart, fork and models for scripts

## Problem

`spawn`, `kill`, `revive`, `archive` and `move` run with no caller, so a script or a GUI can manage sessions. Four row actions the TUI offers have no such command, and the spawn form's model list has no way out of the binary:

| TUI key | Action | CLI today |
| --- | --- | --- |
| `r` | Rename another session | `rename` renames only the calling session |
| `d` | Delete a row | none |
| `R` | Restart on an empty context | none |
| `f` | Fork into a named copy | none |
| `n` form | Models, efforts and profiles a CLI lists | none; the lists live in an internal catalog cache |

## Proposal

```
agent-manager rename <session-id> "<name>" [--json]
agent-manager delete <session-id> [--json]
agent-manager restart <session-id> [--json]
agent-manager fork <session-id> --name <name> [--json]
agent-manager models --tool <cli> [--json]
```

- **rename** with an id renames that session, with the same worktree-branch checks the TUI applies. Without an id it keeps today's meaning, the caller renaming itself.
- **delete** removes the row the way `d` does, refusing a running session unless `--kill` is passed, and keeping a managed worktree another session still uses.
- **restart** follows `R`: a new conversation id, same name, group, tool and directory; the old conversation stays on disk.
- **fork** follows `f`, on the tools that support it, and prints the new session.
- **models** prints what the `n` form shows for one CLI: models, efforts per model, and profiles, read the way the form reads them.

All five run with no caller, like `kill`, and from inside a session they act on the named target, the same rules as `kill` and `move`.

## Coverage

rename, delete and restart are tool-neutral. fork covers the tools with a fork command today and refuses the rest with the reason the TUI gives. models covers every tool whose catalog the form reads.

## Tests

One end-to-end test per command against a test store and tmux server, plus refusal cases: delete of a running session, fork on a tool without fork, rename to a name the worktree branch cannot take.
