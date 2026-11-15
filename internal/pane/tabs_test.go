package pane

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/send"
)

func TestCurrentStep(t *testing.T) {
	steps := []Step{{Label: "Size"}, {Label: "Shape"}, {Label: "Color"}, {Label: "Submit", Submit: true}}
	tests := []struct {
		name   string
		styled string
		want   int
	}{
		{"first question", read(t, "askq-3tabs-q1.ansi"), 0},
		{"third question", read(t, "askq-3tabs-q3.ansi"), 2},
		{"review", read(t, "askq-3tabs-review.ansi"), 3},
		{"truecolor background", "←  ☐ Size \x1b[48;2;10;20;30m ☐ Shape \x1b[0m ☐ Color  ✔ Submit  →", 1},
		{"no highlight", "←  ☐ Size  ☐ Shape  ☐ Color  ✔ Submit  →", -1},
		{"no tab bar", "Pick a size\n❯ 1. Small", -1},
		{"foreground only", "←  \x1b[38;5;153m☐ Size\x1b[39m  ☐ Shape  ☐ Color  ✔ Submit  →", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := currentStep(tt.styled, steps); got != tt.want {
				t.Errorf("currentStep = %d, want %d", got, tt.want)
			}
		})
	}
	if got := currentStep(read(t, "askq-3tabs-q1.ansi"), steps[:1]); got != -1 {
		t.Errorf("one step = %d", got)
	}
}

func TestParse3Tabs(t *testing.T) {
	for file, q := range map[string]string{"askq-3tabs-q1.txt": "Pick a size", "askq-3tabs-q3.txt": "Pick a color"} {
		d := Parse(read(t, file))
		if d == nil || d.Question != q || len(d.Steps) != 4 {
			t.Fatalf("%s: %+v", file, d)
		}
	}
}

// tabReader serves a plain screen and its styled copy.
type tabReader struct{ plain, styled string }

func (r tabReader) Read(context.Context, string) (string, error)       { return r.plain, nil }
func (r tabReader) ReadStyled(context.Context, string) (string, error) { return r.styled, nil }

func TestTabAnswer(t *testing.T) {
	q1 := tabReader{read(t, "askq-3tabs-q1.txt"), read(t, "askq-3tabs-q1.ansi")}
	review := tabReader{read(t, "askq-3tabs-review.txt"), read(t, "askq-3tabs-review.ansi")}
	tests := []struct {
		name     string
		reader   tabReader
		ans      Answer
		wantKeys []string
		reason   send.Reason
	}{
		{"jump ahead two", q1, Answer{Action: "tab", Index: 2, Label: "Color"}, []string{"Right", "Right"}, ""},
		{"back from review", review, Answer{Action: "tab", Index: 0, Label: "Size"}, []string{"Left", "Left", "Left"}, ""},
		{"already there", q1, Answer{Action: "tab", Index: 0, Label: "Size"}, nil, ""},
		{"label changed", q1, Answer{Action: "tab", Index: 2, Label: "Shape"}, nil, reasonChanged},
		{"out of range", q1, Answer{Action: "tab", Index: 9, Label: "Color"}, nil, reasonChanged},
		{"current unknown", tabReader{q1.plain, q1.plain}, Answer{Action: "tab", Index: 2, Label: "Color"}, nil, reasonChanged},
		{"one question", tabReader{read(t, "ask-question.txt"), ""}, Answer{Action: "tab", Index: 0, Label: "Color"}, nil, reasonNoDialog},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := &recordKeys{}
			a := &Answerer{
				Sessions: &fakeSessions{st: hub.State{Sessions: []hub.SessionView{{ID: "s1", Running: true}}}},
				Reader:   tt.reader,
				Keys:     keys,
			}
			err := a.Answer(context.Background(), "s1", tt.ans)
			if tt.reason != "" {
				var refused *send.Refused
				if !errors.As(err, &refused) || refused.Reason != tt.reason || len(keys.got) != 0 {
					t.Fatalf("err = %v keys = %v, want %s", err, keys.got, tt.reason)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(keys.got, tt.wantKeys) {
				t.Fatalf("keys = %v, err = %v; want %v", keys.got, err, tt.wantKeys)
			}
		})
	}
}

func TestScreenMarksCurrentStep(t *testing.T) {
	a := &Answerer{
		Sessions: &fakeSessions{st: hub.State{Sessions: []hub.SessionView{{ID: "s1", Running: true}}}},
		Reader:   tabReader{read(t, "askq-3tabs-q3.txt"), read(t, "askq-3tabs-q3.ansi")},
	}
	sc, err := a.Screen(context.Background(), "s1")
	if err != nil || sc.Dialog == nil {
		t.Fatal(err)
	}
	for i, s := range sc.Dialog.Steps {
		if s.Current != (i == 2) {
			t.Fatalf("steps = %+v", sc.Dialog.Steps)
		}
	}
}
