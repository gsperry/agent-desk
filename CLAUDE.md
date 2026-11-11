# Agent Desk

Local GUI over agent-manager. Go backend with the React frontend embedded, one binary, served on 127.0.0.1.

Background: [docs/integration-notes.md](docs/integration-notes.md) (how agent-manager and Claude Code data work), [docs/plan.md](docs/plan.md) (architecture and milestones).

## Hard rules

- **Never write to `~/.claude`.** Transcripts are read-only input.
- **Never call the Anthropic API or any model API, and never ask for or read an API key.** Claude Code is billed through Vertex. Agent Desk only reads local files and runs local CLIs (`agent-manager`, `tmux`).
- **Do not fork or vendor agent-manager.** Integrate through its CLI. Gaps that need upstream changes go in `docs/upstream/` as PR proposals.
- **Every tmux call names the socket**: `tmux -L agentmgr`. A bare `tmux` hits the user's own server.
- **The JSONL parser is tolerant.** Unknown or malformed records become raw items. Never crash on input, never silently drop a record. New record shapes get an anonymized sample in `testdata/` with the Claude Code version it came from.
- **Send through `agent-manager type`**, gated on status and on a dialog check of the pane. Never `send`, which labels text as coming from another agent.
- Keep the server's checks: loopback bind, token on every API request and WebSocket, Host check on every request, Origin check on state-changing requests and upgrades.

## Commands

```sh
make dev     # air + Vite, hot reload
make build   # bin/agent-desk with embedded frontend
make test    # go test -race ./... and vitest
make lint    # gofmt, go vet, staticcheck, eslint, tsc
make e2e     # Playwright smoke test on testdata/fixtures/desk
go run ./tools/parsecheck ~/.claude/projects/*/*.jsonl   # raw-item report on real data
go run ./tools/anonymize in.jsonl > testdata/transcripts/x.jsonl   # new sample
```

Check every new sample for leaks before committing: grep it for names, hosts and paths.

## Layout

- `cmd/agent-desk`: flags, token, listener, browser open, fixture mode
- `internal/run`: argv-only process runner with timeouts
- `internal/am`: agent-manager CLI client (`snapshot --json`, `serve --background`, `env show`) and the fixture client
- `internal/grouping`: session to project (group, git repo via common dir, or directory)
- `internal/locate`: conversation id to `<config>/projects/*/<id>.jsonl`
- `internal/tail`: offset follower (partial lines, truncation, replacement, deletion)
- `internal/transcript`: tolerant JSONL parser to typed items
- `internal/hub`: session poller (1 s plus hook-file events) and live transcripts
- `internal/manage`: spawn, kill, revive, archive, move and groups through agent-manager, validated against current state
- `internal/search`: transcript search (raw-line prefilter, parse candidates only)
- `internal/open`: open a file in VS Code (`code -g`), existing paths only
- `internal/pane`: dialog parser and answers (allow-listed keys through `tmux -L agentmgr send-keys`)
- `internal/send`: reply gate (status, dialog footer, composer draft, text cleaning) and delivery through `agent-manager type`
- `internal/server`: HTTP, WebSocket, auth checks
- `web/src/model`: pure view-model logic, covered by Vitest
- `web/src/render`, `web/src/components`: React views
- `web/embed.go` embeds `web/dist`.

## Conventions

- Conventional commits, one per milestone step. Feature branch per milestone, merged by PR.
- Go: table-driven tests next to the code. `gofmt`, `go vet`, `staticcheck` clean.
- TS: strict mode, ESLint `strictTypeChecked`, Vitest for view-model logic.
- Docs and comments are lean and declarative. No em dashes.
- UI milestones end with a Playwright screenshot that has been looked at.
