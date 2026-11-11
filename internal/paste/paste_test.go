package paste

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var png, _ = base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=")

func TestSave(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pastes")
	s := Store{Dir: dir}
	path, err := s.Save(png)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, ".png") || filepath.Dir(path) != dir {
		t.Fatalf("path %q", path)
	}
	for _, c := range []struct {
		p    string
		mode os.FileMode
	}{{dir, 0o700}, {path, 0o600}} {
		info, err := os.Stat(c.p)
		if err != nil || info.Mode().Perm() != c.mode {
			t.Fatalf("%s mode %v, err %v", c.p, info.Mode().Perm(), err)
		}
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, png) {
		t.Fatal("content changed")
	}
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"html claiming to be an image", []byte("<html><script>alert(1)</script></html>")},
		{"text", []byte("hello")},
		{"too big", append(append([]byte{}, png...), make([]byte, MaxBytes)...)},
	}
	for _, tt := range tests {
		if _, err := s.Save(tt.data); err == nil {
			t.Errorf("%s saved", tt.name)
		}
	}
}

func TestCleanup(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	old, _ := s.Save(png)
	fresh, _ := s.Save(png)
	keep := filepath.Join(dir, "other.txt")
	if err := os.WriteFile(keep, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	past := now.Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(keep, past, past); err != nil {
		t.Fatal(err)
	}
	if n := s.Cleanup(7*24*time.Hour, now); n != 1 {
		t.Fatalf("removed %d", n)
	}
	for _, p := range []string{fresh, keep} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s removed", p)
		}
	}
	if _, err := os.Stat(old); err == nil {
		t.Error("old paste kept")
	}
}
