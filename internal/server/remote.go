package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/coder/websocket"

	"github.com/gsperry/agent-desk/internal/remote"
)

type remoteKey struct{}

type socketKey struct{}

// OnSocket marks a connection's context as arriving on the remote Unix
// socket, which only this account and root (tailscaled) can connect to.
func OnSocket(ctx context.Context) context.Context {
	return context.WithValue(ctx, socketKey{}, true)
}

// requestHost is the host a remote request names. tailscale serve sends
// requests for a Unix socket with Host "localhost" and the tailnet name in
// X-Forwarded-Host, which it always overwrites. That header is trusted only
// on the socket, only in that shape, and only when Tailscale is the
// configured proxy: cloudflared can share the socket and passes the header
// through from the client.
func requestHost(r *http.Request, st remote.Settings) string {
	onSocket, _ := r.Context().Value(socketKey{}).(bool)
	if !onSocket || r.Host != "localhost" || st.Proxy() != remote.ViaTailscale {
		return r.Host
	}
	if v := r.Header.Values("X-Forwarded-Host"); len(v) == 1 {
		return v[0]
	}
	return ""
}

// remoteCaller is the identity and settings of a remote request, set by
// requireRemote. Local requests have none.
type remoteCaller struct {
	id       remote.Identity
	settings remote.Settings
}

func remoteFrom(ctx context.Context) (remoteCaller, bool) {
	c, ok := ctx.Value(remoteKey{}).(remoteCaller)
	return c, ok
}

func (s *Server) remoteSettingsNow() remote.Settings {
	if s.cfg.Remote == nil {
		return remote.Defaults()
	}
	return s.cfg.Remote.Get()
}

// requireRemote admits a remote request only when remote access is on, the
// request names the configured host, a cross-site origin is absent, the
// proxy vouches for an allowed user, and the level covers what the route
// needs. Local-only routes are refused at every level.
func (s *Server) requireRemote(need remote.Need, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st := s.remoteSettingsNow()
		if !st.Enabled {
			http.Error(w, "remote access is off", http.StatusNotFound)
			return
		}
		if requestHost(r, st) != st.Host {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if !remoteOriginAllowed(r, st.Host, s.cfg.RemoteHTTPOrigin) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		id, ok := remote.Identify(r, st, s.cfg.Access)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !st.Level.Allows(need) {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"reason":  "remote_level",
				"message": "Not allowed from a remote device at the current level. Change it on the desk.",
			})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), remoteKey{}, remoteCaller{id: id, settings: st})))
	})
}

// remoteOriginAllowed accepts the remote host's own HTTPS origin, and no
// origin only on reads that are not WebSocket upgrades.
func remoteOriginAllowed(r *http.Request, host string, allowHTTP bool) bool {
	origin := r.Header.Get("Origin")
	if origin != "" {
		return origin == "https://"+host || (allowHTTP && origin == "http://"+host)
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		return !isUpgrade(r)
	default:
		return false
	}
}

// whoami tells the web app whether it runs locally or remotely, and at
// which level.
func (s *Server) whoami(w http.ResponseWriter, r *http.Request) {
	if c, ok := remoteFrom(r.Context()); ok {
		writeJSON(w, http.StatusOK, map[string]any{"remote": true, "user": c.id.User, "via": c.id.Via, "level": c.settings.Level})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"remote": false})
}

// remoteSettings returns the settings to a local client, and only the level
// to a remote one.
func (s *Server) remoteSettings(w http.ResponseWriter, r *http.Request) {
	if c, ok := remoteFrom(r.Context()); ok {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "level": c.settings.Level})
		return
	}
	if s.cfg.Remote == nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": true, "settings": s.cfg.Remote.Get(), "status": s.cfg.Remote.Status()})
}

// setRemoteSettings changes the settings. It is registered as local only,
// and checks again here.
func (s *Server) setRemoteSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := remoteFrom(r.Context()); ok || s.cfg.Remote == nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var in remote.Settings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be JSON settings"})
		return
	}
	saved, err := s.cfg.Remote.Set(in)
	logOutcome("remote", "settings", "enabled="+boolString(saved.Enabled)+" level="+string(saved.Level), "ok", err)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": true, "settings": saved, "status": s.cfg.Remote.Status()})
}

// trackRemote records a remote WebSocket so a settings change can close
// it; the returned function forgets it.
func (s *Server) trackRemote(c *websocket.Conn, rc remoteConn) func() {
	s.remoteMu.Lock()
	s.remoteConns[c] = rc
	s.remoteMu.Unlock()
	return func() {
		s.remoteMu.Lock()
		delete(s.remoteConns, c)
		s.remoteMu.Unlock()
	}
}

// remoteConn is who opened a remote WebSocket, through which proxy, host
// and Access application.
type remoteConn struct{ user, via, host, team, aud string }

func connOf(c remoteCaller) remoteConn {
	return remoteConn{user: c.id.User, via: c.id.Via, host: c.settings.Host, team: c.settings.CloudflareTeam, aud: c.settings.CloudflareAUD}
}

// stillAllowed reports whether settings still admit a remote connection.
func stillAllowed(st remote.Settings, rc remoteConn) bool {
	if !st.Enabled || st.Host != rc.host || st.Proxy() != rc.via || !st.Allowed(rc.user) {
		return false
	}
	if rc.via == remote.ViaCloudflare {
		return st.CloudflareTeam == rc.team && st.CloudflareAUD == rc.aud
	}
	return true
}

// DropRemote closes remote WebSockets the settings no longer allow: all of
// them when remote access is off, else those whose login was removed or
// whose proxy, host name or Access application changed.
// Closing the listener does not end connections that were upgraded.
func (s *Server) DropRemote(st remote.Settings) {
	s.remoteMu.Lock()
	var drop []*websocket.Conn
	for c, rc := range s.remoteConns {
		if !stillAllowed(st, rc) {
			drop = append(drop, c)
		}
	}
	s.remoteMu.Unlock()
	// Closed at once, without waiting on a close handshake: a revoked
	// device gets nothing more, and the settings save does not wait.
	for _, c := range drop {
		_ = c.CloseNow()
	}
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
