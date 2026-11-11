# Agent Desk

A local GUI for reading and driving the agent sessions that [agent-manager](https://github.com/YoanWai/agent-manager) runs.

Website: [agent-desk.app](https://agent-desk.app)

agent-manager stays the session engine: every agent runs in its tmux server, and closing Agent Desk leaves them all running. Agent Desk is the viewing and talking surface: transcripts rendered as documents, a proper composer, and notifications when a session needs you.

Agent Desk is an independent project. It is not affiliated with or endorsed by the agent-manager authors or Anthropic.

![A session rendered in Agent Desk: the sidebar groups sessions by project with live status, and the transcript reads as a document](docs/screenshot.png)

The screenshot shows anonymized fixture data (`make screenshot`).

Status: early. Built and used on macOS (Apple Silicon); CI also builds and tests on Linux. The design and milestones are in [docs/plan.md](docs/plan.md).

## Prerequisites

- [agent-manager](https://github.com/YoanWai/agent-manager) 0.48 or later, with tmux
- [Claude Code](https://docs.anthropic.com/en/docs/claude-code)
- Go 1.27 and Node 24 or later, to build

## Install

There are no prebuilt binaries yet. Build from source:

```sh
git clone https://github.com/gsperry/agent-desk
cd agent-desk
make build
install -m 0755 bin/agent-desk ~/.local/bin/agent-desk   # or any directory on your PATH
```

To update, pull and repeat the last two commands.

## Run

```sh
agent-desk
```

It opens Agent Desk in your browser. The link carries a token that changes on every start; the terminal prints it too, in case the browser does not open. Closing the terminal stops Agent Desk and leaves every agent running.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--port` | `7317` | Port on 127.0.0.1. A busy port falls back to a free one. |
| `--no-open` | off | Print the URL without opening a browser. |

## Oversight

- **Overview:** the home screen (click "Agent Desk" in the sidebar) shows a card per session: what it is doing now, its last reply and your last prompt, context use, and when it last changed. Sessions that need you come first.
- **Subagents:** the Agents button in a session's header lists the subagents its conversation spawned, running first, with type, description and duration. Click one to read its transcript, live while it runs. Agent tool calls link to their subagent.
- **Since you last looked:** sidebar rows with new activity get a dot, and opening the session shows a divider before what arrived since your last visit.
- **Context meter:** the session header shows how much context the latest turn used and, once that model has auto-compacted in a transcript Agent Desk has read, how close it is to compacting again.

## Review and steer

- **Changes:** the Changes button shows the session's git changes, uncommitted or since its branch left the default branch. Click a diff line to comment; Send review sends all comments as one numbered prompt. Git runs read-only with external diff tools, text converters, the repository's filter drivers, hooks and fsmonitor disabled, so a repository's config cannot make it run commands.
- **Shared files:** ⚠ on a sidebar row and in the session header means another live session wrote the same file in the last six hours.
- **Activity filter:** above the transcript, show all activity, only shell commands, only file changes, or only errors. Risky shell commands (recursive deletes, force pushes, hard resets, `sudo` and others) carry a warning badge.
- **Snippets:** the composer's ⋯ menu inserts a saved reply, or type `/name` and press Tab. Edit them in Settings; `{session}`, `{branch}`, `{project}` and `{directory}` are filled in.
- **Images:** paste an image into the composer. It is saved owner-only under your user cache directory, its path is inserted for the agent to read, and it is removed after seven days.

## History and insight

- **All conversations** (sidebar footer): every Claude Code conversation on disk, not only those agent-manager runs, newest first with title, directory and branch. Filter by any of them, and open one read-only in the normal viewer.
- **Resume:** a past conversation no session holds can be reopened as a new agent-manager session (`agent-manager spawn --resume`), in the directory it was saved in. One a session already holds links to that session instead.
- **Export:** the Export button in a session or conversation downloads it as Markdown: prompts and replies in full, tool calls one line each, failed tool output in short. Thinking is left out.
- **Templates:** "Save as template" in the new session form keeps its CLI, directory, group, worktree, prompt, model and effort; pick one at the top of the form next time. Remove them in Settings.
- **Pins and notes:** pin a prompt or reply with the pin beside it, and keep a note per conversation in the Notes panel. Both stay in this browser.
- **Usage** (sidebar footer): tokens, replies, agent working time and time waiting on you, by day, model, project and conversation, subagents included. Cost is what Claude Code reported, at list price, which can differ from what your provider bills.

## Replying

Each session has a composer under its transcript. Enter adds a line; Cmd+Enter (or Ctrl+Enter) sends through `agent-manager type`. Drafts are kept per session.

A reply is refused, with the reason, when it would land somewhere other than the agent's prompt: the session is waiting on a question or permission prompt, a dialog is on screen, or text is already typed into the agent's own prompt in the terminal. Prompts and dialogs open the Terminal panel described below. A reply sent while the agent works is queued by Claude Code and shows as queued.

## Managing sessions

- **New session:** ＋ in the sidebar header, or the ＋ on a project or group heading to prefill its directory and group. Pick the directory, name (or let the agent name itself), CLI, group, worktree, first prompt, and optionally model and effort.
- **Stop, revive, archive, restore, move:** buttons in the session header. Stop asks first, since it interrupts the agent; the session stays listed and Revive resumes the same conversation.
- **Groups:** "New group" in the sidebar footer; × on a group heading deletes it, moving its sessions to the root.

Rename, delete, restart with a fresh context, and fork have no agent-manager command yet, so they stay in agent-manager's TUI (`r`, `d`, `R`, `f`). See [docs/upstream/session-commands.md](docs/upstream/session-commands.md).

## Answering prompts

When a session waits on a permission prompt or a question, a Terminal panel opens under its transcript with the agent's screen and a button for each option, plus Cancel (Esc). The Terminal button in the session header opens the panel for any running session. Options that need typing, such as "Type something", are left to agent-manager.

"Color" in the panel shows the screen as the terminal draws it, colors included, through `agent-manager read --styled`. It is read-only: nothing typed there reaches the agent.

Agent Desk answers by pressing keys through agent-manager's tmux server (`tmux -L agentmgr send-keys`), limited to Up, Down, Enter and Escape. Before choosing an option it reads the screen again and refuses if the choice changed.

## Search, files and settings

- **Search:** the box at the top of the sidebar (or `/`) searches every Claude session's transcript: prompts, replies, thinking, tool input and tool output. Click a result to see it in context, then open the session.
- **Files:** inline code that names a file, and the Open button on file tools, open the file in VS Code at the right line (`code -g path:line`). Relative paths resolve against the session's directory. If `code` is not on your PATH, run "Shell Command: Install 'code' command in PATH" in VS Code.
- **Settings** (gear in the sidebar): theme (system, light or dark), font size, reading width, and whether each kind of tool call and thinking starts expanded or collapsed.

## Catch-up summaries

Off by default. Settings → Catch-up summaries adds a "Catch me up" button to sessions: it runs your own Claude Code CLI (`claude -p`) on what happened since you last looked, with no tools, no MCP servers and nothing saved, and shows the summary with the tokens and cost it used. **Each summary spends tokens on your Claude Code provider**, about $0.002 of fixed overhead on the default `haiku` model plus the excerpt. Nothing runs until you press the button; repeating the same request is answered from cache.

## Rules

Settings → Rules: when a session matching a name, project or group reaches a status (waits for you, finishes a turn, hits an error, goes idle), notify you, suggest a reply, or both. A suggested reply appears above the composer with Send, Edit and Dismiss, and is only sent when you press Send. It goes away when the session's status moves on. Rules run while an Agent Desk tab is open.

## Sessions that need you

A session needs you when it is waiting on a question or permission prompt, has errored, or has finished a turn you have not looked at yet. The sidebar's "Needs you" filter and the tab title show the count.

| Key | Action |
| --- | --- |
| ⌘K / Ctrl+K | Switch to any session by name |
| j / k | Next / previous session that needs you |
| / | Search transcripts |
| Esc | Leave the composer or the search box |

"Turn on notifications" in the sidebar enables browser notifications when a session starts waiting or errors, and optionally when a turn finishes. Notifications need an open Agent Desk tab. A session you are looking at does not notify.

## Development

```sh
make dev     # backend on :7318 under air plus Vite on :5173, with hot reload on both
make test    # Go and Vitest
make lint    # gofmt, go vet, staticcheck, ESLint, tsc
make e2e     # Playwright smoke test against fixture sessions
```

`--fixtures <dir>` serves sessions from `<dir>/snapshot.json` and transcripts from `<dir>/claude`, with no agent-manager or tmux. `testdata/fixtures/desk` is the set the smoke test uses.

After a Claude Code upgrade, check the parser against your own transcripts. It prints counts and labels, never content:

```sh
go run ./tools/parsecheck ~/.claude/projects/*/*.jsonl
```

## Security model

- Listens on 127.0.0.1 only.
- A random token is generated at startup and required on every API request and WebSocket. It travels in the URL fragment, which never reaches the server, and the page moves it into session storage.
- Every request must name the server's own host, which blocks DNS rebinding. State-changing requests and WebSocket upgrades must come from the app's own origin.
- No telemetry and no network calls beyond localhost. Agent Desk never calls a model API and never asks for an API key. The optional catch-up summaries, off by default, run your own Claude Code CLI only when you press the button.
- Read-only toward Claude Code's data: nothing is written under `~/.claude`.

Report security problems privately; see [SECURITY.md](SECURITY.md).

## Known limits

Agent Desk reads Claude Code's session transcripts (`~/.claude/projects/**/*.jsonl`). That format is undocumented and changes between releases. Records Agent Desk does not recognize are shown as collapsed raw events rather than dropped, but rendering can lag behind new Claude Code versions.

Session status comes from agent-manager and only advances while agent-manager (its TUI or `agent-manager serve`) is running. It reaches Agent Desk about 2.3 s after a change, because agent-manager stores status on a 2 s cycle. Proposed fixes for this and other gaps are in [docs/upstream](docs/upstream/README.md).

## License

[Apache 2.0](LICENSE). See [NOTICE](NOTICE).
