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
		// text is what a free option's field holds, by option.
		text []string
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
			// Back on a question answered with typed text: the field shows
			// the answer in place of "Type something.".
			file:     "askq-typed-back.txt",
			labels:   []string{"Small", "Large", "Medium, about 40 cm", "Chat about this"},
			selected: 0,
			question: "Pick a size",
			free:     []bool{false, false, true, false},
			text:     []string{"", "", "Medium, about 40 cm", ""},
		},
		{
			// The cursor on the field draws one more space; labels match.
			file:     "askq-typed-selected.txt",
			labels:   []string{"Small", "Large", "Medium, about 40 cm", "Chat about this"},
			selected: 2,
			question: "Pick a size",
			free:     []bool{false, false, true, false},
			text:     []string{"", "", "Medium, about 40 cm", ""},
		},
		{
			file:     "askq-typed-cleared.txt",
			labels:   []string{"Small", "Large", "Type something.", "Chat about this"},
			selected: 2,
			question: "Pick a size",
			free:     []bool{false, false, true, false},
			text:     []string{"", "", "", ""},
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
			// A newer Claude Code leaves out "Ready to submit your answers?"
			// and draws each answer under a bar.
			file:     "askq-review-no-prompt.txt",
			labels:   []string{"Submit answers", "Cancel"},
			selected: 0,
			question: "Ready to submit your answers?",
		},
		{
			// The plan-mode exit dialog draws no footer.
			file:     "plan-mode-exit.txt",
			labels:   []string{"Yes, and use auto mode", "No"},
			selected: 0,
			question: "Exit plan mode?\n\nClaude wants to exit plan mode",
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
				if tt.text != nil && o.Text != tt.text[i] {
					t.Fatalf("option %d text = %q, want %q", i, o.Text, tt.text[i])
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

// A numbered list with no cursor, or one that does not end the screen, is
// not a menu being asked: the footerless fallback must not turn the
// assistant's own lists into dialogs.
func TestParseFooterlessIsNarrow(t *testing.T) {
	tests := []struct {
		name   string
		screen string
	}{
		{"list without a cursor", "Here are the steps:\n  1. Install it\n  2. Run it\n"},
		{"list above the prompt box", "  1. Install it\n  2. Run it\n──────────────────────────\n❯ \n──────────────────────────\n  ? for shortcuts\n"},
		{"cursor on a lone option", "Pick:\n ❯ 1. Only one\n"},
	}
	for _, tt := range tests {
		if d := Parse(tt.screen); d != nil {
			t.Errorf("%s: unexpected dialog %+v", tt.name, d)
		}
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

// A several-answer question: checkbox options, an unnumbered Next between
// the last option and "Chat about this", and a question behind a bar that
// wraps. Constructed in the layout of a Claude Code 2.1.322 screen, with
// neutral text.
func TestMultiSelect(t *testing.T) {
	d := Parse(read(t, "askq-multi.txt"))
	if d == nil || !d.Multi {
		t.Fatalf("not a several-answer dialog: %+v", d)
	}
	wantQ := "Release window for the docs site: which checks do you want before it ships? Everything runs again after the deploy."
	if d.Question != wantQ {
		t.Fatalf("question = %q", d.Question)
	}
	type row struct {
		label      string
		number     int
		free, next bool
		checkbox   bool
	}
	want := []row{
		{"Link check", 1, false, false, true},
		{"Spelling", 2, false, false, true},
		{"Screenshots", 3, false, false, true},
		{"Load test", 4, false, false, true},
		{"Type something", 5, true, false, true},
		{"Next", 0, false, true, false},
		{"Chat about this", 6, false, false, false},
	}
	if len(d.Options) != len(want) {
		t.Fatalf("options = %+v", d.Options)
	}
	for i, w := range want {
		o := d.Options[i]
		if o.Index != i || o.Label != w.label || o.Number != w.number || o.Free != w.free || o.Next != w.next || (o.Checked != nil) != w.checkbox {
			t.Errorf("option %d = %+v, want %+v", i, o, w)
		}
		if o.Checked != nil && *o.Checked {
			t.Errorf("option %d ticked", i)
		}
	}
	if d.Options[0].Description != "Follow every link in the built site and report the broken ones." || d.Options[4].Description != "" || d.Options[6].Description != "" {
		t.Fatalf("descriptions: %q / %q / %q", d.Options[0].Description, d.Options[4].Description, d.Options[6].Description)
	}
	// "Chat about this" is past Next: six presses down, not five.
	if got := strings.Join(d.Moves(6), " "); got != "Down Down Down Down Down Down Enter" {
		t.Fatalf("moves to Chat about this = %s", got)
	}
	if got := strings.Join(d.Moves(5), " "); got != "Down Down Down Down Down Enter" {
		t.Fatalf("moves to Next = %s", got)
	}
}

// Screens from a probe session on Claude Code 2.1.322: Enter ticks a box
// ([✔]) and leaves the cursor; the cursor on Next is drawn at the margin;
// answered questions are ☒ in the tab bar.
func TestMultiSelectProbe(t *testing.T) {
	initial := Parse(read(t, "askq-multi-initial.txt"))
	if initial == nil || !initial.Multi || len(initial.Options) != 6 || !initial.Options[4].Next || initial.Options[5].Label != "Chat about this" {
		t.Fatalf("initial: %+v", initial)
	}
	if initial.Question != "Which checks should run before the docs release? Pick every one that applies." {
		t.Fatalf("question = %q", initial.Question)
	}
	wantSteps := []Step{{Label: "Checks"}, {Label: "Timing"}, {Label: "Submit", Submit: true}}
	if !reflect.DeepEqual(initial.Steps, wantSteps) {
		t.Fatalf("steps = %+v", initial.Steps)
	}

	ticked := Parse(read(t, "askq-multi-ticked.txt"))
	if ticked == nil || ticked.Options[0].Checked == nil || !*ticked.Options[0].Checked || *ticked.Options[1].Checked || !ticked.Options[0].Selected {
		t.Fatalf("ticked: %+v", ticked)
	}
	// Ticking a box marks the question answered while it is still shown.
	if !ticked.Steps[0].Done || ticked.Steps[1].Done {
		t.Fatalf("ticked steps = %+v", ticked.Steps)
	}

	onNext := Parse(read(t, "askq-multi-on-next.txt"))
	if onNext == nil || !onNext.Options[4].Next || !onNext.Options[4].Selected {
		t.Fatalf("on next: %+v", onNext)
	}
	if got := strings.Join(onNext.Moves(0), " "); got != "Up Up Up Up Enter" {
		t.Fatalf("moves back to the first option = %s", got)
	}
	if got := strings.Join(onNext.Moves(5), " "); got != "Down Enter" {
		t.Fatalf("moves to Chat about this = %s", got)
	}

	step2 := Parse(read(t, "askq-multi-step2.txt"))
	if step2 == nil || step2.Multi || step2.Question != "When should they run?" || !step2.Steps[0].Done {
		t.Fatalf("step 2: %+v", step2)
	}
}

func TestSingleQuestionHasNoSteps(t *testing.T) {
	if d := Parse(read(t, "ask-question.txt")); d == nil || d.Steps != nil {
		t.Fatalf("steps on a single question: %+v", d)
	}
	if d := Parse(read(t, "askq-review.txt")); d == nil || len(d.Steps) != 3 || !d.Steps[2].Submit {
		t.Fatalf("review steps: %+v", d)
	}
}

// Claude Code draws the session's title into a rule, and on the review step
// such a rule can close the screen below the options.
func TestReviewWithTitledRule(t *testing.T) {
	rule := strings.Repeat("─", 80) + " Docs Release ─"
	screen := strings.TrimRight(read(t, "askq-review.txt"), "\n") + "\n" + rule + "\n"
	d := Parse(screen)
	if d == nil || len(d.Options) != 2 || d.Options[0].Label != "Submit answers" || d.Question != "Ready to submit your answers?" {
		t.Fatalf("review: %+v", d)
	}
	if len(d.Steps) != 3 {
		t.Fatalf("steps: %+v", d.Steps)
	}
	if !isRule(rule) || isRule("──── just a short line of text that is not a rule") {
		t.Fatal("titled rule detection")
	}
}

// A described row above "Chat about this" is an option the question
// offers, even when its label is not "Type something.".
func TestTypedFieldNeedsNoDescription(t *testing.T) {
	d := &Dialog{Options: []Option{
		{Index: 0, Label: "Small", Description: "Small size"},
		{Index: 1, Label: "Large", Description: "Large size"},
		{Index: 2, Label: "Chat about this"},
	}}
	typedField(d)
	if d.Options[1].Free || d.Options[1].Text != "" {
		t.Fatalf("option = %+v", d.Options[1])
	}
	d.Options[1].Description = ""
	typedField(d)
	if !d.Options[1].Free || d.Options[1].Text != "Large" {
		t.Fatalf("option = %+v", d.Options[1])
	}
}

// In a question that takes several answers, the text field is the
// checkbox row above Next; once it holds text it shows the text.
func TestParseMultiTypedField(t *testing.T) {
	tests := []struct {
		file     string
		label    string
		text     string
		checked  bool
		selected bool
	}{
		{"askq-multi-typed-back.txt", "Security scan", "Security scan", false, false},
		{"askq-multi-typed-selected.txt", "Security scan", "Security scan", false, true},
		{"askq-multi-typed-cleared.txt", "Type something", "", false, true},
		{"askq-multi-typed-ticking.txt", "Perf", "Perf", true, true},
		{"askq-multi-typed-ticked.txt", "Security scan", "Security scan", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			d := Parse(read(t, tt.file))
			if d == nil || !d.Multi || len(d.Options) != 6 {
				t.Fatalf("dialog = %+v", d)
			}
			o := d.Options[3]
			if !o.Free || o.Label != tt.label || o.Text != tt.text || o.Checked == nil || *o.Checked != tt.checked || o.Selected != tt.selected {
				t.Fatalf("field = %+v", o)
			}
			if d.Options[1].Free || !d.Options[4].Next {
				t.Fatalf("options = %+v", d.Options)
			}
		})
	}
}

// Back on an answered question, the answer carries Claude Code's ✔, which
// Agent Desk reports as Answered and leaves out of the label.
func TestParseAnswered(t *testing.T) {
	d := Parse(read(t, "askq-typed-back.txt"))
	for i, o := range d.Options {
		if o.Answered != (i == 2) {
			t.Fatalf("option %d = %+v", i, o)
		}
	}
	d = Parse(read(t, "askq-multi-typed-ticked.txt"))
	for _, o := range d.Options {
		if o.Answered {
			t.Fatalf("a ticked checkbox is not Answered: %+v", o)
		}
	}
}
