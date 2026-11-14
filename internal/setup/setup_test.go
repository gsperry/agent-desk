package setup

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/gsperry/agent-desk/internal/am"
	"github.com/gsperry/agent-desk/internal/send"
)

type backend struct {
	settings []am.Setting
	err      error
	calls    []string
}

func (b *backend) Settings(context.Context) ([]am.Setting, error) {
	return slices.Clone(b.settings), b.err
}

func (b *backend) SetSetting(_ context.Context, key, value string) error {
	b.calls = append(b.calls, key+"="+value)
	for i := range b.settings {
		if b.settings[i].Key == key {
			b.settings[i].Value = value
		}
	}
	return nil
}

var all = []string{"claude", "opencode", "codex"}

func newBackend(hidden, def string) *backend {
	return &backend{settings: []am.Setting{
		{Key: "default_tool", Value: def, Values: all},
		{Key: "hidden_tools", Value: hidden, Values: all},
	}}
}

func onPath(names ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		if slices.Contains(names, name) {
			return "/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
}

func TestCheck(t *testing.T) {
	c := &Checker{Backend: newBackend("codex,opencode", ""), LookPath: onPath("agent-manager", "tmux")}
	r := c.Check(context.Background())
	want := []Program{{"agent-manager", true}, {"tmux", true}, {"claude", false}}
	if !slices.Equal(r.Programs, want) {
		t.Errorf("programs = %v, want %v", r.Programs, want)
	}
	if !slices.Equal(r.CLIs, all) || !slices.Equal(r.Shown, []string{"claude"}) || r.DefaultTool != "" {
		t.Errorf("report = %+v", r)
	}
}

func TestCheckSettingsError(t *testing.T) {
	c := &Checker{Backend: &backend{err: errors.New("boom")}, LookPath: onPath()}
	r := c.Check(context.Background())
	if r.SettingsError != "boom" || len(r.Programs) != 3 || r.CLIs == nil || r.Shown == nil {
		t.Errorf("report = %+v", r)
	}
}

func TestApply(t *testing.T) {
	tests := []struct {
		name    string
		choice  Choice
		calls   []string
		refused string
	}{
		{"hides the rest", Choice{Shown: []string{"codex", "claude"}, DefaultTool: "codex"}, []string{"hidden_tools=opencode", "default_tool=codex"}, ""},
		{"every CLI", Choice{Shown: all, DefaultTool: "claude"}, []string{"hidden_tools=", "default_tool=claude"}, ""},
		{"none chosen", Choice{DefaultTool: "claude"}, nil, "at least one"},
		{"unknown CLI", Choice{Shown: []string{"claude", "vim"}, DefaultTool: "claude"}, nil, `no CLI "vim"`},
		{"default not shown", Choice{Shown: []string{"claude"}, DefaultTool: "codex"}, nil, "must be one of"},
		{"option-like value", Choice{Shown: []string{"--help"}, DefaultTool: "--help"}, nil, "no CLI"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newBackend("codex", "")
			c := &Checker{Backend: b, LookPath: onPath()}
			r, err := c.Apply(context.Background(), tt.choice)
			if tt.refused != "" {
				var refused *send.Refused
				if !errors.As(err, &refused) || !strings.Contains(refused.Message, tt.refused) {
					t.Fatalf("err = %v, want refusal %q", err, tt.refused)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(b.calls, tt.calls) {
				t.Errorf("calls = %v, want %v", b.calls, tt.calls)
			}
			if tt.refused == "" && (r.DefaultTool != tt.choice.DefaultTool || len(r.Shown) != len(tt.choice.Shown)) {
				t.Errorf("report after = %+v", r)
			}
		})
	}
}

func TestApplySettingsError(t *testing.T) {
	b := &backend{err: errors.New("boom")}
	c := &Checker{Backend: b, LookPath: onPath()}
	if _, err := c.Apply(context.Background(), Choice{Shown: []string{"claude"}, DefaultTool: "claude"}); err == nil || len(b.calls) != 0 {
		t.Errorf("err = %v, calls = %v", err, b.calls)
	}
}
