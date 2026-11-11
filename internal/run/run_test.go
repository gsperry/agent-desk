package run

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExec(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		args     []string
		timeout  time.Duration
		wantOut  string
		wantCode int
		wantErr  string
	}{
		{"stdout", "sh", []string{"-c", "printf hello"}, 0, "hello", 0, ""},
		{"argv is not shell-split", "printf", []string{"%s", "a b; echo injected"}, 0, "a b; echo injected", 0, ""},
		{"exit code and stderr", "sh", []string{"-c", "echo oops >&2; exit 3"}, 0, "", 3, "oops"},
		{"not found", "agent-desk-no-such-command", nil, 0, "", 0, ErrNotFound.Error()},
		{"timeout", "sleep", []string{"5"}, 100 * time.Millisecond, "", 0, "timed out"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := Exec{Timeout: tt.timeout}.Run(context.Background(), tt.cmd, tt.args...)
			if tt.wantCode != 0 {
				var exitErr *ExitError
				if !errors.As(err, &exitErr) || exitErr.Code != tt.wantCode || exitErr.Stderr != tt.wantErr {
					t.Fatalf("err = %v, want exit %d with %q", err, tt.wantCode, tt.wantErr)
				}
				return
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || string(out) != tt.wantOut {
				t.Fatalf("out = %q, err = %v; want %q", out, err, tt.wantOut)
			}
		})
	}
}

func TestRunInput(t *testing.T) {
	dir := t.TempDir()
	out, err := Exec{}.RunInput(context.Background(), dir, []byte("from stdin"), "sh", "-c", "pwd -P; cat")
	if err != nil {
		t.Fatal(err)
	}
	real, _ := filepath.EvalSymlinks(dir)
	if string(out) != real+"\nfrom stdin" {
		t.Fatalf("out = %q", out)
	}
}

func TestEnvironDropsSessionVariables(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1/default,1,0")
	t.Setenv("TMUX_PANE", "%1")
	t.Setenv("AGENT_MANAGER_SESSION_ID", "abcd1234")
	t.Setenv("AGENT_DESK_KEEP", "yes")
	t.Setenv("AGENT_DESK_TOKEN", "secret")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "df2d1f97-0000-4000-8000-000000000000")
	env := strings.Join(Environ(), "\n")
	for _, k := range []string{"TMUX=", "TMUX_PANE=", "AGENT_MANAGER_SESSION_ID=", "CLAUDE_CODE_SESSION_ID=", "AGENT_DESK_TOKEN="} {
		if strings.Contains("\n"+env, "\n"+k) {
			t.Errorf("%s still present", k)
		}
	}
	if !strings.Contains(env, "AGENT_DESK_KEEP=yes") {
		t.Error("unrelated variable dropped")
	}
}
