# Proposed agent-manager changes

Gaps Agent Desk works around today. Each file is written as a pull request proposal for [agent-manager](https://github.com/YoanWai/agent-manager), against v0.48.1. None is filed yet.

| Proposal | Agent Desk workaround today |
| --- | --- |
| [type --at-rest](type-at-rest.md) | Reads the screen and matches Claude Code footer text before `type` |
| [mark-idle](mark-idle.md) | Keeps its own "seen" marks in the browser |
| [snapshot --watch](snapshot-watch.md) | Polls `snapshot --json` every second |
| [dialog field](dialog-field.md) | Parses dialogs from `read` output by footer text and option markers |
| [keys](send-keys.md) | Calls `tmux -L agentmgr send-keys` directly for dialog answers |
