package hub

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/gsperry/agent-desk/internal/agents"
	"github.com/gsperry/agent-desk/internal/locate"
	"github.com/gsperry/agent-desk/internal/tail"
	"github.com/gsperry/agent-desk/internal/transcript"
)

// DocStatus says why a transcript has or lacks content.
type DocStatus string

const (
	DocOK DocStatus = "ok"
	// DocPending is a Claude session whose transcript does not exist yet;
	// Claude writes it after the first turn.
	DocPending DocStatus = "pending"
	// DocGone is a transcript that existed and was deleted.
	DocGone DocStatus = "gone"
	// DocUnsupported is a session whose tool is not Claude Code.
	DocUnsupported DocStatus = "unsupported"
	DocUnknown     DocStatus = "unknown_session"
)

// Page is a window of a transcript, newest last.
type Page struct {
	SessionID string            `json:"sessionId"`
	Status    DocStatus         `json:"status"`
	Path      string            `json:"path,omitempty"`
	Items     []transcript.Item `json:"items"`
	HasMore   bool              `json:"hasMore"`
	Stats     transcript.Stats  `json:"stats"`
}

// Event is a change to an open transcript. Reset means the client should
// replace what it has with Page; otherwise Items are appended.
type Event struct {
	SessionID string
	Reset     bool
	Page      Page
	Items     []transcript.Item
}

type sessionLookup interface {
	State() State
	Subscribe() (<-chan State, func())
}

// Transcripts owns the open transcripts: one parse per file, then
// incremental reads as Claude appends.
type Transcripts struct {
	Locator  *locate.Locator
	Sessions sessionLookup
	// Interval is the fallback refresh when no file event arrives.
	Interval time.Duration
	// Idle closes transcripts nobody has subscribed to for this long.
	Idle time.Duration

	mu      sync.Mutex
	docs    map[string]*doc
	watcher *fsnotify.Watcher
	watched map[string]int
}

type doc struct {
	mu         sync.Mutex
	sessionID  string
	convID     string
	status     DocStatus
	path       string
	follower   *tail.Follower
	parser     *transcript.Parser
	items      []transcript.Item
	subs       map[chan Event]struct{}
	lastActive time.Time
}

const defaultWindow = 300

func (t *Transcripts) init() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.docs == nil {
		t.docs = map[string]*doc{}
		t.watched = map[string]int{}
	}
}

// Window returns up to limit items older than before (a Seq), or the newest
// items when before is negative.
func (t *Transcripts) Window(ctx context.Context, sessionID string, before, limit int) Page {
	d := t.open(ctx, sessionID)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastActive = time.Now()
	return d.window(before, limit)
}

func (d *doc) window(before, limit int) Page {
	if limit <= 0 {
		limit = defaultWindow
	}
	end := len(d.items)
	if before >= 0 {
		end = sort.Search(len(d.items), func(i int) bool { return d.items[i].Seq >= before })
	}
	start := max(end-limit, 0)
	page := Page{SessionID: d.sessionID, Status: d.status, Path: d.path, HasMore: start > 0, Items: append([]transcript.Item{}, d.items[start:end]...)}
	if d.parser != nil {
		page.Stats = d.parser.Stats()
	}
	return page
}

// Around returns up to radius items on each side of the first item with
// uuid, for showing a search hit in context. Items is empty when the uuid
// is not in the transcript.
func (t *Transcripts) Around(ctx context.Context, sessionID, uuid string, radius int) Page {
	d := t.open(ctx, sessionID)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastActive = time.Now()
	page := Page{SessionID: d.sessionID, Status: d.status, Path: d.path, Items: []transcript.Item{}}
	for i, it := range d.items {
		if it.UUID != uuid {
			continue
		}
		start, end := max(i-radius, 0), min(i+radius+1, len(d.items))
		page.Items = append(page.Items, d.items[start:end]...)
		page.HasMore = start > 0
		break
	}
	return page
}

// Agents lists the subagents the session's conversation spawned.
func (t *Transcripts) Agents(ctx context.Context, sessionID string) ([]agents.Agent, error) {
	d := t.open(ctx, sessionID)
	d.mu.Lock()
	path := d.path
	items := append([]transcript.Item(nil), d.items...)
	d.lastActive = time.Now()
	d.mu.Unlock()
	if path == "" {
		return []agents.Agent{}, nil
	}
	return agents.List(path, items)
}

// AgentTranscript reads one subagent's transcript.
func (t *Transcripts) AgentTranscript(ctx context.Context, sessionID, agentID string) ([]transcript.Item, error) {
	d := t.open(ctx, sessionID)
	d.mu.Lock()
	path := d.path
	d.mu.Unlock()
	if path == "" {
		return nil, agents.ErrNotFound
	}
	return agents.Transcript(path, agentID)
}

// Subscribe opens the transcript and returns a channel of changes after the
// returned first page.
func (t *Transcripts) Subscribe(ctx context.Context, sessionID string) (Page, <-chan Event, func()) {
	d := t.open(ctx, sessionID)
	ch := make(chan Event, 64)
	d.mu.Lock()
	d.subs[ch] = struct{}{}
	d.lastActive = time.Now()
	page := d.window(-1, defaultWindow)
	d.mu.Unlock()
	return page, ch, func() {
		d.mu.Lock()
		delete(d.subs, ch)
		d.lastActive = time.Now()
		d.mu.Unlock()
	}
}

func (t *Transcripts) open(ctx context.Context, sessionID string) *doc {
	t.init()
	t.mu.Lock()
	d, ok := t.docs[sessionID]
	if !ok {
		d = &doc{sessionID: sessionID, subs: map[chan Event]struct{}{}, lastActive: time.Now()}
		t.docs[sessionID] = d
	}
	t.mu.Unlock()
	if !ok {
		d.mu.Lock()
		t.resolve(ctx, d, t.session(sessionID))
		d.mu.Unlock()
	}
	return d
}

