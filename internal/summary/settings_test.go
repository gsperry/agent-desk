package summary

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-desk", "summaries.json")
	st := &SettingsStore{Path: path}
	if got := st.Get(); got.Enabled || got.Model != DefaultModel {
		t.Fatalf("default = %+v", got)
	}
	if _, err := st.Set(Settings{Enabled: true, Model: " sonnet "}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("file %v, %v", info, err)
	}
	if got := (&SettingsStore{Path: path}).Get(); !got.Enabled || got.Model != "sonnet" {
		t.Fatalf("reloaded = %+v", got)
	}
	if _, err := st.Set(Settings{Enabled: true, Model: "--x; rm"}); err == nil {
		t.Fatal("bad model saved")
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := (&SettingsStore{Path: path}).Get(); got.Enabled {
		t.Fatal("a broken file turned summaries on")
	}
}
