package overview

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/locate"
)

const conv = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

func lines(ls ...string) string { return strings.Join(ls, "\n") + "\n" }

var sample = lines(
	`{"type":"user","uuid":"u1","timestamp":"2026-11-10T10:00:00Z","origin":{"kind":"human"},"message":{"content":"Fix the flaky test\nthen open a PR"}}`,
	`{"type":"assistant","uuid":"a1","message":{"id":"m1","model":"opus","usage":{"input_tokens":5,"cache_read_input_tokens":40000,"cache_creation_input_tokens":0,"output_tokens":9},"content":[{"type":"text","text":"Looking at the test now."}]}}`,
	`{"type":"assistant","uuid":"a2","message":{"id":"m2","model":"opus","usage":{"input_tokens":5,"cache_read_input_tokens":50000,"cache_creation_input_tokens":100,"output_tokens":9},"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"go test ./...","description":"Run the tests"}}]}}`,
)

func TestSummarize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.jsonl")
	if err := os.WriteFile(path, []byte(sample), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Summarize(path)
	if err != nil {
		t.Fatal(err)
	}
	want := Summary{
		Activity:      "Bash: Run the tests",
		ActivityKind:  "tool",
		LastReply:     "Looking at the test now.",
		LastPrompt:    "Fix the flaky test\nthen open a PR",
		PromptAt:      "2026-11-10T10:00:00Z",
		ContextTokens: 50105,
		Model:         "opus",
	}
	if s != want {
		t.Fatalf("got %+v\nwant %+v", s, want)
	}
}

func TestSummarizeReadsOnlyTheTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.jsonl")
	filler := `{"type":"user","uuid":"OLD","message":{"content":"` + strings.Repeat("x", 1000) + `"}}` + "\n"
	big := strings.Repeat(filler, (tailBytes/len(filler))+50) + lines(`{"type":"assistant","uuid":"z","message":{"id":"mz","content":[{"type":"text","text":"latest"}]}}`)
	if err := os.WriteFile(path, []byte(big), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Summarize(path)
	if err != nil || s.LastReply != "latest" || s.Activity != "latest" {
		t.Fatalf("%+v %v", s, err)
	}
}

type sessions []hub.SessionView

func (s sessions) State() hub.State { return hub.State{Sessions: s} }

func TestSummariesAndCache(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "projects", "p", conv+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(sample), 0o600); err != nil {
		t.Fatal(err)
	}
	o := &Overview{
		Sessions: sessions{
			{ID: "s1", Tool: "claude", AgentSessionID: conv},
			{ID: "s2", Tool: "claude", AgentSessionID: conv, Archived: true},
			{ID: "s3", Tool: "codex", AgentSessionID: conv},
			{ID: "s4", Tool: "claude", AgentSessionID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"},
		},
		Locator: &locate.Locator{DefaultRoot: root},
	}
	got := o.Summaries(context.Background())
	if len(got) != 1 || got["s1"].Activity != "Bash: Run the tests" || got["s1"].Updated == "" {
		t.Fatalf("summaries %+v", got)
	}
	if err := os.WriteFile(path, []byte(sample+lines(`{"type":"assistant","uuid":"a3","message":{"id":"m3","content":[{"type":"text","text":"Done."}]}}`)), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := o.Summaries(context.Background()); got["s1"].Activity != "Done." {
		t.Fatalf("stale cache: %+v", got["s1"])
	}
}
