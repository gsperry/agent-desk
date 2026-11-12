package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/gsperry/agent-desk/internal/commands"
	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/remote"
)

type fakeRemote struct {
	mu  sync.Mutex
	cur remote.Settings
}

func (f *fakeRemote) Get() remote.Settings {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cur
}

func (f *fakeRemote) Set(s remote.Settings) (remote.Settings, error) {
	v, err := s.Validate()
	if err != nil {
		return remote.Settings{}, err
	}
	f.mu.Lock()
	f.cur = v
	f.mu.Unlock()
	return v, nil
}

func (f *fakeRemote) Status() RemoteStatus {
	return RemoteStatus{Listening: f.Get().Enabled, Address: "unix:/tmp/test.sock"}
}

const remoteHost = "desk.tail1234.ts.net"

func remoteOn(level remote.Level) *fakeRemote {
	return &fakeRemote{cur: remote.Settings{Enabled: true, Level: level, Host: remoteHost, Port: remote.DefaultPort, TailscaleLogins: []string{"dev@example.com"}}}
}

type remoteReq struct {
	method, path, body string
	host, origin, user string
}

func doRemote(t *testing.T, s *Server, q remoteReq) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(q.method, q.path, strings.NewReader(q.body))
	req.Host = q.host
	if q.origin != "" {
		req.Header.Set("Origin", q.origin)
	}
	if q.user != "" {
		req.Header.Set(remote.TailscaleHeader, q.user)
	}
	if q.body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	s.RemoteHandler().ServeHTTP(rec, req)
	return rec
}

