package search

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsperry/agent-desk/internal/grouping"
	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/locate"
)

type sessions []hub.SessionView

func (s sessions) State() hub.State { return hub.State{Sessions: s} }

const (
	convA = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	convB = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	convC = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
)

func write(t *testing.T, root, conv string, lines ...string) {
	t.Helper()
	dir := filepath.Join(root, "projects", "p")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, conv+".jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func setup(t *testing.T) *Searcher {
	root := t.TempDir()
	write(t, root, convA,
		`{"type":"user","uuid":"a1","timestamp":"2026-11-10T10:00:00Z","origin":{"kind":"human"},"message":{"content":"Please fix the Flaky Test in poller_test.go"}}`,
		`{"type":"assistant","uuid":"a2","timestamp":"2026-11-10T10:01:00Z","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"go test -run \"TestHook\" ./internal/hub"}}]}}`,
		`{"type":"user","uuid":"a3","timestamp":"2026-11-10T10:02:00Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"--- FAIL: TestHookWriteTriggersPoll"}]}}`,
		`{"type":"api-request-blob","message":{"content":"flaky test copy that must not match"}}`,
	)
	write(t, root, convB,
		`{"type":"assistant","uuid":"b1","timestamp":"2026-11-10T11:00:00Z","message":{"id":"m2","content":[{"type":"text","text":"The flaky test is fixed. Ünïcode stays: café ✓"}]}}`,
	)
	write(t, root, convC,
		`{"type":"user","uuid":"c1","timestamp":"2026-11-09T09:00:00Z","origin":{"kind":"human"},"message":{"content":"archived flaky test talk"}}`,
	)
	return &Searcher{
		Sessions: sessions{
			{ID: "s1", Name: "one", Tool: "claude", AgentSessionID: convA, Project: hubProject("api")},
			{ID: "s2", Name: "two", Tool: "claude", AgentSessionID: convB, Project: hubProject("web")},
			{ID: "s3", Name: "old", Tool: "claude", AgentSessionID: convC, Archived: true, Project: hubProject("api")},
			{ID: "s4", Name: "codex", Tool: "codex", AgentSessionID: convA},
		},
		Locator: &locate.Locator{DefaultRoot: root},
	}
}

func TestSearch(t *testing.T) {
	s := setup(t)
	tests := []struct {
		name     string
		q        string
		archived bool
		want     []string
	}{
		{"case-insensitive across sessions, newest first", "flaky test", false, []string{"s2/b1/text", "s1/a1/user"}},
		{"archived included on request", "flaky test", true, []string{"s2/b1/text", "s1/a1/user", "s3/c1/user"}},
		{"tool input as written, with quotes", `"TestHook"`, false, []string{"s1/a2/tool_use"}},
		{"tool result", "FAIL: TestHook", false, []string{"s1/a3/tool_result"}},
		{"unicode", "café ✓", false, []string{"s2/b1/text"}},
		{"no match", "zebra stripes", false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := s.Search(context.Background(), tt.q, Options{IncludeArchived: tt.archived})
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, h := range res.Hits {
				got = append(got, h.SessionID+"/"+h.UUID+"/"+string(h.Kind))
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("hits = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSearchRejectsShortQueries(t *testing.T) {
	s := setup(t)
	for _, q := range []string{"", " a ", strings.Repeat("x", 201)} {
		if _, err := s.Search(context.Background(), q, Options{}); err != ErrQuery {
			t.Errorf("Search(%q) err = %v", q, err)
		}
	}
}

func TestSearchLimit(t *testing.T) {
	s := setup(t)
	res, err := s.Search(context.Background(), "flaky test", Options{IncludeArchived: true, Limit: 1})
	if err != nil || len(res.Hits) != 1 || !res.Truncated || res.Searched != 3 {
		t.Fatalf("res = %+v, err %v", res, err)
	}
}

func TestSnippet(t *testing.T) {
	text := strings.Repeat("a", 200) + " NEEDLE\nnext " + strings.Repeat("b", 200)
	at := strings.Index(text, "NEEDLE")
	s := snippet(text, at, len("NEEDLE"))
	if !strings.HasPrefix(s, "…") || !strings.HasSuffix(s, "…") || !strings.Contains(s, "NEEDLE next") {
		t.Fatalf("snippet %q", s)
	}
	if got := snippet("short match", 6, 5); got != "short match" {
		t.Fatalf("snippet %q", got)
	}
	multi := "日本語のテキストで検索する"
	if s := snippet(multi, strings.Index(multi, "検索"), len("検索")); !strings.Contains(s, "検索") {
		t.Fatalf("snippet %q", s)
	}
}

func hubProject(name string) grouping.Project {
	return grouping.Project{Key: name, Name: name, Kind: grouping.KindRepo}
}
