package summary

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gsperry/agent-desk/internal/send"
)

// Settings turn catch-up summaries on and choose their model. Summaries
// spend tokens, so they start off, and the server refuses them until the
// user turns them on in Settings.
type Settings struct {
	Enabled bool   `json:"enabled"`
	Model   string `json:"model"`
}

// DefaultModel is the model a summary uses until the user picks another.
const DefaultModel = "haiku"

// Validate trims the model and fills in the default.
func (s Settings) Validate() (Settings, error) {
	s.Model = strings.TrimSpace(s.Model)
	if s.Model == "" {
		s.Model = DefaultModel
	}
	if !modelName.MatchString(s.Model) {
		return Settings{}, &send.Refused{Reason: reasonInvalid, Message: "Choose a model name such as haiku or sonnet."}
	}
	return s, nil
}

// SettingsStore keeps the settings in a file only the user can read.
type SettingsStore struct {
	Path string

	mu  sync.Mutex
	cur *Settings
}

// Get returns the saved settings, or summaries off with the default model.
func (st *SettingsStore) Get() Settings {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.cur != nil {
		return *st.cur
	}
	s := Settings{Model: DefaultModel}
	if data, err := os.ReadFile(st.Path); err == nil {
		var loaded Settings
		if json.Unmarshal(data, &loaded) == nil {
			if v, err := loaded.Validate(); err == nil {
				s = v
			}
		}
	}
	st.cur = &s
	return s
}

// Set validates and saves the settings, through an owner-only temporary
// file renamed into place.
func (st *SettingsStore) Set(s Settings) (Settings, error) {
	v, err := s.Validate()
	if err != nil {
		return Settings{}, err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return Settings{}, err
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	dir := filepath.Dir(st.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Settings{}, err
	}
	tmp, err := os.CreateTemp(dir, ".summaries-*.json")
	if err != nil {
		return Settings{}, err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return Settings{}, err
	}
	if err := os.Rename(tmp.Name(), st.Path); err != nil {
		return Settings{}, err
	}
	st.cur = &v
	return v, nil
}
