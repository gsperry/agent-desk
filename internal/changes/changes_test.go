package changes

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsperry/agent-desk/internal/run"
)

const sample = `diff --git a/main.go b/main.go
index 1111111..2222222 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
-import "fmt"
+import (
+	"fmt"
+)
@@ -10 +11 @@ func main() {
-	fmt.Println("hi")
+	fmt.Println("hello")
\ No newline at end of file
diff --git a/new.txt b/new.txt
new file mode 100644
index 0000000..3333333
--- /dev/null
+++ b/new.txt
@@ -0,0 +1 @@
+-- not a header
diff --git a/old.txt b/old.txt
deleted file mode 100644
--- a/old.txt
+++ /dev/null
@@ -1 +0,0 @@
-gone
diff --git a/a.go b/b.go
similarity index 90%
rename from a.go
rename to b.go
diff --git a/logo.png b/logo.png
Binary files a/logo.png and b/logo.png differ
`

func TestParse(t *testing.T) {
	files := Parse(sample)
	type sum struct {
		path, old, status string
		added, removed    int
		hunks             int
		binary            bool
	}
	var got []sum
	for _, f := range files {
		got = append(got, sum{f.Path, f.OldPath, f.Status, f.Added, f.Removed, len(f.Hunks), f.Binary})
	}
	want := []sum{
		{"main.go", "", "modified", 4, 2, 2, false},
		{"new.txt", "", "added", 1, 0, 1, false},
		{"old.txt", "", "deleted", 0, 1, 1, false},
		{"b.go", "a.go", "renamed", 0, 0, 0, false},
		{"logo.png", "", "modified", 0, 0, 0, true},
	}
	if len(got) != len(want) {
		t.Fatalf("files = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("file %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	h := files[0].Hunks[1]
	if h.OldStart != 10 || h.OldLines != 1 || h.NewStart != 11 || h.NewLines != 1 || len(h.Lines) != 3 {
		t.Fatalf("hunk %+v", h)
	}
	if files[1].Hunks[0].Lines[0] != "+-- not a header" {
		t.Fatalf("content line read as header: %q", files[1].Hunks[0].Lines)
	}
	if len(Parse("")) != 0 || len(Parse("garbage\n@@ nope")) != 0 {
		t.Fatal("parsed something from nothing")
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "init.defaultBranch=main", "-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestCollect(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	write := func(name, s string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git(t, dir, "init", "-q")
	write("a.txt", "one\ntwo\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "base")
	git(t, dir, "checkout", "-q", "-b", "feature")
	write("b.txt", "committed on the branch\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "feature work")
	write("a.txt", "one\nTWO\n")
	write("scratch.txt", "untracked\n")
	// A diff driver in the repo's own config must not run.
	git(t, dir, "config", "diff.evil.textconv", "touch "+filepath.Join(dir, "PWNED")+"; cat")
	// So must a clean filter, which git diff runs on changed worktree files.
	git(t, dir, "config", "filter.sneaky.clean", "touch "+filepath.Join(dir, "PWNED-FILTER")+"; cat")
	git(t, dir, "config", "filter.sneaky.required", "true")
	write(".gitattributes", "*.txt diff=evil filter=sneaky\n")

	r := Reader{Runner: run.Exec{Env: append(run.Environ(), "GIT_OPTIONAL_LOCKS=0")}}
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	unc, err := r.Collect(context.Background(), sub, ModeUncommitted)
	if err != nil {
		t.Fatal(err)
	}
	if unc.Repo != dir || unc.Branch != "feature" {
		t.Fatalf("repo %q branch %q", unc.Repo, unc.Branch)
	}
	names := func(c Changes) string {
		var out []string
		for _, f := range c.Files {
			out = append(out, f.Path+":"+f.Status)
		}
		return strings.Join(out, ",")
	}
	if got := names(unc); got != "a.txt:modified,.gitattributes:untracked,scratch.txt:untracked" {
		t.Fatalf("uncommitted = %s", got)
	}
	br, err := r.Collect(context.Background(), dir, ModeBranch)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(br); !strings.Contains(got, "a.txt:modified") || !strings.Contains(got, "b.txt:added") || br.Base == "" {
		t.Fatalf("branch = %s base %q", got, br.Base)
	}
	if _, err := os.Stat(filepath.Join(dir, "PWNED")); err == nil {
		t.Fatal("a repo-configured textconv ran")
	}
	if _, err := os.Stat(filepath.Join(dir, "PWNED-FILTER")); err == nil {
		t.Fatal("a repo-configured clean filter ran")
	}
	if _, err := r.Collect(context.Background(), t.TempDir(), ModeUncommitted); err != ErrNotRepo {
		t.Fatalf("non-repo err = %v", err)
	}
}

func TestEnv(t *testing.T) {
	got := strings.Join(Env([]string{"HOME=/h", "GIT_DIR=/x", "GIT_CONFIG_PARAMETERS='core.pager=sh'", "PATH=/bin"}), " ")
	if got != "HOME=/h PATH=/bin GIT_OPTIONAL_LOCKS=0 GIT_TERMINAL_PROMPT=0" {
		t.Fatalf("env = %s", got)
	}
}
