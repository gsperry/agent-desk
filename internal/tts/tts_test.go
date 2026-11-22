package tts

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, mode os.FileMode, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "tts.json")
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestStatus(t *testing.T) {
	tests := []struct {
		name      string
		mode      os.FileMode
		body      string
		available bool
		problem   string
	}{
		{"configured", 0o600, `{"provider":"elevenlabs","apiKey":"sk-test","voiceId":"abc123"}`, true, ""},
		{"readable by others", 0o644, `{"provider":"elevenlabs","apiKey":"sk-test"}`, false, "chmod 600"},
		{"no key", 0o600, `{"provider":"elevenlabs"}`, false, "apiKey"},
		{"another provider", 0o600, `{"provider":"other","apiKey":"k"}`, false, "provider"},
		{"not JSON", 0o600, `{`, false, "not valid JSON"},
		{"a voice that is not an id", 0o600, `{"provider":"elevenlabs","apiKey":"k","voiceId":"../x"}`, false, "plain identifier"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := (&Speaker{Path: writeConfig(t, tt.mode, tt.body)}).Status()
			if st.Available != tt.available || !strings.Contains(st.Problem, tt.problem) {
				t.Fatalf("status = %+v", st)
			}
			// The key never reaches the status the page reads.
			data, _ := json.Marshal(st)
			if strings.Contains(string(data), "sk-test") {
				t.Fatal("status carries the key")
			}
		})
	}
	if st := (&Speaker{Path: filepath.Join(t.TempDir(), "none.json")}).Status(); st.Available || st.Problem != "" {
		t.Fatalf("missing file = %+v", st)
	}
}

// fakeElevenLabs records requests and answers with audio or an error.
type fakeElevenLabs struct {
	calls  int
	path   string
	key    string
	body   map[string]string
	status int
}

func (f *fakeElevenLabs) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.calls++
	f.path = r.URL.RequestURI()
	f.key = r.Header.Get("xi-api-key")
	data, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(data, &f.body)
	if f.status != 0 {
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(`{"detail":{"status":"x","message":"quota exceeded"}}`))
		return
	}
	w.Header().Set("Content-Type", "audio/mpeg")
	_, _ = w.Write([]byte("ID3fake-mp3"))
}

func TestSpeak(t *testing.T) {
	fake := &fakeElevenLabs{}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	s := &Speaker{Path: writeConfig(t, 0o600, `{"provider":"elevenlabs","apiKey":"sk-test","voiceId":"abc123","model":"eleven_flash_v2_5"}`), base: srv.URL}
	audio, err := s.Speak(context.Background(), "Done. Here is the fix:\n\n```go\nfunc main() {}\n```\n\nAll **tests** pass.")
	if err != nil || string(audio) != "ID3fake-mp3" {
		t.Fatalf("audio = %q, err = %v", audio, err)
	}
	if fake.path != "/v1/text-to-speech/abc123?output_format=mp3_44100_128" || fake.key != "sk-test" || fake.body["model_id"] != "eleven_flash_v2_5" {
		t.Fatalf("request = %s key=%q body=%v", fake.path, fake.key, fake.body)
	}
	if strings.Contains(fake.body["text"], "func main") || !strings.Contains(fake.body["text"], "Code omitted.") || strings.Contains(fake.body["text"], "**") {
		t.Fatalf("sent text = %q", fake.body["text"])
	}
	// The same reply again comes from memory.
	if _, err := s.Speak(context.Background(), "Done. Here is the fix:\n\n```go\nfunc main() {}\n```\n\nAll **tests** pass."); err != nil || fake.calls != 1 {
		t.Fatalf("second call: err=%v calls=%d", err, fake.calls)
	}
}

func TestSpeakRefusals(t *testing.T) {
	fake := &fakeElevenLabs{status: http.StatusUnauthorized}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	cfg := writeConfig(t, 0o600, `{"provider":"elevenlabs","apiKey":"sk-secret"}`)
	s := &Speaker{Path: cfg, base: srv.URL}
	_, err := s.Speak(context.Background(), "hello")
	if err == nil || !strings.Contains(err.Error(), "refused the API key") || strings.Contains(err.Error(), "sk-secret") {
		t.Fatalf("err = %v", err)
	}
	fake.status = http.StatusTooManyRequests
	if _, err := s.Speak(context.Background(), "hello there"); err == nil || !strings.Contains(err.Error(), "credits") {
		t.Fatalf("err = %v", err)
	}
	if _, err := s.Speak(context.Background(), "```\nonly code\n```"); err == nil {
		t.Fatal("code only was spoken")
	}
	if _, err := (&Speaker{Path: filepath.Join(t.TempDir(), "x.json")}).Speak(context.Background(), "hi"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v", err)
	}
}

func TestNoRedirects(t *testing.T) {
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("followed a redirect")
	}))
	defer elsewhere.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL, http.StatusFound)
	}))
	defer srv.Close()
	s := &Speaker{Path: writeConfig(t, 0o600, `{"provider":"elevenlabs","apiKey":"k"}`), base: srv.URL}
	if _, err := s.Speak(context.Background(), "hi"); err == nil {
		t.Fatal("a redirect was taken as audio")
	}
}

func TestRateLimit(t *testing.T) {
	s := &Speaker{}
	now := time.Now()
	for i := range perMinute {
		if !s.allow(now.Add(time.Duration(i) * time.Millisecond)) {
			t.Fatalf("request %d refused", i)
		}
	}
	if s.allow(now.Add(time.Second)) {
		t.Fatal("over the limit was allowed")
	}
	if !s.allow(now.Add(61 * time.Second)) {
		t.Fatal("refused after a minute")
	}
}

func TestPrepare(t *testing.T) {
	tests := []struct{ in, want string }{
		{"## Summary\n\n- **Fixed** the `parser`.\n- See [the docs](https://x.y).", "Summary\n\nFixed the parser.\nSee the docs."},
		{"Before\n```\ncode\n```\nAfter", "Before\n\nCode omitted.\n\nAfter"},
		{"| a | b |\n|---|---|\n| 1 | 2 |\nText", "Text"},
		{"```\nunclosed code", "Code omitted."},
	}
	for _, tt := range tests {
		if got := Prepare(tt.in); got != tt.want {
			t.Errorf("Prepare(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	long := strings.Repeat("A sentence here. ", 400)
	if got := Prepare(long); len(got) > MaxChars || !strings.HasSuffix(got, ".") {
		t.Errorf("long text: %d chars, ends %q", len(got), got[len(got)-5:])
	}
}

func TestFixture(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "tts.json")
	f := &Fixture{Marker: marker}
	if f.Status().Available {
		t.Fatal("available without its marker")
	}
	_ = os.WriteFile(marker, []byte("{}"), 0o600)
	audio, err := f.Speak(context.Background(), "hello")
	if err != nil || len(audio) < 100 || !f.Status().Available {
		t.Fatalf("fixture: %d bytes, %v", len(audio), err)
	}
}