func TestRemoteAdmission(t *testing.T) {
	const origin = "https://" + remoteHost
	tests := []struct {
		name string
		rc   *fakeRemote
		q    remoteReq
		want int
	}{
		{"read", remoteOn(remote.LevelAnswer), remoteReq{"GET", "/api/sessions", "", remoteHost, "", "dev@example.com"}, 200},
		{"app shell", remoteOn(remote.LevelAnswer), remoteReq{"GET", "/", "", remoteHost, "", "dev@example.com"}, 200},
		{"off", &fakeRemote{cur: remote.Defaults()}, remoteReq{"GET", "/api/sessions", "", remoteHost, "", "dev@example.com"}, 404},
		{"wrong host", remoteOn(remote.LevelAnswer), remoteReq{"GET", "/api/sessions", "", "127.0.0.1:7327", "", "dev@example.com"}, 403},
		{"no identity", remoteOn(remote.LevelAnswer), remoteReq{"GET", "/api/sessions", "", remoteHost, "", ""}, 401},
		{"login not allowed", remoteOn(remote.LevelAnswer), remoteReq{"GET", "/api/sessions", "", remoteHost, "", "eve@example.com"}, 401},
		{"shell needs identity too", remoteOn(remote.LevelAnswer), remoteReq{"GET", "/", "", remoteHost, "", ""}, 401},
		{"foreign origin", remoteOn(remote.LevelManage), remoteReq{"POST", "/api/sessions/a1/answer", `{"action":"key","key":"Escape"}`, remoteHost, "https://evil.example", "dev@example.com"}, 403},
		{"state change needs origin", remoteOn(remote.LevelManage), remoteReq{"POST", "/api/sessions/a1/answer", `{"action":"key","key":"Escape"}`, remoteHost, "", "dev@example.com"}, 403},
		{"answer at answer", remoteOn(remote.LevelAnswer), remoteReq{"POST", "/api/sessions/a1/answer", `{"action":"key","key":"Escape"}`, remoteHost, origin, "dev@example.com"}, 200},
		{"reply refused at answer", remoteOn(remote.LevelAnswer), remoteReq{"POST", "/api/sessions/a1/send", `{"text":"hi"}`, remoteHost, origin, "dev@example.com"}, 403},
		{"reply at reply", remoteOn(remote.LevelReply), remoteReq{"POST", "/api/sessions/a1/send", `{"text":"hi"}`, remoteHost, origin, "dev@example.com"}, 200},
		{"interrupt refused at answer", remoteOn(remote.LevelAnswer), remoteReq{"POST", "/api/sessions/a1/interrupt", "", remoteHost, origin, "dev@example.com"}, 403},
		{"interrupt at reply", remoteOn(remote.LevelReply), remoteReq{"POST", "/api/sessions/a1/interrupt", "", remoteHost, origin, "dev@example.com"}, 200},
		{"spawn refused at reply", remoteOn(remote.LevelReply), remoteReq{"POST", "/api/sessions", `{"directory":"/tmp"}`, remoteHost, origin, "dev@example.com"}, 403},
		{"spawn at manage", remoteOn(remote.LevelManage), remoteReq{"POST", "/api/sessions", `{"directory":"/tmp"}`, remoteHost, origin, "dev@example.com"}, 200},
		{"action at manage", remoteOn(remote.LevelManage), remoteReq{"POST", "/api/sessions/a1/action", `{"action":"kill"}`, remoteHost, origin, "dev@example.com"}, 200},
		{"paste never remote", remoteOn(remote.LevelManage), remoteReq{"POST", "/api/paste", "png", remoteHost, origin, "dev@example.com"}, 403},
		{"open never remote", remoteOn(remote.LevelManage), remoteReq{"POST", "/api/open", `{"path":"x"}`, remoteHost, origin, "dev@example.com"}, 403},
		{"summary never remote", remoteOn(remote.LevelManage), remoteReq{"POST", "/api/sessions/a1/summary", `{"since":1,"model":"haiku"}`, remoteHost, origin, "dev@example.com"}, 403},
		{"settings never remote", remoteOn(remote.LevelManage), remoteReq{"POST", "/api/remote", `{"enabled":false}`, remoteHost, origin, "dev@example.com"}, 403},
		{"http origin refused", remoteOn(remote.LevelManage), remoteReq{"POST", "/api/sessions/a1/answer", `{"action":"key","key":"Escape"}`, remoteHost, "http://" + remoteHost, "dev@example.com"}, 403},
		{"host with another port", remoteOn(remote.LevelAnswer), remoteReq{"GET", "/api/sessions", "", remoteHost + ":443", "", "dev@example.com"}, 403},
		{"host in other case", remoteOn(remote.LevelAnswer), remoteReq{"GET", "/api/sessions", "", "DESK.tail1234.ts.net", "", "dev@example.com"}, 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t, Config{Remote: tt.rc})
			rec := doRemote(t, s, tt.q)
			if rec.Code != tt.want {
				t.Fatalf("status %d, want %d: %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestRemoteSettingsStayOnTheDesk(t *testing.T) {
	rc := remoteOn(remote.LevelReply)
	s := newTestServer(t, Config{Remote: rc})
	// A remote client sees the level, not the allowed logins or host.
	rec := doRemote(t, s, remoteReq{"GET", "/api/remote", "", remoteHost, "", "dev@example.com"})
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "dev@example.com") || !strings.Contains(rec.Body.String(), `"level":"reply"`) {
		t.Fatalf("remote view: %d %s", rec.Code, rec.Body.String())
	}
	rec = doRemote(t, s, remoteReq{"GET", "/api/whoami", "", remoteHost, "", "dev@example.com"})
	if !strings.Contains(rec.Body.String(), `"remote":true`) || !strings.Contains(rec.Body.String(), `"user":"dev@example.com"`) {
		t.Fatalf("remote whoami: %s", rec.Body.String())
	}
	// The local client can change them, with its token and origin.
	req := httptest.NewRequest("POST", "/api/remote", strings.NewReader(`{"enabled":true,"level":"manage","host":"`+remoteHost+`","tailscaleLogins":["dev@example.com"]}`))
	req.Host = "127.0.0.1:7317"
	req.Header.Set("Origin", "http://127.0.0.1:7317")
	req.Header.Set("Authorization", "Bearer "+testToken)
	lrec := httptest.NewRecorder()
	s.Handler().ServeHTTP(lrec, req)
	if lrec.Code != 200 || rc.Get().Level != remote.LevelManage {
		t.Fatalf("local set: %d %s", lrec.Code, lrec.Body.String())
	}
	// Invalid settings are refused and leave the old ones.
	req = httptest.NewRequest("POST", "/api/remote", strings.NewReader(`{"enabled":true,"level":"manage","host":"","tailscaleLogins":[]}`))
	req.Host = "127.0.0.1:7317"
	req.Header.Set("Origin", "http://127.0.0.1:7317")
	req.Header.Set("Authorization", "Bearer "+testToken)
	lrec = httptest.NewRecorder()
	s.Handler().ServeHTTP(lrec, req)
	if lrec.Code != 400 || rc.Get().Host != remoteHost {
		t.Fatalf("invalid set: %d %s", lrec.Code, lrec.Body.String())
	}
	// The local whoami says local.
	req = httptest.NewRequest("GET", "/api/whoami", nil)
	req.Host = "127.0.0.1:7317"
	req.Header.Set("Authorization", "Bearer "+testToken)
	lrec = httptest.NewRecorder()
	s.Handler().ServeHTTP(lrec, req)
	if !strings.Contains(lrec.Body.String(), `"remote":false`) {
		t.Fatalf("local whoami: %s", lrec.Body.String())
	}
}

func TestRemoteIdentityDoesNotWorkLocally(t *testing.T) {
	s := newTestServer(t, Config{Remote: remoteOn(remote.LevelManage)})
	// The local listener ignores the Tailscale header: no token, no entry.
	req := httptest.NewRequest("GET", "/api/sessions", nil)
	req.Host = "127.0.0.1:7317"
	req.Header.Set(remote.TailscaleHeader, "dev@example.com")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("local with identity header, no token: %d", rec.Code)
	}
	// And the remote listener ignores the token: no identity, no entry.
	req = httptest.NewRequest("GET", "/api/sessions", nil)
	req.Host = remoteHost
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec = httptest.NewRecorder()
	s.RemoteHandler().ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("remote with token, no identity: %d", rec.Code)
	}
}

