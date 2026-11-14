package am

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// SpawnRequest is a new session. Empty fields take agent-manager's
// defaults.
type SpawnRequest struct {
	Name      string `json:"name"`
	Tool      string `json:"tool"`
	Group     string `json:"group"`
	Directory string `json:"directory"`
	Worktree  bool   `json:"worktree"`
	Prompt    string `json:"prompt"`
	Model     string `json:"model"`
	Effort    string `json:"effort"`
	// Resume is a saved conversation id to open instead of a new one.
	Resume string `json:"resume,omitempty"`
}

// Setting is one row of `agent-manager settings list --json`.
type Setting struct {
	Key    string   `json:"key"`
	Value  string   `json:"value"`
	Values []string `json:"values"`
}

// Values are passed as --flag=value so a value starting with a dash is
// never read as a flag.
func (c *CLI) Spawn(ctx context.Context, r SpawnRequest) (Session, error) {
	args := []string{"spawn", "--json"}
	add := func(flag, v string) {
		if v != "" {
			args = append(args, "--"+flag+"="+v)
		}
	}
	add("name", r.Name)
	add("tool", r.Tool)
	add("group", r.Group)
	add("directory", r.Directory)
	add("prompt", r.Prompt)
	add("model", r.Model)
	add("effort", r.Effort)
	add("resume", r.Resume)
	if r.Worktree {
		args = append(args, "--worktree")
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return Session{}, err
	}
	var s Session
	if err := json.Unmarshal(out, &s); err != nil {
		return Session{}, fmt.Errorf("agent-manager spawn: %w", err)
	}
	return s, nil
}

func (c *CLI) Kill(ctx context.Context, id string) error {
	_, err := c.run(ctx, "kill", "--json", "--", id)
	return err
}

func (c *CLI) Revive(ctx context.Context, id string) error {
	_, err := c.run(ctx, "revive", "--json", "--", id)
	return err
}

func (c *CLI) Archive(ctx context.Context, id string, restore bool) error {
	args := []string{"archive", "--json"}
	if restore {
		args = append(args, "--restore")
	}
	_, err := c.run(ctx, append(args, "--", id)...)
	return err
}

// Move files a session under group; an empty group is the root.
func (c *CLI) Move(ctx context.Context, id, group string) error {
	_, err := c.run(ctx, "move", "--json", "--", id, group)
	return err
}

func (c *CLI) CreateGroup(ctx context.Context, path, directory string) error {
	args := []string{"create-group", "--json"}
	if directory != "" {
		args = append(args, "--directory="+directory)
	}
	_, err := c.run(ctx, append(args, "--", path)...)
	return err
}

func (c *CLI) DeleteGroup(ctx context.Context, path string) error {
	_, err := c.run(ctx, "delete-group", "--json", "--", path)
	return err
}

func (c *CLI) Settings(ctx context.Context) ([]Setting, error) {
	out, err := c.run(ctx, "settings", "list", "--json")
	if err != nil {
		return nil, err
	}
	var s []Setting
	if err := json.Unmarshal(out, &s); err != nil {
		return nil, fmt.Errorf("agent-manager settings: %w", err)
	}
	return s, nil
}

// SetSetting stores one agent-manager setting, which agent-manager checks
// against the values its Settings row takes.
func (c *CLI) SetSetting(ctx context.Context, key, value string) error {
	_, err := c.run(ctx, "settings", "set", "--", key, value)
	return err
}

// OfferedTools is the tools agent-manager's pickers offer: every tool
// minus the hidden ones, in agent-manager's order.
func OfferedTools(settings []Setting) (tools []string, defaultTool string, worktree bool) {
	hidden := map[string]bool{}
	var all []string
	for _, s := range settings {
		switch s.Key {
		case "hidden_tools":
			all = s.Values
			for _, t := range strings.Split(s.Value, ",") {
				hidden[strings.TrimSpace(t)] = true
			}
		case "default_tool":
			defaultTool = s.Value
		case "worktree_default":
			worktree = s.Value == "on"
		}
	}
	for _, t := range all {
		if !hidden[t] {
			tools = append(tools, t)
		}
	}
	if defaultTool == "" && len(tools) > 0 {
		defaultTool = tools[0]
	}
	return tools, defaultTool, worktree
}
