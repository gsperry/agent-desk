package manage

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gsperry/agent-desk/internal/am"
	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/send"
)

type sessions struct {
	st       hub.State
	triggers int
}

func (s *sessions) State() hub.State { return s.st }
func (s *sessions) Trigger()         { s.triggers++ }

type backend struct {
	calls []string
	spawn am.SpawnRequest
}

func (b *backend) Spawn(_ context.Context, r am.SpawnRequest) (am.Session, error) {
	b.spawn = r
	b.calls = append(b.calls, "spawn")
	return am.Session{ID: "new1"}, nil
}
func (b *backend) Kill(_ context.Context, id string) error {
	b.calls = append(b.calls, "kill "+id)
	return nil
}
func (b *backend) Revive(_ context.Context, id string) error {
	b.calls = append(b.calls, "revive "+id)
	return nil
}
func (b *backend) Archive(_ context.Context, id string, restore bool) error {
	if restore {
		b.calls = append(b.calls, "restore "+id)
	} else {
		b.calls = append(b.calls, "archive "+id)
	}
	return nil
}
func (b *backend) Move(_ context.Context, id, group string) error {
	b.calls = append(b.calls, "move "+id+" "+group)
	return nil
}
func (b *backend) CreateGroup(_ context.Context, path, dir string) error {
	b.calls = append(b.calls, "create "+path+" "+dir)
	return nil
}
func (b *backend) DeleteGroup(_ context.Context, path string) error {
	b.calls = append(b.calls, "delete "+path)
	return nil
}
func (b *backend) Settings(context.Context) ([]am.Setting, error) {
	return []am.Setting{
		{Key: "default_tool", Value: ""},
		{Key: "hidden_tools", Value: "codex", Values: []string{"claude", "opencode", "codex"}},
	}, nil
}

func setup(t *testing.T) (*Manager, *backend, *sessions, string) {
	dir := t.TempDir()
	ss := &sessions{st: hub.State{
		Sessions: []hub.SessionView{
			{ID: "live", Name: "live", Running: true, Group: "", AgentSessionID: "conv-live"},
			{ID: "dead", Name: "dead", Running: false, AgentSessionID: "conv-dead"},
			{ID: "old", Name: "old", Running: false, Archived: true},
		},
		Groups: []hub.GroupView{{Path: "web"}, {Path: "backend/api"}},
	}}
	b := &backend{}
	return &Manager{Sessions: ss, Backend: b}, b, ss, dir
}

func reason(err error) send.Reason {
	var r *send.Refused
	if errors.As(err, &r) {
		return r.Reason
	}
	return ""
}

func TestSpawn(t *testing.T) {
	m, b, ss, dir := setup(t)
	tests := []struct {
		name    string
		req     am.SpawnRequest
		refused bool
	}{
		{"defaults", am.SpawnRequest{Directory: dir}, false},
		{"model with brackets", am.SpawnRequest{Directory: dir, Model: "opus[1m]", Effort: "xhigh"}, false},
		{"full", am.SpawnRequest{Name: " fix-login ", Tool: "opencode", Group: "backend/api", Directory: dir, Worktree: true, Prompt: "Go\r\n", Model: "opus", Effort: "high"}, false},
		{"resume", am.SpawnRequest{Directory: dir, Resume: " conv-past "}, false},
		{"resume a running conversation", am.SpawnRequest{Directory: dir, Resume: "conv-live"}, true},
		{"resume a dead session's conversation", am.SpawnRequest{Directory: dir, Resume: "conv-dead"}, true},
		{"resume in a worktree", am.SpawnRequest{Directory: dir, Resume: "conv-past", Worktree: true}, true},
		{"resume another CLI", am.SpawnRequest{Directory: dir, Resume: "conv-past", Tool: "opencode"}, true},
		{"resume a bad id", am.SpawnRequest{Directory: dir, Resume: "../x"}, true},
		{"hidden tool", am.SpawnRequest{Tool: "codex", Directory: dir}, true},
		{"relative directory", am.SpawnRequest{Directory: "src"}, true},
		{"missing directory", am.SpawnRequest{Directory: dir + "/nope"}, true},
		{"unknown group", am.SpawnRequest{Directory: dir, Group: "nope"}, true},
		{"long name", am.SpawnRequest{Directory: dir, Name: strings.Repeat("n", 81)}, true},
		{"model with spaces", am.SpawnRequest{Directory: dir, Model: "opus --flag"}, true},
		{"model with shell syntax", am.SpawnRequest{Directory: dir, Model: "$(curl${IFS}x|sh)"}, true},
		{"directory with an escape code", am.SpawnRequest{Directory: dir + "\x1b[2J"}, true},
		{"control characters in prompt", am.SpawnRequest{Directory: dir, Prompt: "a\x1b[201~b"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b.calls = nil
			_, err := m.Spawn(context.Background(), tt.req)
			if tt.refused {
				if reason(err) != reasonInvalid || len(b.calls) != 0 {
					t.Fatalf("err %v calls %v", err, b.calls)
				}
				return
			}
			if err != nil || len(b.calls) != 1 {
				t.Fatalf("err %v calls %v", err, b.calls)
			}
		})
	}
	m.Spawn(context.Background(), am.SpawnRequest{Directory: dir, Resume: " conv-past "})
	if b.spawn.Resume != "conv-past" || b.spawn.Tool != "claude" {
		t.Fatalf("resume spawn = %+v", b.spawn)
	}
	m.Spawn(context.Background(), am.SpawnRequest{Name: " fix-login ", Tool: "opencode", Group: "backend/api", Directory: dir, Worktree: true, Prompt: "Go\r\n", Model: "opus", Effort: "high"})
	if b.spawn.Tool != "opencode" || b.spawn.Name != "fix-login" || b.spawn.Prompt != "Go" || b.spawn.Directory != dir {
		t.Fatalf("spawn = %+v", b.spawn)
	}
	m.Spawn(context.Background(), am.SpawnRequest{Directory: dir})
	if b.spawn.Tool != "claude" {
		t.Fatalf("default tool = %q", b.spawn.Tool)
	}
	if ss.triggers == 0 {
		t.Fatal("no poll triggered")
	}
}

