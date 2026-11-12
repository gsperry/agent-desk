package send

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsperry/agent-desk/internal/hub"
)

func TestClean(t *testing.T) {
	tests := []struct {
		name, in, want string
		reason         Reason
	}{
		{"plain", "hello", "hello", ""},
		{"trims", "  hi\n\n", "hi", ""},
		{"CRLF and CR", "a\r\nb\rc", "a\nb\nc", ""},
		{"keeps tabs and inner blank lines", "a\n\n\tb", "a\n\n\tb", ""},
		{"strips bracketed paste end", "safe\x1b[201~\x1b[2Jrm -rf", "safe[201~[2Jrm -rf", ""},
		{"strips other controls", "a\x00b\x07c\x7fd", "abcd", ""},
		{"keeps unicode", "héllo → 世界", "héllo → 世界", ""},
		{"empty", " \n\t ", "", ReasonEmpty},
		{"only controls", "\x1b\x00", "", ReasonEmpty},
		{"too long", strings.Repeat("x", MaxBytes+1), "", ReasonTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, refused := Clean(tt.in)
			if tt.reason != "" {
				if refused == nil || refused.Reason != tt.reason {
					t.Fatalf("refused = %v, want %s", refused, tt.reason)
				}
				return
			}
			if refused != nil || got != tt.want {
				t.Fatalf("got %q, %v; want %q", got, refused, tt.want)
			}
		})
	}
}

func TestCheckStatus(t *testing.T) {
	running := func(status string) *hub.SessionView {
		return &hub.SessionView{ID: "s", Status: status, Running: true}
	}
	tests := []struct {
		name   string
		s      *hub.SessionView
		inside bool
		want   Reason
	}{
		{"idle", running("idle"), false, ""},
		{"working queues in Claude", running("working"), false, ""},
		{"finished", running("finished"), false, ""},
		{"errored", running("errored"), false, ""},
		{"waiting is decided from the screen", running("waiting"), false, ""},
		{"starting", running("starting"), false, ReasonStarting},
		{"dead", &hub.SessionView{Status: "dead"}, false, ReasonNotRunning},
		{"not running", &hub.SessionView{Status: "idle", Running: false}, false, ReasonNotRunning},
		{"unknown", nil, false, ReasonUnknownSession},
		{"inside a session", running("idle"), true, ReasonInsideSession},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckStatus(tt.s, tt.inside)
			if tt.want == "" {
				if got != nil {
					t.Fatalf("refused: %v", got)
				}
				return
			}
			if got == nil || got.Reason != tt.want {
				t.Fatalf("got %v, want %s", got, tt.want)
			}
		})
	}
}

func TestCheckPane(t *testing.T) {
	tests := []struct {
		file string
		want Reason
	}{
		{"idle.txt", ""},
		{"draft.txt", ReasonDraft},
		{"trust-dialog.txt", ReasonDialog},
		{"ask-question.txt", ReasonDialog},
		{"auto-mode-dialog.txt", ReasonDialog},
		{"agent-quotes-dialog.txt", ""},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			screen, err := os.ReadFile(filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatal(err)
			}
			got := CheckPane(string(screen))
			if tt.want == "" {
				if got != nil {
					t.Fatalf("refused: %v", got)
				}
				return
			}
			if got == nil || got.Reason != tt.want {
				t.Fatalf("got %v, want %s", got, tt.want)
			}
		})
	}
	if CheckPane("") != nil {
		t.Fatal("empty screen refused")
	}
}

func TestIsDraft(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"❯ ", false},
		{"❯\u00a0 ", false},
		{"  ❯\u00a0\u00a0", false},
		{"❯ fix the bug", true},
		{"❯\u00a0fix", true},
		{"  ❯ 1. Yes", true},
		{"no marker", false},
	}
	for _, tt := range tests {
		if got := isDraft(tt.line); got != tt.want {
			t.Errorf("isDraft(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestAtPrompt(t *testing.T) {
	rule := strings.Repeat("─", 40)
	tests := []struct {
		name   string
		screen string
		want   bool
	}{
		{"empty prompt", rule + "\n❯ \n" + rule + "\n", true},
		{"prompt padded with a no-break space", rule + "\n❯ \n" + rule + "\n  ⏵⏵ auto mode on\n", true},
		{"draft in the prompt", rule + "\n❯ hello\n" + rule + "\n", false},
		{"a dialog option", rule + "\n❯ 1. Yes\n  2. No\n" + rule + "\n", false},
		{"no rules", "❯ \n", false},
	}
	for _, tt := range tests {
		if got := AtPrompt(tt.screen); got != tt.want {
			t.Errorf("%s: %v", tt.name, got)
		}
	}
	if CheckWaiting("waiting", rule+"\n❯ 1. Yes\n"+rule+"\n") == nil {
		t.Error("waiting with a dialog was allowed")
	}
	if CheckWaiting("idle", "anything") != nil {
		t.Error("an idle session was refused")
	}
}

// The idle screen captured from Claude Code counts as at the prompt.
func TestAtPromptRealScreen(t *testing.T) {
	data, err := os.ReadFile("../pane/testdata/idle.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !AtPrompt(string(data)) {
		t.Fatal("captured idle screen not seen as at the prompt")
	}
	perm, err := os.ReadFile("../pane/testdata/permission.txt")
	if err != nil {
		t.Fatal(err)
	}
	if AtPrompt(string(perm)) {
		t.Fatal("permission dialog seen as at the prompt")
	}
}
