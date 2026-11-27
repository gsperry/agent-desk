package pane

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/send"
)

// The layout below is Claude Code 2.1.332's picker: two model rows at a
// time with "↑ N more" and "↓ N more", and the highlighted row's effort
// under them. The words are made up.
const pickerTop = ` ▐▛███▛█   Claude Code v2.1.332
▝▜██████▀  Sonnet 5.5 · Example provider
 ▝▝   ▝▝   /tmp/project
▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔
  Select model
  Your pick is the default for new sessions. For other models, use --model.
    1. Default       Use the default model (currently Opus 5.5)
  ❯ 2. Sonnet 5.5 ✔  Custom Sonnet model (claude-sonnet-5-5)
    ↓ 3 more
  ◐ Medium effort (default) ←/→ to adjust
  Use /fast to turn on fast mode (Fable 5.2).
  s to use this session only · Enter to save · Esc to cancel
`

func TestParsePicker(t *testing.T) {
	p := ParsePicker(pickerTop)
	if p == nil {
		t.Fatal("picker not read")
	}
	if p.Cursor != 1 || len(p.Rows) != 2 || p.Above != 0 || p.Below != 3 || p.Total() != 5 {
		t.Fatalf("picker = %+v", p)
	}
	if got := p.at(); got.Number != 2 || got.Label != "Sonnet 5.5" || !got.Current || got.Description != "Custom Sonnet model (claude-sonnet-5-5)" {
		t.Fatalf("highlighted row = %+v", got)
	}
	if p.Rows[0].Label != "Default" || p.Rows[0].Current {
		t.Fatalf("first row = %+v", p.Rows[0])
	}
	if p.Effort != "Medium" || !p.EffortDefault {
		t.Fatalf("effort = %q default=%v", p.Effort, p.EffortDefault)
	}
	// Scrolled to the middle: rows above and below, an effort that is not the default.
	mid := strings.NewReplacer(
		"    1. Default       Use the default model (currently Opus 5.5)", "    3. Fable 5.1     Custom Fable model (claude-fable-5-1)",
		"  ❯ 2. Sonnet 5.5 ✔  Custom Sonnet model (claude-sonnet-5-5)", "  ❯ 4. Opus 5.5      Custom Opus model (claude-opus-5-5)",
		"    ↓ 3 more", "    ↑ 2 more\n    ↓ 1 more",
		"◐ Medium effort (default)", "◉ xHigh effort",
	).Replace(pickerTop)
	m := ParsePicker(mid)
	if m == nil || m.at().Label != "Opus 5.5" || m.Above != 2 || m.Below != 1 || m.Effort != "xHigh" || m.EffortDefault {
		t.Fatalf("middle = %+v", m)
	}
}

func TestParsePickerNarrow(t *testing.T) {
	tests := map[string]string{
		"no picker":          "❯ hello\n  ⎿ Set model to Haiku 5.5 for this session only\n",
		"no session footer":  strings.Replace(pickerTop, "s to use this session only · ", "", 1),
		"no highlighted row": strings.Replace(pickerTop, "  ❯ 2.", "    2.", 1),
		"no effort line":     strings.Replace(pickerTop, "  ◐ Medium effort (default) ←/→ to adjust\n", "", 1),
	}
	for name, screen := range tests {
		if p := ParsePicker(screen); p != nil {
			t.Errorf("%s: read %+v", name, p)
		}
	}
}

// simRow is a model in the simulated picker.
type simRow struct {
	label, desc string
	levels      []string
	def         string
}

var effortGlyph = map[string]string{"Low": "○", "Medium": "◐", "High": "●", "xHigh": "◉", "Max": "◈"}

