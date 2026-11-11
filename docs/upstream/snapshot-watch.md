# feat(cli): snapshot --watch streams changes

## Problem

A client that shows live status polls `agent-manager snapshot --json`. Each call opens the store and lists tmux panes, about 160 ms. Status itself changes on the poller's 2 s cycle, so a client polling every second still lags up to 2 s plus its own interval. Agent Desk measured 2.25 to 2.3 s from a Claude Code hook write to its UI.

## Proposal

```
agent-manager snapshot --json --watch
```

prints the full snapshot as one JSON line, then one line each time the poller writes a change to any session, terminal or group, until interrupted. The process stays attached to the store and to the manager's heartbeat; it exits with status 2 when no manager has been awake for the heartbeat's stale window, so the client knows statuses have stopped advancing.

Implementation sketch: the poller already writes status through `UpdateStatusOnSocket`, which reports `changed`. A monotonic change counter in `settings`, bumped in the same transaction, lets the watch loop read the counter cheaply (every 200 ms) and re-read sessions only when it moves.

## Optional

Writing the hook status (`hooks/<id>.status`) into the store as soon as the poller sees it, instead of on the next 2 s cycle, would close most of the remaining lag for Claude Code.

## Tests

- A watch started against a test store emits the initial snapshot, then one line after a status change, and nothing while idle.
- It exits 2 when the heartbeat goes stale.
