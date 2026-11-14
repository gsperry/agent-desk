// Package server serves the Agent Desk API and frontend on a loopback address.
//
// Every request must name the server's own host (DNS rebinding defense).
// API requests need the session token. Requests that change state, and
// WebSocket upgrades, need an Origin the server trusts.
package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/gsperry/agent-desk/internal/agents"
	"github.com/gsperry/agent-desk/internal/am"
	"github.com/gsperry/agent-desk/internal/changes"
	"github.com/gsperry/agent-desk/internal/commands"
	"github.com/gsperry/agent-desk/internal/conflicts"
	"github.com/gsperry/agent-desk/internal/history"
	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/manage"
	"github.com/gsperry/agent-desk/internal/open"
	"github.com/gsperry/agent-desk/internal/overview"
	"github.com/gsperry/agent-desk/internal/pane"
	"github.com/gsperry/agent-desk/internal/paste"
	"github.com/gsperry/agent-desk/internal/remote"
	"github.com/gsperry/agent-desk/internal/search"
	"github.com/gsperry/agent-desk/internal/send"
	"github.com/gsperry/agent-desk/internal/setup"
	"github.com/gsperry/agent-desk/internal/summary"
	"github.com/gsperry/agent-desk/internal/transcript"
	"github.com/gsperry/agent-desk/internal/usage"
)

// WSProtocol is the WebSocket subprotocol the server speaks. Browsers cannot
// set headers on a WebSocket, so the client also offers the token as a second
// subprotocol, TokenProtocolPrefix followed by the token.
const (
	WSProtocol          = "agentdesk.v1"
	TokenProtocolPrefix = "agentdesk.token."
)

type Config struct {
	// Addr is the listener's address, such as 127.0.0.1:7317.
	Addr    string
	Token   string
	Version string
	// DevOrigins are extra trusted origins, such as the Vite dev server.
	DevOrigins []string
	// Assets is the built frontend. A nil or empty FS serves a placeholder.
	Assets fs.FS
	// Sessions is the live session list.
	Sessions SessionSource
	// Manager starts a headless agent-manager on request.
	Manager ManagerStarter
	// Transcripts serves session transcripts.
	Transcripts TranscriptSource
	// Sender delivers replies to sessions.
	Sender Sender
	// Panes serves the live pane.
	Panes Panes
	// Opener opens files in the editor.
	Opener Opener
	// Searcher searches transcripts.
	Searcher Searcher
	// Lifecycle starts, stops and files sessions.
	Lifecycle Lifecycle
	// Overview summarizes every session for the home screen.
	Overview Overview
	// Changes reads a session's git changes.
	Changes ChangesSource
	// Conflicts finds files written by more than one live session.
	Conflicts ConflictSource
	// Pastes stores images pasted into the composer.
	Pastes PasteStore
	// History lists and reads every conversation on disk.
	History HistorySource
	Usage   UsageSource
	// Summaries writes catch-up summaries with the user's Claude Code CLI,
	// only when asked.
	Summaries SummarySource
	// Remote holds the remote access settings. Nil turns remote access off.
	Remote RemoteControl
	// Access verifies Cloudflare Access tokens when remote access comes
	// through a Cloudflare Tunnel.
	Access remote.Verifier
	// Commands lists the slash commands a session accepts, for autocomplete.
	Commands CommandSource
	// Setup checks the programs Agent Desk needs and changes the CLIs
	// agent-manager offers, for the welcome screen. Nil leaves it out.
	Setup SetupSource
	// RemoteHTTPOrigin also accepts an http:// origin for the remote host.
	// Only fixture mode sets it, for browser tests without TLS.
	RemoteHTTPOrigin bool
}

type SetupSource interface {
	Check(ctx context.Context) setup.Report
	Apply(ctx context.Context, c setup.Choice) (setup.Report, error)
}

type CommandSource interface {
	List(dir string) []commands.Command
}

// RemoteControl reads and changes the remote access settings. Set applies
// them, starting or stopping the remote listener; Status says whether it
// is listening, and where.
type RemoteControl interface {
	Get() remote.Settings
	Set(remote.Settings) (remote.Settings, error)
	Status() RemoteStatus
}

