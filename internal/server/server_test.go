package server

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/gsperry/agent-desk/internal/agents"
	"github.com/gsperry/agent-desk/internal/am"
	"github.com/gsperry/agent-desk/internal/changes"
	"github.com/gsperry/agent-desk/internal/conflicts"
	"github.com/gsperry/agent-desk/internal/history"
	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/manage"
	"github.com/gsperry/agent-desk/internal/open"
	"github.com/gsperry/agent-desk/internal/overview"
	"github.com/gsperry/agent-desk/internal/pane"
	"github.com/gsperry/agent-desk/internal/search"
	"github.com/gsperry/agent-desk/internal/send"
	"github.com/gsperry/agent-desk/internal/setup"
	"github.com/gsperry/agent-desk/internal/summary"
	"github.com/gsperry/agent-desk/internal/transcript"
	"github.com/gsperry/agent-desk/internal/usage"
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

func (f *fakeTranscripts) Around(_ context.Context, id, uuid string, _ int) hub.Page {
	return hub.Page{SessionID: id, Status: hub.DocOK, Items: []transcript.Item{{Seq: 3, Kind: transcript.KindUser, UUID: uuid}}}
}

func (f *fakeTranscripts) Agents(_ context.Context, id string) ([]agents.Agent, error) {
	if id == "broken" {
		return nil, errors.New("boom")
	}
	return []agents.Agent{{ID: "aaa", Status: agents.StatusRunning}}, nil
}

func (f *fakeTranscripts) AgentTranscript(_ context.Context, _, agentID string) ([]transcript.Item, error) {
	if agentID != "aaa" {
		return nil, agents.ErrNotFound
	}
	return []transcript.Item{{Seq: 0, Kind: transcript.KindText, Text: "sub"}}, nil
}

func (f *fakeTranscripts) Subscribe(_ context.Context, id string) (hub.Page, <-chan hub.Event, func()) {
	return hub.Page{SessionID: id, Status: hub.DocOK, Items: []transcript.Item{{Seq: 0, Kind: transcript.KindUser, Text: "hi"}}}, f.events, func() {}
}

type fakeSender struct {
	res  send.Result
	err  error
	text string
}

func (f *fakeSender) Send(_ context.Context, _ string, text string) (send.Result, error) {
	f.text = text
	return f.res, f.err
}

type fakePanes struct {
	screen pane.Screen
	err    error
	answer pane.Answer
	// interrupted is set when Interrupt is called.
	interrupted bool
}

func (f *fakePanes) Screen(context.Context, string) (pane.Screen, error) { return f.screen, f.err }
func (f *fakePanes) Styled(context.Context, string) (string, error) {
	return "\x1b[1m" + f.screen.Text, f.err
}
func (f *fakePanes) Answer(_ context.Context, _ string, a pane.Answer) error {
	f.answer = a
	return f.err
}

func (f *fakePanes) Interrupt(context.Context, string) error {
	f.interrupted = true
	return f.err
}

type fakeOpener struct {
	req open.Request
	err error
}

func (f *fakeOpener) Open(_ context.Context, req open.Request) error {
	f.req = req
	return f.err
}

type fakeSearcher struct {
	q    string
	opts search.Options
	err  error
}

func (f *fakeSearcher) Search(_ context.Context, q string, opts search.Options) (search.Result, error) {
	f.q, f.opts = q, opts
	return search.Result{Query: q, Hits: []search.Hit{}}, f.err
}

type fakeLifecycle struct {
	err    error
	spawn  am.SpawnRequest
	action manage.Action
	id     string
	group  manage.Group
	del    string
}

func (f *fakeLifecycle) Options(context.Context) (manage.Options, error) {
	return manage.Options{Tools: []string{"claude"}, DefaultTool: "claude"}, f.err
}
func (f *fakeLifecycle) Spawn(_ context.Context, r am.SpawnRequest) (am.Session, error) {
	f.spawn = r
	return am.Session{ID: "n1"}, f.err
}
func (f *fakeLifecycle) Act(_ context.Context, id string, a manage.Action) error {
	f.id, f.action = id, a
	return f.err
}
func (f *fakeLifecycle) CreateGroup(_ context.Context, g manage.Group) error {
	f.group = g
	return f.err
}
func (f *fakeLifecycle) DeleteGroup(_ context.Context, path string) error {
	f.del = path
	return f.err
}