func TestRemoteWebSocket(t *testing.T) {
	s := newTestServer(t, Config{Remote: remoteOn(remote.LevelAnswer)})
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Stand in for the proxy, which forwards the tailnet host name.
		r.Host = remoteHost
		s.RemoteHandler().ServeHTTP(w, r)
	}), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dial := func(user string) (*websocket.Conn, error) {
		h := http.Header{"Origin": {"https://" + remoteHost}}
		if user != "" {
			h.Set(remote.TailscaleHeader, user)
		}
		c, _, err := websocket.Dial(ctx, "ws://"+l.Addr().String()+"/api/ws", &websocket.DialOptions{Subprotocols: []string{WSProtocol}, HTTPHeader: h})
		return c, err
	}
	if _, err := dial(""); err == nil {
		t.Fatal("WebSocket opened without identity")
	}
	for _, origin := range []string{"", "https://evil.example", "http://" + remoteHost} {
		h := http.Header{remote.TailscaleHeader: {"dev@example.com"}}
		if origin != "" {
			h.Set("Origin", origin)
		}
		if c, _, err := websocket.Dial(ctx, "ws://"+l.Addr().String()+"/api/ws", &websocket.DialOptions{Subprotocols: []string{WSProtocol}, HTTPHeader: h}); err == nil {
			c.CloseNow()
			t.Fatalf("WebSocket opened with origin %q", origin)
		}
	}
	conn, err := dial("dev@example.com")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	var msg map[string]any
	if err := wsjson.Read(ctx, conn, &msg); err != nil || msg["type"] != "hello" {
		t.Fatalf("hello = %v, %v", msg, err)
	}
	// Turning remote access off closes the open socket.
	rc := s.cfg.Remote.(*fakeRemote)
	off := rc.Get()
	off.Enabled = false
	rc.mu.Lock()
	rc.cur = off
	rc.mu.Unlock()
	s.remoteMu.Lock()
	tracked := len(s.remoteConns)
	s.remoteMu.Unlock()
	if tracked != 1 {
		t.Fatalf("tracked remote sockets = %d, want 1", tracked)
	}
	s.DropRemote(off)
	readCtx, cancelRead := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelRead()
	for {
		if err := wsjson.Read(readCtx, conn, &msg); err != nil {
			if readCtx.Err() != nil {
				t.Fatal("the socket stayed open after remote access was turned off")
			}
			break
		}
	}
}

