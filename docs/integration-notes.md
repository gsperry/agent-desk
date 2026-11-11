# Integration notes

Phase 0 findings. Probed on 2026-11-10 against:

- agent-manager 0.48.1 (source read at tag `v0.48.1`)
- Claude Code 2.1.322
- tmux 3.8, macOS (Apple Silicon)

## 1. Listing sessions

`agent-manager snapshot --json` and `agent-manager sessions --json` both run with no caller and print machine-readable JSON. Each takes about 160 ms.

`snapshot --json` shape:

```json
{
  "version": 1,
  "manager_awake": true,
  "sessions": [
    {
      "id": "3486b2c0",
      "name": "agent-desk-probe",
      "tool": "claude",
      "group": "",
      "directory": "/private/tmp/agentdesk-scratch/probe",
      "status": "finished",
      "running": true,
      "archived": false,
      "branch": "am/...",
      "environment": "...",
      "agent_session_id": "e392f4f9-1ea2-4338-af11-af32d1ef84da",
      "self": false
    }
  ],
  "terminals": [],
  "groups": []
}
```

- `group` is a slash path. Empty means root. All current sessions are in root, so the GUI groups by repository when `group` is empty.
- `directory` is the live pane cwd when running, else the launch directory.
- `branch` and `environment` are omitted when empty.
- `status` is one of `starting`, `working`, `waiting`, `finished`, `idle`, `errored`, `dead`.
- `snapshot` adds `manager_awake` and includes terminals and groups. Agent Desk uses `snapshot`.

## 2. Status detection

Status is computed by agent-manager's poller and stored in `state.db`. The CLI reads the stored value (`internal/sessioncmd/session.go`, `sessionInfo`). Agent Desk reads it and does not recompute it.

How the poller decides, for Claude Code:

- Each session launches with a generated `--settings` file (`hooks/<id>.settings.json`) whose hooks write `working`, `waiting`, `finished` or `idle` to `hooks/<id>.status`.
- Pane regex rules refine the hook value (plain-text questions, interrupts, error lines) and take over when the hook file is missing or stale.
- The pane is polled every 2 s.

Failure modes:

- **Status only advances while a manager runs.** `manager_awake` is false when no TUI or `agent-manager serve` holds a fresh poller heartbeat. Agent Desk shows a banner and offers `agent-manager serve --background` (supported, headless, macOS and Linux).
- **`finished` clears to `idle` only in the TUI** (entering the session or the `mark_idle` key). There is no CLI or MCP path. Agent Desk keeps its own per-session "seen" mark. Upstream gap, see below.
- **Not every dialog reads as `waiting`.** The workspace trust dialog read `waiting`. Claude's "Teach auto mode about your environment?" dialog read `finished`.
- **Latency.** Worst case is the 2 s poller interval plus Agent Desk's own poll. Watching `hooks/<id>.status` with fsnotify gives an earlier signal for hook-driven transitions.

## 3. Sending text

Two CLI paths:

| Command | Behavior | Fit |
| --- | --- | --- |
| `agent-manager type <id> "<text>"` | Pastes now as the user, presses Enter. No envelope, no queue, no wait for rest. Refused from inside a session. | Agent Desk uses this. |
| `agent-manager send <id> "<text>" --from <name>` | Queues until the agent is at rest, wraps the text in an envelope saying it came from another agent on another machine. Needs a running manager. Rate limited. | Wrong framing for a human reply. |

`type` internals (`internal/tmux/input.go`): writes the text to a temp file, `load-buffer` into a pid-named buffer, `paste-buffer -p -r` (bracketed paste, newlines kept as LF), waits for the echo, then `send-keys Enter`. A per-pane lock file serializes it against the manager's own sends. Exit status 3 means the paste may have reached the pane and must not be retried.

Experiments on the probe session:

| Input | Result |
| --- | --- |
| 5 lines, internal spaces, one blank line | One prompt. Spaces and blank line kept. Agent answered "5". |
| 22 KB, 91 lines | One prompt in 1.4 s, intact. Agent read the last line correctly. |
| Text typed while an AskUserQuestion dialog was open | Landed in the dialog's free-text option and was submitted as the answer. Exit 0, no warning. |
| (Observed, not sent) Workspace trust dialog | Default option is "No, exit". A `type` here would press Enter on it and quit Claude. |
| (Observed, not sent) "Teach auto mode" dialog | Default option is "1. Yes". Status read `finished`. |

Rules for Agent Desk:

- Pass text as one argv element. No shell, no escaping beyond what `exec.Command` does. Trim nothing beyond what `type` trims (leading and trailing whitespace).
- Refuse to send when status is `waiting`, `starting` or `dead`. Show the live pane (M4) instead.
- Before sending on any other status, capture the pane and check for a dialog footer (`Enter to confirm`, `Enter to select`, `Esc to cancel`). If found, block and show the live pane. This covers dialogs that read `finished`.
- Treat exit 3 as "maybe sent": show the message as unconfirmed and let the transcript reconcile it.

Caller pitfall: the CLI identifies its caller by process ancestry, then `TMUX_PANE`, then `AGENT_MANAGER_SESSION_ID` (`main.go`, `callerSession`). If Agent Desk is started from a shell inside an `am_*` pane (a `T` terminal or an agent), every `type` is refused. Agent Desk checks `self: true` in the snapshot at startup and shows a clear error.

## 4. Mapping a session to its transcript

