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
	"time"
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
	found    map[string]cached
	profiles map[string]cached
	// now is the clock; tests set it.
	now func() time.Time
}

// cached is a path or root, trusted until a time.
type cached struct {
	value string
	until time.Time
}

const (
	// recheck is how long a found transcript is trusted before the
	// project directories are searched again, in case the conversation
	// has since appeared under a second one.
	recheck = 30 * time.Second
	// profileTTL is how long a profile's config directory is kept, and
	// profileRetry how long a failed lookup waits before it is tried
	// again, so a broken profile does not run agent-manager every poll.
	profileTTL   = 10 * time.Minute
	profileRetry = time.Minute
)

func (l *Locator) clock() time.Time {
	if l.now != nil {
		return l.now()
	}
	return time.Now()
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
	now := l.clock()
	l.mu.Lock()
	hit, ok := l.found[id]
	l.mu.Unlock()
	if ok && now.Before(hit.until) {
		if _, err := os.Stat(hit.value); err == nil {
			return hit.value, nil
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
		l.found = map[string]cached{}
	}
	l.found[id] = cached{path, now.Add(recheck)}
	l.mu.Unlock()
	return path, nil
}

func (l *Locator) root(ctx context.Context, environment string) string {
	if environment == "" || l.Profile == nil {
		return l.DefaultRoot
	}
	now := l.clock()
	l.mu.Lock()
	hit, ok := l.profiles[environment]
	l.mu.Unlock()
	if ok && now.Before(hit.until) {
		return hit.value
	}
	root := l.DefaultRoot
	until := now.Add(profileTTL)
	v, err := l.Profile(ctx, environment, "CLAUDE_CONFIG_DIR")
	switch {
	case err != nil && ctx.Err() != nil:
		// A cancelled lookup says nothing about the profile.
		return root
	case err != nil:
		until = now.Add(profileRetry)
	case v != "":
		root = v
	}
	l.mu.Lock()
	if l.profiles == nil {
		l.profiles = map[string]cached{}
	}
	l.profiles[environment] = cached{root, until}
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
