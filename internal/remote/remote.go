// Package remote decides what a client away from the desk may do.
//
// Remote clients reach Agent Desk through a second listener, still on
// 127.0.0.1, behind a proxy that proves who they are (Tailscale first).
// They never use the local token. Every route declares the level it needs;
// some routes are local only at any level. The settings are changed only
// from a local client.
package remote

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
)

// Level is what a remote client may do. Each level includes the ones
// before it.
type Level string

const (
	// LevelAnswer reads everything and answers waiting prompts.
	LevelAnswer Level = "answer"
	// LevelReply also sends replies through the normal reply gate.
	LevelReply Level = "reply"
	// LevelManage also starts, stops, revives, archives and moves sessions.
	LevelManage Level = "manage"
)

var order = map[Level]int{LevelAnswer: 1, LevelReply: 2, LevelManage: 3}

// Need is what a route requires of a remote client.
type Need int

const (
	// NeedLocal routes are never served remotely: settings, rules,
	// summaries (they spend tokens), opening files, pasting images.
	NeedLocal Need = iota
	NeedRead
	NeedAnswer
	NeedReply
	NeedManage
)

// Allows reports whether a client at level l may use a route with need n.
func (l Level) Allows(n Need) bool {
	switch n {
	case NeedRead, NeedAnswer:
		return order[l] >= order[LevelAnswer]
	case NeedReply:
		return order[l] >= order[LevelReply]
	case NeedManage:
		return order[l] >= order[LevelManage]
	}
	return false
}

// Settings are the remote access settings, kept on the desk.
type Settings struct {
	Enabled bool  `json:"enabled"`
	Level   Level `json:"level"`
	// Host is the name remote clients use, such as
	// desk.tail1234.ts.net. Requests naming another host are refused.
	Host string `json:"host"`
	// Port is the 127.0.0.1 port the proxy forwards to.
	Port int `json:"port"`
	// TailscaleLogins are the Tailscale logins allowed in.
	TailscaleLogins []string `json:"tailscaleLogins"`
}

// DefaultPort is where the remote listener waits unless set.
const DefaultPort = 7327

// Defaults are remote access off, at the lowest level.
func Defaults() Settings {
	return Settings{Level: LevelAnswer, Port: DefaultPort, TailscaleLogins: []string{}}
}

var (
	hostName = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,62}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,62}[A-Za-z0-9])?)*(:[0-9]{1,5})?$`)
	login    = regexp.MustCompile(`^[^\s,]{1,200}$`)
)

// Validate checks and tidies settings before they are saved.
func (s Settings) Validate() (Settings, error) {
	s.Host = strings.ToLower(strings.TrimSpace(s.Host))
	if s.Level == "" {
		s.Level = LevelAnswer
	}
	if _, ok := order[s.Level]; !ok {
		return s, fmt.Errorf("level must be answer, reply or manage")
	}
	if s.Port == 0 {
		s.Port = DefaultPort
	}
	if s.Port < 1024 || s.Port > 65535 {
		return s, errors.New("port must be 1024 to 65535")
	}
	logins := []string{}
	for _, l := range s.TailscaleLogins {
		l = strings.ToLower(strings.TrimSpace(l))
		if l == "" {
			continue
		}
		if !login.MatchString(l) {
			return s, fmt.Errorf("%q is not a Tailscale login", l)
		}
		if !slices.Contains(logins, l) {
			logins = append(logins, l)
		}
	}
	s.TailscaleLogins = logins
	if s.Enabled {
		if !hostName.MatchString(s.Host) {
			return s, errors.New("set the host name remote clients use, such as desk.tail1234.ts.net")
		}
		if len(s.TailscaleLogins) == 0 {
			return s, errors.New("allow at least one Tailscale login")
		}
	}
	return s, nil
}

// Store keeps settings in a file only the user can read.
type Store struct {
	Path string

	mu  sync.Mutex
	cur *Settings
}

// Get returns the saved settings, or the defaults.
func (st *Store) Get() Settings {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.cur != nil {
		return clone(*st.cur)
	}
	s := Defaults()
	if data, err := os.ReadFile(st.Path); err == nil {
		var loaded Settings
		if json.Unmarshal(data, &loaded) == nil {
			if v, err := loaded.Validate(); err == nil {
				s = v
			}
		}
	}
	st.cur = &s
	return clone(s)
}

// Set validates and saves settings.
func (st *Store) Set(s Settings) (Settings, error) {
	v, err := s.Validate()
	if err != nil {
		return Settings{}, err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return Settings{}, err
	}
	if err := os.MkdirAll(filepath.Dir(st.Path), 0o700); err != nil {
		return Settings{}, err
	}
	tmp := st.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return Settings{}, err
	}
	if err := os.Rename(tmp, st.Path); err != nil {
		return Settings{}, err
	}
	st.mu.Lock()
	st.cur = &v
	st.mu.Unlock()
	return clone(v), nil
}

func clone(s Settings) Settings {
	s.TailscaleLogins = append([]string{}, s.TailscaleLogins...)
	return s
}

// Identity is who a remote request comes from.
type Identity struct {
	User string `json:"user"`
	Via  string `json:"via"`
}

// TailscaleHeader is the login `tailscale serve` adds to requests from
// tailnet users. Requests from tagged devices carry none.
const TailscaleHeader = "Tailscale-User-Login"

// Identify returns the identity of a remote request, or false. Tailscale
// sets the login header on every request it proxies; only logins the user
// allowed are let in.
func Identify(r *http.Request, s Settings) (Identity, bool) {
	values := r.Header.Values(TailscaleHeader)
	if len(values) != 1 {
		return Identity{}, false
	}
	user := strings.ToLower(strings.TrimSpace(values[0]))
	if user == "" || !slices.Contains(s.TailscaleLogins, user) {
		return Identity{}, false
	}
	return Identity{User: user, Via: "tailscale"}, true
}
