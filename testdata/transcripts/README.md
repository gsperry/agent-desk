# Transcript samples

Real Claude Code transcripts, anonymized with `go run ./tools/anonymize`. Structure, keys, ids, timestamps, versions, tool names and parser markers are kept. Other letters are replaced with filler of the same case. Request payload records (`api-request*`) are cut to the first three.

| File | Claude Code versions | Records | Source |
| --- | --- | --- | --- |
| `session-a.jsonl` | 2.1.312, 2.1.319 | 516 | A complete short session with edits, Bash, AskUserQuestion and subagents |
| `session-b.jsonl` | 2.1.319 | 600 | The first 600 records of a session that compacts early |
| `probe.jsonl` | 2.1.322 | 64 | The Phase 0 probe: agent-manager preamble, multi-line and long prompts, AskUserQuestion answered by a typed prompt |
| `edge-cases.jsonl` | n/a | 12 | Hand-written malformed and unknown records |

Add a sample when Claude Code introduces a record shape the parser does not know, and record its version here.
