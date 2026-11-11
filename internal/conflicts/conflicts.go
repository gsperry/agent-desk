// Package conflicts finds files that two live sessions both wrote
// recently, so parallel agents sharing a checkout are caught before
// their edits collide.
package conflicts

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/locate"
	"github.com/gsperry/agent-desk/internal/transcript"
)

// Conflict is a file a session wrote that other live sessions also wrote.
type Conflict struct {
	Path   string   `json:"path"`
	Others []string `json:"others"`
}

type Sessions interface {
	State() hub.State
}

type Finder struct {
	Sessions Sessions
	Locator  *locate.Locator
	// Window is how far back writes count. Zero means six hours.
	Window time.Duration
	Now    func() time.Time

	mu    sync.Mutex
	cache map[string]cachedWrites
}

type cachedWrites struct {
	size    int64
	modTime time.Time
	writes  []write
}

type write struct {
	path string
	at   time.Time
}

// tailBytes is how much of each transcript is scanned for writes.
const tailBytes = 4 << 20

var writeTools = map[string]bool{"Edit": true, "Write": true, "MultiEdit": true, "NotebookEdit": true}

// Find maps each live session id to the files it shares with others.
func (f *Finder) Find(ctx context.Context) map[string][]Conflict {
	window := f.Window
	if window == 0 {
		window = 6 * time.Hour
	}
	now := time.Now
	if f.Now != nil {
		now = f.Now
	}
	since := now().Add(-window)
	byPath := map[string]map[string]bool{}
	for _, s := range f.Sessions.State().Sessions {
		if !s.Running || s.Archived || s.Tool != "claude" || s.AgentSessionID == "" {
			continue
		}
		path, err := f.Locator.Find(ctx, s.AgentSessionID, s.Environment)
		if err != nil {
			continue
		}
		for _, w := range f.writes(path) {
			if w.at.Before(since) {
				continue
			}
			if byPath[w.path] == nil {
				byPath[w.path] = map[string]bool{}
			}
			byPath[w.path][s.ID] = true
		}
	}
	out := map[string][]Conflict{}
	for path, ids := range byPath {
		if len(ids) < 2 {
			continue
		}
		for id := range ids {
			var others []string
			for o := range ids {
				if o != id {
					others = append(others, o)
				}
			}
			sort.Strings(others)
			out[id] = append(out[id], Conflict{Path: path, Others: others})
		}
	}
	for id := range out {
		sort.Slice(out[id], func(i, j int) bool { return out[id][i].Path < out[id][j].Path })
	}
	return out
}

func (f *Finder) writes(path string) []write {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	f.mu.Lock()
	c, ok := f.cache[path]
	f.mu.Unlock()
	if ok && c.size == info.Size() && c.modTime.Equal(info.ModTime()) {
		return c.writes
	}
	ws := scan(path)
	f.mu.Lock()
	if f.cache == nil {
		f.cache = map[string]cachedWrites{}
	}
	f.cache[path] = cachedWrites{size: info.Size(), modTime: info.ModTime(), writes: ws}
	f.mu.Unlock()
	return ws
}

func scan(path string) []write {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil
	}
	start := max(info.Size()-tailBytes, 0)
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return nil
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	if start > 0 && len(lines) > 0 {
		lines = lines[1:]
	}
	p := transcript.NewParser()
	var ws []write
	for _, l := range lines {
		if !strings.Contains(l, `"tool_use"`) {
			continue
		}
		for _, it := range p.Line([]byte(l)) {
			if it.Kind != transcript.KindToolUse || it.Tool == nil || !writeTools[it.Tool.Name] {
				continue
			}
			var in struct {
				FilePath     string `json:"file_path"`
				NotebookPath string `json:"notebook_path"`
			}
			_ = json.Unmarshal(it.Tool.Input, &in)
			target := in.FilePath
			if target == "" {
				target = in.NotebookPath
			}
			if !filepath.IsAbs(target) {
				continue
			}
			at, _ := time.Parse(time.RFC3339Nano, it.Time)
			ws = append(ws, write{path: filepath.Clean(target), at: at})
		}
	}
	return ws
}
