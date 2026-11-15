# feat(cli): type --no-enter enters text without pressing Enter

## Problem

`agent-manager type` pastes the text and presses Enter. That is right at a prompt, but not in a text field inside a dialog, where Enter means something of its own.

In Claude Code's AskUserQuestion, a question that takes several answers has a "Type something" row with a checkbox. Typing into it ticks the box, and Enter toggles it. So `type` on that row ticks the box with the text, then its Enter unticks it, and the typed answer is left out of the reply. It exits 0.

A client acting for the human cannot avoid this without pressing keys in the pane itself. Agent Desk now reads the screen after `type` and presses Enter once more through `tmux -L agentmgr send-keys` when the row ended unticked.

## Proposal

Add `--no-enter` to `type`:

```
agent-manager type <session-id> "<text>" --no-enter [--json]
```

With the flag, `type` pastes the text and stops. Without it nothing changes.

A client then types into a dialog's text field and decides on its own whether to press Enter, through `agent-manager keys` if that lands (see [keys](send-keys.md)), or leaves the field as it is.

## Tests

- A test that `type --no-enter` pastes into a pane and sends no Enter, against a fake tmux runner, alongside the existing `type` tests.
- The help text lists the flag.
