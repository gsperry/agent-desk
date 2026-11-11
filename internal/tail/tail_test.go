package tail

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func strs(lines [][]byte) []string {
	out := []string{}
	for _, l := range lines {
		out = append(out, string(l))
	}
	return out
}

func appendFile(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

func TestFollower(t *testing.T) {
	type step struct {
		name      string
		do        func(t *testing.T, path string)
		want      []string
		wantReset bool
		wantErr   error
	}
	replace := func(content string) func(t *testing.T, path string) {
		return func(t *testing.T, path string) {
			tmp := path + ".new"
			if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(tmp, path); err != nil {
				t.Fatal(err)
			}
		}
	}
	add := func(s string) func(t *testing.T, path string) {
		return func(t *testing.T, path string) { appendFile(t, path, s) }
	}
	steps := []step{
		{"initial lines", add("a\nb\n"), []string{"a", "b"}, false, nil},
		{"nothing new", func(*testing.T, string) {}, []string{}, false, nil},
		{"partial line is held", add(`{"half":`), []string{}, false, nil},
		{"partial completes", add("1}\nc\n"), []string{`{"half":1}`, "c"}, false, nil},
		{"blank lines and CRLF", add("\n\nd\r\n"), []string{"d"}, false, nil},
		{"truncated", func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte("x\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, []string{"x"}, true, nil},
		{"replaced by a longer file", replace("p\nq\nr\ns\nt\nu\n"), []string{"p", "q", "r", "s", "t", "u"}, true, nil},
		{"deleted", func(t *testing.T, path string) {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}, []string{}, false, ErrGone},
		{"recreated", add("z\n"), []string{"z"}, true, nil},
	}
	path := filepath.Join(t.TempDir(), "s.jsonl")
	f := New(path)
	for _, s := range steps {
		s.do(t, path)
		lines, reset, err := f.Read()
		if !errors.Is(err, s.wantErr) {
			t.Fatalf("%s: err = %v, want %v", s.name, err, s.wantErr)
		}
		if got := strs(lines); !reflect.DeepEqual(got, s.want) || reset != s.wantReset {
			t.Fatalf("%s: got %q reset=%v, want %q reset=%v", s.name, got, reset, s.want, s.wantReset)
		}
	}
}
