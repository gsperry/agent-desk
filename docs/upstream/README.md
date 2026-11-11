# Proposed agent-manager changes

Gaps Agent Desk works around today. Each file is written as a pull request proposal for [agent-manager](https://github.com/YoanWai/agent-manager), against v0.48.1. None is filed yet.

| Proposal | Agent Desk workaround today |
| --- | --- |
| [type --at-rest](type-at-rest.md) | Reads the screen and matches Claude Code footer text before `type` |
| [mark-idle](mark-idle.md) | Keeps its own "seen" marks in the browser |
| [snapshot --watch](snapshot-watch.md) | Polls `snapshot --json` every second |

Still to write, for M4: a `dialog` field in session JSON, and a way to send named keys to an agent session.
