package history

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gsperry/agent-desk/internal/transcript"
)

func write(t *testing.T, root, project, id string, lines []string, mod time.Time) string {
	t.Helper()
	dir := filepath.Join(root, "projects", project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, id+".jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mod, mod); err != nil {
		t.Fatal(err)
	}
	return p
}

const (
	userLine  = `{"type":"user","uuid":"u1","cwd":"/src/app","gitBranch":"feat-x","timestamp":"2026-11-01T10:00:00Z","promptSource":"typed","origin":{"kind":"human"},"message":{"role":"user","content":"Fix the   login\nbug"}}`
	replyLine = `{"type":"assistant","uuid":"a1","timestamp":"2026-11-01T10:00:05Z","message":{"id":"m1","content":[{"type":"text","text":"Done."}]}}`
	aiTitle   = `{"type":"ai-title","aiTitle":"Login bug fix","sessionId":"s"}`
	aiTitle2  = `{"type":"ai-title","aiTitle":"Later AI title","sessionId":"s"}`
	userTitle = `{"type":"custom-title","customTitle":"My title","sessionId":"s"}`
)

func TestList(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	write(t, root, "-src-app", "aaaa-1", []string{userLine, replyLine, aiTitle}, now.Add(-time.Hour))
	write(t, root, "-src-app", "bbbb-2", []string{userLine, aiTitle, userTitle, aiTitle2}, now)
	write(t, root, "-src-app", "cccc-3", []string{`not json`, `{"type":"mystery"}`}, now.Add(-2*time.Hour))
	// A subagent transcript is not a conversation of its own.
	sub := filepath.Join(root, "projects", "-src-app", "aaaa-1", "subagents")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "agent-x.jsonl"), []byte(userLine+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	x := &Index{Root: root}
	got, err := x.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d conversations, want 3: %+v", len(got), got)
	}
	tests := []struct {
		i                          int
		id, title, dir, branch, fp string
	}{
		{0, "bbbb-2", "My title", "/src/app", "feat-x", "Fix the login bug"},
		{1, "aaaa-1", "Login bug fix", "/src/app", "feat-x", "Fix the login bug"},
		{2, "cccc-3", "", "", "", ""},
	}
	for _, tc := range tests {
		c := got[tc.i]
		if c.ID != tc.id || c.Title != tc.title || c.Directory != tc.dir || c.Branch != tc.branch || c.FirstPrompt != tc.fp {
			t.Errorf("conversation %d = %+v, want id %q title %q dir %q branch %q prompt %q", tc.i, c, tc.id, tc.title, tc.dir, tc.branch, tc.fp)
		}
	}
	if got[1].Started != "2026-11-01T10:00:00Z" || got[1].Size == 0 {
		t.Errorf("started/size not read: %+v", got[1])
	}
}

func TestListReadsTitleFromTail(t *testing.T) {
	root := t.TempDir()
	pad := `{"type":"assistant","uuid":"p","timestamp":"2026-11-01T10:00:05Z","message":{"id":"mp","content":[{"type":"text","text":"` + strings.Repeat("x", 1000) + `"}]}}`
	lines := []string{userLine, aiTitle}
	for range 600 {
		lines = append(lines, pad)
	}
	lines = append(lines, aiTitle2)
	write(t, root, "-p", "dddd-4", lines, time.Now())
	got, err := (&Index{Root: root}).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Later AI title" || got[0].FirstPrompt != "Fix the login bug" {
		t.Fatalf("got %+v", got)
	}
}

func TestListCachesBySizeAndTime(t *testing.T) {
	root := t.TempDir()
	mod := time.Now().Add(-time.Minute)
	p := write(t, root, "-p", "eeee-5", []string{userLine, aiTitle}, mod)
	x := &Index{Root: root}
	if _, err := x.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	x.mu.Lock()
	e := x.cache[p]
	e.conv.Title = "cached"
	x.cache[p] = e
	x.mu.Unlock()
	got, _ := x.List(context.Background())
	if got[0].Title != "cached" {
		t.Fatalf("unchanged file was re-read: %q", got[0].Title)
	}
	write(t, root, "-p", "eeee-5", []string{userLine, aiTitle2}, time.Now())
	got, _ = x.List(context.Background())
	if got[0].Title != "Later AI title" {
		t.Fatalf("changed file was not re-read: %q", got[0].Title)
	}
}

func TestRead(t *testing.T) {
	root := t.TempDir()
	lines := []string{userLine}
	for range 5 {
		lines = append(lines, replyLine)
	}
	// Distinct replies, so duplicate detection keeps them all.
	for i := range lines[1:] {
		lines[i+1] = strings.Replace(replyLine, `"a1"`, `"a`+string(rune('2'+i))+`"`, 1)
		lines[i+1] = strings.Replace(lines[i+1], `"m1"`, `"m`+string(rune('2'+i))+`"`, 1)
	}
	write(t, root, "-p", "ffff-6", lines, time.Now())
	x := &Index{Root: root}
	page, err := x.Read(context.Background(), "ffff-6", -1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || !page.HasMore {
		t.Fatalf("newest page: %d items, hasMore %v", len(page.Items), page.HasMore)
	}
	older, err := x.Read(context.Background(), "ffff-6", page.Items[0].Seq, 100)
	if err != nil {
		t.Fatal(err)
	}
	if older.HasMore || len(older.Items) != 4 || older.Items[0].Kind != transcript.KindUser {
		t.Fatalf("older page: %+v", older)
	}
}

func TestReadRejectsBadIDs(t *testing.T) {
	x := &Index{Root: t.TempDir()}
	for _, id := range []string{"", "../x", "a/b", "*", "missing"} {
		if _, err := x.Read(context.Background(), id, -1, 10); !errors.Is(err, ErrNotFound) {
			t.Errorf("Read(%q) err = %v, want ErrNotFound", id, err)
		}
	}
}

func TestExport(t *testing.T) {
	root := t.TempDir()
	write(t, root, "-p", "gggg-7", []string{userLine, replyLine, aiTitle}, time.Now())
	name, body, err := (&Index{Root: root}).Export(context.Background(), "gggg-7")
	if err != nil {
		t.Fatal(err)
	}
	if name != "login-bug-fix.md" {
		t.Errorf("name = %q", name)
	}
	for _, want := range []string{"# Login bug fix", "- Directory: `/src/app`", "Fix the   login", "Done."} {
		if !strings.Contains(body, want) {
			t.Errorf("export lacks %q:\n%s", want, body)
		}
	}
	if _, _, err := (&Index{Root: root}).Export(context.Background(), "../x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("bad id err = %v", err)
	}
}
