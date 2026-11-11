package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/gsperry/agent-desk/internal/hub"
)

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

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	if !s.wsTokenOK(r) {
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
	ctx := conn.CloseRead(r.Context())
	if err := wsjson.Write(ctx, conn, map[string]any{"type": "hello", "version": s.cfg.Version}); err != nil {
		return
	}
	updates, unsubscribe := s.cfg.Sessions.Subscribe()
	defer unsubscribe()
	if err := writeSessions(ctx, conn, s.cfg.Sessions.State()); err != nil {
		return
	}
	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case st := <-updates:
			if err := writeSessions(ctx, conn, st); err != nil {
				return
			}
		case <-ping.C:
			pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := conn.Ping(pctx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

func writeSessions(ctx context.Context, conn *websocket.Conn, st hub.State) error {
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return wsjson.Write(wctx, conn, map[string]any{"type": "sessions", "state": st})
}
