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

func (f Fixture) Snapshot(context.Context) (Snapshot, error) {
	data, err := os.ReadFile(f.Path)
	if err != nil {
		return Snapshot{}, err
	}
	return ParseSnapshot(data)
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
