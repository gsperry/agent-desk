package pane

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/send"
)

type fakeSessions struct{ st hub.State }

func (f *fakeSessions) State() hub.State { return f.st }
func (f *fakeSessions) Trigger()         {}

type fakeReader struct{ screen string }

func (f fakeReader) Read(context.Context, string) (string, error) { return f.screen, nil }
func (f fakeReader) ReadStyled(context.Context, string) (string, error) {
	return "\x1b[31m" + f.screen, nil
}

type recordKeys struct{ got []string }

func (r *recordKeys) Send(_ context.Context, _ string, keys ...string) error {
	r.got = append(r.got, keys...)
	return nil
}

func TestAnswer(t *testing.T) {
	perm := read(t, "permission.txt")
	ask := read(t, "ask-question.txt")
	idle := read(t, "idle.txt")
	tests := []struct {
		name     string
		screen   string
		running  bool
		ans      Answer
		wantKeys []string
		reason   send.Reason
	}{
		{"choose first", perm, true, Answer{Action: "choose", Index: 0, Label: "Yes"}, []string{"Enter"}, ""},
		{"choose last", perm, true, Answer{Action: "choose", Index: 3, Label: "No"}, []string{"Down", "Down", "Down", "Enter"}, ""},
		{"label changed", perm, true, Answer{Action: "choose", Index: 0, Label: "No"}, nil, reasonChanged},
		{"index out of range", perm, true, Answer{Action: "choose", Index: 9, Label: "No"}, nil, reasonChanged},
		{"free option", ask, true, Answer{Action: "choose", Index: 2, Label: "Type something."}, nil, reasonFree},
		{"no dialog", idle, true, Answer{Action: "choose", Index: 0, Label: "Yes"}, nil, reasonNoDialog},
		{"escape", perm, true, Answer{Action: "key", Key: "Escape"}, []string{"Escape"}, ""},
		{"disallowed key", perm, true, Answer{Action: "key", Key: "C-c"}, nil, reasonBadKey},
		{"literal text as key", perm, true, Answer{Action: "key", Key: "rm -rf /"}, nil, reasonBadKey},
		{"unknown action", perm, true, Answer{Action: "type"}, nil, reasonBadKey},
		{"not running", perm, false, Answer{Action: "key", Key: "Enter"}, nil, send.ReasonNotRunning},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := &recordKeys{}
			a := &Answerer{
				Sessions: &fakeSessions{st: hub.State{Sessions: []hub.SessionView{{ID: "s1", Running: tt.running}}}},
				Reader:   fakeReader{screen: tt.screen},
				Keys:     keys,
			}
			err := a.Answer(context.Background(), "s1", tt.ans)
			if tt.reason != "" {
				var refused *send.Refused
				if !errors.As(err, &refused) || refused.Reason != tt.reason {
					t.Fatalf("err = %v, want %s", err, tt.reason)
				}
				if len(keys.got) != 0 {
					t.Fatalf("keys sent on refusal: %v", keys.got)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(keys.got, tt.wantKeys) {
				t.Fatalf("keys = %v, err = %v; want %v", keys.got, err, tt.wantKeys)
			}
		})
	}
}

// seqReader returns its screens in turn, then repeats the last.
type seqReader struct {
	screens []string
	n       int
}

func (r *seqReader) Read(context.Context, string) (string, error) {
	s := r.screens[min(r.n, len(r.screens)-1)]
	r.n++
	return s, nil
}
func (r *seqReader) ReadStyled(ctx context.Context, id string) (string, error) {
	return r.Read(ctx, id)
}

type recordTyper struct{ got []string }

func (r *recordTyper) Type(_ context.Context, _ string, text string) error {
	r.got = append(r.got, text)
	return nil
}

