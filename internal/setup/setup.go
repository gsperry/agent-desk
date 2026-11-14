// Package setup checks what Agent Desk needs on first run, and changes the
// agent-manager settings the welcome screen offers through agent-manager's
// own settings command.
package setup

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/gsperry/agent-desk/internal/am"
	"github.com/gsperry/agent-desk/internal/send"
)

// Programs are the commands Agent Desk runs or reads the output of.
var Programs = []string{"agent-manager", "tmux", "claude"}

type Program struct {
	Name  string `json:"name"`
	Found bool   `json:"found"`
}

// Report is the welcome screen's checklist.
type Report struct {
	Programs []Program `json:"programs"`
	// CLIs is every CLI agent-manager can run, in its order.
	CLIs []string `json:"clis"`
	// Shown is the CLIs agent-manager's pickers offer.
	Shown []string `json:"shown"`
	// DefaultTool is the stored default CLI; empty when none is stored.
	DefaultTool string `json:"defaultTool"`
	// SettingsError says why agent-manager's settings could not be read.
	SettingsError string `json:"settingsError,omitempty"`
}

// Choice is the CLIs to offer and the default among them.
type Choice struct {
	Shown       []string `json:"shown"`
	DefaultTool string   `json:"defaultTool"`
}

type Backend interface {
	Settings(ctx context.Context) ([]am.Setting, error)
	SetSetting(ctx context.Context, key, value string) error
}

type Checker struct {
	Backend Backend
	// LookPath reports whether a program is on PATH.
	LookPath func(name string) (string, error)
}

func (c *Checker) Check(ctx context.Context) Report {
	r := Report{Programs: make([]Program, 0, len(Programs)), CLIs: []string{}, Shown: []string{}}
	for _, name := range Programs {
		_, err := c.LookPath(name)
		r.Programs = append(r.Programs, Program{Name: name, Found: err == nil})
	}
	settings, err := c.Backend.Settings(ctx)
	if err != nil {
		r.SettingsError = err.Error()
		return r
	}
	hidden := map[string]bool{}
	for _, s := range settings {
		switch s.Key {
		case "hidden_tools":
			r.CLIs = append(r.CLIs, s.Values...)
			for _, t := range strings.Split(s.Value, ",") {
				hidden[strings.TrimSpace(t)] = true
			}
		case "default_tool":
			r.DefaultTool = s.Value
		}
	}
	for _, t := range r.CLIs {
		if !hidden[t] {
			r.Shown = append(r.Shown, t)
		}
	}
	return r
}

// Apply hides every CLI not chosen and stores the default. The default must
// be one of the chosen CLIs, which agent-manager itself does not require.
func (c *Checker) Apply(ctx context.Context, ch Choice) (Report, error) {
	r := c.Check(ctx)
	if r.SettingsError != "" {
		return r, fmt.Errorf("agent-manager settings: %s", r.SettingsError)
	}
	if len(ch.Shown) == 0 {
		return r, refuse("choose at least one CLI")
	}
	chosen := map[string]bool{}
	for _, t := range ch.Shown {
		if !slices.Contains(r.CLIs, t) {
			return r, refuse("agent-manager has no CLI %q", t)
		}
		chosen[t] = true
	}
	if !chosen[ch.DefaultTool] {
		return r, refuse("the default CLI must be one of the CLIs offered")
	}
	var hidden []string
	for _, t := range r.CLIs {
		if !chosen[t] {
			hidden = append(hidden, t)
		}
	}
	if err := c.Backend.SetSetting(ctx, "hidden_tools", strings.Join(hidden, ",")); err != nil {
		return r, err
	}
	if err := c.Backend.SetSetting(ctx, "default_tool", ch.DefaultTool); err != nil {
		return r, err
	}
	return c.Check(ctx), nil
}

func refuse(format string, args ...any) error {
	return &send.Refused{Reason: "invalid", Message: fmt.Sprintf(format, args...)}
}