// RemoteStatus is the remote listener's state, for the settings screen.
type RemoteStatus struct {
	Listening bool   `json:"listening"`
	Address   string `json:"address,omitempty"`
	Error     string `json:"error,omitempty"`
}

type SummarySource interface {
	Summarize(ctx context.Context, sessionID string, since int, model string) (summary.Result, error)
}

type UsageSource interface {
	Report(ctx context.Context, days int) (usage.Report, error)
}

type HistorySource interface {
	List(ctx context.Context) ([]history.Conversation, error)
	Read(ctx context.Context, id string, before, limit int) (hub.Page, error)
	Export(ctx context.Context, id string) (filename, body string, err error)
}

type PasteStore interface {
	Save(data []byte) (string, error)
}

type ConflictSource interface {
	Find(ctx context.Context) map[string][]conflicts.Conflict
}

type ChangesSource interface {
	Changes(ctx context.Context, sessionID, mode string) (changes.Changes, error)
}

type Overview interface {
	Summaries(ctx context.Context) map[string]overview.Summary
}

type Lifecycle interface {
	Options(ctx context.Context) (manage.Options, error)
	Spawn(ctx context.Context, r am.SpawnRequest) (am.Session, error)
	Act(ctx context.Context, id string, a manage.Action) error
	CreateGroup(ctx context.Context, g manage.Group) error
	DeleteGroup(ctx context.Context, path string) error
}

type Searcher interface {
	Search(ctx context.Context, query string, opts search.Options) (search.Result, error)
}

type Opener interface {
	Open(ctx context.Context, req open.Request) error
}

type Sender interface {
	Send(ctx context.Context, sessionID, text string) (send.Result, error)
}

// Panes reads a session's screen and answers its dialogs.
type Panes interface {
	Screen(ctx context.Context, sessionID string) (pane.Screen, error)
	Styled(ctx context.Context, sessionID string) (string, error)
	Answer(ctx context.Context, sessionID string, a pane.Answer) error
	// Interrupt stops a working session's turn, as Esc does.
	Interrupt(ctx context.Context, sessionID string) error
}

type TranscriptSource interface {
	Window(ctx context.Context, sessionID string, before, limit int) hub.Page
	Around(ctx context.Context, sessionID, uuid string, radius int) hub.Page
	Agents(ctx context.Context, sessionID string) ([]agents.Agent, error)
	AgentTranscript(ctx context.Context, sessionID, agentID string) ([]transcript.Item, error)
	Subscribe(ctx context.Context, sessionID string) (hub.Page, <-chan hub.Event, func())
}

type SessionSource interface {
	State() hub.State
	Subscribe() (<-chan hub.State, func())
	Trigger()
}

type ManagerStarter interface {
	StartServe(ctx context.Context) error
}

type Server struct {
	cfg       Config
	hosts     map[string]bool
	origins   map[string]bool
	mux       *http.ServeMux
	remoteMux *http.ServeMux
	// needs records each route's remote need, so a test can pin them.
	needs map[string]remote.Need

	remoteMu    sync.Mutex
	remoteConns map[*websocket.Conn]remoteConn
}

func New(cfg Config) (*Server, error) {
	if cfg.Sessions == nil || cfg.Manager == nil || cfg.Transcripts == nil || cfg.Sender == nil || cfg.Panes == nil || cfg.Opener == nil || cfg.Searcher == nil || cfg.Lifecycle == nil || cfg.Overview == nil || cfg.Changes == nil || cfg.Conflicts == nil || cfg.Pastes == nil || cfg.History == nil || cfg.Usage == nil || cfg.Summaries == nil {
		return nil, errors.New("every Config source is required")
	}
	if len(cfg.Token) < 32 {
		return nil, errors.New("token must be at least 32 characters")
	}
	_, port, err := net.SplitHostPort(cfg.Addr)
	if err != nil {
		return nil, err
	}
	s := &Server{
		cfg: cfg,
		// Only the IPv4 loopback the server binds. localhost can resolve to
		// ::1, where another local account could listen on the same port.
		hosts:       map[string]bool{"127.0.0.1:" + port: true},
		origins:     map[string]bool{"http://127.0.0.1:" + port: true},
		mux:         http.NewServeMux(),
		remoteMux:   http.NewServeMux(),
		needs:       map[string]remote.Need{},
		remoteConns: map[*websocket.Conn]remoteConn{},
	}
	for _, o := range cfg.DevOrigins {
		s.origins[strings.TrimRight(o, "/")] = true
	}
	s.routes()
	return s, nil
}