// pickerSim plays Claude Code's model picker: it renders what the real one
// draws, moves on Up, Down, Left and Right, applies on s, cancels on Escape,
// and notes an Enter, which would save the choice as the user's default.
type pickerSim struct {
	rows    []simRow
	current int
	cursor  int
	effort  int
	open    bool
	confirm bool
	// confirmSwitch makes a change of model ask first, as Claude Code does
	// while its prompt cache is warm.
	confirmSwitch bool
	pending       *[2]int
	history       []string
	keys          []string
	typed         []string
	enterPressed  bool
	// draft is text in the session's own prompt.
	draft string
}

func newSim() *pickerSim {
	all := []string{"Low", "Medium", "High", "xHigh", "Max"}
	return &pickerSim{
		rows: []simRow{
			{"Default", "Use the default model (currently Opus 5.5)", all, "Medium"},
			{"Sonnet 5.5", "Custom Sonnet model (claude-sonnet-5-5)", all, "Medium"},
			{"Fable 5.1", "Custom Fable model (claude-fable-5-1)", all, "High"},
			{"Opus 4.6", "Custom Opus model (claude-opus-4-6)", []string{"Low", "Medium", "High", "Max"}, "High"},
			{"Haiku 5.5", "Custom Haiku model (claude-haiku-5-5)", all, "Medium"},
		},
		current: 1,
	}
}

func (s *pickerSim) defaultEffort(row int) int {
	for i, l := range s.rows[row].levels {
		if l == s.rows[row].def {
			return i
		}
	}
	return 0
}

func (s *pickerSim) Type(_ context.Context, _, text string) error {
	s.typed = append(s.typed, text)
	if text == "/model" && !s.open && !s.confirm {
		s.history = append(s.history, "❯ /model")
		s.open, s.cursor = true, s.current
		s.effort = s.defaultEffort(s.cursor)
	}
	return nil
}

func (s *pickerSim) apply(row, effort int) {
	line := fmt.Sprintf("  ⎿  Set model to %s for this session only with %s effort", s.rows[row].label, strings.ToLower(s.rows[row].levels[effort]))
	s.history = append(s.history, "❯ /model", line)
	s.current = row
}

func (s *pickerSim) Send(_ context.Context, _ string, keys ...string) error {
	for _, k := range keys {
		s.keys = append(s.keys, k)
		switch {
		case s.confirm:
			switch k {
			case "Enter":
				s.apply(s.pending[0], s.pending[1])
				s.confirm = false
			case "Escape":
				s.confirm = false
			}
		case s.open:
			n := len(s.rows)
			switch k {
			case "Down":
				s.cursor = (s.cursor + 1) % n
				s.effort = s.defaultEffort(s.cursor)
			case "Up":
				s.cursor = (s.cursor + n - 1) % n
				s.effort = s.defaultEffort(s.cursor)
			case "Left":
				s.effort = max(s.effort-1, 0)
			case "Right":
				s.effort = min(s.effort+1, len(s.rows[s.cursor].levels)-1)
			case "s":
				s.open = false
				if s.confirmSwitch && s.cursor != s.current {
					s.confirm, s.pending = true, &[2]int{s.cursor, s.effort}
				} else {
					s.apply(s.cursor, s.effort)
				}
			case "Escape":
				s.open = false
				s.history = append(s.history, "  ⎿  Kept model as "+s.rows[s.current].label)
			case "Enter":
				s.enterPressed = true
			}
		}
	}
	return nil
}

