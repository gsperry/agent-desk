// Package tts reads a reply aloud through a text-to-speech service, when
// the user has configured one. It is off unless tts.json in Agent Desk's
// config folder holds a provider and an API key the user put there; Agent
// Desk never asks for the key, never sends it to the page, and never logs
// it. Each call sends one reply's text, without its code, to the
// provider's speech endpoint, and nothing else; it costs the user's
// credits. ElevenLabs is the provider supported.
package tts

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Config is tts.json.
type Config struct {
	// Provider is "elevenlabs".
	Provider string `json:"provider"`
	APIKey   string `json:"apiKey"`
	// VoiceID is the ElevenLabs voice; a stock voice when empty.
	VoiceID string `json:"voiceId"`
	// Model is the ElevenLabs model; eleven_multilingual_v2 when empty.
	Model string `json:"model"`
}

// Status is what the page may know: whether read aloud is available, and
// with what. It never carries the key.
type Status struct {
	Available bool   `json:"available"`
	Provider  string `json:"provider,omitempty"`
	Voice     string `json:"voice,omitempty"`
	// Problem says why a tts.json that exists cannot be used.
	Problem string `json:"problem,omitempty"`
}

const (
	ProviderElevenLabs = "elevenlabs"
	elevenLabsAPI      = "https://api.elevenlabs.io"
	defaultVoice       = "21m00Tcm4TlvDq8ikWAM"
	defaultModel       = "eleven_multilingual_v2"
	// MaxChars bounds the text of one request, after code is taken out.
	MaxChars = 4000
	// perMinute bounds requests, so a stuck client cannot spend credits.
	perMinute = 20
)

var (
	validVoice = regexp.MustCompile(`^[A-Za-z0-9]{1,64}$`)
	validModel = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

	// ErrNotConfigured is a request with no usable tts.json.
	ErrNotConfigured = errors.New("read aloud is not set up")
	// ErrBusy is a request over the rate limit.
	ErrBusy = errors.New("too many read-aloud requests in the last minute")
	// ErrEmpty is text with nothing to say once code is taken out.
	ErrEmpty = errors.New("this reply has nothing to read aloud")
)

// Speaker turns text into MP3 audio with the provider in tts.json.
type Speaker struct {
	// Path is tts.json.
	Path   string
	Client *http.Client
	// base is the provider's address; tests point it elsewhere.
	base string

	mu     sync.Mutex
	cache  []cached
	recent []time.Time
}

type cached struct {
	key   [32]byte
	audio []byte
}

// cacheItems bounds the audio kept, so a reply played again costs nothing.
const cacheItems = 24

// load reads tts.json, refusing one others can read.
func (s *Speaker) load() (Config, string, error) {
	info, err := os.Stat(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, "", ErrNotConfigured
	}
	if err != nil {
		return Config{}, "", err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return Config{}, "tts.json holds an API key, so it must be readable only by you: chmod 600 " + s.Path, ErrNotConfigured
	}
	data, err := os.ReadFile(s.Path)
	if err != nil {
		return Config{}, "", err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, "tts.json is not valid JSON", ErrNotConfigured
	}
	switch {
	case c.Provider != ProviderElevenLabs:
		return Config{}, `tts.json needs "provider": "elevenlabs"`, ErrNotConfigured
	case strings.TrimSpace(c.APIKey) == "":
		return Config{}, `tts.json needs "apiKey"`, ErrNotConfigured
	}
	if c.VoiceID == "" {
		c.VoiceID = defaultVoice
	}
	if c.Model == "" {
		c.Model = defaultModel
	}
	if !validVoice.MatchString(c.VoiceID) || !validModel.MatchString(c.Model) {
		return Config{}, "tts.json has a voiceId or model that is not a plain identifier", ErrNotConfigured
	}
	return c, "", nil
}

func (s *Speaker) Status() Status {
	c, problem, err := s.load()
	if err != nil {
		return Status{Problem: problem}
	}
	return Status{Available: true, Provider: c.Provider, Voice: c.VoiceID}
}

