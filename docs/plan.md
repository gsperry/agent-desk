# Agent Desk plan

Basis: [integration-notes.md](integration-notes.md).

## Architecture

The default architecture holds: a Go backend with an embedded React/TS frontend, REST plus one WebSocket. Phase 0 changes these parts:

| Area | Decision | Why |
| --- | --- | --- |
| Session list | Poll `agent-manager snapshot --json` every 1 s. fsnotify on `hooks/*.status` triggers an immediate extra poll. | No push API. Snapshot costs ~160 ms. Hook files change before the poller stores the status. |
| Manager not running | Banner plus a "Start headless manager" button that runs `agent-manager serve --background`. | Status is frozen without a poller. |
| Grouping | `group` path when set. Otherwise the repository: `git rev-parse --git-common-dir` from the session directory, so `<repo>-worktrees/<name>` lands under its repo. Otherwise the directory. | All current sessions sit in root. |
| Archived sessions | Hidden by default, toggle in the sidebar. | 22 of 29 sessions are archived and dead. |
| Send | `agent-manager type`, not `send`. Gated: refused on `waiting`, `starting`, `dead`, and on any pane showing a dialog footer. | `send` labels the text as coming from another agent. Unguarded `type` answers or dismisses dialogs. |
| Live pane | `agent-manager read <id> --json` for text. M4 key buttons use `tmux -L agentmgr send-keys -t am_<id>.0`, the documented fallback. | The CLI has no key-sending command for agents. |
| Seen / finished | Agent Desk keeps a per-session "seen" mark in browser storage. | `finished` clears to `idle` only in the TUI. |
| Transcript | Backend parses the full file once on open (skipping `api-request*` cheaply), keeps normalized items in memory, tails by offset. Frontend gets the newest ~300 items and pages backwards. | Files reach 28 MB. |
| Parser location | Go owns record parsing and tolerance. The frontend owns grouping items into turns and pairing tool calls with results. | One tolerant parser, tested once. The view-model mapping is what Vitest covers. |
| Non-Claude sessions | Listed with status. The session view shows the live pane only. | Parsing is Claude only. |

### Backend layout

```
cmd/agent-desk/        main: flags, open browser, signal handling
internal/am/           agent-manager CLI client (snapshot, type, read, serve, env), plus a fixture client
internal/grouping/     session -> project group
internal/locate/       agent_session_id -> JSONL path, across config roots
internal/tail/         offset follower: partial lines, truncation, rotation, deletion
internal/transcript/   tolerant JSONL parser -> normalized items
internal/hub/          state, subscriptions, change fan-out
internal/server/       HTTP, WebSocket, auth, embedded assets
web/                   Vite + React + TS
testdata/              anonymized JSONL samples, each with its Claude Code version
```

### Protocol

- `GET /api/sessions`
- `GET /api/sessions/{id}/transcript?before=<seq>&limit=<n>`
- `POST /api/sessions/{id}/send` `{text, clientId}`
- `GET /api/sessions/{id}/pane` (M4)
- `POST /api/sessions/{id}/keys` `{key}` from a fixed allow list (M4)
- `POST /api/manager/start`
- `WS /api/ws`: server pushes `sessions` and `transcript.append`. Client sends `subscribe` and `unsubscribe` for transcripts.

### Security

- Listen on `127.0.0.1` only, port from `--port` (default 7317, falls back to a free port).
- 32-byte random token per start. Startup opens `http://127.0.0.1:<port>/#t=<token>`. The page moves the token into `sessionStorage` and clears the URL.
- REST requires `Authorization: Bearer <token>`. WebSocket carries it as a subprotocol, since browsers cannot set headers there.
- Every request checks `Host` is `127.0.0.1:<port>` or `localhost:<port>` (DNS rebinding). State-changing requests and WebSocket upgrades also check `Origin`.
- Static assets need no token and hold no data.
- No network calls besides localhost. No telemetry. Nothing written under `~/.claude`. The app never reads `ANTHROPIC_API_KEY` or calls any model API.

### Child processes

- Every `agent-manager` and `tmux` call goes through one runner with a timeout and argv only (no shell).
- At startup Agent Desk checks the snapshot for `self: true`. If set, it was started inside an `am_*` pane, where `type` is refused, and it says so.

## Milestones

Each step is one conventional commit. Each milestone is a feature branch merged to `main` by PR.

**M0: Scaffold.** Repo, Apache 2.0 `LICENSE`, `NOTICE`, `README` stub, `CLAUDE.md`, Go module, Vite app, `Makefile` (`dev`, `build`, `test`, `lint`), server skeleton with auth, Host and Origin checks and their tests, CI on macOS and Linux. Runnable result: `make build` gives one binary that opens a page showing "connected".

**M1: Read-only viewer.**
1. `am` client, snapshot poller, hook watcher, grouping. Table tests with a fixture client.
2. `locate` and `tail`, with tests for truncation, rotation and deletion.
3. `transcript` parser with anonymized samples in `testdata/` (an anonymizer keeps structure and replaces text). Unknown or malformed records become `raw` items.
4. WebSocket fan-out.
5. Sidebar: groups, status glyph and color, archived toggle, manager-down and agent-manager-missing banners.
6. Transcript view: proportional font, `react-markdown` + `remark-gfm`, Shiki, collapsed tool calls, diffs from `structuredPatch`, empty-thinking markers, raw-event blocks.
7. Auto-scroll that pauses on scroll-up, "jump to latest".
8. Failure states: no transcript yet, file deleted, session dead, non-Claude tool.
9. Playwright smoke on fixtures, screenshot reviewed before calling M1 done.

Prior art read in step 3 and 6: VibeDeck (Apache 2.0) and claude-code-log (MIT) for parsing and tool or diff rendering. CloudCLI for UX ideas only. Any ported logic is credited in `NOTICE` and file headers.

**M2: Talk back.** Composer (Enter newline, Cmd+Enter send), drafts per session in `localStorage`, send gate with a clear reason when blocked, optimistic bubble reconciled against the next human `user` record, exit 3 shown as "may have been sent".

**M3: Waiting for me.** Attention = `waiting`, `errored`, or `finished` not yet seen. Filter and badge, count in the tab title. Notifications on flips to waiting (see open question 3). Cmd+K switcher. j/k through attention sessions.

**M4: Live pane.** Monospace pane from `read`, refreshed while open. Buttons for allow-listed keys (1 to 4, Up, Down, Enter, Esc). Shown automatically when a session is `waiting` or a dialog is detected.

**M5: Quality of life.** Transcript search, `code -g path:line` links, per-type collapse defaults, light and dark themes, font size and reading width.

**Stretch.** Spawn from the GUI, cost tracking from `cost-state` and usage fields, subagent transcripts, Tauri wrapper.

## Risks

- **JSONL drift.** Mitigated by raw-event fallback, version-tagged samples, and a parser test per sample.
- **Status latency target (2 s).** agent-manager polls every 2 s, so the stored status can lag up to 2 s by itself. The hook watcher closes most of the gap for Claude. The rest is upstream gap 3.
- **Dialog detection by footer text.** It is a heuristic on Claude's TUI strings. A miss means the send gate falls back to status alone. Upstream gap 1 replaces it.
- **Large transcripts in memory.** Several 30 MB sessions open at once is fine. Items for unsubscribed sessions are dropped after a while.

## Upstream proposals

Written up as PR proposals in [docs/upstream](upstream/README.md): `type --at-rest`, `mark-idle` and `snapshot --watch` (done in M3); a `dialog` field and key sending for agents (M4).