func (s *pickerSim) Read(context.Context, string) (string, error) {
	var b strings.Builder
	b.WriteString("▝▜██████▀  Sonnet 5.5 · Example provider\n")
	for _, l := range s.history[max(len(s.history)-6, 0):] {
		b.WriteString(l + "\n")
	}
	switch {
	case s.confirm:
		b.WriteString("▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔\n")
		b.WriteString("  Switch model?\n  Your next response will be slower and use more tokens\n")
		b.WriteString("  This conversation is cached for the current model. Switching to " + s.rows[s.pending[0]].label + " means\n  the full history gets re-read.\n")
		b.WriteString("  ❯ 1. Yes, switch to " + s.rows[s.pending[0]].label + "\n    2. No, go back\n  Enter to confirm · Esc to cancel\n")
	case s.open:
		start := 0
		if s.cursor > 0 {
			start = s.cursor - 1
		}
		b.WriteString("▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔\n")
		b.WriteString("  Select model\n  Your pick is the default for new sessions. For other models, use --model.\n")
		if start > 0 {
			fmt.Fprintf(&b, "    ↑ %d more\n", start)
		}
		for i := start; i < start+2; i++ {
			cur, tick := "  ", ""
			if i == s.cursor {
				cur = "❯ "
			}
			if i == s.current {
				tick = " ✔"
			}
			fmt.Fprintf(&b, "  %s%d. %s%s  %s\n", cur, i+1, s.rows[i].label, tick, s.rows[i].desc)
		}
		if below := len(s.rows) - (start + 2); below > 0 {
			fmt.Fprintf(&b, "    ↓ %d more\n", below)
		}
		level := s.rows[s.cursor].levels[s.effort]
		def := ""
		if s.effort == s.defaultEffort(s.cursor) {
			def = " (default)"
		}
		fmt.Fprintf(&b, "  %s %s effort%s ←/→ to adjust\n  Use /fast to turn on fast mode (Fable 5.2).\n", effortGlyph[level], level, def)
		b.WriteString("  s to use this session only · Enter to save · Esc to cancel\n")
	default:
		b.WriteString("──────────────────────────────────────────────────────────────────────────────────\n")
		b.WriteString("❯ " + s.draft + "\n")
		b.WriteString("──────────────────────────────────────────────────────────────────────────────────\n")
		b.WriteString("  ⏵⏵ auto mode on (shift+tab to cycle)\n")
	}
	return b.String(), nil
}

func (s *pickerSim) ReadStyled(ctx context.Context, id string) (string, error) {
	return s.Read(ctx, id)
}

func simAnswerer(s *pickerSim, tool, status string) *Answerer {
	return &Answerer{
		Sessions: &fakeSessions{st: hub.State{Sessions: []hub.SessionView{{ID: "s1", Tool: tool, Status: status, Running: true}}}},
		Reader:   s,
		Keys:     s,
		Typer:    s,
		Pause:    func(time.Duration) {},
	}
}

