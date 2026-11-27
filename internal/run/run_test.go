package run

import (
	"context"
	"errors"
	"os"
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

// A command's own background children are killed with it at the deadline,
// and do not stretch the wait.
func TestTimeoutKillsTheGroup(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "survived")
	start := time.Now()
	_, err := Exec{Timeout: 300 * time.Millisecond}.Run(context.Background(), "sh", "-c", "(sleep 1; touch "+marker+") & sleep 30")
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("returned after %s", d)
	}
	time.Sleep(1500 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a background child outlived the timeout")
	}
}

func TestLimit(t *testing.T) {
	long, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	if got := (Exec{}).limit(long); got < 149*time.Second {
		t.Fatalf("caller's 150s deadline gave %s", got)
	}
	if got := (Exec{}).limit(context.Background()); got != defaultTimeout {
		t.Fatalf("no deadline gave %s", got)
	}
	if got := (Exec{Timeout: time.Second}).limit(long); got != time.Second {
		t.Fatalf("explicit timeout gave %s", got)
	}
}

// With no Timeout, the caller's deadline is the limit.
func TestCallerDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := (Exec{}).Run(ctx, "sleep", "2"); !errors.Is(err, ErrTimeout) {
		t.Fatalf("short deadline: err = %v", err)
	}
}

// A clean exit that leaves a background process holding the output still
// returns once the process itself is done.
func TestBackgroundChildHoldingOutput(t *testing.T) {
	start := time.Now()
	out, err := Exec{Timeout: 10 * time.Second}.Run(context.Background(), "sh", "-c", "echo hi; sleep 6 &")
	if err != nil || string(out) != "hi\n" {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	if d := time.Since(start); d > 4*time.Second {
		t.Fatalf("returned after %s", d)
	}
}