type fakeChanges struct{}

func (fakeChanges) Changes(_ context.Context, id, mode string) (changes.Changes, error) {
	if id == "norepo" {
		return changes.Changes{}, &send.Refused{Reason: "not_repo"}
	}
	return changes.Changes{Repo: "/r", Mode: mode, Files: []changes.File{}}, nil
}

type fakeHistory struct{}

func (fakeHistory) List(context.Context) ([]history.Conversation, error) {
	return []history.Conversation{{ID: "c1", Title: "Old work"}}, nil
}

func (fakeHistory) Read(_ context.Context, id string, before, limit int) (hub.Page, error) {
	if id != "c1" {
		return hub.Page{}, history.ErrNotFound
	}
	return hub.Page{SessionID: id, Status: hub.DocOK, Items: []transcript.Item{{Seq: before, Kind: transcript.KindText, Text: strconv.Itoa(limit)}}}, nil
}

func (fakeHistory) Export(_ context.Context, id string) (string, string, error) {
	if id != "c1" {
		return "", "", history.ErrNotFound
	}
	return "old-work.md", "# Old work\n", nil
}

type fakeUsage struct{}

func (fakeUsage) Report(_ context.Context, days int) (usage.Report, error) {
	return usage.Report{From: strconv.Itoa(days)}, nil
}

type fakeSummaries struct{}

func (fakeSummaries) Summarize(_ context.Context, id string, since int, model string) (summary.Result, error) {
	if id == "none" {
		return summary.Result{}, &send.Refused{Reason: "nothing_new", Message: "Nothing new"}
	}
	return summary.Result{Text: "since " + strconv.Itoa(since), Model: model}, nil
}

type fakePastes struct{}

func (fakePastes) Save(data []byte) (string, error) {
	if string(data) == "png" {
		return "/cache/paste-1.png", nil
	}
	return "", errors.New("only PNG, JPEG, GIF and WebP images can be pasted")
}

type fakeConflicts struct{}

func (fakeConflicts) Find(context.Context) map[string][]conflicts.Conflict {
	return map[string][]conflicts.Conflict{"a1": {{Path: "/x", Others: []string{"b2"}}}}
}

type fakeOverview struct{}