func (t *Transcripts) session(id string) *SessionView {
	for _, s := range t.Sessions.State().Sessions {
		if s.ID == id {
			return &s
		}
	}
	return nil
}

// resolve points a doc at its session's current conversation and reads it
// from the start. The caller holds d.mu.
func (t *Transcripts) resolve(ctx context.Context, d *doc, s *SessionView) {
	if d.path != "" {
		t.unwatch(d.path)
	}
	d.items, d.follower, d.path = nil, nil, ""
	d.parser = transcript.NewParser()
	switch {
	case s == nil:
		d.status, d.convID = DocUnknown, ""
		return
	case s.Tool != "claude":
		d.status, d.convID = DocUnsupported, s.AgentSessionID
		return
	}
	d.convID = s.AgentSessionID
	path, err := t.Locator.Find(ctx, s.AgentSessionID, s.Environment)
	if err != nil {
		d.status = DocPending
		return
	}
	d.path, d.status = path, DocOK
	d.follower = tail.New(path)
	t.watch(path)
	d.readLocked()
}

// readLocked appends new lines and reports what changed. reset is true
// when the file was replaced or truncated and the doc was rebuilt.
func (d *doc) readLocked() (added []transcript.Item, reset bool) {
	if d.follower == nil {
		return nil, false
	}
	lines, truncated, err := d.follower.Read()
	if errors.Is(err, tail.ErrGone) {
		changed := d.status != DocGone
		d.status = DocGone
		return nil, changed
	}
	if err != nil {
		return nil, false
	}
	if d.status == DocGone {
		reset = true
	}
	d.status = DocOK
	if truncated {
		d.items, d.parser, reset = nil, transcript.NewParser(), true
	}
	for _, line := range lines {
		added = append(added, d.parser.Line(line)...)
	}
	d.items = append(d.items, added...)
	return added, reset
}

// refresh reads one doc and notifies its subscribers.
func (t *Transcripts) refresh(ctx context.Context, d *doc) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := t.session(d.sessionID)
	var added []transcript.Item
	reset := false
	switch {
	case s != nil && s.AgentSessionID != d.convID, d.status == DocPending, d.status == DocUnknown && s != nil:
		before := d.status
		t.resolve(ctx, d, s)
		reset = d.status != before || d.status == DocOK
	default:
		added, reset = d.readLocked()
	}
	if !reset && len(added) == 0 {
		return
	}
	ev := Event{SessionID: d.sessionID, Reset: reset, Items: added}
	if reset {
		ev.Page = d.window(-1, defaultWindow)
	}
	for ch := range d.subs {
		select {
		case ch <- ev:
		default:
			// A subscriber that fell behind gets a fresh page instead of a gap.
			drain(ch)
			ch <- Event{SessionID: d.sessionID, Reset: true, Page: d.window(-1, defaultWindow)}
		}
	}
}

func drain(ch chan Event) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func (t *Transcripts) watch(path string) {
	dir := filepath.Dir(path)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.watcher == nil {
		return
	}
	if t.watched[dir] == 0 {
		if err := t.watcher.Add(dir); err != nil {
			return
		}
	}
	t.watched[dir]++
}

func (t *Transcripts) unwatch(path string) {
	dir := filepath.Dir(path)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.watched[dir] == 0 {
		return
	}
	if t.watched[dir]--; t.watched[dir] == 0 && t.watcher != nil {
		_ = t.watcher.Remove(dir)
	}
}

func (t *Transcripts) snapshot() []*doc {
	t.mu.Lock()
	defer t.mu.Unlock()
	docs := make([]*doc, 0, len(t.docs))
	for _, d := range t.docs {
		docs = append(docs, d)
	}
	return docs
}

// Run refreshes open transcripts on file events, on session changes and on
// a fallback interval, and closes idle ones.
func (t *Transcripts) Run(ctx context.Context) {
	t.init()
	if w, err := fsnotify.NewWatcher(); err == nil {
		t.mu.Lock()
		t.watcher = w
		t.mu.Unlock()
		defer w.Close()
	}
	interval, idle := t.Interval, t.Idle
	if interval == 0 {
		interval = time.Second
	}
	if idle == 0 {
		idle = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	sessions, unsubscribe := t.Sessions.Subscribe()
	defer unsubscribe()
	var events chan fsnotify.Event
	if t.watcher != nil {
		events = t.watcher.Events
	}
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-events:
			for _, d := range t.snapshot() {
				d.mu.Lock()
				match := d.path == ev.Name || (d.status == DocPending && filepath.Ext(ev.Name) == ".jsonl")
				d.mu.Unlock()
				if match {
					t.refresh(ctx, d)
				}
			}
		case <-sessions:
			for _, d := range t.snapshot() {
				t.refresh(ctx, d)
			}
		case <-ticker.C:
			for _, d := range t.snapshot() {
				t.refresh(ctx, d)
			}
			t.evict(idle)
		}
	}
}

// evict closes idle transcripts. It never holds t.mu and a doc's lock at
// once in that order, since refresh takes them the other way round.
func (t *Transcripts) evict(idle time.Duration) {
	for _, d := range t.snapshot() {
		d.mu.Lock()
		stale := len(d.subs) == 0 && time.Since(d.lastActive) > idle
		path := d.path
		d.mu.Unlock()
		if !stale {
			continue
		}
		t.mu.Lock()
		if t.docs[d.sessionID] == d {
			delete(t.docs, d.sessionID)
		}
		t.mu.Unlock()
		if path != "" {
			t.unwatch(path)
		}
	}
}
