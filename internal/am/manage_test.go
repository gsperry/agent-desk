package am

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestManageArgs(t *testing.T) {
	tests := []struct {
		name string
		run  func(c *CLI) error
		want string
	}{
		{"spawn with everything", func(c *CLI) error {
			_, err := c.Spawn(context.Background(), SpawnRequest{Name: "fix-login", Tool: "claude", Group: "web", Directory: "/src/web", Worktree: true, Prompt: "--help is a prompt", Model: "opus", Effort: "high"})
			return err
		}, "agent-manager|spawn|--json|--name=fix-login|--tool=claude|--group=web|--directory=/src/web|--prompt=--help is a prompt|--model=opus|--effort=high|--worktree"},
		{"spawn with defaults", func(c *CLI) error {
			_, err := c.Spawn(context.Background(), SpawnRequest{Directory: "/src"})
			return err
		}, "agent-manager|spawn|--json|--directory=/src"},
		{"kill", func(c *CLI) error { return c.Kill(context.Background(), "a1") }, "agent-manager|kill|--json|--|a1"},
		{"revive", func(c *CLI) error { return c.Revive(context.Background(), "a1") }, "agent-manager|revive|--json|--|a1"},
		{"archive", func(c *CLI) error { return c.Archive(context.Background(), "a1", false) }, "agent-manager|archive|--json|--|a1"},
		{"restore", func(c *CLI) error { return c.Archive(context.Background(), "a1", true) }, "agent-manager|archive|--json|--restore|--|a1"},
		{"move to root", func(c *CLI) error { return c.Move(context.Background(), "a1", "") }, "agent-manager|move|--json|--|a1|"},
		{"create group", func(c *CLI) error { return c.CreateGroup(context.Background(), "backend/api", "/src/api") }, "agent-manager|create-group|--json|--directory=/src/api|--|backend/api"},
		{"delete group", func(c *CLI) error { return c.DeleteGroup(context.Background(), "-weird") }, "agent-manager|delete-group|--json|--|-weird"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &fakeRunner{out: []byte(`{"id":"n1"}`)}
			if err := tt.run(&CLI{Runner: r, Bin: "agent-manager"}); err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(r.args, "|"); got != tt.want {
				t.Fatalf("args = %q\nwant   %q", got, tt.want)
			}
		})
	}
}

func TestOfferedTools(t *testing.T) {
	settings := []Setting{
		{Key: "default_tool", Value: ""},
		{Key: "worktree_default", Value: "on"},
		{Key: "hidden_tools", Value: "codex,gemini", Values: []string{"claude", "opencode", "codex", "gemini"}},
	}
	tools, def, wt := OfferedTools(settings)
	if !reflect.DeepEqual(tools, []string{"claude", "opencode"}) || def != "claude" || !wt {
		t.Fatalf("tools %v def %q worktree %v", tools, def, wt)
	}
	settings[0].Value = "opencode"
	if _, def, _ := OfferedTools(settings); def != "opencode" {
		t.Fatalf("default %q", def)
	}
}
