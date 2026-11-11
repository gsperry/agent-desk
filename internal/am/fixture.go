package am

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Fixture serves a snapshot from a JSON file, re-read on every call so a
// test can change it while the app runs. It never starts processes.
//
// Type appends a human prompt to the session's transcript under
// <dir>/claude, the way Claude records a typed prompt, so tests can watch a
// send reconcile. Read serves <dir>/panes/<id>.txt, or an idle prompt.
type Fixture struct {
	Path string
}

// Snapshot expands ${FIXTURES} in session directories to the fixture
// directory, so tests can point a session at a repository they create.
func (f Fixture) Snapshot(context.Context) (Snapshot, error) {
	data, err := os.ReadFile(f.Path)
	if err != nil {
		return Snapshot{}, err
	}
	snap, err := ParseSnapshot(data)
	if err != nil {
		return Snapshot{}, err
	}
	if dir, err := filepath.Abs(filepath.Dir(f.Path)); err == nil {
		for i := range snap.Sessions {
			snap.Sessions[i].Directory = strings.ReplaceAll(snap.Sessions[i].Directory, "${FIXTURES}", dir)
		}
	}
	return snap, nil
}

func (f Fixture) StartServe(context.Context) error { return nil }

func (f Fixture) ProfileVar(context.Context, string, string) (string, error) { return "", nil }

const idleScreen = "────────────────────────────────────────\n❯ \n────────────────────────────────────────\n"

func (f Fixture) Read(_ context.Context, id string) (string, error) {
	data, err := os.ReadFile(filepath.Join(filepath.Dir(f.Path), "panes", filepath.Base(id)+".txt"))
	if errors.Is(err, os.ErrNotExist) {
		return idleScreen, nil
	}
	return string(data), err
}

func (f Fixture) Type(ctx context.Context, id, text string) error {
	snap, err := f.Snapshot(ctx)
	if err != nil {
		return err
	}
	conv := ""
	for _, s := range snap.Sessions {
		if s.ID == id {
			conv = s.AgentSessionID
		}
	}
	if conv == "" {
		return fmt.Errorf("fixture session %s has no conversation", id)
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(f.Path), "claude", "projects", "*", conv+".jsonl"))
	if len(matches) == 0 {
		return fmt.Errorf("fixture transcript for %s not found", id)
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	rec, err := json.Marshal(map[string]any{
		"type":         "user",
		"uuid":         "fixture-" + hex.EncodeToString(b),
		"timestamp":    time.Now().UTC().Format(time.RFC3339Nano),
		"promptSource": "typed",
		"origin":       map[string]string{"kind": "human"},
		"message":      map[string]any{"role": "user", "content": text},
	})
	if err != nil {
		return err
	}
	file, err := os.OpenFile(matches[0], os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(append(rec, '\n'))
	return err
}

var fixtureMu sync.Mutex

// update applies f to the fixture snapshot and writes it back.
func (f Fixture) update(fn func(*Snapshot) error) error {
	fixtureMu.Lock()
	defer fixtureMu.Unlock()
	data, err := os.ReadFile(f.Path)
	if err != nil {
		return err
	}
	snap, err := ParseSnapshot(data)
	if err != nil {
		return err
	}
	if err := fn(&snap); err != nil {
		return err
	}
	out, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(f.Path, append(out, '\n'), 0o600)
}

func (f Fixture) session(snap *Snapshot, id string) (*Session, error) {
	for i := range snap.Sessions {
		if snap.Sessions[i].ID == id {
			return &snap.Sessions[i], nil
		}
	}
	return nil, fmt.Errorf("session %s does not exist", id)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (f Fixture) Spawn(_ context.Context, r SpawnRequest) (Session, error) {
	s := Session{
		ID:             randomHex(4),
		Name:           r.Name,
		Tool:           r.Tool,
		Group:          r.Group,
		Directory:      r.Directory,
		Status:         "idle",
		Running:        true,
		AgentSessionID: fmt.Sprintf("%s-0000-4000-8000-%s", randomHex(4), randomHex(6)),
	}
	if r.Resume != "" {
		s.AgentSessionID = r.Resume
	}
	if s.Name == "" {
		s.Name = s.Tool + "-" + s.ID[:4]
	}
	err := f.update(func(snap *Snapshot) error {
		snap.Sessions = append(snap.Sessions, s)
		return nil
	})
	return s, err
}

func (f Fixture) Kill(_ context.Context, id string) error {
	return f.update(func(snap *Snapshot) error {
		s, err := f.session(snap, id)
		if err == nil {
			s.Status, s.Running = "dead", false
		}
		return err
	})
}

func (f Fixture) Revive(_ context.Context, id string) error {
	return f.update(func(snap *Snapshot) error {
		s, err := f.session(snap, id)
		if err == nil {
			s.Status, s.Running = "idle", true
		}
		return err
	})
}

func (f Fixture) Archive(_ context.Context, id string, restore bool) error {
	return f.update(func(snap *Snapshot) error {
		s, err := f.session(snap, id)
		if err == nil {
			s.Archived = !restore
		}
		return err
	})
}

func (f Fixture) Move(_ context.Context, id, group string) error {
	return f.update(func(snap *Snapshot) error {
		s, err := f.session(snap, id)
		if err == nil {
			s.Group = group
		}
		return err
	})
}

func (f Fixture) CreateGroup(_ context.Context, path, directory string) error {
	return f.update(func(snap *Snapshot) error {
		snap.Groups = append(snap.Groups, Group{Path: path, Directory: directory})
		return nil
	})
}

func (f Fixture) DeleteGroup(_ context.Context, path string) error {
	return f.update(func(snap *Snapshot) error {
		groups := snap.Groups[:0]
		for _, g := range snap.Groups {
			if g.Path != path {
				groups = append(groups, g)
			}
		}
		snap.Groups = groups
		for i := range snap.Sessions {
			if snap.Sessions[i].Group == path {
				snap.Sessions[i].Group = ""
			}
		}
		return nil
	})
}

func (f Fixture) Settings(context.Context) ([]Setting, error) {
	return []Setting{
		{Key: "default_tool", Value: ""},
		{Key: "worktree_default", Value: "off"},
		{Key: "hidden_tools", Value: "codex", Values: []string{"claude", "opencode", "codex"}},
	}, nil
}
