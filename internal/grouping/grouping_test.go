package grouping

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gsperry/agent-desk/internal/am"
	"github.com/gsperry/agent-desk/internal/run"
)

type gitStub struct {
	answers map[string]string
	calls   int
}

func (g *gitStub) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	g.calls++
	if out, ok := g.answers[args[1]]; ok {
		return []byte(out + "\n"), nil
	}
	return nil, &run.ExitError{Name: "git", Code: 128, Stderr: "fatal: not a git repository"}
}

func TestResolve(t *testing.T) {
	stub := &gitStub{answers: map[string]string{
		"/src/api":             "/src/api/.git",
		"/src/api/internal":    "/src/api/.git",
		"/src/api-worktrees/x": "/src/api/.git",
		"/srv/bare.git":        "/srv/bare.git",
	}}
	r := &Resolver{Runner: stub, Home: "/home/dev"}
	tests := []struct {
		name    string
		session am.Session
		want    Project
	}{
		{"group wins", am.Session{Group: "backend/api", Directory: "/src/api"}, Project{Key: "group:backend/api", Name: "backend/api", Kind: KindGroup}},
		{"repo root", am.Session{Directory: "/src/api"}, Project{Key: "repo:/src/api", Name: "api", Root: "/src/api", Kind: KindRepo}},
		{"repo subdirectory", am.Session{Directory: "/src/api/internal"}, Project{Key: "repo:/src/api", Name: "api", Root: "/src/api", Kind: KindRepo}},
		{"worktree files under its repo", am.Session{Directory: "/src/api-worktrees/x"}, Project{Key: "repo:/src/api", Name: "api", Root: "/src/api", Kind: KindRepo}},
		{"bare repo", am.Session{Directory: "/srv/bare.git"}, Project{Key: "repo:/srv/bare.git", Name: "bare", Root: "/srv/bare.git", Kind: KindRepo}},
		{"not a repo", am.Session{Directory: "/tmp/scratch"}, Project{Key: "dir:/tmp/scratch", Name: "scratch", Root: "/tmp/scratch", Kind: KindDir}},
		{"home directory", am.Session{Directory: "/home/dev/"}, Project{Key: "dir:/home/dev", Name: "~", Root: "/home/dev", Kind: KindDir}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.Resolve(context.Background(), tt.session); got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestResolveCaches(t *testing.T) {
	stub := &gitStub{answers: map[string]string{"/src/api": "/src/api/.git"}}
	r := &Resolver{Runner: stub}
	for range 3 {
		r.Resolve(context.Background(), am.Session{Directory: "/src/api"})
		r.Resolve(context.Background(), am.Session{Directory: "/tmp/x"})
	}
	if stub.calls != 2 {
		t.Fatalf("git calls = %d, want 2", stub.calls)
	}
}

type cancelled struct{}

func (cancelled) Run(context.Context, string, ...string) ([]byte, error) {
	return nil, errors.New("killed")
}

func TestCancelledLookupIsNotCached(t *testing.T) {
	r := &Resolver{Runner: cancelled{}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.Resolve(ctx, am.Session{Directory: "/src/api"})
	if len(r.cache) != 0 {
		t.Fatal("cancelled lookup was cached")
	}
}

func TestResolveRealWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(base, "proj")
	worktree := filepath.Join(base, "proj-worktrees", "feature")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "init.defaultBranch=main"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	git("-C", repo, "init", "-q")
	git("-C", repo, "commit", "-q", "--allow-empty", "-m", "init")
	git("-C", repo, "worktree", "add", "-q", "-b", "feature", worktree)

	r := &Resolver{Runner: run.Exec{}}
	for _, dir := range []string{repo, worktree} {
		got := r.Resolve(context.Background(), am.Session{Directory: dir})
		if got.Key != "repo:"+repo || got.Name != "proj" {
			t.Fatalf("%s: got %+v", dir, got)
		}
	}
}
