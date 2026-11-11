// Package run executes local CLIs with a timeout and argv only, never a shell.
package run

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Runner runs a command and returns its stdout.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ErrNotFound reports a command that is not on PATH.
var ErrNotFound = errors.New("command not found")

// ExitError is a command that ran and failed. Stderr is trimmed.
type ExitError struct {
	Name   string
	Code   int
	Stderr string
}

func (e *ExitError) Error() string {
	if e.Stderr == "" {
		return fmt.Sprintf("%s exited with status %d", e.Name, e.Code)
	}
	return fmt.Sprintf("%s exited with status %d: %s", e.Name, e.Code, e.Stderr)
}

// Exec runs real processes.
type Exec struct {
	Timeout time.Duration
	// Env overrides the environment. Nil means Environ().
	Env []string
}

func (x Exec) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	timeout := x.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = x.Env
	if cmd.Env == nil {
		cmd.Env = Environ()
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if errors.Is(err, exec.ErrNotFound) {
		return nil, fmt.Errorf("%s: %w", name, ErrNotFound)
	}
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("%s timed out after %s", name, timeout)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return stdout.Bytes(), &ExitError{Name: name, Code: exitErr.ExitCode(), Stderr: strings.TrimSpace(stderr.String())}
	}
	return stdout.Bytes(), err
}

// Environ is the process environment without the variables that make
// agent-manager treat a child process as one of its own sessions, and
// without Agent Desk's token. Process ancestry can still identify a
// caller; see docs/integration-notes.md.
func Environ() []string {
	drop := map[string]bool{"TMUX": true, "TMUX_PANE": true, "AGENT_MANAGER_SESSION_ID": true, "AGENT_DESK_TOKEN": true}
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if !drop[k] {
			env = append(env, kv)
		}
	}
	return env
}
