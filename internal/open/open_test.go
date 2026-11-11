package open

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/send"
)

type sessions struct{ dir string }

func (s sessions) State() hub.State {
	return hub.State{Sessions: []hub.SessionView{{ID: "s1", Directory: s.dir}}}
}

type recorder struct{ args []string }

func (r *recorder) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.args = append([]string{name}, args...)
	return nil, nil
}

func TestOpen(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "src", "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		req    Request
		want   string
		reason send.Reason
	}{
		{"absolute with line", Request{SessionID: "s1", Path: file, Line: 12}, "code -g " + file + ":12", ""},
		{"relative to the session", Request{SessionID: "s1", Path: "src/main.go"}, "code -g " + file, ""},
		{"dot relative", Request{SessionID: "s1", Path: "./src/../src/main.go", Line: 1}, "code -g " + file + ":1", ""},
		{"file URL", Request{SessionID: "s1", Path: "file://" + file}, "code -g " + file, ""},
		{"directory", Request{SessionID: "s1", Path: "src"}, "", reasonBadRequest},
		{"control characters", Request{SessionID: "s1", Path: file + "\x1b[2J"}, "", reasonBadRequest},
		{"missing file", Request{SessionID: "s1", Path: "src/nope.go"}, "", reasonNotFound},
		{"relative without session", Request{SessionID: "other", Path: "src/main.go"}, "", reasonNotFound},
		{"empty", Request{SessionID: "s1", Path: " "}, "", reasonBadRequest},
		{"negative line", Request{SessionID: "s1", Path: file, Line: -1}, "", reasonBadRequest},
		{"flag-like relative path", Request{SessionID: "s1", Path: "--help"}, "", reasonNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recorder{}
			o := &Opener{Sessions: sessions{dir: dir}, Runner: r, Editor: "code"}
			err := o.Open(context.Background(), tt.req)
			if tt.reason != "" {
				var refused *send.Refused
				if !errors.As(err, &refused) || refused.Reason != tt.reason {
					t.Fatalf("err = %v, want %s", err, tt.reason)
				}
				if r.args != nil {
					t.Fatalf("ran %v on refusal", r.args)
				}
				return
			}
			if err != nil || strings.Join(r.args, " ") != tt.want {
				t.Fatalf("ran %q, err %v; want %q", strings.Join(r.args, " "), err, tt.want)
			}
		})
	}
}

func TestLogRunner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "open.log")
	if _, err := (LogRunner{Path: path}).Run(context.Background(), "/usr/bin/code", "-g", "/a.go:3"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "code -g /a.go:3\n" {
		t.Fatalf("log = %q", b)
	}
}
