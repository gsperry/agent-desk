package conflicts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/locate"
)

type sessions []hub.SessionView

func (s sessions) State() hub.State { return hub.State{Sessions: s} }

func edit(uuid, tool, path, at string) string {
	return fmt.Sprintf(`{"type":"assistant","uuid":"%s","timestamp":"%s","message":{"id":"m%s","content":[{"type":"tool_use","id":"t%s","name":"%s","input":{"file_path":"%s"}}]}}`, uuid, at, uuid, uuid, tool, path)
}

func TestFind(t *testing.T) {
	root := t.TempDir()
	conv := func(n int) string { return fmt.Sprintf("%08d-0000-4000-8000-000000000000", n) }
	write := func(n int, lines ...string) {
		p := filepath.Join(root, "projects", "p", conv(n)+".jsonl")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	recent := "2026-11-10T11:00:00Z"
	old := "2026-11-09T01:00:00Z"
	write(1, edit("a1", "Edit", "/src/api/main.go", recent), edit("a2", "Write", "/src/api/README.md", recent), edit("a3", "Bash", "/src/api/x", recent))
	write(2, edit("b1", "Edit", "/src/api/main.go", recent), edit("b2", "Edit", "/src/api/README.md", old))
	write(3, edit("c1", "MultiEdit", "/src/api/main.go", recent))
	write(4, edit("d1", "Edit", "/src/api/README.md", recent))
	write(5, edit("e1", "Edit", "relative/path.go", recent))
	f := &Finder{
		Sessions: sessions{
			{ID: "s1", Tool: "claude", Running: true, AgentSessionID: conv(1)},
			{ID: "s2", Tool: "claude", Running: true, AgentSessionID: conv(2)},
			{ID: "s3", Tool: "claude", Running: true, AgentSessionID: conv(3)},
			{ID: "s4", Tool: "claude", Running: false, AgentSessionID: conv(4)},
			{ID: "s5", Tool: "claude", Running: true, AgentSessionID: conv(5)},
		},
		Locator: &locate.Locator{DefaultRoot: root},
		Now:     func() time.Time { return now },
	}
	got := f.Find(context.Background())
	want := map[string]string{
		"s1": "/src/api/main.go<s2,s3>",
		"s2": "/src/api/main.go<s1,s3>",
		"s3": "/src/api/main.go<s1,s2>",
	}
	if len(got) != len(want) {
		t.Fatalf("conflicts = %+v", got)
	}
	for id, w := range want {
		var parts []string
		for _, c := range got[id] {
			parts = append(parts, c.Path+"<"+strings.Join(c.Others, ",")+">")
		}
		if strings.Join(parts, ";") != w {
			t.Errorf("%s = %v, want %s", id, parts, w)
		}
	}
}
