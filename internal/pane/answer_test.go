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
