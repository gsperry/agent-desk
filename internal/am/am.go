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
}

// ErrNotInstalled reports agent-manager missing from PATH.
var ErrNotInstalled = errors.New("agent-manager is not installed or not on PATH")

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