// Speak returns MP3 audio of text, read by the configured voice. Code is
// taken out first (see Prepare), and the same text is served from memory
// when it was read recently.
func (s *Speaker) Speak(ctx context.Context, text string) ([]byte, error) {
	c, problem, err := s.load()
	if err != nil {
		if problem != "" {
			return nil, fmt.Errorf("%w: %s", err, problem)
		}
		return nil, err
	}
	text = Prepare(text)
	if text == "" {
		return nil, ErrEmpty
	}
	key := sha256.Sum256([]byte(c.Provider + "\x00" + c.VoiceID + "\x00" + c.Model + "\x00" + text))
	if audio := s.cached(key); audio != nil {
		return audio, nil
	}
	if !s.allow(time.Now()) {
		return nil, ErrBusy
	}
	audio, err := s.elevenLabs(ctx, c, text)
	if err != nil {
		return nil, err
	}
	s.store(key, audio)
	return audio, nil
}

func (s *Speaker) elevenLabs(ctx context.Context, c Config, text string) ([]byte, error) {
	body, _ := json.Marshal(map[string]string{"text": text, "model_id": c.Model})
	base := s.base
	if base == "" {
		base = elevenLabsAPI
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/text-to-speech/"+c.VoiceID+"?output_format=mp3_44100_128", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("xi-api-key", c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/mpeg")
	res, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach ElevenLabs: %w", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, providerError(res.StatusCode, data)
	}
	return data, nil
}

