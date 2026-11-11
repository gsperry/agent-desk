package usage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func prompt(uuid, at string) string {
	return fmt.Sprintf(`{"type":"user","uuid":%q,"timestamp":%q,"promptSource":"typed","origin":{"kind":"human"},"message":{"role":"user","content":"go"}}`, uuid, at)
}

func reply(id, at string, in, out int) string {
	return fmt.Sprintf(`{"type":"assistant","uuid":"a-%s-%d","timestamp":%q,"message":{"id":%q,"model":"claude-opus","content":[{"type":"text","text":"x"}],"usage":{"input_tokens":%d,"output_tokens":%d,"cache_read_input_tokens":10,"cache_creation_input_tokens":0}}}`, id, out, at, id, in, out)
}

func toolResult(at string) string {
	return fmt.Sprintf(`{"type":"user","uuid":"tr","timestamp":%q,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]},"toolUseResult":{}}`, at)
}

func cost(start string, usd float64) string {
	return fmt.Sprintf(`{"type":"cost-state","startTime":%q,"totalCostUSD":%g}`, start, usd)
}

func writeLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func appendLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		t.Fatal(err)
	}
	// Make sure the modification time moves on coarse clocks.
	later := time.Now().Add(time.Second)
	_ = os.Chtimes(path, later, later)
}

func scanner(root string) *Scanner {
	return &Scanner{Root: root, Location: time.UTC, Now: func() time.Time { return time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC) }}
}

func TestReport(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "projects", "-p", "conv-1.jsonl")
	writeLines(t, main,
		prompt("p1", "2026-11-01T10:00:00Z"),
		reply("m1", "2026-11-01T10:00:30Z", 100, 5),
		// The same reply written again with its final usage.
		reply("m1", "2026-11-01T10:00:40Z", 100, 20),
		toolResult("2026-11-01T10:00:50Z"),
		reply("m2", "2026-11-01T10:01:00Z", 50, 10),
		cost("s1", 0.5),
		// Answered 30 minutes later: 30 min waiting.
		prompt("p2", "2026-11-01T10:31:00Z"),
		// A 10 minute pause is not working time.
		reply("m3", "2026-11-01T10:41:00Z", 10, 1),
		cost("s1", 0.75),
		// Answered the next day after a long gap: waiting capped at an hour.
		prompt("p3", "2026-11-02T09:00:00Z"),
		reply("m4", "2026-11-02T09:00:10Z", 1, 1),
		cost("s2", 0.1),
		`{"type":"mystery"}`,
		`not json`,
	)
	sub := filepath.Join(root, "projects", "-p", "conv-1", "subagents", "agent-a.jsonl")
	writeLines(t, sub,
		prompt("sp", "2026-11-01T10:00:35Z"),
		reply("s1", "2026-11-01T10:00:45Z", 1000, 100),
	)

	rep, err := scanner(root).Report(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if rep.From != "2026-10-27" || rep.To != "2026-11-02" || len(rep.Days) != 7 || rep.Files != 2 {
		t.Fatalf("range %s..%s days %d files %d", rep.From, rep.To, len(rep.Days), rep.Files)
	}
	nov1 := rep.Days[5]
	if nov1.Day != "2026-11-01" {
		t.Fatalf("day order: %+v", rep.Days)
	}
	// Main: 100+50+10 input, 20+10+1 output, 30 cache read; subagent 1000/100/10.
	wantTok := Tokens{Input: 1160, Output: 131, CacheRead: 40}
	if nov1.Tokens != wantTok || nov1.Replies != 4 || nov1.Prompts != 2 {
		t.Fatalf("nov 1 = %+v", nov1.Bucket)
	}
	// Working: 30s + 10s + 10s + 10s (the reply after the tool result); the
	// 10 minute pause is left out.
	if nov1.WorkingMs != 60_000 {
		t.Errorf("working = %d ms", nov1.WorkingMs)
	}
	if nov1.WaitingMs != (30 * time.Minute).Milliseconds() {
		t.Errorf("nov 1 waiting = %d ms", nov1.WaitingMs)
	}
	nov2 := rep.Days[6]
	if nov2.WaitingMs != time.Hour.Milliseconds() || nov2.Prompts != 1 {
		t.Errorf("nov 2 = %+v", nov2.Bucket)
	}
	if got := rep.Totals.CostUSD; got < 0.8499 || got > 0.8501 {
		t.Errorf("cost = %v, want 0.85", got)
	}
	if len(rep.Conversations) != 1 || rep.Conversations[0].ID != "conv-1" || rep.Conversations[0].SubagentTokens != 1110 {
		t.Fatalf("conversations = %+v", rep.Conversations)
	}
	if len(rep.Models) != 1 || rep.Models[0].Model != "claude-opus" {
		t.Fatalf("models = %+v", rep.Models)
	}
}

func TestReportFollowsAppends(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "projects", "-p", "conv-2.jsonl")
	writeLines(t, p, prompt("p1", "2026-11-02T10:00:00Z"), reply("m1", "2026-11-02T10:00:10Z", 5, 5))
	s := scanner(root)
	rep, _ := s.Report(context.Background(), 1)
	if rep.Totals.Input != 5 {
		t.Fatalf("first read input = %d", rep.Totals.Input)
	}
	before := s.files[p].offset
	appendLines(t, p, reply("m2", "2026-11-02T10:00:20Z", 7, 1))
	rep, _ = s.Report(context.Background(), 1)
	if rep.Totals.Input != 12 || rep.Totals.Replies != 2 {
		t.Fatalf("after append = %+v", rep.Totals)
	}
	if s.files[p].offset <= before {
		t.Fatal("offset did not advance")
	}
	// A rewritten, shorter file is read again from the start.
	writeLines(t, p, reply("m9", "2026-11-02T11:00:00Z", 1, 1))
	later := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(p, later, later)
	rep, _ = s.Report(context.Background(), 1)
	if rep.Totals.Input != 1 {
		t.Fatalf("after rewrite input = %d", rep.Totals.Input)
	}
	// A deleted file stops counting.
	_ = os.Remove(p)
	rep, _ = s.Report(context.Background(), 1)
	if rep.Totals.Input != 0 || rep.Files != 0 {
		t.Fatalf("after delete = %+v files %d", rep.Totals, rep.Files)
	}
}

func TestPartialLastLineWaits(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "projects", "-p", "conv-3.jsonl")
	line := reply("m1", "2026-11-02T10:00:10Z", 9, 9)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(line[:40]), 0o600); err != nil {
		t.Fatal(err)
	}
	s := scanner(root)
	rep, _ := s.Report(context.Background(), 1)
	if rep.Totals.Input != 0 {
		t.Fatal("counted a partial line")
	}
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	_, _ = f.WriteString(line[40:] + "\n")
	f.Close()
	later := time.Now().Add(time.Second)
	_ = os.Chtimes(p, later, later)
	rep, _ = s.Report(context.Background(), 1)
	if rep.Totals.Input != 9 {
		t.Fatalf("completed line input = %d", rep.Totals.Input)
	}
}
