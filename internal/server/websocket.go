package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/gsperry/agent-desk/internal/hub"
)

// maxSubscriptions bounds the transcripts one connection can follow.
const maxSubscriptions = 8

func (s *Server) wsTokenOK(r *http.Request) bool {
	for _, header := range r.Header.Values("Sec-WebSocket-Protocol") {
		for _, p := range strings.Split(header, ",") {
			if token, ok := strings.CutPrefix(strings.TrimSpace(p), TokenProtocolPrefix); ok && s.tokenOK(token) {
				return true
			}
		}
	}
	return false
}

type clientMessage struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionId"`
}

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	// A remote request was admitted by requireRemote; a local one needs the
	// token.
	if _, remote := remoteFrom(r.Context()); !remote && !s.wsTokenOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	// Handler has already checked Origin against the trusted set, which
	// includes dev origins the library's same-host check would refuse.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols:       []string{WSProtocol},
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	if conn.Subprotocol() != WSProtocol {
		conn.Close(websocket.StatusPolicyViolation, "subprotocol "+WSProtocol+" required")
		return
	}
	conn.SetReadLimit(64 << 10)
	if c, ok := remoteFrom(r.Context()); ok {
		defer s.trackRemote(conn, connOf(c))()
		// Settings may have changed between admission and tracking.
		if !stillAllowed(s.remoteSettingsNow(), connOf(c)) {
			return
		}
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	incoming := make(chan clientMessage)
	go func() {
		defer cancel()
		for {
			var m clientMessage
			if err := wsjson.Read(ctx, conn, &m); err != nil {
				return
			}
			select {
			case incoming <- m:
			case <-ctx.Done():
				return
			}
		}
	}()

	c := &wsConn{server: s, conn: conn, ctx: ctx, out: make(chan any, 64), subs: map[string]context.CancelFunc{}}
	defer c.stopAll()
	if err := c.send(map[string]any{"type": "hello", "version": s.cfg.Version}); err != nil {
		return
	}
	updates, unsubscribe := s.cfg.Sessions.Subscribe()
	defer unsubscribe()
	if err := c.send(sessionsMessage(s.cfg.Sessions.State())); err != nil {
		return
	}
	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()
	for {
		var err error
		select {
		case <-ctx.Done():
			return
		case st := <-updates:
			err = c.send(sessionsMessage(st))
		case m := <-incoming:
			err = c.handle(m)
		case v := <-c.out:
			err = c.send(v)
		case <-ping.C:
			err = c.ping()
		}
		if err != nil {
			return
		}
	}
}

// wsConn is one client's WebSocket and its transcript subscriptions.
type wsConn struct {
	server *Server
	conn   *websocket.Conn
	ctx    context.Context
	out    chan any
	subs   map[string]context.CancelFunc
}

func (c *wsConn) send(v any) error {
	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()
	return wsjson.Write(ctx, c.conn, v)
}

func (c *wsConn) ping() error {
	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()
	return c.conn.Ping(ctx)
}

func (c *wsConn) handle(m clientMessage) error {
	switch m.Type {
	case "subscribe":
		if _, ok := c.subs[m.SessionID]; ok || m.SessionID == "" {
			return nil
		}
		if len(c.subs) >= maxSubscriptions {
			return c.send(map[string]any{"type": "error", "message": "too many transcript subscriptions"})
		}
		ctx, stop := context.WithCancel(c.ctx)
		c.subs[m.SessionID] = stop
		go c.server.followTranscript(ctx, m.SessionID, c.out)
	case "unsubscribe":
		if stop, ok := c.subs[m.SessionID]; ok {
			stop()
			delete(c.subs, m.SessionID)
		}
	}
	return nil
}

func (c *wsConn) stopAll() {
	for _, stop := range c.subs {
		stop()
	}
}

// followTranscript forwards one transcript's first page and its changes to
// out until ctx ends.
func (s *Server) followTranscript(ctx context.Context, sessionID string, out chan<- any) {
	page, events, unsubscribe := s.cfg.Transcripts.Subscribe(ctx, sessionID)
	defer unsubscribe()
	forward := func(v any) bool {
		select {
		case out <- v:
			return true
		case <-ctx.Done():
			return false
		}
	}
	if !forward(pageMessage(page)) {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-events:
			var msg any
			if ev.Reset {
				msg = pageMessage(ev.Page)
			} else {
				msg = map[string]any{"type": "transcript.append", "sessionId": ev.SessionID, "items": ev.Items}
			}
			if !forward(msg) {
				return
			}
		}
	}
}

func sessionsMessage(st hub.State) map[string]any {
	return map[string]any{"type": "sessions", "state": st}
}

func pageMessage(p hub.Page) map[string]any {
	return map[string]any{"type": "transcript.page", "page": p}
}

// aroundRadius is how many items a search hit shows on each side.
const aroundRadius = 12

// transcriptWindow serves older pages for scrollback, or a search hit in
// context with around=<uuid>.
func (s *Server) transcriptWindow(w http.ResponseWriter, r *http.Request) {
	if uuid := r.URL.Query().Get("around"); uuid != "" {
		if len(uuid) > 128 {
			http.Error(w, "around is too long", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, s.cfg.Transcripts.Around(r.Context(), r.PathValue("id"), uuid, aroundRadius))
		return
	}
	before, limit, err := windowParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, s.cfg.Transcripts.Window(r.Context(), r.PathValue("id"), before, limit))
}

// windowParams reads before (a Seq, -1 for the newest) and limit.
func windowParams(r *http.Request) (before, limit int, err error) {
	before = -1
	if v := r.URL.Query().Get("before"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return 0, 0, errors.New("before must be a non-negative integer")
		}
		before = n
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 2000 {
			return 0, 0, errors.New("limit must be 1 to 2000")
		}
		limit = n
	}
	return before, limit, nil
}
