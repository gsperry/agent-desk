// Package grouping files sessions under projects for the sidebar.
//
// A session's agent-manager group wins when it has one. Otherwise the
// project is the git repository its directory belongs to, with worktrees
// filed under the repository they were made from. A directory outside git
// is its own project.
package grouping

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gsperry/agent-desk/internal/am"
	"github.com/gsperry/agent-desk/internal/run"
)

type Kind string

const (
	KindGroup Kind = "group"
	KindRepo  Kind = "repo"
	KindDir   Kind = "dir"
)

type Project struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Root string `json:"root,omitempty"`
	Kind Kind   `json:"kind"`
}

type Resolver struct {
	Runner run.Runner
	Home   string

	mu    sync.Mutex
	cache map[string]Project
}

func (r *Resolver) Resolve(ctx context.Context, s am.Session) Project {
	if s.Group != "" {
		return Project{Key: "group:" + s.Group, Name: s.Group, Kind: KindGroup}
	}
	return r.directory(ctx, s.Directory)
}

func (r *Resolver) directory(ctx context.Context, dir string) Project {
	dir = filepath.Clean(dir)
	r.mu.Lock()
	p, ok := r.cache[dir]
	r.mu.Unlock()
	if ok {
		return p
	}
	p, cacheable := r.lookup(ctx, dir)
	if cacheable {
		r.mu.Lock()
		if r.cache == nil {
			r.cache = map[string]Project{}
		}
		r.cache[dir] = p
		r.mu.Unlock()
	}
	return p
}

// lookup asks git for the shared .git directory, which is the same for a
// repository and every worktree made from it.
func (r *Resolver) lookup(ctx context.Context, dir string) (Project, bool) {
	out, err := r.Runner.Run(ctx, "git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		// A directory that is not a repo, or no longer exists, stays a
		// directory; a cancelled lookup is retried on the next poll.
		return r.dirProject(dir), ctx.Err() == nil
	}
	common := filepath.Clean(strings.TrimSpace(string(out)))
	root := common
	if filepath.Base(common) == ".git" {
		root = filepath.Dir(common)
	}
	return Project{Key: "repo:" + root, Name: strings.TrimSuffix(filepath.Base(root), ".git"), Root: root, Kind: KindRepo}, true
}

func (r *Resolver) dirProject(dir string) Project {
	name := filepath.Base(dir)
	if r.Home != "" && dir == filepath.Clean(r.Home) {
		name = "~"
	}
	return Project{Key: "dir:" + dir, Name: name, Root: dir, Kind: KindDir}
}
