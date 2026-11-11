package server

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/transcript"
)

const testToken = "0123456789abcdef0123456789abcdef"

type fakeSource struct {
	mu       sync.Mutex
	state    hub.State
	ch       chan hub.State
	triggers int
}

func newFakeSource() *fakeSource {
	return &fakeSource{state: hub.State{ManagerAwake: true, Sessions: []hub.SessionView{{ID: "a1", Name: "one", Status: "idle"}}}, ch: make(chan hub.State, 1)}
}

func (f *fakeSource) State() hub.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}
func (f *fakeSource) Subscribe() (<-chan hub.State, func()) { return f.ch, func() {} }
func (f *fakeSource) Trigger() {
	f.mu.Lock()
	f.triggers++
	f.mu.Unlock()
}

type fakeTranscripts struct {
	events chan hub.Event
	before int
	limit  int
}

func newFakeTranscripts() *fakeTranscripts {
	return &fakeTranscripts{events: make(chan hub.Event, 4)}
}

func (f *fakeTranscripts) Window(_ context.Context, id string, before, limit int) hub.Page {
	f.before, f.limit = before, limit
	return hub.Page{SessionID: id, Status: hub.DocOK}
}

func (f *fakeTranscripts) Subscribe(_ context.Context, id string) (hub.Page, <-chan hub.Event, func()) {
	return hub.Page{SessionID: id, Status: hub.DocOK, Items: []transcript.Item{{Seq: 0, Kind: transcript.KindUser, Text: "hi"}}}, f.events, func() {}
}

type fakeManager struct{ err error }

func (m fakeManager) StartServe(context.Context) error { return m.err }