func TestAnswerText(t *testing.T) {
	initial := read(t, "askq-free-initial.txt")
	selected := read(t, "askq-free-selected.txt")
	perm := read(t, "permission.txt")
	text := func(label, s string) Answer { return Answer{Action: "text", Index: 2, Label: label, Text: s} }
	tests := []struct {
		name     string
		screens  []string
		ans      Answer
		typer    bool
		wantKeys []string
		wantText []string
		reason   send.Reason
	}{
		{"moves to the field, confirms it, types", []string{initial, selected}, text("Type something.", "  Green, with teal  "), true, []string{"Down", "Down"}, []string{"Green, with teal"}, ""},
		{"already on the field", []string{selected, selected}, text("Type something.", "Green"), true, nil, []string{"Green"}, ""},
		{"field did not become active", []string{initial, initial}, text("Type something.", "Green"), true, []string{"Down", "Down"}, nil, reasonChanged},
		{"label changed", []string{initial}, text("Something else", "Green"), true, nil, nil, reasonChanged},
		{"not a text option", []string{perm}, Answer{Action: "text", Index: 0, Label: "Yes", Text: "x"}, true, nil, nil, reasonChanged},
		{"empty text", []string{initial}, text("Type something.", "  "), true, nil, nil, reasonBadText},
		{"multi-line text", []string{initial}, text("Type something.", "a\nb"), true, nil, nil, reasonBadText},
		{"escape sequence", []string{initial}, text("Type something.", "a\x1b[2Jb"), true, nil, nil, reasonBadText},
		{"too long", []string{initial}, text("Type something.", strings.Repeat("x", maxAnswerText+1)), true, nil, nil, reasonBadText},
		{"no typer", []string{initial}, text("Type something.", "Green"), false, nil, nil, reasonBadText},
		{"dialog gone", []string{read(t, "idle.txt")}, text("Type something.", "Green"), true, nil, nil, reasonNoDialog},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := &recordKeys{}
			typer := &recordTyper{}
			a := &Answerer{
				Sessions: &fakeSessions{st: hub.State{Sessions: []hub.SessionView{{ID: "s1", Running: true}}}},
				Reader:   &seqReader{screens: tt.screens},
				Keys:     keys,
			}
			if tt.typer {
				a.Typer = typer
			}
			err := a.Answer(context.Background(), "s1", tt.ans)
			if tt.reason != "" {
				var refused *send.Refused
				if !errors.As(err, &refused) || refused.Reason != tt.reason {
					t.Fatalf("err = %v, want %s", err, tt.reason)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(keys.got, tt.wantKeys) || !reflect.DeepEqual(typer.got, tt.wantText) {
				t.Fatalf("keys = %v, typed = %q; want %v, %q", keys.got, typer.got, tt.wantKeys, tt.wantText)
			}
		})
	}
}

func TestChatAboutThisIsAButton(t *testing.T) {
	keys := &recordKeys{}
	a := &Answerer{
		Sessions: &fakeSessions{st: hub.State{Sessions: []hub.SessionView{{ID: "s1", Running: true}}}},
		Reader:   fakeReader{screen: read(t, "askq-free-initial.txt")},
		Keys:     keys,
	}
	if err := a.Answer(context.Background(), "s1", Answer{Action: "choose", Index: 3, Label: "Chat about this"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(keys.got, []string{"Down", "Down", "Down", "Enter"}) {
		t.Fatalf("keys = %v", keys.got)
	}
}

func TestScreen(t *testing.T) {
	a := &Answerer{
		Sessions: &fakeSessions{st: hub.State{Sessions: []hub.SessionView{{ID: "s1", Running: true}}}},
		Reader:   fakeReader{screen: strings.Repeat("old line\n", 100) + read(t, "permission.txt")},
		Keys:     &recordKeys{},
	}
	s, err := a.Screen(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(strings.Split(s.Text, "\n")); n != screenLines {
		t.Fatalf("lines = %d", n)
	}
	if s.Dialog == nil || len(s.Dialog.Options) != 4 {
		t.Fatalf("dialog = %+v", s.Dialog)
	}
	if _, err := a.Screen(context.Background(), "missing"); err == nil {
		t.Fatal("unknown session read")
	}
}

func TestStyled(t *testing.T) {
	a := &Answerer{
		Sessions: &fakeSessions{st: hub.State{Sessions: []hub.SessionView{{ID: "s1", Running: true}, {ID: "s2"}}}},
		Reader:   fakeReader{screen: "hello"},
		Keys:     &recordKeys{},
	}
	got, err := a.Styled(context.Background(), "s1")
	if err != nil || got != "\x1b[31mhello" {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, id := range []string{"s2", "missing", "../x"} {
		if _, err := a.Styled(context.Background(), id); err == nil {
			t.Errorf("Styled(%q) read a session it should refuse", id)
		}
	}
}

type argsRunner struct{ args []string }

func (r *argsRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.args = append([]string{name}, args...)
	return nil, nil
}

func TestTmuxKeys(t *testing.T) {
	r := &argsRunner{}
	if err := (TmuxKeys{Runner: r}).Send(context.Background(), "3486b2c0", "Down", "Enter"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.args, " "); got != "tmux -L agentmgr send-keys -t am_3486b2c0.0 Down Enter" {
		t.Fatalf("args = %q", got)
	}
	tests := []struct {
		id   string
		keys []string
	}{
		{"3486b2c0; rm", []string{"Enter"}},
		{"../x", []string{"Enter"}},
		{"ok", []string{"C-c"}},
		{"ok", []string{"-l"}},
	}
	for _, tt := range tests {
		r.args = nil
		if err := (TmuxKeys{Runner: r}).Send(context.Background(), tt.id, tt.keys...); err == nil || r.args != nil {
			t.Errorf("Send(%q, %v) ran %v, err %v", tt.id, tt.keys, r.args, err)
		}
	}
}

func TestFileKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.log")
	k := FileKeys{Path: path}
	if err := k.Send(context.Background(), "a1", "Down", "Enter"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "a1 Down Enter\n" {
		t.Fatalf("log = %q", b)
	}
}
