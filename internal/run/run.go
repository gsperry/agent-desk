// Package run executes local CLIs with a timeout and argv only, never a shell.
package run

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
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

// ErrTimeout reports a command stopped at its deadline.
var ErrTimeout = errors.New("timed out")

// defaultTimeout bounds a command when neither the Exec nor the caller's
// context sets a limit.
const defaultTimeout = 10 * time.Second

// maxStderr bounds the error output kept for a failure message.
const maxStderr = 64 << 10

// waitDelay is how long a finished or stopped command's output pipes may
// stay open, held by a process it started, before they are closed.
const waitDelay = 2 * time.Second

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

// Exec runs real processes. Each runs in its own process group, and the
// whole group is killed at the deadline, so nothing it started is left
// running.
type Exec struct {
	// Timeout bounds each command. Zero uses the caller's deadline, or
	// ten seconds when the caller sets none.
	Timeout time.Duration
	// Env overrides the environment. Nil means Environ().
	Env []string
	// MaxOutput, when set, keeps at most this many bytes of stdout plus
	// one, so a caller can tell the output ran over; the rest is dropped
	// as it arrives instead of held in memory.
	MaxOutput int
}

// capped keeps the first n bytes written to it and drops the rest.
type capped struct {
	buf bytes.Buffer
	n   int
}

func (c *capped) Write(p []byte) (int, error) {
	if room := c.n - c.buf.Len(); room > 0 {
		c.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

func (x Exec) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return x.RunInput(ctx, "", nil, name, args...)
}

// RunInput runs a command in dir (the current directory when empty) with
// input on its stdin.
func (x Exec) RunInput(ctx context.Context, dir string, input []byte, name string, args ...string) ([]byte, error) {
	timeout := x.limit(ctx)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	ownGroup(cmd)
	cmd.WaitDelay = waitDelay
	cmd.Dir = dir
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	cmd.Env = x.Env
	if cmd.Env == nil {
		cmd.Env = Environ()
	}
	stdout := &capped{n: math.MaxInt}
	if x.MaxOutput > 0 {
		stdout.n = x.MaxOutput + 1
	}
	stderr := &capped{n: maxStderr}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	if errors.Is(err, exec.ErrNotFound) {
		return nil, fmt.Errorf("%s: %w", name, ErrNotFound)
	}
	switch ctx.Err() {
	case context.DeadlineExceeded:
		return nil, fmt.Errorf("%s %w after %s", name, ErrTimeout, timeout.Round(time.Second))
	case context.Canceled:
		return nil, fmt.Errorf("%s: %w", name, context.Canceled)
	}
	if errors.Is(err, exec.ErrWaitDelay) {
		// The command exited cleanly; something it started in the
		// background still held its output.
		err = nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return stdout.buf.Bytes(), &ExitError{Name: name, Code: exitErr.ExitCode(), Stderr: strings.TrimSpace(stderr.buf.String())}
	}
	return stdout.buf.Bytes(), err
}

// limit is the command's time limit: Timeout, else the caller's deadline,
// else the default. A spawn the caller gives two minutes gets them.
func (x Exec) limit(ctx context.Context) time.Duration {
	if x.Timeout != 0 {
		return x.Timeout
	}
	if deadline, ok := ctx.Deadline(); ok {
		return time.Until(deadline)
	}
	return defaultTimeout
}

// Environ is the process environment without the variables that make
// agent-manager treat a child process as one of its own sessions, and
// without Agent Desk's token. Process ancestry can still identify a
// caller; see docs/integration-notes.md.
func Environ() []string {
	// CLAUDE_CODE_SESSION_ID matters when Agent Desk is started from inside
	// a Claude Code session: agent-manager maps it to the session that owns
	// the conversation and then refuses to type for "that session".
	drop := map[string]bool{
		"TMUX": true, "TMUX_PANE": true, "AGENT_MANAGER_SESSION_ID": true,
		"CLAUDE_CODE_SESSION_ID": true, "AGENT_DESK_TOKEN": true,
	}
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if !drop[k] {
			env = append(env, kv)
		}
	}
	return env
}
