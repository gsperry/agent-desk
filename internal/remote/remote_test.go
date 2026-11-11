package remote

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAllows(t *testing.T) {
	needs := []Need{NeedLocal, NeedRead, NeedAnswer, NeedReply, NeedManage}
	tests := []struct {
		level Level
		want  []bool
	}{
		{LevelAnswer, []bool{false, true, true, false, false}},
		{LevelReply, []bool{false, true, true, true, false}},
		{LevelManage, []bool{false, true, true, true, true}},
		{"", []bool{false, false, false, false, false}},
		{"root", []bool{false, false, false, false, false}},
	}
	for _, tt := range tests {
		for i, n := range needs {
			if got := tt.level.Allows(n); got != tt.want[i] {
				t.Errorf("%q.Allows(%d) = %v", tt.level, n, got)
			}
		}
	}
}

func TestValidate(t *testing.T) {
	ok := Settings{Enabled: true, Level: LevelReply, Host: " Desk.Tail1234.TS.net ", TailscaleLogins: []string{" Guy@Example.com", "guy@example.com", ""}}
	got, err := ok.Validate()
	if err != nil {
		t.Fatal(err)
	}
	want := Settings{Enabled: true, Level: LevelReply, Host: "desk.tail1234.ts.net", Port: DefaultPort, TailscaleLogins: []string{"guy@example.com"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	bad := []Settings{
		{Level: "admin"},
		{Port: 80},
		{Enabled: true, Host: "", TailscaleLogins: []string{"a@b"}},
		{Enabled: true, Host: "bad host", TailscaleLogins: []string{"a@b"}},
		{Enabled: true, Host: "desk.ts.net"},
		{TailscaleLogins: []string{"two words"}},
	}
	for _, b := range bad {
		if _, err := b.Validate(); err == nil {
			t.Errorf("%+v validated", b)
		}
	}
	// Off needs no host or logins.
	if _, err := (Settings{}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestStore(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "remote.json")
	st := &Store{Path: p}
	if got := st.Get(); got.Enabled || got.Level != LevelAnswer || got.Port != DefaultPort {
		t.Fatalf("defaults = %+v", got)
	}
	saved, err := st.Set(Settings{Enabled: true, Level: LevelManage, Host: "desk.ts.net", TailscaleLogins: []string{"a@b.c"}})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode %v err %v", info.Mode(), err)
	}
	if got := (&Store{Path: p}).Get(); !reflect.DeepEqual(got, saved) {
		t.Fatalf("reloaded %+v, want %+v", got, saved)
	}
	if _, err := st.Set(Settings{Level: "admin"}); err == nil {
		t.Fatal("invalid settings saved")
	}
	if got := st.Get(); got.Level != LevelManage {
		t.Fatalf("invalid save changed settings: %+v", got)
	}
	// A hand-edited invalid file falls back to the defaults.
	if err := os.WriteFile(p, []byte(`{"enabled":true,"level":"root"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := (&Store{Path: p}).Get(); got.Enabled {
		t.Fatalf("invalid file enabled remote access: %+v", got)
	}
}

func TestIdentify(t *testing.T) {
	s := Settings{TailscaleLogins: []string{"guy@example.com"}}
	tests := []struct {
		name   string
		values []string
		want   bool
	}{
		{"allowed", []string{"Guy@Example.com"}, true},
		{"not allowed", []string{"eve@example.com"}, false},
		{"missing", nil, false},
		{"empty", []string{" "}, false},
		{"two headers", []string{"guy@example.com", "eve@example.com"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			for _, v := range tt.values {
				r.Header.Add(TailscaleHeader, v)
			}
			id, ok := Identify(r, s)
			if ok != tt.want || (ok && id.User != "guy@example.com") {
				t.Fatalf("got %+v %v", id, ok)
			}
		})
	}
}
