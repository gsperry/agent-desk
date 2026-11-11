package am

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsperry/agent-desk/internal/run"
)

type fakeRunner struct {
	out  []byte
	err  error
	args []string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.args = append([]string{name}, args...)
	return f.out, f.err
}

func TestSnapshot(t *testing.T) {
	sample, err := os.ReadFile(filepath.Join("testdata", "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		out       string
		err       error
		wantErr   string
		wantCount int
	}{
		{"sample", string(sample), nil, "", 3},
		{"not installed", "", fmt.Errorf("agent-manager: %w", run.ErrNotFound), ErrNotInstalled.Error(), 0},
		{"exit error", "", &run.ExitError{Name: "agent-manager", Code: 1, Stderr: "boom"}, "boom", 0},
		{"invalid json", "{", nil, "not valid JSON", 0},
		{"unknown version", `{"version": 2, "sessions": []}`, nil, "version 2", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &fakeRunner{out: []byte(tt.out), err: tt.err}
			c := &CLI{Runner: r, Bin: "agent-manager"}
			snap, err := c.Snapshot(context.Background())
			if got := strings.Join(r.args, " "); got != "agent-manager snapshot --json" {
				t.Errorf("args = %q", got)
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(snap.Sessions) != tt.wantCount {
				t.Fatalf("sessions = %d, want %d", len(snap.Sessions), tt.wantCount)
			}
		})
	}
}

func TestSnapshotFields(t *testing.T) {
	snap, err := Fixture{Path: filepath.Join("testdata", "snapshot.json")}.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s := snap.Sessions[1]
	if s.Group != "frontend" || s.Branch != "am/login" || s.Status != "waiting" || !s.Running || s.AgentSessionID == "" {
		t.Fatalf("unexpected session %+v", s)
	}
	if !snap.ManagerAwake || len(snap.Groups) != 1 {
		t.Fatalf("unexpected snapshot %+v", snap)
	}
}

func TestNotInstalledIsDetectable(t *testing.T) {
	c := &CLI{Runner: &fakeRunner{err: fmt.Errorf("x: %w", run.ErrNotFound)}, Bin: "agent-manager"}
	if err := c.StartServe(context.Background()); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("err = %v", err)
	}
}

func TestProfileVar(t *testing.T) {
	r := &fakeRunner{out: []byte(`{"name":"alt","keys":["CLAUDE_CONFIG_DIR","SECRET"],"vars":{"CLAUDE_CONFIG_DIR":"/x/claude","SECRET":"s"}}`)}
	c := &CLI{Runner: r, Bin: "agent-manager"}
	tests := []struct{ key, want string }{
		{"CLAUDE_CONFIG_DIR", "/x/claude"},
		{"MISSING", ""},
	}
	for _, tt := range tests {
		got, err := c.ProfileVar(context.Background(), "alt", tt.key)
		if err != nil || got != tt.want {
			t.Fatalf("%s: got %q, %v; want %q", tt.key, got, err, tt.want)
		}
	}
	if got := strings.Join(r.args, " "); got != "agent-manager env show --json -- alt" {
		t.Fatalf("args = %q", got)
	}
}

func TestType(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantErr error
	}{
		{"sent", nil, nil},
		{"maybe sent", &run.ExitError{Name: "agent-manager", Code: 3, Stderr: "prompt may have reached"}, ErrMaybeSent},
		{"not installed", fmt.Errorf("x: %w", run.ErrNotFound), ErrNotInstalled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &fakeRunner{err: tt.err}
			c := &CLI{Runner: r, Bin: "agent-manager"}
			err := c.Type(context.Background(), "a1b2", "--help me")
			if got := strings.Join(r.args, "|"); got != "agent-manager|type|--json|--|a1b2|--help me" {
				t.Fatalf("args = %q", got)
			}
			if tt.wantErr == nil && err != nil || tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
	other := &run.ExitError{Name: "agent-manager", Code: 1, Stderr: "session is not running"}
	c := &CLI{Runner: &fakeRunner{err: other}, Bin: "agent-manager"}
	if err := c.Type(context.Background(), "a", "b"); errors.Is(err, ErrMaybeSent) || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("exit 1: %v", err)
	}
}

func TestRead(t *testing.T) {
	r := &fakeRunner{out: []byte(`{"session":{"id":"a1"},"output":"line one\nline two"}`)}
	c := &CLI{Runner: r, Bin: "agent-manager"}
	got, err := c.Read(context.Background(), "a1")
	if err != nil || got != "line one\nline two" {
		t.Fatalf("got %q, %v", got, err)
	}
	if strings.Join(r.args, " ") != "agent-manager read --json -- a1" {
		t.Fatalf("args = %v", r.args)
	}
}

func TestReadStyled(t *testing.T) {
	r := &fakeRunner{out: []byte(`{"output":"\u001b[31mred\u001b[0m"}`)}
	c := &CLI{Runner: r, Bin: "agent-manager"}
	got, err := c.ReadStyled(context.Background(), "a1")
	if err != nil || got != "\x1b[31mred\x1b[0m" {
		t.Fatalf("got %q, %v", got, err)
	}
	if strings.Join(r.args, " ") != "agent-manager read --styled --json -- a1" {
		t.Fatalf("args = %v", r.args)
	}
}

func TestConfigDir(t *testing.T) {
	tests := []struct {
		name, amDir, xdg, want string
	}{
		{"explicit", "/x/am", "/x/xdg", "/x/am"},
		{"xdg", "", "/x/xdg", "/x/xdg/agent-manager"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AGENT_MANAGER_CONFIG_DIR", tt.amDir)
			t.Setenv("XDG_CONFIG_HOME", tt.xdg)
			got, err := ConfigDir()
			if err != nil || got != tt.want {
				t.Fatalf("ConfigDir() = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestFixtureExpandsDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshot.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"sessions":[{"id":"x","directory":"${FIXTURES}/repo"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	snap, err := Fixture{Path: path}.Snapshot(context.Background())
	if err != nil || snap.Sessions[0].Directory != filepath.Join(dir, "repo") {
		t.Fatalf("directory %q, err %v", snap.Sessions[0].Directory, err)
	}
}
