package am

import (
	"context"
	"os"
)

// Fixture serves a snapshot from a JSON file, re-read on every call so a
// test can change it while the app runs. It never starts processes.
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
