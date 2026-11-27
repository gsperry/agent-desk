# Proposed agent-manager changes

Gaps Agent Desk works around today. Each file is written as a pull request proposal for [agent-manager](https://github.com/YoanWai/agent-manager), against v0.48.1. None is filed yet.

| Proposal | Agent Desk workaround today |
| --- | --- |
| [type --at-rest](type-at-rest.md) | Reads the screen and matches Claude Code footer text before `type` |
| [mark-idle](mark-idle.md) | Keeps its own "seen" marks in the browser |
| [snapshot --watch](snapshot-watch.md) | Polls `snapshot --json` every second |
| [dialog field](dialog-field.md) | Parses dialogs from `read` output by footer text and option markers |
| [keys](send-keys.md) | Calls `tmux -L agentmgr send-keys` directly for dialog answers |
| [--if-screen](if-screen.md) | Checks a dialog fingerprint on a fresh read under a per-session lock, then presses keys in a separate command |
| [type --no-enter](type-no-enter.md) | Presses Enter again after `type` when a typed answer to a several-answer question ends unticked |
| [session commands](session-commands.md) | Rename and delete landed upstream (rename-session in 0.50, delete in 0.49); restart and fork stay in the TUI; the spawn form takes model and effort as free text |
| [reorder](reorder.md) | Follows the order `snapshot --json` lists; reordering stays in the TUI |