// route registers a handler on the local mux behind the token, and on the
// remote mux behind identity and the level it needs.
func (s *Server) route(pattern string, need remote.Need, h http.HandlerFunc) {
	s.needs[pattern] = need
	s.mux.Handle(pattern, s.requireToken(h))
	s.remoteMux.Handle(pattern, s.requireRemote(need, h))
}

// routes is every API route and what a remote client needs to use it.
func (s *Server) routes() {
	s.route("GET /api/health", remote.NeedRead, s.health)
	s.route("GET /api/whoami", remote.NeedRead, s.whoami)
	s.route("GET /api/sessions", remote.NeedRead, s.sessions)
	s.route("GET /api/sessions/{id}/transcript", remote.NeedRead, s.transcriptWindow)
	s.route("POST /api/sessions/{id}/send", remote.NeedReply, s.send)
	s.route("GET /api/sessions/{id}/pane", remote.NeedRead, s.screen)
	s.route("GET /api/sessions/{id}/terminal", remote.NeedRead, s.terminal)
	s.route("POST /api/sessions/{id}/answer", remote.NeedAnswer, s.answer)
	s.route("POST /api/sessions/{id}/interrupt", remote.NeedReply, s.interrupt)
	s.route("GET /api/sessions/{id}/agents", remote.NeedRead, s.listAgents)
	s.route("GET /api/sessions/{id}/agents/{agent}/transcript", remote.NeedRead, s.agentTranscript)
	s.route("GET /api/sessions/{id}/changes", remote.NeedRead, s.changes)
	s.route("GET /api/sessions/{id}/commands", remote.NeedRead, s.sessionCommands)
	s.route("POST /api/paste", remote.NeedLocal, s.paste)
	s.route("GET /api/conflicts", remote.NeedRead, s.conflicts)
	s.route("GET /api/conversations", remote.NeedRead, s.conversations)
	s.route("GET /api/conversations/{id}/transcript", remote.NeedRead, s.conversationTranscript)
	s.route("GET /api/conversations/{id}/export", remote.NeedRead, s.exportConversation)
	s.route("POST /api/sessions/{id}/summary", remote.NeedLocal, s.summarize)
	s.route("GET /api/usage", remote.NeedRead, s.usage)
	s.route("GET /api/overview", remote.NeedRead, s.overview)
	s.route("GET /api/spawn-options", remote.NeedManage, s.spawnOptions)
	s.route("POST /api/sessions", remote.NeedManage, s.spawn)
	s.route("POST /api/sessions/{id}/action", remote.NeedManage, s.action)
	s.route("POST /api/groups", remote.NeedManage, s.createGroup)
	s.route("POST /api/groups/delete", remote.NeedManage, s.deleteGroup)
	s.route("GET /api/search", remote.NeedRead, s.search)
	s.route("POST /api/open", remote.NeedLocal, s.open)
	s.route("POST /api/manager/start", remote.NeedManage, s.startManager)
	s.route("GET /api/setup", remote.NeedLocal, s.setupReport)
	s.route("POST /api/setup", remote.NeedLocal, s.applySetup)
	s.route("GET /api/remote", remote.NeedRead, s.remoteSettings)
	s.route("POST /api/remote", remote.NeedLocal, s.setRemoteSettings)
	// The WebSocket only reads; it checks its own token locally.
	s.needs["GET /api/ws"] = remote.NeedRead
	s.mux.HandleFunc("GET /api/ws", s.websocket)
	s.remoteMux.Handle("GET /api/ws", s.requireRemote(remote.NeedRead, s.websocket))
	s.mux.Handle("/api/", s.requireToken(http.NotFoundHandler()))
	s.remoteMux.Handle("/api/", s.requireRemote(remote.NeedRead, http.NotFoundHandler().ServeHTTP))
	s.mux.Handle("/", s.static())
	s.remoteMux.Handle("/", s.requireRemote(remote.NeedRead, s.static().ServeHTTP))
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.hosts[r.Host] {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if !s.originAllowed(r) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		securityHeaders(w)
		s.mux.ServeHTTP(w, r)
	})
}

func securityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'; object-src 'none'")
}

// RemoteHandler serves remote clients. It is mounted on its own listener
// behind a proxy that proves identity; every route checks the host, the
// origin, the identity and the level it needs.
func (s *Server) RemoteHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		securityHeaders(w)
		s.remoteMux.ServeHTTP(w, r)
	})
}

// originAllowed rejects a foreign Origin on any request, and a missing one on
// requests that change state or upgrade to a WebSocket.
func (s *Server) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin != "" {
		return s.origins[origin]
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		return !isUpgrade(r)
	default:
		return false
	}
}

func isUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

func (s *Server) tokenOK(candidate string) bool {
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(s.cfg.Token)) == 1
}

func (s *Server) requireToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || !s.tokenOK(bearer) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": s.cfg.Version})
}

func (s *Server) sessions(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.Sessions.State())
}

func (s *Server) startManager(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := s.cfg.Manager.StartServe(ctx); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	s.cfg.Sessions.Trigger()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) setupReport(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Setup == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "setup is not available"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, s.cfg.Setup.Check(ctx))
}

func (s *Server) applySetup(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Setup == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "setup is not available"})
		return
	}
	var c setup.Choice
	if !decode(w, r, 4096, &c) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	rep, err := s.cfg.Setup.Apply(ctx, c)
	logOutcome("setup", "", fmt.Sprintf("shown=%d default=%s", len(c.Shown), c.DefaultTool), "ok", err)
	if writeRefusal(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// maxSendBody bounds the request; the sender's own cap is lower.
const maxSendBody = 1 << 20

func (s *Server) send(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSendBody)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be JSON with a text field"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	res, err := s.cfg.Sender.Send(ctx, r.PathValue("id"), body.Text)
	logOutcome("send", r.PathValue("id"), fmt.Sprintf("bytes=%d", len(body.Text)), res.Status, err)
	if writeRefusal(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) screen(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	sc, err := s.cfg.Panes.Screen(ctx, r.PathValue("id"))
	if writeRefusal(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, sc)
}

// terminal serves the screen with color for the read-only terminal view.
func (s *Server) terminal(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	screen, err := s.cfg.Panes.Styled(ctx, r.PathValue("id"))
	if writeRefusal(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"screen": screen})
}

func (s *Server) answer(w http.ResponseWriter, r *http.Request) {
	var a pane.Answer
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&a); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be a JSON answer"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	err := s.cfg.Panes.Answer(ctx, r.PathValue("id"), a)
	logOutcome("answer", r.PathValue("id"), fmt.Sprintf("action=%s index=%d key=%s", a.Action, a.Index, a.Key), "ok", err)
	if writeRefusal(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// interrupt stops a working session's current turn. It needs the reply
// level remotely: it steers the agent, like a reply does.
func (s *Server) interrupt(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	err := s.cfg.Panes.Interrupt(ctx, r.PathValue("id"))
	logOutcome("interrupt", r.PathValue("id"), "", "ok", err)
	if writeRefusal(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) changes(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	c, err := s.cfg.Changes.Changes(ctx, r.PathValue("id"), r.URL.Query().Get("mode"))
	if writeRefusal(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) paste(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, paste.MaxBytes+1))
	if err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "image is over 10 MB"})
		return
	}
	path, err := s.cfg.Pastes.Save(data)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

func (s *Server) conflicts(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, map[string]any{"conflicts": s.cfg.Conflicts.Find(ctx)})
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, map[string]any{"summaries": s.cfg.Overview.Summaries(ctx)})
}

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	list, err := s.cfg.Transcripts.Agents(r.Context(), r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": list})
}

