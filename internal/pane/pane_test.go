package pane

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func labels(d *Dialog) []string {
	var out []string
	for _, o := range d.Options {
		out = append(out, o.Label)
	}
	return out
}

func TestParse(t *testing.T) {
	tests := []struct {
		file     string
		labels   []string
		selected int
		question string
		free     []bool
	}{
		{
			file:     "permission.txt",
			labels:   []string{"Yes", "Yes, and always allow access to /tmp/agentdesk-scratch/probe from this project", "Yes, and switch to auto mode · auto mode handles these prompts for you", "No"},
			selected: 0,
			question: "Do you want to proceed?",
		},
		{
			file:     "trust-dialog.txt",
			labels:   []string{"No, exit", "Yes, I trust this folder"},
			selected: 0,
			question: "Accessing workspace:\n/private/tmp/probe\nQuick safety check: Is this a project you created or one you trust?\nClaude Code'll be able to read, edit, and execute files here.\nSecurity guide",
		},
		{
			file:     "ask-question.txt",
			labels:   []string{"Red", "Blue", "Type something.", "Chat about this"},
			selected: 0,
			question: "Which color do you pick?",
			free:     []bool{false, false, true, false},
		},
		{
			file:     "askq-free-initial.txt",
			labels:   []string{"Red", "Blue", "Type something.", "Chat about this"},
			selected: 0,
			question: "Pick a color",
			free:     []bool{false, false, true, false},
		},
		{
			file:     "askq-free-selected.txt",
			labels:   []string{"Red", "Blue", "Type something.", "Chat about this"},
			selected: 2,
			question: "Pick a color",
			free:     []bool{false, false, true, false},
		},
		{
			file:     "askq-tabs.txt",
			labels:   []string{"Small", "Large", "Type something.", "Chat about this"},
			selected: 0,
			question: "Pick a size",
		},
		{
			file:     "askq-review.txt",
			labels:   []string{"Submit answers", "Cancel"},
			selected: 0,
			question: "Ready to submit your answers?",
		},
		{
			file:     "auto-mode-dialog.txt",
			labels:   []string{"Yes", "Not now", "Don't show again"},
			selected: 0,
			question: "Teach auto mode about your environment?\nAuto mode works better when it knows your environment. Takes about a minute.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			d := Parse(read(t, tt.file))
			if d == nil {
				t.Fatal("no dialog")
			}
			if !reflect.DeepEqual(labels(d), tt.labels) {
				t.Fatalf("labels = %q, want %q", labels(d), tt.labels)
			}
			for i, o := range d.Options {
				if o.Index != i || o.Selected != (i == tt.selected) {
					t.Fatalf("option %d = %+v", i, o)
				}
				if tt.free != nil && o.Free != tt.free[i] {
					t.Fatalf("option %d free = %v", i, o.Free)
				}
			}
			if d.Question != tt.question {
				t.Fatalf("question = %q, want %q", d.Question, tt.question)
			}
		})
	}
}

func TestParseNoDialog(t *testing.T) {
	for _, f := range []string{"idle.txt", "agent-quotes-dialog.txt"} {
		if d := Parse(read(t, f)); d != nil {
			t.Errorf("%s: unexpected dialog %+v", f, d)
		}
	}
	if Parse("") != nil {
		t.Error("empty screen")
	}
}

func TestParseSelectionAway(t *testing.T) {
	screen := " Pick one\n   1. Alpha\n ❯ 2. Beta\n   3. Gamma\n Enter to select · Esc to cancel\n"
	d := Parse(screen)
	if d == nil || d.Options[1].Selected != true || d.Options[0].Selected {
		t.Fatalf("%+v", d)
	}
	tests := []struct {
		index int
		want  []string
	}{
		{0, []string{"Up", "Enter"}},
		{1, []string{"Enter"}},
		{2, []string{"Down", "Enter"}},
	}
	for _, tt := range tests {
		if got := d.Moves(tt.index); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Moves(%d) = %v, want %v", tt.index, got, tt.want)
		}
	}
}

func TestParseIgnoresEarlierNumberedLists(t *testing.T) {
	screen := "⏺ Plan:\n  1. first step\n  2. second step\n Do it?\n ❯ 1. Yes\n   2. No\n Esc to cancel\n"
	d := Parse(screen)
	if d == nil || !reflect.DeepEqual(labels(d), []string{"Yes", "No"}) || d.Question != "Do it?" {
		t.Fatalf("%+v", d)
	}
}

// A question longer than the terminal is wrapped over several lines; the
// whole of it is the question, joined back into one paragraph, and blank
// lines between paragraphs are kept. Constructed in the layout of
// ask-question.txt, 60 columns wide.
func TestLongQuestion(t *testing.T) {
	sep := strings.Repeat("─", 60)
	screen := strings.Join([]string{
		"❯ Ask me which deployment target to use.",
		sep,
		" ☐ Target",
		"Which environment should the release go to first? Staging",
		"runs the full suite, production skips it.",
		"",
		"Pick one.",
		"❯ 1. Staging",
		"     Safer",
		"  2. Production",
		"     Faster",
		"  3. Type something.",
		sep,
		"  4. Chat about this",
		"Enter to select · ↑/↓ to navigate · Esc to cancel",
	}, "\n")
	d := Parse(screen)
	if d == nil {
		t.Fatal("no dialog")
	}
	want := "Which environment should the release go to first? Staging runs the full suite, production skips it.\n\nPick one."
	if d.Question != want {
		t.Fatalf("question = %q\nwant      %q", d.Question, want)
	}
}