func TestModelOptions(t *testing.T) {
	s := newSim()
	opts, err := simAnswerer(s, "claude", "idle").ModelOptions(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	var labels []string
	for _, m := range opts.Models {
		labels = append(labels, m.Label)
	}
	if strings.Join(labels, ",") != "Default,Sonnet 5.5,Fable 5.1,Opus 4.6,Haiku 5.5" {
		t.Fatalf("models = %v", labels)
	}
	if !opts.Models[1].Current || opts.Models[0].Current {
		t.Fatalf("current = %+v", opts.Models)
	}
	if got := strings.Join(opts.Models[1].Efforts, ","); got != "Low,Medium,High,xHigh,Max" || opts.Models[1].DefaultEffort != "Medium" {
		t.Fatalf("sonnet efforts %s default %s", got, opts.Models[1].DefaultEffort)
	}
	if got := strings.Join(opts.Models[3].Efforts, ","); got != "Low,Medium,High,Max" || opts.Models[3].DefaultEffort != "High" {
		t.Fatalf("a model with four levels: %s default %s", got, opts.Models[3].DefaultEffort)
	}
	// It only looked: the picker is closed, nothing was saved or applied.
	if s.open || s.enterPressed || s.current != 1 {
		t.Fatalf("open=%v enter=%v current=%d", s.open, s.enterPressed, s.current)
	}
	for _, k := range s.keys {
		if k == "s" || k == "Enter" {
			t.Fatalf("pressed %s while only reading the options: %v", k, s.keys)
		}
	}
	if len(s.typed) != 1 || s.typed[0] != "/model" {
		t.Fatalf("typed %v", s.typed)
	}
}

func TestSetModel(t *testing.T) {
	tests := []struct {
		name, model, effort string
		confirm             bool
		wantApplied         string
		wantPending         bool
	}{
		{"model and effort", "Fable 5.1", "xHigh", false, "Set model to Fable 5.1 for this session only with xhigh effort", false},
		{"lower effort", "Haiku 5.5", "Low", false, "Set model to Haiku 5.5 for this session only with low effort", false},
		{"the model it already runs", "Sonnet 5.5", "High", false, "Set model to Sonnet 5.5 for this session only with high effort", false},
		{"model with fewer levels", "Opus 4.6", "Max", false, "Set model to Opus 4.6 for this session only with max effort", false},
		{"asks before switching", "Haiku 5.5", "Medium", true, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSim()
			s.confirmSwitch = tt.confirm
			res, err := simAnswerer(s, "claude", "idle").SetModel(context.Background(), "s1", tt.model, tt.effort)
			if err != nil {
				t.Fatal(err)
			}
			if res.Applied != tt.wantApplied || res.Pending != tt.wantPending {
				t.Fatalf("result = %+v", res)
			}
			if s.enterPressed {
				t.Fatal("Enter was pressed in the picker, which saves the choice as the default")
			}
			if tt.wantPending {
				// The confirmation is left for the user: nothing pressed it.
				if !s.confirm || s.keys[len(s.keys)-1] != "s" {
					t.Fatalf("confirm=%v keys=%v", s.confirm, s.keys)
				}
				return
			}
			if s.keys[len(s.keys)-1] != "s" || s.open {
				t.Fatalf("keys = %v open=%v", s.keys, s.open)
			}
		})
	}
}

func TestSetModelRefusals(t *testing.T) {
	tests := []struct {
		name         string
		tool, status string
		model        string
		effort       string
		draft        string
		reason       send.Reason
		typed        bool
		closedAfter  bool
	}{
		{"working", "claude", "working", "Haiku 5.5", "", "", reasonNotIdle, false, false},
		{"waiting", "claude", "waiting", "Haiku 5.5", "", "", reasonNotIdle, false, false},
		{"another tool", "codex", "idle", "Haiku 5.5", "", "", reasonNotClaude, false, false},
		{"text in the prompt", "claude", "idle", "Haiku 5.5", "", "half a thought", send.ReasonDraft, false, false},
		{"a model it does not offer", "claude", "idle", "Opus 9", "", "", reasonNoModel, true, true},
		{"an effort the model lacks", "claude", "idle", "Opus 4.6", "xHigh", "", reasonNoEffort, true, true},
		{"an effort that is not a level", "claude", "idle", "Haiku 5.5", "Extreme", "", reasonNoEffort, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSim()
			s.draft = tt.draft
			_, err := simAnswerer(s, tt.tool, tt.status).SetModel(context.Background(), "s1", tt.model, tt.effort)
			if reason(err) != tt.reason {
				t.Fatalf("err = %v", err)
			}
			if (len(s.typed) > 0) != tt.typed {
				t.Fatalf("typed %v", s.typed)
			}
			if s.open || s.enterPressed {
				t.Fatalf("picker left open=%v or Enter pressed=%v", s.open, s.enterPressed)
			}
			for _, k := range s.keys {
				if k == "s" {
					t.Fatalf("applied a choice that was refused: %v", s.keys)
				}
			}
			if tt.closedAfter && s.keys[len(s.keys)-1] != "Escape" {
				t.Fatalf("keys = %v, want to end with Escape", s.keys)
			}
			if s.current != 1 {
				t.Fatalf("model changed to %d", s.current)
			}
		})
	}
}

// reason is the Reason of a refusal, or "" for any other error.
func reason(err error) send.Reason {
	if r, ok := err.(*send.Refused); ok {
		return r.Reason
	}
	return ""
}
