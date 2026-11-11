package locate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const id = "e392f4f9-1ea2-4338-af11-af32d1ef84da"

func write(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFind(t *testing.T) {
	root := t.TempDir()
	alt := t.TempDir()
	write(t, filepath.Join(root, "projects", "-private-tmp-probe", id+".jsonl"))
	write(t, filepath.Join(alt, "projects", "-src-api", "33333333-3333-4333-8333-333333333333.jsonl"))
	profiles := map[string]string{"alt": alt}
	l := &Locator{DefaultRoot: root, Profile: func(_ context.Context, name, key string) (string, error) {
		if key != "CLAUDE_CONFIG_DIR" {
			t.Fatalf("asked for %s", key)
		}
		return profiles[name], nil
	}}
	tests := []struct {
		name, id, env, want string
		wantErr             error
	}{
		{"default root", id, "", filepath.Join(root, "projects", "-private-tmp-probe", id+".jsonl"), nil},
		{"profile root", "33333333-3333-4333-8333-333333333333", "alt", filepath.Join(alt, "projects", "-src-api", "33333333-3333-4333-8333-333333333333.jsonl"), nil},
		{"profile without the variable uses default", id, "plain", filepath.Join(root, "projects", "-private-tmp-probe", id+".jsonl"), nil},
		{"not written yet", "44444444-4444-4444-8444-444444444444", "", "", ErrNotFound},
		{"glob characters refused", "*", "", "", ErrNotFound},
		{"path traversal refused", "../../etc/passwd", "", "", ErrNotFound},
		{"empty id", "", "", "", ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := l.Find(context.Background(), tt.id, tt.env)
			if !errors.Is(err, tt.wantErr) || got != tt.want {
				t.Fatalf("got %q, %v; want %q, %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestFindFollowsMovedFile(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "projects", "a", id+".jsonl")
	write(t, first)
	l := &Locator{DefaultRoot: root}
	if got, _ := l.Find(context.Background(), id, ""); got != first {
		t.Fatalf("got %q", got)
	}
	if err := os.Remove(first); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(root, "projects", "b", id+".jsonl")
	write(t, second)
	if got, _ := l.Find(context.Background(), id, ""); got != second {
		t.Fatalf("stale cache: got %q", got)
	}
}

func TestFindPrefersNewest(t *testing.T) {
	root := t.TempDir()
	older := filepath.Join(root, "projects", "a", id+".jsonl")
	newer := filepath.Join(root, "projects", "b", id+".jsonl")
	write(t, older)
	write(t, newer)
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(older, past, past); err != nil {
		t.Fatal(err)
	}
	l := &Locator{DefaultRoot: root}
	if got, _ := l.Find(context.Background(), id, ""); got != newer {
		t.Fatalf("got %q, want newest", got)
	}
}