func (fakeOverview) Summaries(context.Context) map[string]overview.Summary {
	return map[string]overview.Summary{"a1": {Activity: "Bash: go test"}}
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
	if cfg.Sender == nil {
		cfg.Sender = &fakeSender{res: send.Result{Status: send.StatusSent}}
	}
	if cfg.Panes == nil {
		cfg.Panes = &fakePanes{screen: pane.Screen{Text: "screen"}}
	}
	if cfg.Opener == nil {
		cfg.Opener = &fakeOpener{}
	}
	if cfg.Searcher == nil {
		cfg.Searcher = &fakeSearcher{}
	}
	if cfg.Lifecycle == nil {
		cfg.Lifecycle = &fakeLifecycle{}
	}
	if cfg.Overview == nil {
		cfg.Overview = fakeOverview{}
	}
	if cfg.Changes == nil {
		cfg.Changes = fakeChanges{}
	}
	if cfg.Conflicts == nil {
		cfg.Conflicts = fakeConflicts{}
	}
	if cfg.Pastes == nil {
		cfg.Pastes = fakePastes{}
	}
	if cfg.History == nil {
		cfg.History = fakeHistory{}
	}
	if cfg.Usage == nil {
		cfg.Usage = fakeUsage{}
	}
	if cfg.Summaries == nil {
		cfg.Summaries = fakeSummaries{}
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNewRejectsShortToken(t *testing.T) {
	if _, err := New(Config{Addr: "127.0.0.1:1", Token: "short", Sessions: newFakeSource(), Manager: fakeManager{}, Transcripts: newFakeTranscripts(), Sender: &fakeSender{}, Panes: &fakePanes{}, Opener: &fakeOpener{}, Searcher: &fakeSearcher{}, Lifecycle: &fakeLifecycle{}, Overview: fakeOverview{}, Changes: fakeChanges{}, Conflicts: fakeConflicts{}, Pastes: fakePastes{}, History: fakeHistory{}, Usage: fakeUsage{}, Summaries: fakeSummaries{}}); err == nil {
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
		{"localhost host is not trusted", "GET", "/api/health", "localhost:7317", "", "Bearer " + testToken, 403},
		{"localhost origin is not trusted", "GET", "/api/health", "127.0.0.1:7317", "http://localhost:7317", "Bearer " + testToken, 403},
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
		{"search", "GET", "/api/search?q=flaky", "127.0.0.1:7317", "", "Bearer " + testToken, 200},
		{"search without token", "GET", "/api/search?q=flaky", "127.0.0.1:7317", "", "", 401},
		{"transcript around a hit", "GET", "/api/sessions/a1/transcript?around=u-1", "127.0.0.1:7317", "", "Bearer " + testToken, 200},
		{"conflicts", "GET", "/api/conflicts", "127.0.0.1:7317", "", "Bearer " + testToken, 200},
		{"changes", "GET", "/api/sessions/a1/changes?mode=branch", "127.0.0.1:7317", "", "Bearer " + testToken, 200},
		{"changes outside a repo", "GET", "/api/sessions/norepo/changes", "127.0.0.1:7317", "", "Bearer " + testToken, 409},
		{"changes without token", "GET", "/api/sessions/a1/changes", "127.0.0.1:7317", "", "", 401},
		{"overview", "GET", "/api/overview", "127.0.0.1:7317", "", "Bearer " + testToken, 200},
		{"overview without token", "GET", "/api/overview", "127.0.0.1:7317", "", "", 401},
		{"agents", "GET", "/api/sessions/a1/agents", "127.0.0.1:7317", "", "Bearer " + testToken, 200},
		{"agents failure", "GET", "/api/sessions/broken/agents", "127.0.0.1:7317", "", "Bearer " + testToken, 502},
		{"agent transcript", "GET", "/api/sessions/a1/agents/aaa/transcript", "127.0.0.1:7317", "", "Bearer " + testToken, 200},
		{"missing agent transcript", "GET", "/api/sessions/a1/agents/zzz/transcript", "127.0.0.1:7317", "", "Bearer " + testToken, 404},
		{"agents without token", "GET", "/api/sessions/a1/agents", "127.0.0.1:7317", "", "", 401},
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

func TestSend(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		sender     *fakeSender
		origin     string
		wantStatus int
		wantBody   string
	}{
		{"sent", `{"text":"hello"}`, &fakeSender{res: send.Result{Status: send.StatusSent}}, "http://127.0.0.1:7317", 200, `"status":"sent"`},
		{"maybe", `{"text":"hello"}`, &fakeSender{res: send.Result{Status: send.StatusMaybe}}, "http://127.0.0.1:7317", 200, `"status":"maybe"`},
		{"refused", `{"text":"hello"}`, &fakeSender{err: &send.Refused{Reason: send.ReasonDialog, Message: "dialog"}}, "http://127.0.0.1:7317", 409, `"reason":"dialog"`},
		{"delivery error", `{"text":"hello"}`, &fakeSender{err: errors.New("tmux gone")}, "http://127.0.0.1:7317", 502, "tmux gone"},
		{"bad body", `nope`, &fakeSender{}, "http://127.0.0.1:7317", 400, "JSON"},
		{"foreign origin", `{"text":"hello"}`, &fakeSender{}, "http://evil.example", 403, "forbidden"},
		{"no origin", `{"text":"hello"}`, &fakeSender{}, "", 403, "forbidden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t, Config{Sender: tt.sender})
			req := httptest.NewRequest("POST", "/api/sessions/a1/send", strings.NewReader(tt.body))
			req.Host = "127.0.0.1:7317"
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			req.Header.Set("Authorization", "Bearer "+testToken)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus || !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("status %d body %q, want %d containing %q", rec.Code, rec.Body.String(), tt.wantStatus, tt.wantBody)
			}
		})
	}
}

func TestPaneRoutes(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		panes      *fakePanes
		origin     string
		wantStatus int
		wantBody   string
	}{
		{"screen", "GET", "/api/sessions/a1/pane", "", &fakePanes{screen: pane.Screen{Text: "hello"}}, "", 200, `"text":"hello"`},
		{"screen refused", "GET", "/api/sessions/a1/pane", "", &fakePanes{err: &send.Refused{Reason: send.ReasonNotRunning}}, "", 409, "not_running"},
		{"terminal", "GET", "/api/sessions/a1/terminal", "", &fakePanes{screen: pane.Screen{Text: "hi"}}, "", 200, `"screen":"\u001b[1mhi"`},
		{"terminal refused", "GET", "/api/sessions/a1/terminal", "", &fakePanes{err: &send.Refused{Reason: send.ReasonNotRunning}}, "", 409, "not_running"},
		{"answer", "POST", "/api/sessions/a1/answer", `{"action":"choose","index":1,"label":"No"}`, &fakePanes{}, "http://127.0.0.1:7317", 200, "ok"},
		{"answer refused", "POST", "/api/sessions/a1/answer", `{"action":"key","key":"x"}`, &fakePanes{err: &send.Refused{Reason: "bad_key"}}, "http://127.0.0.1:7317", 409, "bad_key"},
		{"answer bad body", "POST", "/api/sessions/a1/answer", `[`, &fakePanes{}, "http://127.0.0.1:7317", 400, "JSON"},
		{"answer without origin", "POST", "/api/sessions/a1/answer", `{"action":"key","key":"Enter"}`, &fakePanes{}, "", 403, "forbidden"},
		{"interrupt", "POST", "/api/sessions/a1/interrupt", ``, &fakePanes{}, "http://127.0.0.1:7317", 200, "ok"},
		{"interrupt refused", "POST", "/api/sessions/a1/interrupt", ``, &fakePanes{err: &send.Refused{Reason: "not_working"}}, "http://127.0.0.1:7317", 409, "not_working"},
		{"interrupt without origin", "POST", "/api/sessions/a1/interrupt", ``, &fakePanes{}, "", 403, "forbidden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t, Config{Panes: tt.panes})
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Host = "127.0.0.1:7317"
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			req.Header.Set("Authorization", "Bearer "+testToken)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus || !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("status %d body %q, want %d containing %q", rec.Code, rec.Body.String(), tt.wantStatus, tt.wantBody)
			}
		})
	}
	p := &fakePanes{}
	s := newTestServer(t, Config{Panes: p})
	req := httptest.NewRequest("POST", "/api/sessions/a1/answer", strings.NewReader(`{"action":"choose","index":2,"label":"Blue"}`))
	req.Host, req.Header = "127.0.0.1:7317", http.Header{"Origin": {"http://127.0.0.1:7317"}, "Authorization": {"Bearer " + testToken}}
	s.Handler().ServeHTTP(httptest.NewRecorder(), req)
	if p.answer != (pane.Answer{Action: "choose", Index: 2, Label: "Blue"}) {
		t.Fatalf("answer = %+v", p.answer)
	}
}