func newTestServer(t *testing.T, cfg Config) *Server {
	t.Helper()
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:7317"
	}
	if cfg.Token == "" {
		cfg.Token = testToken
	}
	if cfg.Sessions == nil {
		cfg.Sessions = newFakeSource()
	}
	if cfg.Manager == nil {
		cfg.Manager = fakeManager{}
	}
	if cfg.Transcripts == nil {
		cfg.Transcripts = newFakeTranscripts()
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNewRejectsShortToken(t *testing.T) {
	if _, err := New(Config{Addr: "127.0.0.1:1", Token: "short", Sessions: newFakeSource(), Manager: fakeManager{}, Transcripts: newFakeTranscripts()}); err == nil {
		t.Fatal("expected error for short token")
	}
}

func TestRequestChecks(t *testing.T) {
	s := newTestServer(t, Config{DevOrigins: []string{"http://localhost:5173"}})
	tests := []struct {
		name   string
		method string
		path   string
		host   string
		origin string
		auth   string
		want   int
	}{
		{"health with token", "GET", "/api/health", "127.0.0.1:7317", "", "Bearer " + testToken, 200},
		{"health via localhost host", "GET", "/api/health", "localhost:7317", "", "Bearer " + testToken, 200},
		{"health without token", "GET", "/api/health", "127.0.0.1:7317", "", "", 401},
		{"health with wrong token", "GET", "/api/health", "127.0.0.1:7317", "", "Bearer " + strings.Repeat("x", 32), 401},
		{"token without bearer prefix", "GET", "/api/health", "127.0.0.1:7317", "", testToken, 401},
		{"foreign host", "GET", "/api/health", "evil.example:7317", "", "Bearer " + testToken, 403},
		{"other port", "GET", "/api/health", "127.0.0.1:8080", "", "Bearer " + testToken, 403},
		{"foreign origin on GET", "GET", "/api/health", "127.0.0.1:7317", "http://evil.example", "Bearer " + testToken, 403},
		{"own origin on GET", "GET", "/api/health", "127.0.0.1:7317", "http://127.0.0.1:7317", "Bearer " + testToken, 200},
		{"dev origin on GET", "GET", "/api/health", "127.0.0.1:7317", "http://localhost:5173", "Bearer " + testToken, 200},
		{"POST without origin", "POST", "/api/anything", "127.0.0.1:7317", "", "Bearer " + testToken, 403},
		{"POST with foreign origin", "POST", "/api/anything", "127.0.0.1:7317", "http://evil.example", "Bearer " + testToken, 403},
		{"POST with own origin, unknown route", "POST", "/api/anything", "127.0.0.1:7317", "http://127.0.0.1:7317", "Bearer " + testToken, 404},
		{"sessions with token", "GET", "/api/sessions", "127.0.0.1:7317", "", "Bearer " + testToken, 200},
		{"sessions without token", "GET", "/api/sessions", "127.0.0.1:7317", "", "", 401},
		{"start manager", "POST", "/api/manager/start", "127.0.0.1:7317", "http://127.0.0.1:7317", "Bearer " + testToken, 200},
		{"start manager without origin", "POST", "/api/manager/start", "127.0.0.1:7317", "", "Bearer " + testToken, 403},
		{"start manager without token", "POST", "/api/manager/start", "127.0.0.1:7317", "http://127.0.0.1:7317", "", 401},
		{"transcript window", "GET", "/api/sessions/a1/transcript?before=10&limit=50", "127.0.0.1:7317", "", "Bearer " + testToken, 200},
		{"transcript window bad before", "GET", "/api/sessions/a1/transcript?before=x", "127.0.0.1:7317", "", "Bearer " + testToken, 400},
		{"transcript window negative before", "GET", "/api/sessions/a1/transcript?before=-5", "127.0.0.1:7317", "", "Bearer " + testToken, 400},
		{"transcript window huge limit", "GET", "/api/sessions/a1/transcript?limit=99999", "127.0.0.1:7317", "", "Bearer " + testToken, 400},
		{"transcript window without token", "GET", "/api/sessions/a1/transcript", "127.0.0.1:7317", "", "", 401},
		{"unknown api route without token", "GET", "/api/anything", "127.0.0.1:7317", "", "", 401},
		{"static without token", "GET", "/", "127.0.0.1:7317", "", "", 200},
		{"static on foreign host", "GET", "/", "evil.example", "", "", 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.Host = tt.host
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestSessionsBody(t *testing.T) {
	s := newTestServer(t, Config{})
	req := httptest.NewRequest("GET", "/api/sessions", nil)
	req.Host = "127.0.0.1:7317"
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var st hub.State
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if !st.ManagerAwake || len(st.Sessions) != 1 || st.Sessions[0].ID != "a1" {
		t.Fatalf("state %+v", st)
	}
}

func TestStartManager(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		wantStatus   int
		wantTriggers int
	}{
		{"started", nil, 200, 1},
		{"failed", errors.New("serve failed"), 502, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := newFakeSource()
			s := newTestServer(t, Config{Sessions: src, Manager: fakeManager{err: tt.err}})
			req := httptest.NewRequest("POST", "/api/manager/start", nil)
			req.Host = "127.0.0.1:7317"
			req.Header.Set("Origin", "http://127.0.0.1:7317")
			req.Header.Set("Authorization", "Bearer "+testToken)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus || src.triggers != tt.wantTriggers {
				t.Fatalf("status %d triggers %d, want %d and %d", rec.Code, src.triggers, tt.wantStatus, tt.wantTriggers)
			}
		})
	}
}

func TestSecurityHeaders(t *testing.T) {
	s := newTestServer(t, Config{})
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "127.0.0.1:7317"
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	for _, h := range []string{"Content-Security-Policy", "X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy"} {
		if rec.Header().Get(h) == "" {
			t.Errorf("missing %s", h)
		}
	}
}

func TestStatic(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":    {Data: []byte("<html>app</html>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
	tests := []struct {
		name   string
		assets fstest.MapFS
		path   string
		want   string
	}{
		{"index", assets, "/", "app"},
		{"asset", assets, "/assets/app.js", "console.log"},
		{"client route falls back to shell", assets, "/session/abc", "app"},
		{"placeholder when not built", fstest.MapFS{".gitkeep": {}}, "/", "not built"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t, Config{Assets: tt.assets})
			req := httptest.NewRequest("GET", tt.path, nil)
			req.Host = "127.0.0.1:7317"
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != 200 || !strings.Contains(rec.Body.String(), tt.want) {
				t.Fatalf("status %d body %q, want 200 containing %q", rec.Code, rec.Body.String(), tt.want)
			}
		})
	}
}

