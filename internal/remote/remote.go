// Package remote decides what a client away from the desk may do.
//
// Remote clients reach Agent Desk through a second listener, still on
// 127.0.0.1, behind a proxy that proves who they are: Tailscale, or a
// Cloudflare Tunnel behind Cloudflare Access.
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
	// Via is the proxy remote clients come through: ViaTailscale or
	// ViaCloudflare. Only that proxy's identity is accepted.
	Via string `json:"via"`
	// TailscaleLogins are the Tailscale logins allowed in.
	TailscaleLogins []string `json:"tailscaleLogins"`
	// CloudflareTeam is the Zero Trust team name (<team>.cloudflareaccess.com).
	CloudflareTeam string `json:"cloudflareTeam"`
	// CloudflareAUD is the Access application's audience tag.
	CloudflareAUD string `json:"cloudflareAud"`
	// CloudflareEmails are the Access emails allowed in.
	CloudflareEmails []string `json:"cloudflareEmails"`
}

// The proxies remote clients can come through.
const (
	ViaTailscale  = "tailscale"
	ViaCloudflare = "cloudflare"
)

// DefaultPort is where the remote listener waits unless set.
const DefaultPort = 7327

// Defaults are remote access off, at the lowest level.
func Defaults() Settings {
	return Settings{Level: LevelAnswer, Port: DefaultPort, Via: ViaTailscale, TailscaleLogins: []string{}, CloudflareEmails: []string{}}
}

var (
	hostName = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,62}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,62}[A-Za-z0-9])?)*(:[0-9]{1,5})?$`)
	login    = regexp.MustCompile(`^[^\s,]{1,200}$`)
	teamName = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	audTag   = regexp.MustCompile(`^[A-Za-z0-9]{16,128}$`)
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
	if s.Via == "" {
		s.Via = ViaTailscale
	}
	if s.Via != ViaTailscale && s.Via != ViaCloudflare {
		return s, errors.New("via must be tailscale or cloudflare")
	}
	var err error
	if s.TailscaleLogins, err = tidyLogins(s.TailscaleLogins, "a Tailscale login"); err != nil {
		return s, err
	}
	if s.CloudflareEmails, err = tidyLogins(s.CloudflareEmails, "an email"); err != nil {
		return s, err
	}
	team := strings.ToLower(strings.TrimSpace(s.CloudflareTeam))
	team = strings.TrimSuffix(strings.TrimPrefix(team, "https://"), "/")
	s.CloudflareTeam = strings.TrimSuffix(team, ".cloudflareaccess.com")
	s.CloudflareAUD = strings.TrimSpace(s.CloudflareAUD)
	if s.CloudflareTeam != "" && !teamName.MatchString(s.CloudflareTeam) {
		return s, fmt.Errorf("%q is not a Cloudflare team name", s.CloudflareTeam)
	}
	if s.CloudflareAUD != "" && !audTag.MatchString(s.CloudflareAUD) {
		return s, errors.New("the Access audience tag is letters and digits, from the application's overview")
	}
	if !s.Enabled {
		return s, nil
	}
	if !hostName.MatchString(s.Host) {
		return s, errors.New("set the host name remote clients use, such as desk.tail1234.ts.net")
	}
	switch s.Via {
	case ViaTailscale:
		if len(s.TailscaleLogins) == 0 {
			return s, errors.New("allow at least one Tailscale login")
		}
	case ViaCloudflare:
		if s.CloudflareTeam == "" || s.CloudflareAUD == "" {
			return s, errors.New("set the Cloudflare team name and the Access audience tag")
		}
		if len(s.CloudflareEmails) == 0 {
			return s, errors.New("allow at least one email")
		}
	}
	return s, nil
}

// tidyLogins lowercases, trims and dedupes logins, and rejects malformed
// ones.
func tidyLogins(in []string, what string) ([]string, error) {
	out := []string{}
	for _, l := range in {
		l = strings.ToLower(strings.TrimSpace(l))
		if l == "" {
			continue
		}
		if !login.MatchString(l) {
			return nil, fmt.Errorf("%q is not %s", l, what)
		}
		if !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out, nil
}

// Proxy is the configured proxy; settings saved before Cloudflare support
// have none, which means Tailscale.
func (s Settings) Proxy() string {
	if s.Via == "" {
		return ViaTailscale
	}
	return s.Via
}

// Allowed reports whether user may come in through the configured proxy.
func (s Settings) Allowed(user string) bool {
	switch s.Proxy() {
	case ViaTailscale:
		return slices.Contains(s.TailscaleLogins, user)
	case ViaCloudflare:
		return slices.Contains(s.CloudflareEmails, user)
	}
	return false
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

// Set validates and saves settings. Saves are serialized, and each writes
// a fresh owner-only temporary file renamed into place.
func (st *Store) Set(s Settings) (Settings, error) {
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
	tmp, err := os.CreateTemp(dir, ".remote-*.json")
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
	return clone(v), nil
}

func clone(s Settings) Settings {
	s.TailscaleLogins = append([]string{}, s.TailscaleLogins...)
	s.CloudflareEmails = append([]string{}, s.CloudflareEmails...)
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

// cloudflareHeaders are set by Cloudflare's edge on every request it
// forwards.
var cloudflareHeaders = []string{"Cf-Ray", "Cf-Connecting-Ip", AccessHeader}

// Identify returns the identity of a remote request, or false. Only the
// configured proxy's identity counts. Tailscale sets the login header on
// every request it proxies. Cloudflare Access signs a token for every
// request it lets through, which v verifies. Either way only the logins the
// user allowed are let in.
func Identify(r *http.Request, s Settings, v Verifier) (Identity, bool) {
	var user string
	switch s.Proxy() {
	case ViaTailscale:
		// Requests that came through Cloudflare's edge never count as
		// Tailscale, even if a tunnel left running forwards a login header.
		for _, h := range cloudflareHeaders {
			if r.Header.Get(h) != "" {
				return Identity{}, false
			}
		}
		values := r.Header.Values(TailscaleHeader)
		if len(values) != 1 {
			return Identity{}, false
		}
		user = strings.ToLower(strings.TrimSpace(values[0]))
	case ViaCloudflare:
		values := r.Header.Values(AccessHeader)
		if len(values) != 1 || v == nil {
			return Identity{}, false
		}
		email, err := v.Verify(values[0], s.CloudflareTeam, s.CloudflareAUD)
		if err != nil {
			return Identity{}, false
		}
		user = email
	default:
		return Identity{}, false
	}
	if user == "" || !s.Allowed(user) {
		return Identity{}, false
	}
	return Identity{User: user, Via: s.Proxy()}, true
}