func TestAct(t *testing.T) {
	m, b, _, _ := setup(t)
	tests := []struct {
		id   string
		a    Action
		want string
		why  send.Reason
	}{
		{"live", Action{Action: "kill"}, "kill live", ""},
		{"dead", Action{Action: "kill"}, "", reasonInvalid},
		{"dead", Action{Action: "revive"}, "revive dead", ""},
		{"live", Action{Action: "revive"}, "", reasonInvalid},
		{"live", Action{Action: "archive"}, "archive live", ""},
		{"old", Action{Action: "archive"}, "", reasonInvalid},
		{"old", Action{Action: "restore"}, "restore old", ""},
		{"live", Action{Action: "restore"}, "", reasonInvalid},
		{"live", Action{Action: "move", Group: "web"}, "move live web", ""},
		{"live", Action{Action: "move", Group: ""}, "", reasonInvalid},
		{"live", Action{Action: "move", Group: "nope"}, "", reasonInvalid},
		{"live", Action{Action: "delete"}, "", reasonInvalid},
		{"ghost", Action{Action: "kill"}, "", send.ReasonUnknownSession},
	}
	for _, tt := range tests {
		b.calls = nil
		err := m.Act(context.Background(), tt.id, tt.a)
		if tt.why != "" {
			if reason(err) != tt.why || len(b.calls) != 0 {
				t.Errorf("%s %+v: err %v calls %v", tt.id, tt.a, err, b.calls)
			}
			continue
		}
		if err != nil || strings.Join(b.calls, ",") != tt.want {
			t.Errorf("%s %+v: err %v calls %v, want %s", tt.id, tt.a, err, b.calls, tt.want)
		}
	}
}

func TestGroups(t *testing.T) {
	m, b, _, dir := setup(t)
	tests := []struct {
		name string
		run  func() error
		want string
		bad  bool
	}{
		{"create", func() error { return m.CreateGroup(context.Background(), Group{Path: " /ops/infra/ ", Directory: dir}) }, "create ops/infra " + dir, false},
		{"create without directory", func() error { return m.CreateGroup(context.Background(), Group{Path: "Ops 2"}) }, "create Ops 2 ", false},
		{"duplicate", func() error { return m.CreateGroup(context.Background(), Group{Path: "web"}) }, "", true},
		{"bad characters", func() error { return m.CreateGroup(context.Background(), Group{Path: "a//b"}) }, "", true},
		{"flag-like", func() error { return m.CreateGroup(context.Background(), Group{Path: "--help"}) }, "create --help ", false},
		{"bad directory", func() error { return m.CreateGroup(context.Background(), Group{Path: "x", Directory: "rel"}) }, "", true},
		{"delete", func() error { return m.DeleteGroup(context.Background(), "web") }, "delete web", false},
		{"delete missing", func() error { return m.DeleteGroup(context.Background(), "nope") }, "", true},
	}
	for _, tt := range tests {
		b.calls = nil
		err := tt.run()
		if tt.bad {
			if reason(err) != reasonInvalid || len(b.calls) != 0 {
				t.Errorf("%s: err %v calls %v", tt.name, err, b.calls)
			}
			continue
		}
		if err != nil || strings.Join(b.calls, ",") != tt.want {
			t.Errorf("%s: err %v calls %q, want %q", tt.name, err, b.calls, tt.want)
		}
	}
}

func TestOptions(t *testing.T) {
	m, _, _, _ := setup(t)
	o, err := m.Options(context.Background())
	if err != nil || strings.Join(o.Tools, ",") != "claude,opencode" || o.DefaultTool != "claude" || len(o.Groups) != 2 {
		t.Fatalf("%+v %v", o, err)
	}
}
