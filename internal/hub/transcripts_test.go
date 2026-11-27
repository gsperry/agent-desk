package hub

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gsperry/agent-desk/internal/locate"
	"github.com/gsperry/agent-desk/internal/transcript"
)

type staticSessions struct {
	mu sync.Mutex
	st State
	ch chan State
}

func (s *staticSessions) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.st
}

func (s *staticSessions) Subscribe() (<-chan State, func()) { return s.ch, func() {} }

func (s *staticSessions) set(views ...SessionView) {
	s.mu.Lock()
	s.st = State{Sessions: views}
	s.mu.Unlock()
}

const (
	convA = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	convB = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

func userLine(uuid, text string) string {
	return `{"type":"user","uuid":"` + uuid + `","origin":{"kind":"human"},"message":{"content":"` + text + `"}}` + "\n"
}

func appendTo(t *testing.T, path, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

func next(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(3 * time.Second):
		t.Fatal("no transcript event")
		return Event{}
	}
}

func texts(items []transcript.Item) string {
	var out []string
	for _, it := range items {
		out = append(out, it.Text)
	}
	return strings.Join(out, ",")
}

func TestTranscriptLifecycle(t *testing.T) {
	root := t.TempDir()
	pathA := filepath.Join(root, "projects", "-src-api", convA+".jsonl")
	pathB := filepath.Join(root, "projects", "-src-api", convB+".jsonl")
	sessions := &staticSessions{ch: make(chan State, 1)}
	sessions.set(SessionView{ID: "s1", Tool: "claude", AgentSessionID: convA})
	tr := &Transcripts{Locator: &locate.Locator{DefaultRoot: root}, Sessions: sessions, Interval: 20 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tr.Run(ctx)

	page, ch, unsubscribe := tr.Subscribe(ctx, "s1")
	defer unsubscribe()
	if page.Status != DocPending || len(page.Items) != 0 {
		t.Fatalf("before first turn: %+v", page)
	}

	appendTo(t, pathA, userLine("u1", "one"))
	ev := next(t, ch)
	if !ev.Reset || ev.Page.Status != DocOK || texts(ev.Page.Items) != "one" {
		t.Fatalf("file appears: %+v", ev)
	}

	appendTo(t, pathA, userLine("u2", "two")+userLine("u3", "three"))
	ev = next(t, ch)
	if ev.Reset || texts(ev.Items) != "two,three" {
		t.Fatalf("append: %+v", ev)
	}

	appendTo(t, pathB, userLine("v1", "fresh"))
	sessions.set(SessionView{ID: "s1", Tool: "claude", AgentSessionID: convB})
	ev = next(t, ch)
	if !ev.Reset || texts(ev.Page.Items) != "fresh" || ev.Page.Path != pathB {
		t.Fatalf("conversation switch: %+v", ev)
	}

	if err := os.Remove(pathB); err != nil {
		t.Fatal(err)
	}
	ev = next(t, ch)
	if !ev.Reset || ev.Page.Status != DocGone || texts(ev.Page.Items) != "fresh" {
		t.Fatalf("deleted: %+v", ev)
	}
}

func TestWindowPaging(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "projects", "p", convA+".jsonl")
	var b strings.Builder
	for i := range 10 {
		b.WriteString(userLine("u"+string(rune('a'+i)), string(rune('a'+i))))
	}
	appendTo(t, path, b.String())
	sessions := &staticSessions{ch: make(chan State)}
	sessions.set(SessionView{ID: "s1", Tool: "claude", AgentSessionID: convA})
	tr := &Transcripts{Locator: &locate.Locator{DefaultRoot: root}, Sessions: sessions}

	tests := []struct {
		name     string
		before   int
		limit    int
		want     string
		wantMore bool
	}{
		{"newest", -1, 3, "h,i,j", true},
		{"older page", 7, 3, "e,f,g", true},
		{"first page", 2, 5, "a,b", false},
		{"everything", -1, 100, "a,b,c,d,e,f,g,h,i,j", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tr.Window(context.Background(), "s1", tt.before, tt.limit)
			if texts(p.Items) != tt.want || p.HasMore != tt.wantMore {
				t.Fatalf("got %q more=%v, want %q more=%v", texts(p.Items), p.HasMore, tt.want, tt.wantMore)
			}
		})
	}
}