func TestRemoteCannotChangeSettings(t *testing.T) {
	rc := remoteOn(remote.LevelManage)
	s := newTestServer(t, Config{Remote: rc})
	rec := doRemote(t, s, remoteReq{"POST", "/api/remote", `{"enabled":false}`, remoteHost, "https://" + remoteHost, "dev@example.com"})
	if rec.Code != 403 || !rc.Get().Enabled {
		t.Fatalf("status %d, settings %+v", rec.Code, rc.Get())
	}
	// The handler refuses a remote caller even if it were routed to it.
	req := httptest.NewRequest("POST", "/api/remote", strings.NewReader(`{"enabled":false}`))
	req = req.WithContext(context.WithValue(req.Context(), remoteKey{}, remoteCaller{id: remote.Identity{User: "dev@example.com"}}))
	w := httptest.NewRecorder()
	s.setRemoteSettings(w, req)
	if w.Code != 403 || !rc.Get().Enabled {
		t.Fatalf("handler check: %d %+v", w.Code, rc.Get())
	}
}

// TestRouteNeeds pins what every route needs from a remote client, so a new
// route that changes state cannot slip in as a read.
func TestRouteNeeds(t *testing.T) {
	want := map[string]remote.Need{
		"GET /api/health":                                  remote.NeedRead,
		"GET /api/whoami":                                  remote.NeedRead,
		"GET /api/sessions":                                remote.NeedRead,
		"GET /api/sessions/{id}/transcript":                remote.NeedRead,
		"POST /api/sessions/{id}/send":                     remote.NeedReply,
		"GET /api/sessions/{id}/pane":                      remote.NeedRead,
		"GET /api/sessions/{id}/terminal":                  remote.NeedRead,
		"POST /api/sessions/{id}/answer":                   remote.NeedAnswer,
		"POST /api/sessions/{id}/interrupt":                remote.NeedReply,
		"GET /api/sessions/{id}/agents":                    remote.NeedRead,
		"GET /api/sessions/{id}/agents/{agent}/transcript": remote.NeedRead,
		"GET /api/sessions/{id}/changes":                   remote.NeedRead,
		"GET /api/sessions/{id}/commands":                  remote.NeedRead,
		"POST /api/paste":                                  remote.NeedLocal,
		"GET /api/conflicts":                               remote.NeedRead,
		"GET /api/conversations":                           remote.NeedRead,
		"GET /api/conversations/{id}/transcript":           remote.NeedRead,
		"GET /api/conversations/{id}/export":               remote.NeedRead,
		"POST /api/sessions/{id}/summary":                  remote.NeedLocal,
		"GET /api/usage":                                   remote.NeedRead,
		"GET /api/overview":                                remote.NeedRead,
		"GET /api/spawn-options":                           remote.NeedManage,
		"POST /api/sessions":                               remote.NeedManage,
		"POST /api/sessions/{id}/action":                   remote.NeedManage,
		"POST /api/groups":                                 remote.NeedManage,
		"POST /api/groups/delete":                          remote.NeedManage,
		"GET /api/search":                                  remote.NeedRead,
		"POST /api/open":                                   remote.NeedLocal,
		"POST /api/manager/start":                          remote.NeedManage,
		"GET /api/remote":                                  remote.NeedRead,
		"POST /api/remote":                                 remote.NeedLocal,
		"GET /api/ws":                                      remote.NeedRead,
	}
	s := newTestServer(t, Config{})
	for p, n := range s.needs {
		w, ok := want[p]
		if !ok {
			t.Errorf("route %s has no expected need here; add it deliberately", p)
		} else if w != n {
			t.Errorf("route %s needs %d, want %d", p, n, w)
		}
	}
	for p := range want {
		if _, ok := s.needs[p]; !ok {
			t.Errorf("expected route %s is not registered", p)
		}
	}
	// Every non-GET route needs more than reading.
	for p, n := range s.needs {
		if !strings.HasPrefix(p, "GET ") && n == remote.NeedRead {
			t.Errorf("%s changes state but needs only read", p)
		}
	}
}