func TestOpenRoute(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		opener     *fakeOpener
		origin     string
		wantStatus int
	}{
		{"opened", `{"sessionId":"a1","path":"src/x.go","line":4}`, &fakeOpener{}, "http://127.0.0.1:7317", 200},
		{"refused", `{"path":"x"}`, &fakeOpener{err: &send.Refused{Reason: "not_found"}}, "http://127.0.0.1:7317", 409},
		{"bad body", `{`, &fakeOpener{}, "http://127.0.0.1:7317", 400},
		{"no origin", `{"path":"x"}`, &fakeOpener{}, "", 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t, Config{Opener: tt.opener})
			req := httptest.NewRequest("POST", "/api/open", strings.NewReader(tt.body))
			req.Host = "127.0.0.1:7317"
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			req.Header.Set("Authorization", "Bearer "+testToken)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
	o := &fakeOpener{}
	s := newTestServer(t, Config{Opener: o})
	req := httptest.NewRequest("POST", "/api/open", strings.NewReader(`{"sessionId":"a1","path":"src/x.go","line":4}`))
	req.Host, req.Header = "127.0.0.1:7317", http.Header{"Origin": {"http://127.0.0.1:7317"}, "Authorization": {"Bearer " + testToken}}
	s.Handler().ServeHTTP(httptest.NewRecorder(), req)
	if o.req != (open.Request{SessionID: "a1", Path: "src/x.go", Line: 4}) {
		t.Fatalf("req = %+v", o.req)
	}
}

