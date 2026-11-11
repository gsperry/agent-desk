// Package history lists every Claude Code conversation on disk, not only
// the ones agent-manager runs, and reads any of them for the viewer.
package history

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gsperry/agent-desk/internal/export"
	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/transcript"
)

// Conversation is one transcript file's listing.
type Conversation struct {
	ID          string `json:"id"`
	Directory   string `json:"directory,omitempty"`
	Branch      string `json:"branch,omitempty"`
	Title       string `json:"title,omitempty"`
	FirstPrompt string `json:"firstPrompt,omitempty"`
	Started     string `json:"started,omitempty"`
	Modified    string `json:"modified"`
	Size        int64  `json:"size"`
}

var ErrNotFound = errors.New("conversation not found")

var validID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,127}$`)

// Index lists and reads conversations under Root (a Claude config
// directory). Listings are cached by file size and modification time.
type Index struct {
	Root string

	mu    sync.Mutex
	cache map[string]listed
	docs  []parsed
}

type listed struct {
	size    int64
	modTime time.Time
	conv    Conversation
}

type parsed struct {
	path    string
	size    int64
	modTime time.Time
	items   []transcript.Item
	stats   transcript.Stats
}

const (
	// headBytes and tailBytes bound how much of a file a listing reads: the
	// start for directory and first prompt, the end for the latest title.
	headBytes = 256 << 10
	tailBytes = 256 << 10
	// keptDocs is how many fully parsed conversations stay cached for paging.
	keptDocs     = 2
	defaultLimit = 300
)

// List returns every conversation, newest first.
func (x *Index) List(ctx context.Context) ([]Conversation, error) {
	paths, err := filepath.Glob(filepath.Join(x.Root, "projects", "*", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	out := make([]Conversation, 0, len(paths))
	seen := map[string]bool{}
	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c, err := x.listing(p)
		if err != nil {
			continue
		}
		// One id under two project directories (a cwd reached by two paths)
		// is one conversation; the newest copy wins below.
		if seen[c.ID] {
			for i := range out {
				if out[i].ID == c.ID && out[i].Modified < c.Modified {
					out[i] = c
				}
			}
			continue
		}
		seen[c.ID] = true
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Modified > out[j].Modified })
	return out, nil
}

func (x *Index) listing(path string) (Conversation, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Conversation{}, err
	}
	if !info.Mode().IsRegular() {
		return Conversation{}, ErrNotFound
	}
	x.mu.Lock()
	c, ok := x.cache[path]
	x.mu.Unlock()
	if ok && c.size == info.Size() && c.modTime.Equal(info.ModTime()) {
		return c.conv, nil
	}
	conv, err := describe(path, info.Size())
	if err != nil {
		return Conversation{}, err
	}
	conv.Modified = info.ModTime().UTC().Format(time.RFC3339)
	conv.Size = info.Size()
	x.mu.Lock()
	if x.cache == nil {
		x.cache = map[string]listed{}
	}
	x.cache[path] = listed{size: info.Size(), modTime: info.ModTime(), conv: conv}
	x.mu.Unlock()
	return conv, nil
}

// headFields are the record fields a listing reads from the first lines.
type headFields struct {
	Cwd       string `json:"cwd"`
	GitBranch string `json:"gitBranch"`
	Timestamp string `json:"timestamp"`
}

func describe(path string, size int64) (Conversation, error) {
	f, err := os.Open(path)
	if err != nil {
		return Conversation{}, err
	}
	defer f.Close()
	c := Conversation{ID: strings.TrimSuffix(filepath.Base(path), ".jsonl")}

	head, err := io.ReadAll(io.LimitReader(f, headBytes))
	if err != nil {
		return Conversation{}, err
	}
	p := transcript.NewParser()
	for _, line := range lines(head, false, size > headBytes) {
		var h headFields
		if json.Unmarshal(line, &h) == nil {
			if c.Directory == "" {
				c.Directory = h.Cwd
			}
			if c.Branch == "" {
				c.Branch = h.GitBranch
			}
			if c.Started == "" {
				c.Started = h.Timestamp
			}
		}
		for _, it := range p.Line(line) {
			if c.FirstPrompt == "" && it.Kind == transcript.KindUser && strings.TrimSpace(it.Text) != "" {
				c.FirstPrompt = clip(it.Text, 240)
			}
			title(&c, it)
		}
	}

	if size > headBytes {
		start := max(size-tailBytes, headBytes)
		if _, err := f.Seek(start, io.SeekStart); err != nil {
			return Conversation{}, err
		}
		tail, err := io.ReadAll(f)
		if err != nil {
			return Conversation{}, err
		}
		tp := transcript.NewParser()
		for _, line := range lines(tail, true, false) {
			for _, it := range tp.Line(line) {
				title(&c, it)
			}
		}
	}
	c.Title = strings.TrimPrefix(c.Title, "\x00")
	return c, nil
}

// title keeps the latest title, preferring one the user set. A user title
// is stored with a leading NUL mark until describe finishes.
func title(c *Conversation, it transcript.Item) {
	if it.Kind != transcript.KindMeta {
		return
	}
	switch it.Label {
	case "custom-title":
		if v, ok := it.Meta["customTitle"].(string); ok && strings.TrimSpace(v) != "" {
			c.Title = "\x00" + clip(v, 160)
		}
	case "ai-title":
		if v, ok := it.Meta["aiTitle"].(string); ok && strings.TrimSpace(v) != "" && !strings.HasPrefix(c.Title, "\x00") {
			c.Title = clip(v, 160)
		}
	}
}

// lines splits data into records, dropping a first line cut mid-record and a
// last line cut by the read limit.
func lines(data []byte, cutStart, cutEnd bool) [][]byte {
	parts := bytes.Split(data, []byte("\n"))
	if cutStart && len(parts) > 0 {
		parts = parts[1:]
	}
	if cutEnd && len(parts) > 0 {
		parts = parts[:len(parts)-1]
	}
	out := parts[:0]
	for _, l := range parts {
		if len(bytes.TrimSpace(l)) > 0 {
			out = append(out, l)
		}
	}
	return out
}

// Path returns the transcript file for a conversation id.
func (x *Index) Path(id string) (string, error) {
	if !validID.MatchString(id) {
		return "", ErrNotFound
	}
	matches, err := filepath.Glob(filepath.Join(x.Root, "projects", "*", id+".jsonl"))
	if err != nil || len(matches) == 0 {
		return "", ErrNotFound
	}
	best, bestTime := "", time.Time{}
	for _, m := range matches {
		if info, err := os.Stat(m); err == nil && info.Mode().IsRegular() && info.ModTime().After(bestTime) {
			best, bestTime = m, info.ModTime()
		}
	}
	if best == "" {
		return "", ErrNotFound
	}
	return best, nil
}

// Read returns up to limit items older than before (a Seq), or the newest
// items when before is negative.
func (x *Index) Read(ctx context.Context, id string, before, limit int) (hub.Page, error) {
	path, err := x.Path(id)
	if err != nil {
		return hub.Page{}, err
	}
	d, err := x.parse(ctx, path)
	if err != nil {
		return hub.Page{}, err
	}
	if limit <= 0 {
		limit = defaultLimit
	}
	end := len(d.items)
	if before >= 0 {
		end = sort.Search(len(d.items), func(i int) bool { return d.items[i].Seq >= before })
	}
	start := max(end-limit, 0)
	return hub.Page{
		SessionID: id,
		Status:    hub.DocOK,
		Path:      path,
		Items:     append([]transcript.Item{}, d.items[start:end]...),
		HasMore:   start > 0,
		Stats:     d.stats,
	}, nil
}

// Export renders a whole conversation as Markdown and names the file.
func (x *Index) Export(ctx context.Context, id string) (filename, body string, err error) {
	path, err := x.Path(id)
	if err != nil {
		return "", "", err
	}
	d, err := x.parse(ctx, path)
	if err != nil {
		return "", "", err
	}
	c, err := x.listing(path)
	if err != nil {
		return "", "", err
	}
	title := c.Title
	if title == "" {
		title = c.FirstPrompt
	}
	h := export.Header{Title: title, ID: c.ID, Directory: c.Directory, Branch: c.Branch}
	return export.Filename(title, c.ID), export.Markdown(h, d.items), nil
}

func (x *Index) parse(ctx context.Context, path string) (parsed, error) {
	info, err := os.Stat(path)
	if err != nil {
		return parsed{}, err
	}
	x.mu.Lock()
	for _, d := range x.docs {
		if d.path == path && d.size == info.Size() && d.modTime.Equal(info.ModTime()) {
			x.mu.Unlock()
			return d, nil
		}
	}
	x.mu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		return parsed{}, err
	}
	defer f.Close()
	p := transcript.NewParser()
	var items []transcript.Item
	r := bufio.NewReaderSize(f, 1<<20)
	for {
		if err := ctx.Err(); err != nil {
			return parsed{}, err
		}
		line, err := r.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			items = append(items, p.Line(line)...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return parsed{}, err
		}
	}
	d := parsed{path: path, size: info.Size(), modTime: info.ModTime(), items: items, stats: p.Stats()}
	x.mu.Lock()
	x.docs = append([]parsed{d}, x.docs...)
	if len(x.docs) > keptDocs {
		x.docs = x.docs[:keptDocs]
	}
	x.mu.Unlock()
	return d, nil
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