func TestStillAllowed(t *testing.T) {
	on := remote.Settings{Enabled: true, Host: remoteHost, TailscaleLogins: []string{"dev@example.com"}}
	off := on
	off.Enabled = false
	moved := on
	moved.Host = "other.ts.net"
	removed := on
	removed.TailscaleLogins = []string{"someone@example.com"}
	tests := []struct {
		name string
		st   remote.Settings
		want bool
	}{
		{"unchanged", on, true},
		{"turned off", off, false},
		{"host changed", moved, false},
		{"login removed", removed, false},
	}
	for _, tt := range tests {
		if got := stillAllowed(tt.st, "dev@example.com", remoteHost); got != tt.want {
			t.Errorf("%s: %v", tt.name, got)
		}
	}
}

func TestRemoteHostThroughSocket(t *testing.T) {
	tests := []struct {
		name     string
		onSocket bool
		host     string
		forward  []string
		want     int
	}{
		{"socket, as tailscale serve sends it", true, "localhost", []string{remoteHost}, 200},
		{"socket, named directly", true, remoteHost, nil, 200},
		{"socket, wrong forwarded host", true, "localhost", []string{"evil.example"}, 403},
		{"socket, no forwarded host", true, "localhost", nil, 403},
		{"socket, two forwarded hosts", true, "localhost", []string{remoteHost, remoteHost}, 403},
		{"not the socket, forwarded host ignored", false, "localhost", []string{remoteHost}, 403},
		{"socket, forwarded host ignored when Host is not localhost", true, "127.0.0.1", []string{remoteHost}, 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t, Config{Remote: remoteOn(remote.LevelAnswer)})
			req := httptest.NewRequest("GET", "/api/sessions", nil)
			req.Host = tt.host
			for _, f := range tt.forward {
				req.Header.Add("X-Forwarded-Host", f)
			}
			req.Header.Set(remote.TailscaleHeader, "dev@example.com")
			if tt.onSocket {
				req = req.WithContext(OnSocket(req.Context()))
			}
			rec := httptest.NewRecorder()
			s.RemoteHandler().ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status %d, want %d: %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

type fakeCommands struct{}

func (fakeCommands) List(dir string) []commands.Command {
	return []commands.Command{{Name: "compact", Kind: commands.KindBuiltin}, {Name: "in:" + dir, Kind: commands.KindCommand}}
}

func TestSessionCommandsRoute(t *testing.T) {
	src := newFakeSource()
	src.state.Sessions = []hub.SessionView{
		{ID: "c1", Tool: "claude", Directory: "/src/web"},
		{ID: "x1", Tool: "codex", Directory: "/src/api"},
	}
	s := newTestServer(t, Config{Sessions: src, Commands: fakeCommands{}})
	tests := []struct {
		path     string
		want     int
		wantBody string
	}{
		{"/api/sessions/c1/commands", 200, `"name":"in:/src/web"`},
		{"/api/sessions/x1/commands", 200, `"commands":[]`},
		{"/api/sessions/zz/commands", 404, "unknown session"},
	}
	for _, tt := range tests {
		req := httptest.NewRequest("GET", tt.path, nil)
		req.Host = "127.0.0.1:7317"
		req.Header.Set("Authorization", "Bearer "+testToken)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != tt.want || !strings.Contains(rec.Body.String(), tt.wantBody) {
			t.Errorf("%s: status %d body %s", tt.path, rec.Code, rec.Body.String())
		}
	}
}