func (s *Server) agentTranscript(w http.ResponseWriter, r *http.Request) {
	items, err := s.cfg.Transcripts.AgentTranscript(r.Context(), r.PathValue("id"), r.PathValue("agent"))
	switch {
	case errors.Is(err, agents.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "subagent not found"})
	case err != nil:
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

// spawnTimeout covers agent-manager registering its MCP server with a new
// CLI, which it allows two minutes.
const spawnTimeout = 150 * time.Second

func decode(w http.ResponseWriter, r *http.Request, limit int64, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit)).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be JSON"})
		return false
	}
	return true
}

func (s *Server) spawnOptions(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	opts, err := s.cfg.Lifecycle.Options(ctx)
	if writeRefusal(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, opts)
}

func (s *Server) spawn(w http.ResponseWriter, r *http.Request) {
	var req am.SpawnRequest
	if !decode(w, r, 1<<20, &req) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), spawnTimeout)
	defer cancel()
	sess, err := s.cfg.Lifecycle.Spawn(ctx, req)
	logOutcome("spawn", sess.ID, fmt.Sprintf("tool=%s worktree=%v", req.Tool, req.Worktree), "ok", err)
	if writeRefusal(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) action(w http.ResponseWriter, r *http.Request) {
	var a manage.Action
	if !decode(w, r, 4096, &a) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), spawnTimeout)
	defer cancel()
	err := s.cfg.Lifecycle.Act(ctx, r.PathValue("id"), a)
	logOutcome("action", r.PathValue("id"), "action="+a.Action, "ok", err)
	if writeRefusal(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) createGroup(w http.ResponseWriter, r *http.Request) {
	var g manage.Group
	if !decode(w, r, 4096, &g) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if writeRefusal(w, s.cfg.Lifecycle.CreateGroup(ctx, g)) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) deleteGroup(w http.ResponseWriter, r *http.Request) {
	var g manage.Group
	if !decode(w, r, 4096, &g) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if writeRefusal(w, s.cfg.Lifecycle.DeleteGroup(ctx, g.Path)) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	res, err := s.cfg.Searcher.Search(ctx, q.Get("q"), search.Options{IncludeArchived: q.Get("archived") == "1", Limit: limit})
	switch {
	case errors.Is(err, search.ErrQuery):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case err != nil:
		writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": "search took too long: " + err.Error()})
	default:
		writeJSON(w, http.StatusOK, res)
	}
}

func (s *Server) open(w http.ResponseWriter, r *http.Request) {
	var req open.Request
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be a JSON open request"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if writeRefusal(w, s.cfg.Opener.Open(ctx, req)) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// logOutcome records what a reply or answer did, so a failure the browser
// saw leaves a trace. Message text is never logged.
func logOutcome(what, id, detail, ok string, err error) {
	var refused *send.Refused
	switch {
	case errors.As(err, &refused):
		log.Printf("%s session=%s %s refused=%s", what, id, detail, refused.Reason)
	case err != nil:
		log.Printf("%s session=%s %s error=%q", what, id, detail, err.Error())
	default:
		log.Printf("%s session=%s %s result=%s", what, id, detail, ok)
	}
}

// writeRefusal writes err as 409 for a refusal or 502 otherwise, and
// reports whether it wrote anything.
func writeRefusal(w http.ResponseWriter, err error) bool {
	var refused *send.Refused
	switch {
	case err == nil:
		return false
	case errors.As(err, &refused):
		writeJSON(w, http.StatusConflict, refused)
	default:
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

const placeholder = `<!doctype html><title>Agent Desk</title><p>The frontend is not built. Run <code>make build</code>, or <code>make dev</code> for development.</p>`

func (s *Server) static() http.Handler {
	if s.cfg.Assets == nil {
		return placeholderHandler()
	}
	if _, err := fs.Stat(s.cfg.Assets, "index.html"); err != nil {
		return placeholderHandler()
	}
	files := http.FileServerFS(s.cfg.Assets)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name != "" {
			if _, err := fs.Stat(s.cfg.Assets, name); err != nil {
				// Client-side routes fall back to the app shell.
				r2 := r.Clone(r.Context())
				r2.URL.Path = "/"
				files.ServeHTTP(w, r2)
				return
			}
		}
		files.ServeHTTP(w, r)
	})
}

func placeholderHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(placeholder))
	})
}