// providerError explains a refusal from ElevenLabs without repeating
// anything the request carried.
func providerError(code int, body []byte) error {
	var e struct {
		Detail struct {
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"detail"`
	}
	_ = json.Unmarshal(body, &e)
	switch code {
	case http.StatusUnauthorized:
		return errors.New("ElevenLabs refused the API key in tts.json")
	case http.StatusTooManyRequests:
		return errors.New("ElevenLabs is rate limiting or the account is out of credits")
	case http.StatusNotFound:
		return errors.New("ElevenLabs has no voice with the voiceId in tts.json")
	}
	msg := e.Detail.Message
	if len(msg) > 200 {
		msg = msg[:200]
	}
	if msg == "" {
		return fmt.Errorf("ElevenLabs answered %d", code)
	}
	return fmt.Errorf("ElevenLabs answered %d: %s", code, msg)
}

// client calls only the provider: no redirects, and a time limit.
func (s *Speaker) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{
		Timeout: 90 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (s *Speaker) cached(key [32]byte) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.cache {
		if c.key == key {
			return c.audio
		}
	}
	return nil
}

func (s *Speaker) store(key [32]byte, audio []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = append(s.cache, cached{key: key, audio: audio})
	if len(s.cache) > cacheItems {
		s.cache = s.cache[len(s.cache)-cacheItems:]
	}
}

// allow admits a request when fewer than perMinute were made in the
// minute before now.
func (s *Speaker) allow(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.recent[:0]
	for _, t := range s.recent {
		if now.Sub(t) < time.Minute {
			kept = append(kept, t)
		}
	}
	s.recent = kept
	if len(s.recent) >= perMinute {
		return false
	}
	s.recent = append(s.recent, now)
	return true
}

var (
	fenceOpen  = regexp.MustCompile("^[ \t]*(`{3,}|~{3,})")
	preBlock   = regexp.MustCompile(`(?is)<pre[\s>].*?(</pre>|$)`)
	indent     = regexp.MustCompile(`(?m)^[ \t]+`)
	listItem   = regexp.MustCompile(`^[ \t]*([-*+]|\d+[.)])[ \t]`)
	inlineCode = regexp.MustCompile("`([^`]*)`")
	plainWord  = regexp.MustCompile(`^\pL{1,24}$`)
	// spaceBeforeMark is spaces before punctuation, on one line.
	spaceBeforeMark = regexp.MustCompile(`[ \t]+([.,;:!?])`)
	mdLink          = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	mdMarks         = regexp.MustCompile(`(?m)^[ \t]{0,3}(#{1,6}[ \t]+|>[ \t]?|[-*+][ \t]+|\d+[.)][ \t]+)`)
	emphasis        = regexp.MustCompile(`(\*\*|__|\*|_|~~)(\S[^*_~]*?\S|\S)(\*\*|__|\*|_|~~)`)
	tableRow        = regexp.MustCompile(`(?m)^\s*\|.*\|\s*$`)
	spaces          = regexp.MustCompile(`[ \t]+`)
	blankLines      = regexp.MustCompile(`\n{3,}`)
)

// Prepare turns a reply's Markdown into text to read: code blocks (fenced
// with backticks or tildes, indented, or in <pre>) become "Code omitted.",
// tables are left out, inline code is read only when it is one plain word
// (see spokenCode), links and emphasis keep only their words, and the
// result is cut at a sentence end within MaxChars.
func Prepare(md string) string {
	t := withoutCode(md)
	// With code gone, indentation only nests lists.
	t = indent.ReplaceAllString(t, "")
	t = tableRow.ReplaceAllString(t, "")
	t = inlineCode.ReplaceAllStringFunc(t, spokenCode)
	// A span left out leaves a space before the punctuation after it.
	t = spaceBeforeMark.ReplaceAllString(t, "$1")
	t = mdLink.ReplaceAllString(t, "$1")
	t = mdMarks.ReplaceAllString(t, "")
	for range 3 {
		t = emphasis.ReplaceAllString(t, "$2")
	}
	t = spaces.ReplaceAllString(t, " ")
	t = blankLines.ReplaceAllString(t, "\n\n")
	t = strings.TrimSpace(t)
	if len(t) <= MaxChars {
		return t
	}
	cut := t[:MaxChars]
	if i := strings.LastIndexAny(cut, ".!?\n"); i > MaxChars/2 {
		cut = cut[:i+1]
	}
	return strings.TrimSpace(cut)
}

// spokenCode reads an inline code span as its word when it is one plain
// word of letters, such as `parser`, and leaves it out otherwise: a span
// with digits, symbols, a path, a command or a key never leaves the desk.
func spokenCode(span string) string {
	word := strings.Trim(span, "`")
	if plainWord.MatchString(word) {
		return word
	}
	return ""
}

// withoutCode replaces each code block with "Code omitted.": fenced blocks
// at any indent, closed by a fence of the same kind at least as long or
// left open to the end; indented blocks after a blank line, unless they
// continue a list; and HTML <pre> blocks.
func withoutCode(md string) string {
	md = preBlock.ReplaceAllString(md, "\nCode omitted.\n")
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	var out []string
	omit := func() {
		if n := len(out); n < 2 || out[n-2] != "Code omitted." {
			out = append(out, "", "Code omitted.", "")
		}
	}
	inList := false
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if m := fenceOpen.FindStringSubmatch(l); m != nil {
			mark := m[1]
			for i++; i < len(lines); i++ {
				c := strings.TrimLeft(lines[i], " \t")
				if strings.HasPrefix(c, mark) && strings.Trim(c, mark[:1]+" \t") == "" {
					break
				}
			}
			omit()
			continue
		}
		blankBefore := i == 0 || strings.TrimSpace(lines[i-1]) == ""
		if blankBefore && indented(l) && !inList && !listItem.MatchString(l) {
			for i+1 < len(lines) && (indented(lines[i+1]) || strings.TrimSpace(lines[i+1]) == "") {
				i++
			}
			omit()
			continue
		}
		switch {
		case listItem.MatchString(l):
			inList = true
		case strings.TrimSpace(l) != "" && !indented(l):
			inList = false
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// indented is a line that starts with four spaces or a tab and has text.
func indented(l string) bool {
	return strings.TrimSpace(l) != "" && (strings.HasPrefix(l, "    ") || strings.HasPrefix(l, "\t"))
}

//go:embed silence.mp3
var silence []byte

// Fixture stands in for a provider in fixture mode and tests: it is
// available when its marker file exists, and answers with a short silent
// MP3 without any network call.
type Fixture struct {
	// Marker is the file whose presence turns it on.
	Marker string
}

func (f *Fixture) Status() Status {
	if _, err := os.Stat(f.Marker); err != nil {
		return Status{}
	}
	return Status{Available: true, Provider: "fixture", Voice: "silence"}
}

func (f *Fixture) Speak(_ context.Context, text string) ([]byte, error) {
	if !f.Status().Available {
		return nil, ErrNotConfigured
	}
	if Prepare(text) == "" {
		return nil, ErrEmpty
	}
	return silence, nil
}
