# feat(cli): report the dialog on screen in session JSON

## Problem

A client that wants to answer a session's dialog has to capture the pane and parse it. agent-manager already decides whether a dialog is on screen: each tool's rules in `builtinTools` feed `engine.TypingHold`, which holds queued messages while a dialog shows. That knowledge does not reach `sessions --json`, `snapshot --json` or `read --json`. Some dialogs, such as Claude Code's "Teach auto mode" prompt, report `finished`, so the status alone does not say a dialog is up.

## Proposal

Add a `dialog` object to `read --json`, and a `dialog` flag to session rows in `sessions --json` and `snapshot --json`:

```json
"dialog": {
  "hold": "permission",
  "question": "Do you want to proceed?",
  "options": [{"label": "Yes", "selected": true}, {"label": "No", "selected": false}]
}
```

`hold` is the description `TypingHold` already returns. `question` and `options` come from per-tool rules, added alongside the existing status rules, for tools that draw a numbered or marked choice list. A tool without option rules reports `hold` alone.

## Why parsing the pane keeps breaking

Agent Desk parses Claude Code's screen, and each layout change has left a question unanswerable until the parser caught up. From one week, on Claude Code 2.1.319 to 2.1.322:

- A question that wraps across lines lost all but its last line.
- A multi-select question lists an unnumbered "Next" between its last option and "Chat about this"; without it, the key presses for "Chat about this" landed on Next and submitted.
- The review step of a multi-question prompt has no footer, and can end with a rule that carries the session's title ("──── title ─").
- The prompt box's rules can carry the same title, which hid the box from the check that a reply lands in it.
- `waiting` stays set after the user declines a question or permission prompt (no `Stop` hook runs), so the status alone does not say a dialog is up.

agent-manager already decides whether a dialog holds typing. Reporting that decision, and the options when its rules can read them, would let clients stop guessing. Agent Desk keeps `tools/panecheck` to catch drift in the meantime.

## Coverage

Every tool with dialog rules reports `hold`. Option parsing starts with the tools whose status fixtures already include choice dialogs.

## Tests

Table tests over the dialog fixtures in `internal/status/testdata`, one per tool, checking `hold`, the question and the options.