- `agent_session_id` is Claude's session id. Agent-manager mints it and launches Claude with it, so it is known at spawn.
- The transcript is `<claude-config-dir>/projects/<encoded-cwd>/<agent_session_id>.jsonl`.
- Agent Desk resolves it by glob `projects/*/<id>.jsonl`, not by re-encoding the cwd. The encoding is lossy (`/` and `.` both become `-`), and symlinks matter: `/tmp/...` was stored under `-private-tmp-...`.
- `<claude-config-dir>` is `~/.claude`, unless the session's environment profile sets `CLAUDE_CONFIG_DIR`. No profiles exist today. Agent Desk reads profiles with `agent-manager env show <name> --json` and adds those directories to its search roots.

Failure modes:

- **No file yet.** The JSONL appears only after the first turn completes. Before that, the UI shows "waiting for first turn" and the live pane.
- **Conversation switch.** `/clear`, `/resume`, `/fork` or a compaction that continues in a new file (`continued-in` record) changes the conversation. Agent-manager's hook moves `agent_session_id` to the new id. Agent Desk follows the id on each snapshot and re-resolves the file.
- **Non-Claude tools.** `agent_session_id` exists for other CLIs, but transcript parsing is Claude only. Other tools get the live pane view.
- **Large files.** Real transcripts reach 28 MB. The backend parses once on open and tails by offset afterwards. The frontend receives the newest window and pages backwards.

Sibling data under `projects/<encoded-cwd>/<id>/`: `subagents/agent-*.jsonl` (+ `.meta.json`) and `tool-results/` (spilled large outputs). Both are candidates for later milestones.

## 5. Where agent-manager stores state

- `~/Library/Application Support/agent-manager/` on macOS, `~/.config/agent-manager/` on Linux, overridable with `AGENT_MANAGER_CONFIG_DIR` or `XDG_CONFIG_HOME`.
- `state.db` (SQLite, WAL): sessions, groups, settings, message inbox, tasks, poller heartbeat. Schema is internal and migrates every release. Agent Desk does not read it directly.
- `hooks/<id>.settings.json`, `hooks/<id>.status`, `hooks/<id>.agent`: per-session hook config, hook status, pid and tty.
- tmux server `-L agentmgr`, sessions `am_<id>`, agent in pane `.0`.

## 6. MCP session tools

`agent-manager mcp` (stdio) exposes `list_sessions`, `read_session`, `send_session`, `wait_for_session`, `create_session`, `kill_session`, `revive_session`, `archive_session` and more. They refuse every call from a server with no calling session (except `report_issue`). Agent Desk is not a session, so MCP is closed to it. The CLI covers the same ground with no caller.

## 7. Claude Code version and record types

Installed: 2.1.322. Recent files span 2.1.304 to 2.1.322. Survey of 28 files modified in the last 3 days, zero malformed lines.

| Record `type` | Count | Rendering |
| --- | --- | --- |
| `assistant` | 26,515 | Content blocks: `text`, `thinking`, `tool_use`. One record per block, grouped by `message.id`. |
| `user` | 15,422 | String or blocks: `text`, `tool_result`. Classified below. |
| `attachment` | 11,453 | 29 kinds (`environment`, `queued_command`, `file`, `edited_text_file`, reminders). Hidden by default; `queued_command` shown. |
| `system` | 1,227 | Subtypes `turn_duration`, `away_summary`, `stop_hook_summary`, `compact_boundary`, `api_error`, `local_command`, `informational`, `scheduled_task_fire`. |
| `api-request`, `api-request-blob`, `api-request-shape` | 22,072 | Request payload copies. Hidden. Skipped cheaply. |
| `queue-operation` | 9,051 | Queue bookkeeping. Hidden. |
| `ai-title`, `custom-title`, `agent-name`, `last-prompt` | ~13,000 | Session metadata. Used for the header. |
| `mode`, `permission-mode`, `atis-latch`, `agent-setting` | ~11,000 | Mode metadata. Header chip. |
| `file-history-snapshot`, `file-history-delta` | 652 | Hidden. |
| `cost-state`, `tools-used`, `pr-link`, `continued-in` | ~770 | `pr-link` shown as a chip. `continued-in` drives file switching. `cost-state` feeds the cost stretch goal. |

Classifying `user` text records:

| Markers | Meaning | Rendering |
| --- | --- | --- |
| `origin.kind: human`, `promptSource: typed` or `sdk` | Real human prompt. Prompts sent by `type` appear verbatim here. | User bubble. Reconciles optimistic sends. |
| `origin.kind: peer`, `isMeta: true` | Message from another agent | Labelled peer message |
| `origin.kind: task-notification` | Background task result (`<task-notification>`) | Collapsed notice |
| `isCompactSummary: true` | Compaction summary | Collapsed summary block |
| Starts with `<command-name>`, `<local-command-stdout>`, `<system-reminder>` | Slash command and injected context | Command chip, or hidden |

Other observations:

- `thinking` blocks carry an empty `thinking` string (signature only) on this setup. Rendered as a one-line "thought for Ns" marker using `thinkingDurationMs`.
- `toolUseResult` holds structured results: `structuredPatch` and `oldString`/`newString` for edits, `stdout`/`stderr` for Bash, `answers` for AskUserQuestion. Diffs render from `structuredPatch`.
- Agent-manager wraps the first prompt with its own rename directive. The parser collapses that preamble.

## Upstream gaps

Candidates for agent-manager PRs. None blocks v1.

1. **A send that is safe for a human.** `type` with an opt-in gate (for example `type --at-rest`) that refuses when the tool's own rules see a dialog, without the agent envelope `send` adds.
2. **`mark-idle` from the CLI**, so a reply read in another client clears `finished`.
3. **A watch stream** (`snapshot --json --watch`, one JSON line per change) to replace polling.
4. **A `dialog` field** in the session JSON, exposing what the poller already knows.

## Probe session

`agent-desk-probe` (id `3486b2c0`), Claude, root group, directory `/tmp/agentdesk-scratch/probe`. Created for these experiments and still running.
