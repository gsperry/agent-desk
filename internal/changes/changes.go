// Package changes reads what a session has changed in its git checkout.
//
// Every git call is read-only and runs with external diff tools, text
// converters, the repository's own filter drivers, hooks and the fsmonitor
// hook disabled, and with no inherited GIT_ variables, since a
// repository's config could otherwise make a diff run commands.
// GIT_OPTIONAL_LOCKS=0 keeps git from taking the index lock an agent may
// need.
package changes

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/run"
	"github.com/gsperry/agent-desk/internal/send"
)

type Hunk struct {
	OldStart int      `json:"oldStart"`
	OldLines int      `json:"oldLines"`
	NewStart int      `json:"newStart"`
	NewLines int      `json:"newLines"`
	Lines    []string `json:"lines"`
}

type File struct {
	Path    string `json:"path"`
	OldPath string `json:"oldPath,omitempty"`
	Status  string `json:"status"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Binary  bool   `json:"binary,omitempty"`
	Hunks   []Hunk `json:"hunks"`
}

type Changes struct {
	Repo      string `json:"repo"`
	Branch    string `json:"branch"`
	Mode      string `json:"mode"`
	Base      string `json:"base,omitempty"`
	Files     []File `json:"files"`
	Truncated bool   `json:"truncated,omitempty"`
}

const (
	ModeUncommitted = "uncommitted"
	ModeBranch      = "branch"
)

var ErrNotRepo = errors.New("not a git repository")

// maxDiffBytes caps the diff read from git.
const maxDiffBytes = 4 << 20

type Reader struct {
	Runner run.Runner
}

// Env is the environment for git: base without any GIT_ variable, which
// could point git at another repository or inject config
// (GIT_CONFIG_PARAMETERS), plus no optional locks and no prompts.
func Env(base []string) []string {
	out := make([]string, 0, len(base)+2)
	for _, kv := range base {
		if !strings.HasPrefix(kv, "GIT_") {
			out = append(out, kv)
		}
	}
	return append(out, "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
}

// safeArgs prefixes every git call.
func safeArgs(dir string) []string {
	return []string{"-C", dir, "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", "-c", "diff.external=", "--no-pager"}
}

func (r Reader) git(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := r.Runner.Run(ctx, "git", append(safeArgs(dir), args...)...)
	return string(out), err
}

// filterGuard blanks every filter driver the repository itself configures.
// git diff runs a driver's clean or process command on worktree files, so
// a hostile repository could otherwise run a command when its changes are
// shown. Drivers from the user's own global or system config are left
// alone; those are tools the user installed, such as Git LFS.
func (r Reader) filterGuard(ctx context.Context, dir string) []string {
	out, _ := r.git(ctx, dir, "config", "--show-scope", "--get-regexp", `^filter\.`)
	var guard []string
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || f[0] == "global" || f[0] == "system" {
			continue
		}
		key := f[1]
		dot := strings.LastIndexByte(key, '.')
		if !strings.HasPrefix(key, "filter.") || dot <= len("filter.") {
			continue
		}
		name := key[len("filter."):dot]
		if seen[name] {
			continue
		}
		seen[name] = true
		for _, k := range []string{"clean", "smudge", "process"} {
			guard = append(guard, "-c", "filter."+name+"."+k+"=")
		}
		guard = append(guard, "-c", "filter."+name+".required=false")
	}
	return guard
}

// Collect reads the changes in the repository holding dir: uncommitted
// work against HEAD, or everything on the branch since it left the
// default branch.
func (r Reader) Collect(ctx context.Context, dir, mode string) (Changes, error) {
	root, err := r.git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return Changes{}, ErrNotRepo
	}
	root = strings.TrimSpace(root)
	branch, _ := r.git(ctx, root, "rev-parse", "--abbrev-ref", "HEAD")
	c := Changes{Repo: root, Branch: strings.TrimSpace(branch), Mode: mode, Files: []File{}}

	against := "HEAD"
	if mode == ModeBranch {
		base, err := r.mergeBase(ctx, root)
		if err != nil {
			return Changes{}, err
		}
		c.Base, against = base, base
	} else {
		c.Mode = ModeUncommitted
	}
	args := append(r.filterGuard(ctx, root), "diff", "--no-color", "--no-ext-diff", "--no-textconv", "-M", against, "--")
	diff, err := r.git(ctx, root, args...)
	if err != nil {
		return Changes{}, err
	}
	if len(diff) > maxDiffBytes {
		diff, c.Truncated = diff[:maxDiffBytes], true
	}
	c.Files = Parse(diff)
	untracked, _ := r.git(ctx, root, "ls-files", "--others", "--exclude-standard")
	for _, p := range strings.Split(strings.TrimSpace(untracked), "\n") {
		if p != "" {
			c.Files = append(c.Files, File{Path: p, Status: "untracked", Hunks: []Hunk{}})
		}
	}
	return c, nil
}

// mergeBase finds where HEAD left the default branch: origin's HEAD when
// set, else origin/main, origin/master, main or master.
func (r Reader) mergeBase(ctx context.Context, root string) (string, error) {
	candidates := []string{}
	if ref, err := r.git(ctx, root, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); err == nil {
		candidates = append(candidates, strings.TrimPrefix(strings.TrimSpace(ref), "refs/remotes/"))
	}
	candidates = append(candidates, "origin/main", "origin/master", "main", "master")
	for _, c := range candidates {
		if base, err := r.git(ctx, root, "merge-base", "HEAD", c); err == nil {
			return strings.TrimSpace(base), nil
		}
	}
	return "", errors.New("no default branch to compare with (tried origin/HEAD, main and master)")
}

// Parse reads a unified diff from git into files and hunks.
func Parse(diff string) []File {
	files := []File{}
	var cur *File
	var hunk *Hunk
	flush := func() {
		if cur != nil {
			files = append(files, *cur)
		}
		cur, hunk = nil, nil
	}
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flush()
			cur = &File{Status: "modified", Hunks: []Hunk{}}
			if a, b, ok := splitGitPaths(strings.TrimPrefix(line, "diff --git ")); ok {
				cur.OldPath, cur.Path = a, b
			}
		case cur == nil:
			continue
		case strings.HasPrefix(line, "new file mode"):
			cur.Status = "added"
		case strings.HasPrefix(line, "deleted file mode"):
			cur.Status = "deleted"
		case strings.HasPrefix(line, "rename from "):
			cur.Status, cur.OldPath = "renamed", strings.TrimPrefix(line, "rename from ")
		case strings.HasPrefix(line, "rename to "):
			cur.Path = strings.TrimPrefix(line, "rename to ")
		case strings.HasPrefix(line, "Binary files "):
			cur.Binary = true
		case strings.HasPrefix(line, "--- "), strings.HasPrefix(line, "+++ "):
			if hunk == nil {
				continue
			}
			addLine(cur, hunk, line)
		case strings.HasPrefix(line, "@@"):
			h, ok := parseHunkHeader(line)
			if !ok {
				continue
			}
			cur.Hunks = append(cur.Hunks, h)
			hunk = &cur.Hunks[len(cur.Hunks)-1]
		case hunk != nil && (strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\\")):
			addLine(cur, hunk, line)
		}
	}
	flush()
	for i := range files {
		if files[i].OldPath == files[i].Path {
			files[i].OldPath = ""
		}
	}
	return files
}

func addLine(f *File, h *Hunk, line string) {
	h.Lines = append(h.Lines, line)
	switch line[0] {
	case '+':
		f.Added++
	case '-':
		f.Removed++
	}
}

// splitGitPaths reads "a/x b/y" from a diff --git line. Paths with spaces
// are ambiguous there; rename and +++ lines refine them when present.
func splitGitPaths(s string) (string, string, bool) {
	i := strings.Index(s, " b/")
	if !strings.HasPrefix(s, "a/") || i < 0 {
		return "", "", false
	}
	return s[2:i], s[i+3:], true
}

func parseHunkHeader(line string) (Hunk, bool) {
	// @@ -oldStart,oldLines +newStart,newLines @@ optional context
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return Hunk{}, false
	}
	os, ol, ok1 := rangeOf(strings.TrimPrefix(parts[1], "-"))
	ns, nl, ok2 := rangeOf(strings.TrimPrefix(parts[2], "+"))
	if !ok1 || !ok2 {
		return Hunk{}, false
	}
	return Hunk{OldStart: os, OldLines: ol, NewStart: ns, NewLines: nl, Lines: []string{}}, true
}

func rangeOf(s string) (int, int, bool) {
	start, count, found := strings.Cut(s, ",")
	a, err := strconv.Atoi(start)
	if err != nil {
		return 0, 0, false
	}
	if !found {
		return a, 1, true
	}
	b, err := strconv.Atoi(count)
	if err != nil {
		return 0, 0, false
	}
	return a, b, true
}

// Sessions is the live session list.
type Sessions interface {
	State() hub.State
}

// Service reads changes for a session by id.
type Service struct {
	Sessions Sessions
	Reader   Reader
}

func (s *Service) Changes(ctx context.Context, sessionID, mode string) (Changes, error) {
	for _, v := range s.Sessions.State().Sessions {
		if v.ID != sessionID {
			continue
		}
		c, err := s.Reader.Collect(ctx, v.Directory, mode)
		if errors.Is(err, ErrNotRepo) {
			return Changes{}, &send.Refused{Reason: "not_repo", Message: v.Directory + " is not inside a git repository."}
		}
		return c, err
	}
	return Changes{}, &send.Refused{Reason: send.ReasonUnknownSession, Message: "This session no longer exists in agent-manager."}
}
