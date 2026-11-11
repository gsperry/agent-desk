// Package manage starts, stops and files agent sessions through
// agent-manager, checking each request against the current state first.
package manage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/gsperry/agent-desk/internal/am"
	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/send"
)

type Backend interface {
	Spawn(ctx context.Context, r am.SpawnRequest) (am.Session, error)
	Kill(ctx context.Context, id string) error
	Revive(ctx context.Context, id string) error
	Archive(ctx context.Context, id string, restore bool) error
	Move(ctx context.Context, id, group string) error
	CreateGroup(ctx context.Context, path, directory string) error
	DeleteGroup(ctx context.Context, path string) error
	Settings(ctx context.Context) ([]am.Setting, error)
}

type Sessions interface {
	State() hub.State
	Trigger()
}

type Manager struct {
	Sessions Sessions
	Backend  Backend
}

// Options is what the new session form offers.
type Options struct {
	Tools           []string        `json:"tools"`
	DefaultTool     string          `json:"defaultTool"`
	WorktreeDefault bool            `json:"worktreeDefault"`
	Groups          []hub.GroupView `json:"groups"`
}

const reasonInvalid send.Reason = "invalid"

func refuse(format string, args ...any) error {
	return &send.Refused{Reason: reasonInvalid, Message: fmt.Sprintf(format, args...)}
}

func (m *Manager) Options(ctx context.Context) (Options, error) {
	settings, err := m.Backend.Settings(ctx)
	if err != nil {
		return Options{}, err
	}
	tools, def, wt := am.OfferedTools(settings)
	return Options{Tools: tools, DefaultTool: def, WorktreeDefault: wt, Groups: m.Sessions.State().Groups}, nil
}

const (
	maxName   = 80
	maxPrompt = 64 << 10
	maxValue  = 100
)

var groupPath = regexp.MustCompile(`^[\pL\pN _.-]+(/[\pL\pN _.-]+)*$`)

func hasControl(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\t' })
}

// Spawn validates a new session and starts it.
func (m *Manager) Spawn(ctx context.Context, r am.SpawnRequest) (am.Session, error) {
	opts, err := m.Options(ctx)
	if err != nil {
		return am.Session{}, err
	}
	r.Name = strings.TrimSpace(r.Name)
	r.Model = strings.TrimSpace(r.Model)
	r.Effort = strings.TrimSpace(r.Effort)
	r.Prompt = strings.TrimSpace(strings.ReplaceAll(r.Prompt, "\r\n", "\n"))
	switch {
	case r.Tool == "":
		r.Tool = opts.DefaultTool
	case !slices.Contains(opts.Tools, r.Tool):
		return am.Session{}, refuse("%s is not one of the CLIs agent-manager offers (%s).", r.Tool, strings.Join(opts.Tools, ", "))
	}
	if len(r.Name) > maxName || strings.ContainsFunc(r.Name, unicode.IsControl) {
		return am.Session{}, refuse("The name must be at most %d characters with no control characters.", maxName)
	}
	for _, v := range []string{r.Model, r.Effort} {
		if len(v) > maxValue || strings.ContainsFunc(v, unicode.IsSpace) {
			return am.Session{}, refuse("Model and effort are single words.")
		}
	}
	if len(r.Prompt) > maxPrompt || hasControl(r.Prompt) {
		return am.Session{}, refuse("The first prompt is too long or holds control characters.")
	}
	dir, err := existingDir(r.Directory)
	if err != nil {
		return am.Session{}, err
	}
	r.Directory = dir
	if r.Group != "" && !hasGroup(opts.Groups, r.Group) {
		return am.Session{}, refuse("Group %q does not exist.", r.Group)
	}
	s, err := m.Backend.Spawn(ctx, r)
	m.Sessions.Trigger()
	return s, err
}

func existingDir(p string) (string, error) {
	p = strings.TrimSpace(p)
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	if !filepath.IsAbs(p) {
		return "", refuse("Choose a directory with a full path.")
	}
	p = filepath.Clean(p)
	info, err := os.Stat(p)
	if err != nil || !info.IsDir() {
		return "", refuse("%s is not a directory.", p)
	}
	return p, nil
}

func hasGroup(groups []hub.GroupView, path string) bool {
	for _, g := range groups {
		if g.Path == path {
			return true
		}
	}
	return false
}

// Action is a change to an existing session.
type Action struct {
	Action string `json:"action"`
	Group  string `json:"group"`
}

func (m *Manager) Act(ctx context.Context, id string, a Action) error {
	st := m.Sessions.State()
	var s *hub.SessionView
	for i := range st.Sessions {
		if st.Sessions[i].ID == id {
			s = &st.Sessions[i]
		}
	}
	if s == nil {
		return &send.Refused{Reason: send.ReasonUnknownSession, Message: "This session no longer exists in agent-manager."}
	}
	var err error
	switch a.Action {
	case "kill":
		if !s.Running {
			return refuse("%s is not running.", s.Name)
		}
		err = m.Backend.Kill(ctx, id)
	case "revive":
		if s.Running {
			return refuse("%s is already running.", s.Name)
		}
		err = m.Backend.Revive(ctx, id)
	case "archive":
		if s.Archived {
			return refuse("%s is already archived.", s.Name)
		}
		err = m.Backend.Archive(ctx, id, false)
	case "restore":
		if !s.Archived {
			return refuse("%s is not archived.", s.Name)
		}
		err = m.Backend.Archive(ctx, id, true)
	case "move":
		if a.Group != "" && !hasGroup(st.Groups, a.Group) {
			return refuse("Group %q does not exist.", a.Group)
		}
		if a.Group == s.Group {
			return refuse("%s is already in that group.", s.Name)
		}
		err = m.Backend.Move(ctx, id, a.Group)
	default:
		return refuse("Unknown action %q.", a.Action)
	}
	m.Sessions.Trigger()
	return err
}

// Group is a group to create or delete.
type Group struct {
	Path      string `json:"path"`
	Directory string `json:"directory"`
}

func (m *Manager) CreateGroup(ctx context.Context, g Group) error {
	g.Path = strings.Trim(strings.TrimSpace(g.Path), "/")
	if len(g.Path) > maxName || !groupPath.MatchString(g.Path) {
		return refuse("Group names use letters, digits, spaces, dots, dashes and underscores, with / for nesting.")
	}
	if hasGroup(m.Sessions.State().Groups, g.Path) {
		return refuse("Group %q already exists.", g.Path)
	}
	if g.Directory != "" {
		dir, err := existingDir(g.Directory)
		if err != nil {
			return err
		}
		g.Directory = dir
	}
	err := m.Backend.CreateGroup(ctx, g.Path, g.Directory)
	m.Sessions.Trigger()
	return err
}

func (m *Manager) DeleteGroup(ctx context.Context, path string) error {
	if !hasGroup(m.Sessions.State().Groups, path) {
		return refuse("Group %q does not exist.", path)
	}
	err := m.Backend.DeleteGroup(ctx, path)
	m.Sessions.Trigger()
	return err
}
