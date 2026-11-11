// Package am talks to agent-manager through its CLI.
package am

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gsperry/agent-desk/internal/run"
)

// Session is one agent row from `agent-manager snapshot --json`.
type Session struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Tool           string `json:"tool"`
	Group          string `json:"group"`
	Directory      string `json:"directory"`
	Status         string `json:"status"`
	Running        bool   `json:"running"`
	Archived       bool   `json:"archived"`
	Branch         string `json:"branch,omitempty"`
	Environment    string `json:"environment,omitempty"`
	AgentSessionID string `json:"agent_session_id,omitempty"`
	Self           bool   `json:"self"`
}

type Group struct {
	Path      string `json:"path"`
	Directory string `json:"directory,omitempty"`
	Archived  bool   `json:"archived"`
}

type Snapshot struct {
	Version      int       `json:"version"`
	ManagerAwake bool      `json:"manager_awake"`
	Sessions     []Session `json:"sessions"`
	Groups       []Group   `json:"groups"`
}

// Client is the subset of agent-manager Agent Desk uses.
type Client interface {
	Snapshot(ctx context.Context) (Snapshot, error)
	StartServe(ctx context.Context) error
	// ProfileVar is one variable of an environment profile, or "" when the
	// profile does not set it.
	ProfileVar(ctx context.Context, profile, key string) (string, error)
	// Type submits text at the session's prompt as the user.
	Type(ctx context.Context, id, text string) error
	// Read returns the session's visible screen as plain text.
	Read(ctx context.Context, id string) (string, error)
	// ReadStyled returns the screen with its color escape sequences.
	ReadStyled(ctx context.Context, id string) (string, error)
}

// ErrNotInstalled reports agent-manager missing from PATH.
var ErrNotInstalled = errors.New("agent-manager is not installed or not on PATH")

// ErrMaybeSent is a type that failed after the paste started, so the text
// may sit in the pane. It must not be retried automatically.
var ErrMaybeSent = errors.New("the message may have reached the session")

// exitMaybeSent is agent-manager's exit status for ErrMaybeSent.
const exitMaybeSent = 3

type CLI struct {
	Runner run.Runner
	Bin    string
}

func NewCLI() *CLI {
	return &CLI{Runner: run.Exec{}, Bin: "agent-manager"}
}

func (c *CLI) run(ctx context.Context, args ...string) ([]byte, error) {
	out, err := c.Runner.Run(ctx, c.Bin, args...)
	if errors.Is(err, run.ErrNotFound) {
		return nil, ErrNotInstalled
	}
	return out, err
}

func (c *CLI) Snapshot(ctx context.Context) (Snapshot, error) {
	out, err := c.run(ctx, "snapshot", "--json")
	if err != nil {
		return Snapshot{}, err
	}
	return ParseSnapshot(out)
}

// ParseSnapshot decodes snapshot JSON, refusing a version this code does not
// know so a format change shows up as an error instead of a wrong list.
func ParseSnapshot(data []byte) (Snapshot, error) {
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return Snapshot{}, fmt.Errorf("agent-manager snapshot is not valid JSON: %w", err)
	}
	if s.Version != 1 {
		return Snapshot{}, fmt.Errorf("agent-manager snapshot version %d is not supported (expected 1)", s.Version)
	}
	return s, nil
}

// StartServe starts a headless manager so statuses keep advancing while no
// TUI is open. It is a no-op when one is already running.
func (c *CLI) StartServe(ctx context.Context) error {
	_, err := c.run(ctx, "serve", "--background")
	return err
}

func (c *CLI) ProfileVar(ctx context.Context, profile, key string) (string, error) {
	out, err := c.run(ctx, "env", "show", "--json", "--", profile)
	if err != nil {
		return "", err
	}
	var rec struct {
		Vars map[string]string `json:"vars"`
	}
	if err := json.Unmarshal(out, &rec); err != nil {
		return "", fmt.Errorf("agent-manager env show: %w", err)
	}
	return rec.Vars[key], nil
}

// Type runs `agent-manager type`. The separator keeps text that starts with
// a dash, such as "--help", from being read as a flag.
func (c *CLI) Type(ctx context.Context, id, text string) error {
	_, err := c.run(ctx, "type", "--json", "--", id, text)
	var exitErr *run.ExitError
	if errors.As(err, &exitErr) && exitErr.Code == exitMaybeSent {
		return fmt.Errorf("%w: %s", ErrMaybeSent, exitErr.Stderr)
	}
	return err
}

func (c *CLI) Read(ctx context.Context, id string) (string, error) {
	out, err := c.run(ctx, "read", "--json", "--", id)
	if err != nil {
		return "", err
	}
	var screen struct {
		Output string `json:"output"`
	}
	if err := json.Unmarshal(out, &screen); err != nil {
		return "", fmt.Errorf("agent-manager read: %w", err)
	}
	return screen.Output, nil
}

// ReadStyled is Read with the screen's color escape sequences kept.
func (c *CLI) ReadStyled(ctx context.Context, id string) (string, error) {
	out, err := c.run(ctx, "read", "--styled", "--json", "--", id)
	if err != nil {
		return "", err
	}
	var screen struct {
		Output string `json:"output"`
	}
	if err := json.Unmarshal(out, &screen); err != nil {
		return "", fmt.Errorf("agent-manager read: %w", err)
	}
	return screen.Output, nil
}

// ConfigDir is agent-manager's state directory, resolved the way
// agent-manager resolves it.
func ConfigDir() (string, error) {
	if d := os.Getenv("AGENT_MANAGER_CONFIG_DIR"); d != "" {
		return d, nil
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "agent-manager"), nil
	}
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "agent-manager"), nil
}

// HooksDir holds the per-session hook status files.
func HooksDir() (string, error) {
	d, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "hooks"), nil
}
