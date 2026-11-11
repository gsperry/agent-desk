package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const testToken = "0123456789abcdef0123456789abcdef"

func newTestServer(t *testing.T, cfg Config) *Server {
	t.Helper()
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:7317"
	}
	if cfg.Token == "" {
		cfg.Token = testToken
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNewRejectsShortToken(t *testing.T) {
	if _, err := New(Config{Addr: "127.0.0.1:1", Token: "short"}); err == nil {
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

func startLive(t *testing.T, cfg Config) string {
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
		})
	}
}
