// Package locate finds the Claude Code transcript for a conversation id.
//
// Transcripts live at <config>/projects/<encoded cwd>/<id>.jsonl. The cwd
// encoding is lossy and depends on symlinks, so the file is found by id
// across every project directory instead of re-encoding the cwd.
package locate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

var ErrNotFound = errors.New("transcript not found")

var validID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,127}$`)

// ProfileVar reads a variable from an agent-manager environment profile.
type ProfileVar func(ctx context.Context, profile, key string) (string, error)

type Locator struct {
	// DefaultRoot is the Claude config directory sessions use unless their
	// environment profile sets CLAUDE_CONFIG_DIR.
	DefaultRoot string
	Profile     ProfileVar

	mu       sync.Mutex
	found    map[string]string
	profiles map[string]string
}

// DefaultRoot is $CLAUDE_CONFIG_DIR, else ~/.claude.
func DefaultRoot() (string, error) {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude"), nil
}

// Find returns the transcript path for a conversation, or ErrNotFound when
// Claude has not written it yet (it appears after the first turn).
func (l *Locator) Find(ctx context.Context, id, environment string) (string, error) {
	if !validID.MatchString(id) {
		return "", ErrNotFound
	}
	l.mu.Lock()
	cached, ok := l.found[id]
	l.mu.Unlock()
	if ok {
		if _, err := os.Stat(cached); err == nil {
			return cached, nil
		}
	}
	matches, err := filepath.Glob(filepath.Join(l.root(ctx, environment), "projects", "*", id+".jsonl"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", ErrNotFound
	}
	path := newest(matches)
	l.mu.Lock()
	if l.found == nil {
		l.found = map[string]string{}
	}
	l.found[id] = path
	l.mu.Unlock()
	return path, nil
}

func (l *Locator) root(ctx context.Context, environment string) string {
	if environment == "" || l.Profile == nil {
		return l.DefaultRoot
	}
	l.mu.Lock()
	root, ok := l.profiles[environment]
	l.mu.Unlock()
	if ok {
		return root
	}
	root = l.DefaultRoot
	v, err := l.Profile(ctx, environment, "CLAUDE_CONFIG_DIR")
	if err != nil {
		return root
	}
	if v != "" {
		root = v
	}
	l.mu.Lock()
	if l.profiles == nil {
		l.profiles = map[string]string{}
	}
	l.profiles[environment] = root
	l.mu.Unlock()
	return root
}

// newest picks the most recently written file when one id appears under two
// project directories, which happens when a cwd is reached by two paths.
func newest(paths []string) string {
	best, bestTime := paths[0], int64(-1)
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && info.ModTime().UnixNano() > bestTime {
			best, bestTime = p, info.ModTime().UnixNano()
		}
	}
	return best
}
