# Roadmap after v1

Milestones M7 to M10. Each lands on a local feature branch with step commits and is squash-merged into `main`. Nothing is pushed until reviewed.

Constraints that apply to every milestone: the hard rules in [CLAUDE.md](../CLAUDE.md), anonymized fixtures only in `testdata/`, no sends or spawns against real sessions in tests, and screenshots from fixtures only.

## M7: Orchestrator oversight (done)

Goal: see what every agent and subagent is doing without reading transcripts.

| Step | Feature | Data | Design |
| --- | --- | --- | --- |
| 7.1 | Fix: Agent Desk started inside a Claude Code session | `CLAUDE_CODE_SESSION_ID` makes agent-manager treat child processes as that session | Drop it in `run.Environ` |
| 7.2 | Parser carries usage and compaction | `message.usage`, `compactMetadata.preTokens`, `compactMetadata.trigger`, `cost-state` | Usage meta on assistant items; compaction meta on compact items; `cost-state` becomes a meta item |
| 7.3 | Last activity per session | Transcript mtime | Poller stats each session's transcript through the locator's cache; `lastActivity` on the session |
| 7.4 | Since you last looked | `lastActivity`, item seq | Seen marks per conversation in localStorage; a dot on sidebar rows with new activity; a divider in the transcript before the first unseen item |
| 7.5 | Context meter | Last usage: input + cache read + cache creation tokens | Header meter. The limit is the largest auto-compaction `preTokens` seen for that model, learned and kept in localStorage; until one is seen, tokens only |
| 7.6 | Subagent tree | `<conv>/subagents/agent-*.jsonl` and `.meta.json` (`agentType`, `description`, `toolUseId`, `spawnDepth`); parent `task-notification` records (`<tool-use-id>`, `<status>`); sync Agent results | `GET /api/sessions/{id}/agents` lists subagents with status; `GET .../agents/{agentId}/transcript` parses one. An Agents panel in the session view, and a "View transcript" link on Agent tool calls |
| 7.7 | Fleet overview | Last ~512 KB of each running transcript | `GET /api/overview`: per session, the latest activity, last reply line, last prompt, context tokens and model, cached by size and mtime. The home screen shows a card per session, attention first |

Tests: parser tables for usage, compaction and cost records; locator-backed activity; subagent status from notifications (completed, failed, running, sync); overview summaries on fixtures; Vitest for seen marks, the threshold learner and card ordering; Playwright for the overview, the divider, the meter and the agents panel. A new fixture session with subagents.

## M8: Review and steer (done)

Goal: supervise and correct work, not only chat.

| Step | Feature | Design |
| --- | --- | --- |
| 8.1 | Changes view | `GET /api/sessions/{id}/changes`: `git status --porcelain` and `git diff` (working tree, and against the merge base for worktree branches) in the session's repo, read-only, argv only, size-capped. A Changes tab with per-file diffs |
| 8.2 | Review comments | Comment on diff lines; "Send review" composes one numbered prompt (file, line, comment) and sends it through the normal gated send |
| 8.3 | Conflict warnings | Files written by Edit, Write and MultiEdit per session, from transcripts; warn when two live sessions wrote the same absolute path in the same checkout. Sidebar marker and a list in the session header |
| 8.4 | Activity filters | Transcript filter: all, commands, file changes, errors. Risky shell commands (`rm -rf`, `git push --force`, `git reset --hard`, `sudo`, `DROP TABLE`, `chmod -R`) highlighted |
| 8.5 | Reply snippets | Snippets in Settings, inserted from a composer menu or `/name`; variables `{session}`, `{branch}`, `{project}` |
| 8.6 | Image paste | Paste an image into the composer; the backend saves it owner-only under the user cache directory and returns its path, which the composer inserts. Old images are cleaned up after 7 days |

## M9: History and insight (done)

| Step | Feature | Design |
| --- | --- | --- |
| 9.1 | All conversations | Every `projects/*/*.jsonl`: title, directory, last activity, size; searchable; read with the normal viewer |
| 9.2 | Resume into agent-manager | `agent-manager spawn --resume <id> --directory <dir>` from a past conversation, behind the spawn form |
| 9.3 | Session templates | Named templates (prompt, CLI, model, effort, worktree, group) in Settings; the new session form starts from one |
| 9.4 | Export | Transcript to Markdown, downloaded by the browser |
| 9.5 | Pins and notes | Pin items and keep a note per conversation in localStorage; a Pins list in the session view |
| 9.6 | Usage and time | From `cost-state` and per-turn usage: tokens and time per session, project and day, and time sessions waited on the human. Cost shown as Claude Code reports it, labelled list price |

## M10: Bigger bets

| Step | Feature | Approach | Needs a decision |
| --- | --- | --- | --- |
| 10.1 | Live terminal (read-only, done) | xterm.js fed by `agent-manager read --styled` polling; read-only first | Whether to allow typing later |
| 10.2 | Rules (done; suggested replies always need a click) | Local rules: when a session reaches a status, notify; optionally send a configured prompt, which asks for confirmation each time unless marked trusted | Which rules may send without confirmation |
| 10.3 | Catch-up summaries (done, off by default) | [design/catch-up-summaries.md](design/catch-up-summaries.md) | Decided: opt-in, labelled as costing tokens |
| 10.4 | Desktop app | Design in [design/desktop-app.md](design/desktop-app.md); not built (Tauri 2 needs a newer Rust than the installed 1.82) | Rust update, signing and distribution, repository layout |
| 10.5 | Away from the desk | Design in [design/away-from-desk.md](design/away-from-desk.md); not built | All of it; nothing is built without a decision |