func TestSearchRoute(t *testing.T) {
	f := &fakeSearcher{}
	s := newTestServer(t, Config{Searcher: f})
	req := httptest.NewRequest("GET", "/api/search?q=flaky+test&archived=1&limit=7", nil)
	req.Host = "127.0.0.1:7317"
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 || f.q != "flaky test" || !f.opts.IncludeArchived || f.opts.Limit != 7 {
		t.Fatalf("status %d q %q opts %+v", rec.Code, f.q, f.opts)
	}
	bad := newTestServer(t, Config{Searcher: &fakeSearcher{err: search.ErrQuery}})
	rec = httptest.NewRecorder()
	bad.Handler().ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("short query status %d", rec.Code)
	}
}

func TestLifecycleRoutes(t *testing.T) {
	own := "http://127.0.0.1:7317"
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		origin     string
		err        error
		wantStatus int
		check      func(t *testing.T, f *fakeLifecycle)
	}{
		{"options", "GET", "/api/spawn-options", "", "", nil, 200, nil},
		{"spawn", "POST", "/api/sessions", `{"name":"x","directory":"/src","worktree":true}`, own, nil, 200, func(t *testing.T, f *fakeLifecycle) {
			if f.spawn.Name != "x" || f.spawn.Directory != "/src" || !f.spawn.Worktree {
				t.Fatalf("spawn %+v", f.spawn)
			}
		}},
		{"spawn refused", "POST", "/api/sessions", `{}`, own, &send.Refused{Reason: "invalid"}, 409, nil},
		{"spawn without origin", "POST", "/api/sessions", `{}`, "", nil, 403, nil},
		{"action", "POST", "/api/sessions/a1/action", `{"action":"move","group":"web"}`, own, nil, 200, func(t *testing.T, f *fakeLifecycle) {
			if f.id != "a1" || f.action != (manage.Action{Action: "move", Group: "web"}) {
				t.Fatalf("action %s %+v", f.id, f.action)
			}
		}},
		{"action failed", "POST", "/api/sessions/a1/action", `{"action":"kill"}`, own, errors.New("tmux gone"), 502, nil},
		{"create group", "POST", "/api/groups", `{"path":"ops","directory":"/srv"}`, own, nil, 200, func(t *testing.T, f *fakeLifecycle) {
			if f.group != (manage.Group{Path: "ops", Directory: "/srv"}) {
				t.Fatalf("group %+v", f.group)
			}
		}},
		{"delete group", "POST", "/api/groups/delete", `{"path":"ops"}`, own, nil, 200, func(t *testing.T, f *fakeLifecycle) {
			if f.del != "ops" {
				t.Fatalf("delete %q", f.del)
			}
		}},
		{"bad body", "POST", "/api/groups", `{`, own, nil, 400, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeLifecycle{err: tt.err}
			s := newTestServer(t, Config{Lifecycle: f})
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Host = "127.0.0.1:7317"
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			req.Header.Set("Authorization", "Bearer "+testToken)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status %d, want %d (%s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.check != nil {
				tt.check(t, f)
			}
		})
	}
}

type fakeSetup struct {
	choice setup.Choice
	err    error
}

func (f *fakeSetup) Check(context.Context) setup.Report {
	return setup.Report{Programs: []setup.Program{{Name: "tmux", Found: true}}}
}

func (f *fakeSetup) Apply(_ context.Context, c setup.Choice) (setup.Report, error) {
	f.choice = c
	return f.Check(context.Background()), f.err
}