func TestWebSocketTranscript(t *testing.T) {
	tr := newFakeTranscripts()
	addr := startLive(t, Config{Transcripts: tr})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws://"+addr+"/api/ws", &websocket.DialOptions{
		Subprotocols: []string{WSProtocol, TokenProtocolPrefix + testToken},
		HTTPHeader:   http.Header{"Origin": {"http://" + addr}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	var msg map[string]any
	for range 2 { // hello, sessions
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			t.Fatal(err)
		}
	}
	if err := wsjson.Write(ctx, conn, map[string]string{"type": "subscribe", "sessionId": "a1"}); err != nil {
		t.Fatal(err)
	}
	if err := wsjson.Read(ctx, conn, &msg); err != nil {
		t.Fatal(err)
	}
	if msg["type"] != "transcript.page" {
		t.Fatalf("first transcript message = %v", msg)
	}
	tr.events <- hub.Event{SessionID: "a1", Items: []transcript.Item{{Seq: 1, Kind: transcript.KindText, Text: "reply"}}}
	if err := wsjson.Read(ctx, conn, &msg); err != nil {
		t.Fatal(err)
	}
	if msg["type"] != "transcript.append" || msg["sessionId"] != "a1" {
		t.Fatalf("append = %v", msg)
	}
	tr.events <- hub.Event{SessionID: "a1", Reset: true, Page: hub.Page{SessionID: "a1", Status: hub.DocGone}}
	if err := wsjson.Read(ctx, conn, &msg); err != nil {
		t.Fatal(err)
	}
	if msg["type"] != "transcript.page" {
		t.Fatalf("reset = %v", msg)
	}
}

func startLive(t *testing.T, cfg Config) string {
	if cfg.Sessions == nil {
		cfg.Sessions = newFakeSource()
	}
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Addr = l.Addr().String()
	s := newTestServer(t, cfg)
	srv := &httptest.Server{Listener: l, Config: &http.Server{Handler: s.Handler()}}
	srv.Start()
	t.Cleanup(srv.Close)
	return cfg.Addr
}

func TestWebSocketPushesUpdates(t *testing.T) {
	src := newFakeSource()
	addr := startLive(t, Config{Sessions: src})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws://"+addr+"/api/ws", &websocket.DialOptions{
		Subprotocols: []string{WSProtocol, TokenProtocolPrefix + testToken},
		HTTPHeader:   http.Header{"Origin": {"http://" + addr}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	var msg struct {
		Type  string    `json:"type"`
		State hub.State `json:"state"`
	}
	for range 2 { // hello, initial sessions
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			t.Fatal(err)
		}
	}
	src.ch <- hub.State{Sessions: []hub.SessionView{{ID: "b2", Status: "waiting"}}}
	if err := wsjson.Read(ctx, conn, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Type != "sessions" || msg.State.Sessions[0].Status != "waiting" {
		t.Fatalf("update = %+v", msg)
	}
}

func TestWebSocket(t *testing.T) {
	addr := startLive(t, Config{Version: "test"})
	own := "http://" + addr
	tests := []struct {
		name      string
		origin    string
		protocols []string
		wantOK    bool
	}{
		{"token and own origin", own, []string{WSProtocol, TokenProtocolPrefix + testToken}, true},
		{"no token", own, []string{WSProtocol}, false},
		{"wrong token", own, []string{WSProtocol, TokenProtocolPrefix + strings.Repeat("x", 32)}, false},
		{"foreign origin", "http://evil.example", []string{WSProtocol, TokenProtocolPrefix + testToken}, false},
		{"no origin", "", []string{WSProtocol, TokenProtocolPrefix + testToken}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			header := http.Header{}
			if tt.origin != "" {
				header.Set("Origin", tt.origin)
			}
			conn, _, err := websocket.Dial(ctx, "ws://"+addr+"/api/ws", &websocket.DialOptions{
				Subprotocols: tt.protocols,
				HTTPHeader:   header,
			})
			if !tt.wantOK {
				if err == nil {
					conn.CloseNow()
					t.Fatal("dial succeeded, want refusal")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer conn.CloseNow()
			if conn.Subprotocol() != WSProtocol {
				t.Fatalf("subprotocol = %q", conn.Subprotocol())
			}
			var hello map[string]any
			if err := wsjson.Read(ctx, conn, &hello); err != nil {
				t.Fatal(err)
			}
			if hello["type"] != "hello" || hello["version"] != "test" {
				t.Fatalf("hello = %v", hello)
			}
			var first struct {
				Type  string    `json:"type"`
				State hub.State `json:"state"`
			}
			if err := wsjson.Read(ctx, conn, &first); err != nil {
				t.Fatal(err)
			}
			if first.Type != "sessions" || len(first.State.Sessions) != 1 {
				t.Fatalf("first message = %+v", first)
			}
		})
	}
}
