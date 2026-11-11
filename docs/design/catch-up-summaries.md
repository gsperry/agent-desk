# Catch-up summaries

Status: built, off by default (decided: opt-in setting, labelled as costing tokens; default model `haiku`; prompts, replies and tool call lines only). The rest of this page is the design as proposed.

## What

A "Catch me up" button on a session or on the overview that writes a few lines on what happened since you last looked: what the agent did, where it is stuck, what it needs from you.

## Why it is not built yet

The hard rules in `CLAUDE.md` say Agent Desk runs only `agent-manager`, `tmux` and `code`, and never calls a model API. A summary needs a model. The only way to get one without an API key is the user's own Claude Code CLI, which spends tokens on their provider (Anthropic, Vertex AI or Bedrock). That changes a hard rule, so it needs the owner's decision.

## Design

- Runs `claude -p` through `internal/run` (argv only, timeout), never a shell. The user's provider setup is inherited from the environment, as for every other CLI Agent Desk runs. No API key is read or asked for.
- Flags: `--output-format json` for the result and its token use, `--model` from a setting (default: the smallest model the user's provider offers, chosen in Settings), `--no-session-persistence` so no transcript is written under `~/.claude`, no tools (`--disallowedTools` for every tool, or `--tools ""` if available), `--strict-mcp-config` with no MCP servers, and a working directory of an empty temp dir so no `CLAUDE.md` is loaded.
- Input: the session's items since the "since you last looked" mark, rendered with `internal/export` (prompts and replies, tool calls one line each), capped at about 40k characters, newest kept. Passed on stdin, not argv.
- Runs only when asked: one click, one summary. Never on a timer, never from a rule.
- Shows the result with the tokens it used and the model, and caches it by session and transcript size, so asking twice costs nothing.
- Off by default. Settings has "Allow catch-up summaries (spends tokens on your Claude Code provider)".

## Decisions needed

1. Allow Agent Desk to run `claude -p` at all, behind an opt-in setting? This amends the "only local CLIs" hard rule to "only local CLIs; `claude -p` only on an explicit click, opt-in".
2. Which model by default.
3. Whether a summary may include tool output, or only prompts, replies and tool call lines (the design above).
