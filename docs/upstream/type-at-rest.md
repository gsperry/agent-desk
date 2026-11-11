# feat(cli): type --at-rest refuses text that would land on a dialog

## Problem

`agent-manager type` pastes and presses Enter at once. When the session shows a dialog, the paste answers it: on Claude Code's workspace trust dialog, Enter picks "No, exit" and quits the agent. On AskUserQuestion, the text becomes the answer. `type` exits 0 in both cases.

`send` already guards against this. `maybeDeliverInbox` (`internal/execution/polldelivery.go`) holds a message while `engine.TypingHold` matches a dialog rule, and while `promptCarriesTypedText` finds a line part way written. But `send` wraps the text in an envelope that tells the agent it came from another agent session. A client acting for the human, such as a manager on another machine or a GUI, has no way to get that gate without that envelope.

## Proposal

Add `--at-rest` to `type`:

```
agent-manager type <session-id> "<prompt>" --at-rest [--json]
```

With the flag, before the paste, `type` captures the pane and:

1. refuses when `engine.TypingHold(tool, pane)` names a hold (a dialog or approval on screen);
2. refuses when `status.HasDraftBeforeCaret` finds typed text at the prompt;
3. otherwise pastes as today.

A refusal exits with a new status, 4, and prints the hold's description, so a caller can show "the session is showing a dialog" and offer the session instead. JSON output adds `{"refused": "dialog" | "draft", "detail": "..."}`.

Without the flag nothing changes.

## Coverage

The gate comes from each tool's own rules in `builtinTools`, so it applies to every CLI that declares dialog rules and an `activity_cutoff`. A tool without an activity cutoff refuses `--at-rest` outright, the way `send` refuses it.

## Tests

- A table test over captured panes (trust dialog, AskUserQuestion, permission prompt, idle prompt, prompt with a draft) for each tool with fixtures in `internal/status/testdata`.
- An end-to-end test that `type --at-rest` on a pane showing a dialog exits 4 and leaves the pane unchanged.

## Related: text on stdin

`type`, `send` and `spawn --prompt` take the text as a command argument, which other accounts on the machine can read with `ps` while the command runs. Accepting `-` for the text (read from stdin) would keep messages private:

```
printf '%s' "$text" | agent-manager type <session-id> - --at-rest
```

## Why not just use send

`send` is at-most-once queued delivery with sender identity, rate limits and an envelope. `type` is the user's own keystrokes. `--at-rest` keeps `type`'s meaning and adds only the safety check.