func TestTranscriptStatuses(t *testing.T) {
	sessions := &staticSessions{ch: make(chan State)}
	sessions.set(SessionView{ID: "cx", Tool: "codex", AgentSessionID: "x"})
	tr := &Transcripts{Locator: &locate.Locator{DefaultRoot: t.TempDir()}, Sessions: sessions}
	tests := []struct {
		id   string
		want DocStatus
	}{
		{"cx", DocUnsupported},
		{"missing", DocUnknown},
	}
	for _, tt := range tests {
		if got := tr.Window(context.Background(), tt.id, -1, 10).Status; got != tt.want {
			t.Errorf("%s: status %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestAround(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "projects", "p", convA+".jsonl")
	var b strings.Builder
	for i := range 10 {
		b.WriteString(userLine("u"+string(rune('a'+i)), string(rune('a'+i))))
	}
	appendTo(t, path, b.String())
	sessions := &staticSessions{ch: make(chan State)}
	sessions.set(SessionView{ID: "s1", Tool: "claude", AgentSessionID: convA})
	tr := &Transcripts{Locator: &locate.Locator{DefaultRoot: root}, Sessions: sessions}
	tests := []struct {
		uuid string
		want string
	}{
		{"ue", "c,d,e,f,g"},
		{"ua", "a,b,c"},
		{"uj", "h,i,j"},
		{"missing", ""},
	}
	for _, tt := range tests {
		if got := texts(tr.Around(context.Background(), "s1", tt.uuid, 2).Items); got != tt.want {
			t.Errorf("Around(%s) = %q, want %q", tt.uuid, got, tt.want)
		}
	}
}

// Callers that open the same transcript at once all get it read: none gets
// the empty doc before the first caller has read the file.
func TestConcurrentOpenReadsOnce(t *testing.T) {
	root := t.TempDir()
	sessions := &staticSessions{ch: make(chan State)}
	var views []SessionView
	for i := range 50 {
		conv := strings.Replace(convA, "aaaaaaaa", fmt.Sprintf("%08d", i), 1)
		appendTo(t, filepath.Join(root, "projects", "p", conv+".jsonl"), userLine("u1", "hello"))
		views = append(views, SessionView{ID: fmt.Sprintf("s%d", i), Tool: "claude", AgentSessionID: conv})
	}
	sessions.set(views...)
	tr := &Transcripts{Locator: &locate.Locator{DefaultRoot: root}, Sessions: sessions}
	var wg sync.WaitGroup
	empty := make(chan string, 200)
	for _, v := range views {
		for range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				page, _, cancel := tr.Subscribe(context.Background(), v.ID)
				defer cancel()
				if page.Status != DocOK || len(page.Items) != 1 {
					empty <- v.ID
				}
			}()
		}
	}
	wg.Wait()
	close(empty)
	for id := range empty {
		t.Fatalf("%s: a subscriber got an unread page", id)
	}
}

// Reads for ids agent-manager does not list keep nothing.
func TestUnknownIDsAreNotKept(t *testing.T) {
	sessions := &staticSessions{ch: make(chan State)}
	tr := &Transcripts{Locator: &locate.Locator{DefaultRoot: t.TempDir()}, Sessions: sessions}
	for i := range 100 {
		id := fmt.Sprintf("made-up-%d", i)
		tr.Window(context.Background(), id, -1, 10)
		tr.Around(context.Background(), id, "u1", 5)
	}
	tr.mu.Lock()
	n := len(tr.docs)
	tr.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d docs kept for unknown ids", n)
	}
	// A subscriber to a session not listed yet, as right after a spawn,
	// keeps its doc so the session list can fill it in.
	_, _, cancel := tr.Subscribe(context.Background(), "new-session")
	defer cancel()
	tr.mu.Lock()
	_, kept := tr.docs["new-session"]
	tr.mu.Unlock()
	if !kept {
		t.Fatal("a subscriber's doc was not kept")
	}
}

// A subscriber never attaches to a doc that eviction has dropped.
func TestEvictedDocIsReopened(t *testing.T) {
	root := t.TempDir()
	appendTo(t, filepath.Join(root, "projects", "p", convA+".jsonl"), userLine("u1", "hello"))
	sessions := &staticSessions{ch: make(chan State)}
	sessions.set(SessionView{ID: "s1", Tool: "claude", AgentSessionID: convA})
	tr := &Transcripts{Locator: &locate.Locator{DefaultRoot: root}, Sessions: sessions}
	tr.Window(context.Background(), "s1", -1, 10)
	tr.mu.Lock()
	old := tr.docs["s1"]
	tr.mu.Unlock()
	tr.evict(-time.Second)
	_, _, cancel := tr.Subscribe(context.Background(), "s1")
	defer cancel()
	tr.mu.Lock()
	cur := tr.docs["s1"]
	tr.mu.Unlock()
	if cur == old || cur == nil || len(cur.subs) != 1 {
		t.Fatal("subscribed to the evicted doc")
	}
}