func TestSetupRoutes(t *testing.T) {
	own := "http://127.0.0.1:7317"
	tests := []struct {
		name, method, body, origin string
		err                        error
		nilSetup                   bool
		want                       int
	}{
		{"report", "GET", "", "", nil, false, 200},
		{"apply", "POST", `{"shown":["claude"],"defaultTool":"claude"}`, own, nil, false, 200},
		{"refused", "POST", `{"shown":[]}`, own, &send.Refused{Reason: "invalid"}, false, 409},
		{"no origin", "POST", `{}`, "", nil, false, 403},
		{"bad body", "POST", `{`, own, nil, false, 400},
		{"absent", "GET", "", "", nil, true, 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeSetup{err: tt.err}
			cfg := Config{Setup: f}
			if tt.nilSetup {
				cfg.Setup = nil
			}
			s := newTestServer(t, cfg)
			req := httptest.NewRequest(tt.method, "/api/setup", strings.NewReader(tt.body))
			req.Host = "127.0.0.1:7317"
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			req.Header.Set("Authorization", "Bearer "+testToken)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status %d, want %d (%s)", rec.Code, tt.want, rec.Body.String())
			}
			if tt.name == "apply" && (f.choice.DefaultTool != "claude" || len(f.choice.Shown) != 1) {
				t.Fatalf("choice %+v", f.choice)
			}
		})
	}
}

func TestPasteRoute(t *testing.T) {
	tests := []struct {
		name, body, origin string
		want               int
		wantBody           string
	}{
		{"saved", "png", "http://127.0.0.1:7317", 200, "paste-1.png"},
		{"not an image", "<html>", "http://127.0.0.1:7317", 400, "only PNG"},
		{"no origin", "png", "", 403, "forbidden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t, Config{})
			req := httptest.NewRequest("POST", "/api/paste", strings.NewReader(tt.body))
			req.Host = "127.0.0.1:7317"
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			req.Header.Set("Authorization", "Bearer "+testToken)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.want || !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("status %d body %q", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestConversationRoutes(t *testing.T) {
	tests := []struct {
		name, path string
		token      bool
		want       int
		wantBody   string
	}{
		{"list", "/api/conversations", true, 200, `"title":"Old work"`},
		{"list needs token", "/api/conversations", false, 401, ""},
		{"read", "/api/conversations/c1/transcript?before=7&limit=50", true, 200, `"seq":7`},
		{"read passes limit", "/api/conversations/c1/transcript?limit=50", true, 200, `"text":"50"`},
		{"unknown", "/api/conversations/zz/transcript", true, 404, "not found"},
		{"export", "/api/conversations/c1/export", true, 200, "# Old work"},
		{"export needs token", "/api/conversations/c1/export", false, 401, ""},
		{"export unknown", "/api/conversations/zz/export", true, 404, "not found"},
		{"usage default", "/api/usage", true, 200, `"from":"30"`},
		{"usage days", "/api/usage?days=7", true, 200, `"from":"7"`},
		{"usage bad days", "/api/usage?days=0", true, 400, "days"},
		{"usage needs token", "/api/usage", false, 401, ""},
		{"bad before", "/api/conversations/c1/transcript?before=-2", true, 400, "before"},
		{"bad limit", "/api/conversations/c1/transcript?limit=9999", true, 400, "limit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t, Config{})
			req := httptest.NewRequest("GET", tt.path, nil)
			req.Host = "127.0.0.1:7317"
			if tt.token {
				req.Header.Set("Authorization", "Bearer "+testToken)
			}
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.want || !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("status %d body %q", rec.Code, rec.Body.String())
			}
			if tt.name == "export" && rec.Header().Get("Content-Disposition") != `attachment; filename="old-work.md"` {
				t.Fatalf("disposition %q", rec.Header().Get("Content-Disposition"))
			}
		})
	}
}

func TestSummaryRoute(t *testing.T) {
	tests := []struct {
		name, id, body, origin string
		want                   int
		wantBody               string
	}{
		{"summary", "a1", `{"since":7,"model":"haiku"}`, "http://127.0.0.1:7317", 200, `"text":"since 7"`},
		{"nothing new", "none", `{"since":7,"model":"haiku"}`, "http://127.0.0.1:7317", 409, "nothing_new"},
		{"bad body", "a1", `nope`, "http://127.0.0.1:7317", 400, "JSON"},
		{"needs origin", "a1", `{"since":7,"model":"haiku"}`, "", 403, ""},
		{"wrong origin", "a1", `{"since":7,"model":"haiku"}`, "https://evil.example", 403, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t, Config{})
			req := httptest.NewRequest("POST", "/api/sessions/"+tt.id+"/summary", strings.NewReader(tt.body))
			req.Host = "127.0.0.1:7317"
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			req.Header.Set("Authorization", "Bearer "+testToken)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.want || !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("status %d body %q", rec.Code, rec.Body.String())
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
