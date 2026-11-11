package export

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gsperry/agent-desk/internal/transcript"
)

func TestMarkdown(t *testing.T) {
	items := []transcript.Item{
		{Kind: transcript.KindUser, Text: "Fix the login bug", Time: "2026-11-01T10:00:00Z"},
		{Kind: transcript.KindThinking, Text: "secret reasoning"},
		{Kind: transcript.KindText, Text: "Looking.", Time: "2026-11-01T10:00:05Z"},
		{Kind: transcript.KindToolUse, Tool: &transcript.ToolUse{Name: "Bash", Input: json.RawMessage(`{"command":"go test ./...","description":"run tests"}`)}},
		{Kind: transcript.KindToolResult, Result: &transcript.ToolResult{Content: "ok"}},
		{Kind: transcript.KindToolUse, Tool: &transcript.ToolUse{Name: "Read", Input: json.RawMessage(`{"file_path":"/src/a.go"}`)}},
		{Kind: transcript.KindToolResult, Result: &transcript.ToolResult{Content: "no such file\n```\nx", IsError: true}},
		{Kind: transcript.KindText, Text: "Fixed it."},
		{Kind: transcript.KindMeta, Label: "ai-title"},
		{Kind: transcript.KindRaw, Raw: `{"type":"mystery"}`},
		{Kind: transcript.KindText, Text: "side", Sidechain: true},
	}
	got := Markdown(Header{Title: "Login\nfix", ID: "c1", Directory: "/src", Branch: "main"}, items)
	want := "# Login fix\n\n" +
		"- Directory: `/src`\n- Branch: `main`\n- Conversation: `c1`\n\n" +
		"## You · 2026-11-01 10:00 UTC\n\nFix the login bug\n\n" +
		"## Claude · 2026-11-01 10:00 UTC\n\nLooking.\n\n" +
		"- **Bash**: `go test ./...`\n" +
		"- **Read**: `/src/a.go`\n" +
		"  - failed:\n\n    ````\n    no such file\n    ```\n    x\n    ````\n" +
		"\nFixed it.\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	for _, leak := range []string{"secret reasoning", "mystery", "side"} {
		if strings.Contains(got, leak) {
			t.Errorf("export contains %q", leak)
		}
	}
}

func TestCode(t *testing.T) {
	tests := []struct{ in, want string }{
		{"ls", "`ls`"},
		{"echo `date`", "`` echo `date` ``"},
		{"a `b` c", "``a `b` c``"},
		{"`x`", "`` `x` ``"},
	}
	for _, tt := range tests {
		if got := code(tt.in); got != tt.want {
			t.Errorf("code(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFilename(t *testing.T) {
	tests := []struct{ title, id, want string }{
		{"Fix the login bug!", "c1", "fix-the-login-bug.md"},
		{"../../etc/passwd", "c1", "etc-passwd.md"},
		{"", "abcdef123456", "conversation-abcdef12.md"},
		{strings.Repeat("a", 80), "c1", strings.Repeat("a", 60) + ".md"},
	}
	for _, tt := range tests {
		if got := Filename(tt.title, tt.id); got != tt.want {
			t.Errorf("Filename(%q) = %q, want %q", tt.title, got, tt.want)
		}
	}
}
