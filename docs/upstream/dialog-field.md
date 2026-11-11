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

## Coverage

Every tool with dialog rules reports `hold`. Option parsing starts with the tools whose status fixtures already include choice dialogs.

## Tests

Table tests over the dialog fixtures in `internal/status/testdata`, one per tool, checking `hold`, the question and the options.
