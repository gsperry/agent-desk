package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteLauncher(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agent-desk")
	url := `http://127.0.0.1:7317/#t=abc"><script>x</script>`
	path, err := writeLauncher(dir, url)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		path string
		mode os.FileMode
	}{{"directory", dir, 0o700}, {"file", path, 0o600}} {
		info, err := os.Stat(c.path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != c.mode {
			t.Errorf("%s mode = %o, want %o", c.name, got, c.mode)
		}
	}
	b, _ := os.ReadFile(path)
	page := string(b)
	if strings.Contains(page, "<script>") || !strings.Contains(page, "#t=abc&#34;&gt;") {
		t.Fatalf("url not escaped: %s", page)
	}
	if !strings.Contains(page, `http-equiv="refresh"`) {
		t.Fatalf("no redirect: %s", page)
	}
}

func TestSessionTokenIsNotInherited(t *testing.T) {
	t.Setenv("AGENT_DESK_TOKEN", strings.Repeat("a", 64))
	tok, err := sessionToken()
	if err != nil || tok != strings.Repeat("a", 64) {
		t.Fatalf("token %q, err %v", tok, err)
	}
	if _, set := os.LookupEnv("AGENT_DESK_TOKEN"); set {
		t.Fatal("AGENT_DESK_TOKEN still set for child processes")
	}
}
